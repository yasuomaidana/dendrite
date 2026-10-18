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

// StandardPreamble contains the mandatory rules every headless agent job must follow.
const StandardPreamble = `You are running in a headless Dendrite agent session.
Rules you MUST strictly follow:
1. Do NOT run shell commands (auto-denied in this headless session).
2. Do NOT launch subagents (delegated work is lost when you stop).
3. Edit ONLY files inside the declared scope for this ticket.
4. Zero-mock policy: never introduce New` + `Mock, "type ` + `Mock", "type ` + `Fake", go` + `mock, testify/` + `mock, or t.Skip` + `(. Use real fixtures.`

// SubmitSpec specifies the attributes required to submit a new agent job.
type SubmitSpec struct {
	Ticket     string
	Agent      string // "agy" or "oxillama"
	Model      string
	Scope      []string
	Worktree   string
	PromptFile string
	Prompt     string
	Complexity int
}

// Dispatcher coordinates job dispatching, process monitoring, scope checking, and retry policies.
type Dispatcher struct {
	Ledger           *Ledger
	Executable       string
	Sleep            func(time.Duration)
	Now              func() time.Time
	AttemptTimeout   time.Duration
	GitPath          string
	AgyPath          string
	AiderPath        string
	LiteLLMURL       string
	LiteLLMKey       string
	OxillamaEndpoint string
	HTTPClient       *http.Client
}

// DefaultDispatcher is the shared default dispatcher.
var DefaultDispatcher = NewDispatcher(nil)

// NewDispatcher returns an initialized Dispatcher instance.
func NewDispatcher(l *Ledger) *Dispatcher {
	if l == nil {
		l = NewLedger("")
	}
	endpoint := defaultOxillamaEndpoint()
	exe, _ := os.Executable()
	return &Dispatcher{
		Ledger:           l,
		Executable:       exe,
		Sleep:            time.Sleep,
		Now:              time.Now,
		AttemptTimeout:   DefaultAttemptTimeout,
		GitPath:          "git",
		AgyPath:          "agy",
		AiderPath:        "aider",
		OxillamaEndpoint: endpoint,
		HTTPClient:       &http.Client{Timeout: 60 * time.Second},
	}
}

// Submit creates a ledger job and starts it detached via DefaultDispatcher.
func Submit(spec SubmitSpec) (string, error) {
	return DefaultDispatcher.Submit(spec)
}

// RunJob builds the agent command, runs it, and applies outcome policies via DefaultDispatcher.
func RunJob(ctx context.Context, id string) error {
	return DefaultDispatcher.RunJob(ctx, id)
}

func sysProcAttrSetsid() *syscall.SysProcAttr {
	return &syscall.SysProcAttr{
		Setsid: true,
	}
}

// Submit creates a new ledger job and starts it detached by re-executing the current binary.
func (d *Dispatcher) Submit(spec SubmitSpec) (string, error) {
	if spec.Ticket == "" {
		spec.Ticket = "JOB"
	}
	if spec.Agent == "" {
		spec.Agent = "agy"
	}
	if spec.Model == "" {
		if spec.Agent == "agy" {
			spec.Model = "gemini-3.8-flash-high"
		} else {
			spec.Model = "default"
		}
	}
	if spec.Worktree == "" {
		wt, err := os.Getwd()
		if err != nil {
			return "", fmt.Errorf("agentjobs: failed to resolve worktree: %w", err)
		}
		spec.Worktree = wt
	}
	absWorktree, err := filepath.Abs(spec.Worktree)
	if err == nil {
		spec.Worktree = absWorktree
	}

	promptText := spec.Prompt
	if promptText == "" && spec.PromptFile != "" {
		data, err := os.ReadFile(spec.PromptFile)
		if err != nil {
			return "", fmt.Errorf("agentjobs: failed to read prompt file %q: %w", spec.PromptFile, err)
		}
		promptText = string(data)
	}
	if strings.TrimSpace(promptText) == "" {
		return "", errors.New("agentjobs: prompt or prompt-file is required and cannot be empty")
	}

	job := &Job{
		Ticket:     spec.Ticket,
		Complexity: spec.Complexity,
		Worktree:   spec.Worktree,
		Scope:      spec.Scope,
		Agent:      spec.Agent,
		Model:      spec.Model,
		Status:     StatusQueued,
		Attempts:   make(map[string]int),
	}
	created, err := d.Ledger.Create(job)
	if err != nil {
		return "", fmt.Errorf("agentjobs: failed to create job in ledger: %w", err)
	}

	promptPath := d.Ledger.PromptPath(created.ID)
	if err := os.WriteFile(promptPath, []byte(promptText), 0644); err != nil {
		return "", fmt.Errorf("agentjobs: failed to write prompt.md: %w", err)
	}
	created.PromptFile = promptPath
	if err := d.Ledger.Save(created); err != nil {
		return "", fmt.Errorf("agentjobs: failed to save job metadata: %w", err)
	}

	_ = d.Ledger.Append(created.ID, Event{
		Type: "submitted",
		Data: map[string]any{
			"ticket":     created.Ticket,
			"agent":      created.Agent,
			"model":      created.Model,
			"scope":      created.Scope,
			"complexity": created.Complexity,
		},
	})

	pid, err := d.spawnRunner(created.ID)
	if err != nil {
		return "", err
	}
	created.PGID = pid
	_ = d.Ledger.Save(created)

	return created.ID, nil
}

