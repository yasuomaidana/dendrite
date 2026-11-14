package agentjobs

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
	"time"
)

// runChat posts the prompt to an OpenAI-compatible chat endpoint and writes the answer to
// output.md in the job directory.
func (d *Dispatcher) runChat(ctx context.Context, job *Job, promptText, url, key, name string) (int, string, map[string]any, error) {

	reqData := map[string]any{
		"model": job.Model,
		"messages": []map[string]string{
			{"role": "user", "content": promptText},
		},
	}
	bodyBytes, _ := json.Marshal(reqData)

	req, err := http.NewRequestWithContext(ctx, "POST", url, bytes.NewReader(bodyBytes))
	if err != nil {
		return 1, err.Error(), nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	if key != "" {
		req.Header.Set("Authorization", "Bearer "+key)
	}

	client := d.HTTPClient
	if client == nil {
		client = http.DefaultClient
	}

	resp, err := client.Do(req)
	if err != nil {
		return 1, err.Error(), nil, err
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return resp.StatusCode, err.Error(), nil, err
	}

	logPath := d.Ledger.AgentLogPath(job.ID)
	if f, oErr := os.OpenFile(logPath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644); oErr == nil {
		_, _ = f.Write(respBody)
		_, _ = f.Write([]byte("\n"))
		_ = f.Close()
	}

	if resp.StatusCode != http.StatusOK {
		return resp.StatusCode, string(respBody), nil, fmt.Errorf("%s returned HTTP %d", name, resp.StatusCode)
	}

	var chatResp struct {
		Choices []struct {
			Message struct {
				Content string `json:"content"`
			} `json:"message"`
		} `json:"choices"`
		Usage map[string]any `json:"usage"`
	}
	_ = json.Unmarshal(respBody, &chatResp)

	content := ""
	if len(chatResp.Choices) > 0 {
		content = chatResp.Choices[0].Message.Content
	}

	outputPath := filepath.Join(d.Ledger.JobDir(job.ID), "output.md")
	_ = os.WriteFile(outputPath, []byte(content), 0644)

	return 0, string(respBody), chatResp.Usage, nil
}

// CheckScope executes git status --porcelain in the target worktree and verifies
// whether modified files are strictly inside the declared scope path prefixes.
func (d *Dispatcher) CheckScope(ctx context.Context, worktree string, scope []string) (bool, []string, error) {
	return d.checkScopeAgainst(ctx, worktree, scope, nil)
}

// checkScopeAgainst is CheckScope that ignores files whose content still matches the pre-dispatch baseline.
func (d *Dispatcher) checkScopeAgainst(ctx context.Context, worktree string, scope []string, baseline map[string]string) (bool, []string, error) {
	if worktree == "" {
		var err error
		worktree, err = os.Getwd()
		if err != nil {
			return false, nil, err
		}
	}

	changedFiles, err := d.dirtyPaths(ctx, worktree)
	if err != nil {
		return false, nil, err
	}
	if len(baseline) > 0 {
		kept := changedFiles[:0]
		for _, f := range changedFiles {
			if before, ok := baseline[f]; ok && before == d.fileHash(ctx, worktree, f) {
				continue
			}
			kept = append(kept, f)
		}
		changedFiles = kept
	}

	if len(changedFiles) == 0 {
		return false, nil, nil
	}

	cleanScopes := cleanScopeList(scope)

	var offending []string
	changedInScope := false

	for _, file := range changedFiles {
		if fileMatchesScope(file, cleanScopes) {
			changedInScope = true
		} else {
			offending = append(offending, file)
		}
	}

	return changedInScope, offending, nil
}

// dirtyPaths lists the worktree's modified and untracked files, excluding Dendrite state.
func (d *Dispatcher) dirtyPaths(ctx context.Context, worktree string) ([]string, error) {
	cmd := exec.CommandContext(ctx, d.gitPath(), "-C", worktree, "status", "--porcelain", "--untracked-files=all")
	out, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("agentjobs: git status failed: %w", err)
	}

	var changedFiles []string
	for _, line := range strings.Split(string(out), "\n") {
		trimmed := strings.TrimRight(line, "\r")
		if len(trimmed) < 4 {
			continue
		}
		pathPart := trimmed[3:]
		if strings.Contains(pathPart, " -> ") {
			parts := strings.Split(pathPart, " -> ")
			pathPart = parts[len(parts)-1]
		}
		pathPart = strings.TrimSpace(pathPart)
		pathPart = strings.Trim(pathPart, "\"")
		if pathPart != "" && !d.isStatePath(worktree, pathPart) {
			changedFiles = append(changedFiles, filepath.Clean(pathPart))
		}
	}
	return changedFiles, nil
}

