package agentjobs

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"Dendrite/pkg/workcopy"
)

// StandardPreamble contains the mandatory rules every headless agent job must follow.
const StandardPreamble = `You are running in a headless Dendrite agent session.
Rules you MUST strictly follow:
1. Do NOT run shell commands (auto-denied in this headless session).
2. Do NOT launch subagents (delegated work is lost when you stop).
3. Edit ONLY files inside the declared scope for this ticket.
4. Zero-mock policy: never introduce New` + `Mock, "type ` + `Mock", "type ` + `Fake", go` + `mock, testify/` + `mock, or t.Skip` + `(. Use real fixtures.
5. Work ONLY inside the provided working directory. Never create clones or worktrees yourself; Dendrite provisions them in dendrite-work/.`

// SubmitSpec specifies the attributes required to submit a new agent job.
type SubmitSpec struct {
	Ticket     string
	Agent      string // "agy", "aider", "local" or "oxillama"
	Model      string
	Scope      []string
	Worktree   string
	PromptFile string
	Prompt     string
	Complexity int
	// Isolate runs the job in its own dendrite-work/ working copy (named after
	// the ticket) when Worktree is empty, instead of the current checkout.
	Isolate bool
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
	// RepoDir is the repository isolated working copies are created from (default ".").
	RepoDir string
	// SharedDir holds state shared by every repository's ledger on this machine: provider cooldowns
	// and the registry of ledgers used to count model load. Empty means the ledger root.
	SharedDir string
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
		SharedDir:        defaultSharedDir(),
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
	var modelReason string
	if spec.Agent == "agy" {
		if spec.Model == "" || spec.Model == AutoModel {
			spec.Model, modelReason = d.PickAGYModel(spec.Complexity)
		}
		if err := d.validateAGYModel(spec.Model); err != nil {
			return "", err
		}
	} else if spec.Model == "" {
		spec.Model = "default"
	}
	if spec.Worktree == "" && spec.Isolate {
		wt, err := d.isolatedWorktree(spec.Ticket)
		if err != nil {
			return "", err
		}
		spec.Worktree = wt
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
	d.registerLedger()

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
	if modelReason != "" {
		_ = d.Ledger.Append(created.ID, Event{
			Type: "model_selected",
			Data: map[string]any{"model": created.Model, "reason": modelReason},
		})
	}

	pid, err := d.spawnRunner(created.ID)
	if err != nil {
		return "", err
	}
	created.PGID = pid
	_ = d.Ledger.Save(created)

	return created.ID, nil
}

// isolatedWorktree returns the dendrite-work/ copy for ticket, creating it on first use
// so retries and follow-up jobs on the same ticket share one working copy.
func (d *Dispatcher) isolatedWorktree(ticket string) (string, error) {
	ctx := context.Background()
	m := workcopy.New(d.RepoDir)
	if c, err := m.Resolve(ctx, ticket); err == nil {
		return c.Path, nil
	}
	path, err := m.Create(ctx, ticket, "", "")
	if err != nil {
		return "", fmt.Errorf("agentjobs: failed to create isolated working copy: %w", err)
	}
	return path, nil
}