// RunJob executes the job, inspects outcome signatures, enforces scope boundaries, and handles retries.
func (d *Dispatcher) RunJob(ctx context.Context, id string) error {
	job, err := d.Ledger.Load(id)
	if err != nil {
		return fmt.Errorf("agentjobs: failed to load job %q: %w", id, err)
	}

	defer func() {
		_ = d.Ledger.MarkDone(id)
	}()

	job.Status = StatusRunning
	if err := d.Ledger.Save(job); err != nil {
		return fmt.Errorf("agentjobs: failed to update job status: %w", err)
	}

	promptBytes, err := os.ReadFile(d.Ledger.PromptPath(id))
	if err != nil {
		job.Status = StatusFailed
		_ = d.Ledger.Save(job)
		return fmt.Errorf("agentjobs: failed to read prompt for job %q: %w", id, err)
	}
	promptText := string(promptBytes)

	maxTransientRetries := 3
	maxOtherRetries := 1
	transientRetries := 0
	otherRetries := 0

	var lastClass Class

	for {
		if ctx.Err() != nil {
			job.Status = StatusFailed
			_ = d.Ledger.Save(job)
			return ctx.Err()
		}

		d.skipExhaustedInitialTier(job)

		if job.Attempts == nil {
			job.Attempts = make(map[string]int)
		}
		job.Attempts[job.Agent]++
		attemptNum := job.Attempts[job.Agent]
		_ = d.Ledger.Save(job)

		_ = d.Ledger.Append(id, Event{
			Type: "dispatched",
			Data: map[string]any{
				"agent":   job.Agent,
				"model":   job.Model,
				"attempt": attemptNum,
			},
		})

		attemptTimeout := d.AttemptTimeout
		if attemptTimeout <= 0 {
			attemptTimeout = DefaultAttemptTimeout
		}
		attemptCtx, cancelAttempt := context.WithTimeout(ctx, attemptTimeout)

		var (
			exitCode       int
			logTail        string
			usage          map[string]any
			conversationID string
			execErr        error
		)

		switch job.Agent {
		case "oxillama":
			exitCode, logTail, usage, execErr = d.runOxillama(attemptCtx, job, promptText)
		case "aider":
			exitCode, logTail, execErr = d.runAider(attemptCtx, job, promptText)
		default:
			exitCode, logTail, usage, conversationID, execErr = d.runAgy(attemptCtx, job, promptText, lastClass)
			if conversationID != "" {
				job.ConversationID = conversationID
				_ = d.Ledger.Save(job)
			}
		}

		timedOut := errors.Is(attemptCtx.Err(), context.DeadlineExceeded) && ctx.Err() == nil
		cancelAttempt()

		// Text-generation agents (oxillama) deliver output.md in the job dir; the file-scope check
		// applies only to agents that edit the worktree.
		var changedInScope bool
		var offendingPaths []string
		if job.Agent != "oxillama" {
			var scopeErr error
			changedInScope, offendingPaths, scopeErr = d.CheckScope(ctx, job.Worktree, job.Scope)
			if scopeErr != nil {
				logTail += "\nscope_check_error: " + scopeErr.Error()
			}
		}

		if job.Agent == "oxillama" && exitCode == 0 {
			outputPath := filepath.Join(d.Ledger.JobDir(job.ID), "output.md")
			data, _ := os.ReadFile(outputPath)
			if len(bytes.TrimSpace(data)) > 0 {
				changedInScope = true
			} else {
				changedInScope = false
			}
		}

		outcome := Outcome{
			ExitCode:       exitCode,
			LogTail:        logTail,
			ChangedInScope: changedInScope,
		}
		class := Classify(outcome)
		if timedOut && !changedInScope {
			class = ClassQuota
			logTail += "\nthrottled: attempt exceeded deadline with no changes in scope"
		}
		lastClass = class

		exitedData := map[string]any{
			"code":  exitCode,
			"class": string(class),
		}
		if timedOut {
			exitedData["throttled"] = true
		}
		if usage != nil {
			exitedData["usage"] = usage
		}
		if job.ConversationID != "" {
			exitedData["conversation_id"] = job.ConversationID
		}
		if len(offendingPaths) > 0 {
			exitedData["offending_paths"] = offendingPaths
		}
		if execErr != nil && exitCode == 0 {
			exitedData["error"] = execErr.Error()
		}
		_ = d.Ledger.Append(id, Event{
			Type: "exited",
			Data: exitedData,
		})

		// Out-of-scope edits by an agent that finished its turn always need a human/orchestrator review.
		if len(offendingPaths) > 0 && (class == ClassDone || class == ClassEarlyExit) {
			job.Status = StatusNeedsReview
			_ = d.Ledger.Append(id, Event{
				Type: "scope_violation",
				Data: map[string]any{"offending_paths": offendingPaths},
			})
			_ = d.Ledger.Save(job)
			return nil
		}

		switch class {
		case ClassDone:
			if changedInScope {
				job.Status = StatusSucceeded
				_ = d.Ledger.Save(job)
				return nil
			}
			job.Status = StatusEarlyExit
			_ = d.Ledger.Save(job)
			return nil

		case ClassQuota:
			escalated, nextPrompt, fErr := d.handleQuotaFallback(ctx, job, promptText, logTail, timedOut)
			if fErr != nil {
				return fErr
			}
			if escalated {
				promptText = nextPrompt
				transientRetries = 0
				otherRetries = 0
				continue
			}
			return nil

		case ClassTransient:
			if transientRetries < maxTransientRetries {
				transientRetries++
				job.Status = StatusFailedRetryable
				_ = d.Ledger.Save(job)
				backoff := d.backoffDuration(transientRetries)
				d.Sleep(backoff)
				continue
			}
			job.Status = StatusFailed
			_ = d.Ledger.Save(job)
			return nil

		case ClassPermission, ClassEarlyExit:
			if otherRetries < maxOtherRetries {
				otherRetries++
				job.Status = StatusFailedRetryable
				_ = d.Ledger.Save(job)
				continue
			}
			if class == ClassEarlyExit {
				job.Status = StatusEarlyExit
			} else {
				job.Status = StatusFailed
			}
			_ = d.Ledger.Save(job)
			return nil

		default:
			job.Status = StatusFailed
			_ = d.Ledger.Save(job)
			return nil
		}
	}
}