// fileHash is the git blob hash of a worktree file, or "-" when it does not exist.
func (d *Dispatcher) fileHash(ctx context.Context, worktree, rel string) string {
	out, err := exec.CommandContext(ctx, d.gitPath(), "-C", worktree, "hash-object", "--", rel).Output()
	if err != nil {
		return "-"
	}
	return strings.TrimSpace(string(out))
}

// snapshotDirty records the hash of every file that is dirty before the job's first attempt.
func (d *Dispatcher) snapshotDirty(ctx context.Context, worktree string) (map[string]string, error) {
	files, err := d.dirtyPaths(ctx, worktree)
	if err != nil {
		return nil, err
	}
	baseline := make(map[string]string, len(files))
	for _, f := range files {
		baseline[f] = d.fileHash(ctx, worktree, f)
	}
	return baseline, nil
}

// prepareTransientRetry decides how to retry after a transient failure. Resuming the AGY conversation is
// cheap only while it is small: an oversized conversation (often the cause of the stream EOF) or a second
// transient starts a fresh conversation with a handover capsule, and the second transient also moves the job
// to the other Gemini family. It returns true and the new prompt when the conversation is reset.
func (d *Dispatcher) prepareTransientRetry(ctx context.Context, job *Job, basePrompt, output string, usage map[string]any, retry int) (bool, string) {
	if job.Agent != "agy" && job.Agent != "" {
		return false, ""
	}
	tokens := inputTokens(usage)
	if retry < 2 && tokens < resumeTokenLimit() {
		return false, ""
	}
	reason := "transient with oversized conversation"
	if retry >= 2 {
		reason = "repeated transient"
		if alt := CounterpartModel(job.Model); alt != "" &&
			!coolingDown(d.loadExhaustedProviders(), providerKey("agy", alt), d.now()) {
			from := job.Agent + "/" + job.Model
			job.Model = alt
			_ = d.Ledger.Append(job.ID, Event{Type: "escalated", Data: map[string]any{
				"from": from, "to": "agy/" + alt, "reason": reason,
			}})
		}
	}
	capsule := d.buildCapsule(ctx, job, output, reason)
	_ = os.WriteFile(d.Ledger.CapsulePath(job.ID), []byte(capsule), 0o644)
	job.ConversationID = ""
	_ = d.Ledger.Append(job.ID, Event{Type: "conversation_reset", Data: map[string]any{
		"reason": reason, "input_tokens": tokens,
	}})
	return true, basePrompt + "\n\n## Handover capsule\n" + capsule +
		"\nRepair this partial work; do not re-explore the codebase.\n"
}

// Retry re-submits a job detached by re-executing `<binary> bouton jobs run <id>`.
func (d *Dispatcher) Retry(id string) error {
	job, err := d.Ledger.Load(id)
	if err != nil {
		return fmt.Errorf("agentjobs: failed to load job %q: %w", id, err)
	}

	job.Status = StatusQueued
	if err := d.Ledger.Save(job); err != nil {
		return fmt.Errorf("agentjobs: failed to update job status: %w", err)
	}
	_ = d.Ledger.Append(id, Event{Type: "retried"})

	pid, err := d.spawnRunner(id)
	if err != nil {
		return err
	}
	job.PGID = pid
	_ = d.Ledger.Save(job)

	return nil
}

