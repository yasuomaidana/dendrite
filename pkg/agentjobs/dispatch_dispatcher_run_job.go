package agentjobs

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"time"
)

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
	basePrompt := string(promptBytes)
	promptText := basePrompt

	// Files already dirty before the first attempt are not the agent's edits.
	if job.Baseline == nil {
		if baseline, bErr := d.snapshotDirty(ctx, job.Worktree); bErr == nil {
			job.Baseline = baseline
			_ = d.Ledger.Save(job)
		}
	}

	maxTransientRetries := 3
	maxOtherRetries := 1
	transientRetries := 0
	otherRetries := 0

	var lastClass Class
	// tierBase snapshots the worktree whenever the job moves to another agent or model, so a fallback
	// tier is credited only with its own edits, not with the partial work of the tier before it.
	var tierBase map[string]string
	tierKey := ""

	for {
		if ctx.Err() != nil {
			job.Status = StatusFailed
			_ = d.Ledger.Save(job)
			return ctx.Err()
		}

		d.skipExhaustedInitialTier(job)

		if key := job.Agent + "/" + job.Model; key != tierKey {
			if tierKey != "" && !textAgent(job.Agent) {
				tierBase, _ = d.snapshotDirty(ctx, job.Worktree)
			}
			tierKey = key
		}

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
		attemptStart := d.now()
		attemptCtx, cancelAttempt := context.WithTimeout(ctx, attemptTimeout)

		var (
			exitCode       int
			logTail        string
			usage          map[string]any
			conversationID string
			execErr        error
		)

		switch job.Agent {
		case "local":
			exitCode, logTail, usage, execErr = d.runLocal(attemptCtx, job, promptText)
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

		// Text-generation agents (local, oxillama) deliver output.md in the job dir; the
		// file-scope check applies only to agents that edit the worktree.
		var changedInScope bool
		var offendingPaths []string
		if !textAgent(job.Agent) {
			var scopeErr error
			changedInScope, offendingPaths, scopeErr = d.checkScopeAgainst(ctx, job.Worktree, job.Scope, job.Baseline)
			if scopeErr != nil {
				logTail += "\nscope_check_error: " + scopeErr.Error()
			}
			if tierBase != nil && changedInScope {
				changedInScope, _, _ = d.checkScopeAgainst(ctx, job.Worktree, job.Scope, tierBase)
			}
		}

		if textAgent(job.Agent) && exitCode == 0 {
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
			// One stuck conversation is not a quota signal when the same family served other jobs meanwhile.
			if d.familyServedSince(job, attemptStart) {
				class = ClassTransient
				logTail += "\nstalled: attempt exceeded deadline while other jobs on this model family succeeded"
			} else {
				class = ClassQuota
				logTail += "\nthrottled: attempt exceeded deadline with no changes in scope"
			}
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
				if fresh, nextPrompt := d.prepareTransientRetry(ctx, job, basePrompt, logTail, usage, transientRetries); fresh {
					promptText = nextPrompt
				}
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
		// AGY may print its result envelope followed by an AGY_ERROR line; take the first line that parses.
		for _, line := range bytes.Split(stdout.Bytes(), []byte("\n")) {
			if idx := bytes.IndexByte(line, '{'); idx >= 0 && json.Unmarshal(line[idx:], &agyOut) == nil && agyOut.ConversationID != "" {
				break
			}
		}
	}
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

// textAgent reports whether an agent answers in output.md instead of editing the worktree.
func textAgent(agent string) bool {
	return agent == "local" || agent == "oxillama"
}

// runLocal runs the local tier: a chat completion through the LiteLLM gateway, whose local
// aliases are served by GPU engines (Ollama first; OxiLLaMa only on request).
func (d *Dispatcher) runLocal(ctx context.Context, job *Job, promptText string) (int, string, map[string]any, error) {
	base := d.LiteLLMURL
	if base == "" {
		base = envOr("DENDRITE_LITELLM_URL", "http://localhost:4000/v1")
	}
	key := d.LiteLLMKey
	if key == "" {
		key = d.liteLLMKey(job.Worktree)
	}
	return d.runChat(ctx, job, promptText, strings.TrimRight(base, "/")+"/chat/completions", key, "LiteLLM gateway")
}

// runOxillama sends the job to an OxiLLaMa server. Only explicitly requested: OxiLLaMa
// prefills on the CPU.
func (d *Dispatcher) runOxillama(ctx context.Context, job *Job, promptText string) (int, string, map[string]any, error) {
	endpoint := d.OxillamaEndpoint
	if endpoint == "" {
		endpoint = defaultOxillamaEndpoint()
	}
	return d.runChat(ctx, job, promptText, strings.TrimRight(endpoint, "/")+"/v1/chat/completions", "", "oxillama")
}