func (d *Dispatcher) runAgy(ctx context.Context, job *Job, promptText string, lastClass Class) (int, string, map[string]any, string, error) {
	var promptArg string
	if lastClass == ClassTransient && job.ConversationID != "" {
		promptArg = "continue; you were interrupted"
	} else {
		promptArg = StandardPreamble + "\n\n" + promptText
	}

	args := []string{
		"-p", promptArg,
		"--model", job.Model,
		"--mode", "accept-edits",
		"--sandbox",
		"--add-dir", job.Worktree,
		"--output-format", "json",
	}
	if job.ConversationID != "" {
		args = append(args, "--conversation", job.ConversationID)
	}

	cmd := exec.CommandContext(ctx, d.AgyPath, args...)
	cmd.Dir = job.Worktree
	cmd.SysProcAttr = sysProcAttrSetsid()
	cmd.Cancel = func() error {
		if cmd.Process != nil && cmd.Process.Pid > 0 {
			return syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL)
		}
		return nil
	}
	cmd.WaitDelay = 100 * time.Millisecond

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	err := cmd.Run()

	logPath := d.Ledger.AgentLogPath(job.ID)
	if f, oErr := os.OpenFile(logPath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644); oErr == nil {
		_, _ = f.Write(stdout.Bytes())
		_, _ = f.Write(stderr.Bytes())
		_ = f.Close()
	}

	exitCode := 0
	if err != nil {
		var exitErr *exec.ExitError
		if errors.As(err, &exitErr) {
			exitCode = exitErr.ExitCode()
		} else {
			exitCode = 1
		}
	}

	logTail := stdout.String() + "\n" + stderr.String()

	var agyOut struct {
		ConversationID string         `json:"conversation_id"`
		Status         string         `json:"status"`
		Response       string         `json:"response"`
		Usage          map[string]any `json:"usage"`
	}
	_ = json.Unmarshal(stdout.Bytes(), &agyOut)
	if agyOut.ConversationID == "" {
		raw := stdout.Bytes()
		if idx := bytes.IndexByte(raw, '{'); idx >= 0 {
			if lastIdx := bytes.LastIndexByte(raw, '}'); lastIdx > idx {
				_ = json.Unmarshal(raw[idx:lastIdx+1], &agyOut)
			}
		}
	}

	return exitCode, logTail, agyOut.Usage, agyOut.ConversationID, err
}

