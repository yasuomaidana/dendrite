package agentjobs

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"syscall"
	"time"
)

// DefaultAttemptTimeout bounds one agent attempt. AGY was observed ignoring --print-timeout while
// throttled, so the dispatcher enforces its own wall-clock deadline and kills the process group.
const DefaultAttemptTimeout = 20 * time.Minute

// DefaultProviderCooldown is how long a provider that reported QUOTA is skipped when it gave no Retry-After.
const DefaultProviderCooldown = time.Hour

const (
	providerAGY      = "agy"
	providerCopilot  = "copilot"
	providerOxillama = "oxillama"

	copilotModel  = "copilot-sonnet"
	oxillamaModel = "qwen3-8b-q4_k_m"

	maxCapsuleOutput = 4000
)

// tier is one rung of the quota fallback ladder.
type tier struct {
	Agent string
	Model string
}

// ladder lists the tiers a job may use, in escalation order: the requested AGY model, the other
// Gemini family's comparable model, then Copilot, then local oxillama for simple jobs.
func ladder(job *Job) []tier {
	var tiers []tier
	if job.Agent == "agy" || job.Agent == "" {
		tiers = append(tiers, tier{Agent: "agy", Model: job.Model})
		if alt := CounterpartModel(job.Model); alt != "" {
			tiers = append(tiers, tier{Agent: "agy", Model: alt})
		}
	}
	tiers = append(tiers, tier{Agent: "aider", Model: copilotModel})
	if job.Complexity <= 4 {
		tiers = append(tiers, tier{Agent: "oxillama", Model: oxillamaModel})
	}
	return tiers
}

// nextTier returns the first tier after the job's current one whose quota bucket is not cooling down.
func (d *Dispatcher) nextTier(job *Job, exhausted map[string]time.Time) (tier, bool) {
	tiers := ladder(job)
	current := -1
	for i, t := range tiers {
		if t.Agent == job.Agent && (t.Model == job.Model || t.Agent != "agy") {
			current = i
		}
	}
	now := d.now()
	own := providerKey(job.Agent, job.Model)
	for _, t := range tiers[current+1:] {
		key := providerKey(t.Agent, t.Model)
		if key == own && t.Model == job.Model {
			continue
		}
		if key != own && coolingDown(exhausted, key, now) {
			continue
		}
		return t, true
	}
	return tier{}, false
}

func (d *Dispatcher) now() time.Time {
	if d.Now != nil {
		return d.Now()
	}
	return time.Now()
}

func (d *Dispatcher) providersPath() string {
	return filepath.Join(d.sharedDir(), "_providers.json")
}

// loadExhaustedProviders reads the provider cooldown memory shared by all jobs.
func (d *Dispatcher) loadExhaustedProviders() map[string]time.Time {
	exhausted := map[string]time.Time{}
	paths := []string{d.providersPath()}
	if legacy := filepath.Join(d.Ledger.Root(), "_providers.json"); legacy != paths[0] {
		paths = append(paths, legacy) // cooldowns recorded before the state was shared
	}
	for _, p := range paths {
		data, err := os.ReadFile(p)
		if err != nil {
			continue
		}
		var m map[string]time.Time
		if json.Unmarshal(data, &m) != nil {
			continue
		}
		for k, v := range m {
			if v.After(exhausted[k]) {
				exhausted[k] = v
			}
		}
	}
	return exhausted
}

func (d *Dispatcher) markProviderExhausted(provider string, until time.Time) error {
	exhausted := d.loadExhaustedProviders()
	exhausted[provider] = until
	data, _ := json.MarshalIndent(exhausted, "", "  ")
	if err := os.MkdirAll(d.sharedDir(), 0o755); err != nil {
		return err
	}
	return writeFileAtomic(d.providersPath(), data)
}

var retryAfterRegex = regexp.MustCompile(`(?i)retry[-_ ]after["':=\s]+(\d+)`)

// retryAfter extracts a Retry-After hint (seconds) from agent output, defaulting to the cooldown.
func retryAfter(output string) time.Duration {
	if m := retryAfterRegex.FindStringSubmatch(output); m != nil {
		if secs, err := strconv.Atoi(m[1]); err == nil && secs > 0 {
			return time.Duration(secs) * time.Second
		}
	}
	return DefaultProviderCooldown
}

// skipExhaustedInitialTier moves a job off a provider that is still cooling down before dispatch,
// so jobs do not waste an attempt on a provider another job already found exhausted.
func (d *Dispatcher) skipExhaustedInitialTier(job *Job) {
	exhausted := d.loadExhaustedProviders()
	if !coolingDown(exhausted, providerKey(job.Agent, job.Model), d.now()) {
		return
	}
	if next, found := d.nextTier(job, exhausted); found {
		from := job.Agent + "/" + job.Model
		job.Agent, job.Model = next.Agent, next.Model
		job.Tier++
		_ = d.Ledger.Append(job.ID, Event{Type: "escalated", Data: map[string]any{
			"from": from, "to": next.Agent + "/" + next.Model, "reason": "provider cooling down",
		}})
		_ = d.Ledger.Save(job)
	}
}