// Cancel terminates the process group recorded for the job, marks status cancelled, and records the event.
func (d *Dispatcher) Cancel(id string) error {
	job, err := d.Ledger.Load(id)
	if err != nil {
		return fmt.Errorf("agentjobs: failed to load job %q: %w", id, err)
	}

	if job.PGID > 0 {
		_ = syscall.Kill(-job.PGID, syscall.SIGTERM)
	}

	job.Status = StatusCancelled
	if err := d.Ledger.Save(job); err != nil {
		return fmt.Errorf("agentjobs: failed to save job metadata: %w", err)
	}

	_ = d.Ledger.Append(id, Event{Type: "cancelled"})
	_ = d.Ledger.MarkDone(id)
	return nil
}

func (d *Dispatcher) backoffDuration(attempt int) time.Duration {
	base := 2.0
	capSec := 120.0
	secs := base * math.Pow(2, float64(attempt-1))
	if secs > capSec {
		secs = capSec
	}
	jitter := float64(time.Now().UnixNano()%500) / 1000.0
	return time.Duration((secs + jitter) * float64(time.Second))
}

// ErrSelfExecInTest prevents a test binary from re-executing itself as the job runner,
// which would recursively run the whole test suite (a fork bomb).
var ErrSelfExecInTest = errors.New("agentjobs: refusing to re-execute the test binary as job runner; set Dispatcher.Executable")

// executable is the binary re-executed for detached work: Dispatcher.Executable, else this process.
// A test binary never re-executes itself.
func (d *Dispatcher) executable() (string, error) {
	self, _ := os.Executable()
	exe := d.Executable
	if exe == "" {
		exe = self
	}
	if exe == "" {
		return "", errors.New("agentjobs: failed to resolve binary path")
	}
	if testing.Testing() && exe == self {
		return "", ErrSelfExecInTest
	}
	return exe, nil
}

// spawnRunner starts `<binary> bouton jobs run <id>` detached in its own session, logging to agent.log.
// It returns the runner's process id, which is also its process group id (Setsid).
func (d *Dispatcher) spawnRunner(id string) (int, error) {
	exe, err := d.executable()
	if err != nil {
		return 0, err
	}

	logFile, err := os.OpenFile(d.Ledger.AgentLogPath(id), os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
	if err != nil {
		return 0, fmt.Errorf("agentjobs: failed to open agent.log: %w", err)
	}
	defer logFile.Close()

	cmd := exec.Command(exe, "bouton", "jobs", "run", id)
	cmd.Stdout = logFile
	cmd.Stderr = logFile
	cmd.SysProcAttr = sysProcAttrSetsid()
	if err := cmd.Start(); err != nil {
		return 0, fmt.Errorf("agentjobs: failed to start job %s: %w", id, err)
	}
	return cmd.Process.Pid, nil
}

// defaultOxillamaEndpoint honours DENDRITE_OXILLAMA_ENDPOINT and falls back to the local server.
func defaultOxillamaEndpoint() string {
	if endpoint := os.Getenv("DENDRITE_OXILLAMA_ENDPOINT"); endpoint != "" {
		return endpoint
	}
	return "http://127.0.0.1:8080"
}

// isStatePath reports whether a worktree-relative path is Dendrite's own state (the job ledger or
// .dendrite/), which lives inside the repository by default and must never count as agent edits.
func (d *Dispatcher) isStatePath(worktree, rel string) bool {
	clean := filepath.ToSlash(filepath.Clean(rel))
	if clean == ".dendrite" || strings.HasPrefix(clean, ".dendrite/") {
		return true
	}
	if d.Ledger == nil || worktree == "" {
		return false
	}
	abs, err := filepath.Abs(filepath.Join(worktree, rel))
	if err != nil {
		return false
	}
	stateRoot, err := filepath.Abs(d.Ledger.Root())
	if err != nil {
		return false
	}
	if r, err := filepath.EvalSymlinks(stateRoot); err == nil {
		stateRoot = r
	}
	if dir, err := filepath.EvalSymlinks(filepath.Dir(abs)); err == nil {
		abs = filepath.Join(dir, filepath.Base(abs))
	}
	return abs == stateRoot || strings.HasPrefix(abs, stateRoot+string(filepath.Separator))
}