func (d *Dispatcher) runOxillama(ctx context.Context, job *Job, promptText string) (int, string, map[string]any, error) {
	endpoint := d.OxillamaEndpoint
	if endpoint == "" {
		endpoint = defaultOxillamaEndpoint()
	}
	url := strings.TrimRight(endpoint, "/") + "/v1/chat/completions"

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
		return resp.StatusCode, string(respBody), nil, fmt.Errorf("oxillama returned HTTP %d", resp.StatusCode)
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
	if worktree == "" {
		var err error
		worktree, err = os.Getwd()
		if err != nil {
			return false, nil, err
		}
	}

	gitPath := d.GitPath
	if gitPath == "" {
		gitPath = "git"
	}

	cmd := exec.CommandContext(ctx, gitPath, "-C", worktree, "status", "--porcelain", "--untracked-files=all")
	out, err := cmd.Output()
	if err != nil {
		return false, nil, fmt.Errorf("agentjobs: git status failed: %w", err)
	}

	lines := strings.Split(string(out), "\n")
	var changedFiles []string
	for _, line := range lines {
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

	if len(changedFiles) == 0 {
		return false, nil, nil
	}

	cleanScopes := make([]string, 0, len(scope))
	for _, s := range scope {
		s = strings.TrimSpace(s)
		if s != "" {
			cleanScopes = append(cleanScopes, filepath.Clean(s))
		}
	}

	var offending []string
	changedInScope := false

	for _, file := range changedFiles {
		inScope := false
		for _, s := range cleanScopes {
			if s == "." || file == s || strings.HasPrefix(file, s+string(filepath.Separator)) || strings.HasPrefix(file, s+"/") {
				inScope = true
				break
			}
		}
		if inScope {
			changedInScope = true
		} else {
			offending = append(offending, file)
		}
	}

	return changedInScope, offending, nil
}

// Retry re-submits a job detached by re-executing `<binary> agent jobs run <id>`.
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

// spawnRunner starts `<binary> agent jobs run <id>` detached in its own session, logging to agent.log.
// It returns the runner's process id, which is also its process group id (Setsid).
func (d *Dispatcher) spawnRunner(id string) (int, error) {
	self, _ := os.Executable()
	exe := d.Executable
	if exe == "" {
		exe = self
	}
	if exe == "" {
		return 0, errors.New("agentjobs: failed to resolve binary path")
	}
	if testing.Testing() && exe == self {
		return 0, ErrSelfExecInTest
	}

	logFile, err := os.OpenFile(d.Ledger.AgentLogPath(id), os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
	if err != nil {
		return 0, fmt.Errorf("agentjobs: failed to open agent.log: %w", err)
	}
	defer logFile.Close()

	cmd := exec.Command(exe, "agent", "jobs", "run", id)
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