// handleQuotaFallback records the exhausted provider, writes a handover capsule and either moves the
// job to the next tier (returning the repair prompt) or parks it until the earliest provider reset.
func (d *Dispatcher) handleQuotaFallback(ctx context.Context, job *Job, prompt, output string, timedOut bool) (bool, string, error) {
	reason := "quota"
	if timedOut {
		reason = "throttled (attempt deadline exceeded)"
	}
	provider := providerKey(job.Agent, job.Model)
	if err := d.markProviderExhausted(provider, d.now().Add(retryAfter(output))); err != nil {
		return false, "", fmt.Errorf("agentjobs: failed to record exhausted provider: %w", err)
	}

	capsule := d.buildCapsule(ctx, job, output, reason)
	if err := os.WriteFile(d.Ledger.CapsulePath(job.ID), []byte(capsule), 0o644); err != nil {
		return false, "", fmt.Errorf("agentjobs: failed to write capsule: %w", err)
	}

	next, ok := d.nextTier(job, d.loadExhaustedProviders())
	if !ok {
		job.Status = StatusParked
		job.ResumeAfter = d.earliestReset()
		_ = d.Ledger.Append(job.ID, Event{Type: "parked", Data: map[string]any{
			"reason": reason, "resume_after": job.ResumeAfter.Format(time.RFC3339),
		}})
		return false, "", d.Ledger.Save(job)
	}

	from := job.Agent + "/" + job.Model
	job.Agent, job.Model = next.Agent, next.Model
	job.Tier++
	job.ConversationID = ""
	job.Status = StatusEscalated
	_ = d.Ledger.Append(job.ID, Event{Type: "escalated", Data: map[string]any{
		"from": from, "to": next.Agent + "/" + next.Model, "reason": reason,
	}})
	if err := d.Ledger.Save(job); err != nil {
		return false, "", err
	}
	return true, prompt + "\n\n## Handover capsule\n" + capsule +
		"\nRepair this partial work; do not re-explore the codebase.\n", nil
}

// earliestReset is the soonest time any exhausted provider becomes available again.
func (d *Dispatcher) earliestReset() time.Time {
	var earliest time.Time
	for _, until := range d.loadExhaustedProviders() {
		if earliest.IsZero() || until.Before(earliest) {
			earliest = until
		}
	}
	if earliest.IsZero() {
		earliest = d.now().Add(DefaultProviderCooldown)
	}
	return earliest
}

// buildCapsule summarises the attempt for the next tier: classification, scope diff and output tail.
func (d *Dispatcher) buildCapsule(ctx context.Context, job *Job, output, reason string) string {
	var sb strings.Builder
	fmt.Fprintf(&sb, "- Ticket: %s\n- Previous tier: %s/%s\n- Reason: %s\n", job.Ticket, job.Agent, job.Model, reason)
	if job.Worktree != "" && len(job.Scope) > 0 {
		args := append([]string{"-C", job.Worktree, "diff", "--"}, job.Scope...)
		if diff, err := exec.CommandContext(ctx, d.gitPath(), args...).Output(); err == nil && len(diff) > 0 {
			fmt.Fprintf(&sb, "\n### Partial work (git diff of the scope)\n```diff\n%s```\n", tail(string(diff), maxCapsuleOutput))
		}
	}
	if strings.TrimSpace(output) != "" {
		fmt.Fprintf(&sb, "\n### Last agent output\n```\n%s\n```\n", tail(output, maxCapsuleOutput))
	}
	return sb.String()
}

func (d *Dispatcher) gitPath() string {
	if d.GitPath != "" {
		return d.GitPath
	}
	return "git"
}

func tail(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[len(s)-n:]
}

// runAider runs the Copilot tier: Aider talking to GitHub Copilot through the LiteLLM gateway.
// Scope entries that are existing files are added to the chat; the process group is killed on deadline.
func (d *Dispatcher) runAider(ctx context.Context, job *Job, prompt string) (int, string, error) {
	promptPath := filepath.Join(d.Ledger.JobDir(job.ID), "prompt-aider.md")
	if err := os.WriteFile(promptPath, []byte(prompt), 0o644); err != nil {
		return 1, err.Error(), err
	}
	url := d.LiteLLMURL
	if url == "" {
		url = envOr("DENDRITE_LITELLM_URL", "http://localhost:4000/v1")
	}
	key := d.LiteLLMKey
	if key == "" {
		key = os.Getenv("LITELLM_MASTER_KEY")
	}
	args := []string{"--model", "openai/" + job.Model, "--openai-api-base", url, "--openai-api-key", key,
		"--edit-format", "diff", "--no-auto-commits", "--yes-always", "--no-show-model-warnings",
		"--no-check-update", "--no-gitignore", "--map-tokens", "0", "--no-stream", "--no-pretty",
		"--message-file", promptPath}
	for _, s := range job.Scope {
		if info, err := os.Stat(filepath.Join(job.Worktree, s)); err == nil && !info.IsDir() {
			args = append(args, "--file", s)
		}
	}
	bin := d.AiderPath
	if bin == "" {
		bin = "aider"
	}
	cmd := exec.CommandContext(ctx, bin, args...)
	cmd.Dir = job.Worktree
	cmd.SysProcAttr = sysProcAttrSetsid()
	cmd.Cancel = func() error { return syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL) }
	cmd.WaitDelay = 100 * time.Millisecond
	out, err := cmd.CombinedOutput()
	code := 0
	if err != nil {
		code = 1
		var exitErr *exec.ExitError
		if errors.As(err, &exitErr) {
			code = exitErr.ExitCode()
		}
	}
	return code, tail(string(out), maxCapsuleOutput), err
}

func envOr(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

// ResumeParked re-dispatches parked jobs whose resume_after has passed and whose provider has cooled down.
func (d *Dispatcher) ResumeParked() ([]string, error) {
	jobs, err := d.Ledger.List()
	if err != nil {
		return nil, err
	}
	exhausted := d.loadExhaustedProviders()
	now := d.now()
	var resumed []string
	for _, job := range jobs {
		if job.Status != StatusParked || job.ResumeAfter.After(now) {
			continue
		}
		if coolingDown(exhausted, providerKey(job.Agent, job.Model), now) {
			continue
		}
		if err := d.Retry(job.ID); err != nil {
			return resumed, err
		}
		resumed = append(resumed, job.ID)
	}
	return resumed, nil
}
