package agentjobs

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestAGYFamilyAndCatalog(t *testing.T) {
	for model, fam := range map[string]string{
		"gemini-3.8-flash-high":  FamilyGemini38,
		"gemini-4-argon-low":     FamilyArgon,
		"claude-opus-5-5-medium": FamilyClaude,
		"gpt-oss-120b-medium":    FamilyGPTOSS,
		"gemini-2.5-pro":         "",
	} {
		if got := AGYFamily(model); got != fam {
			t.Errorf("AGYFamily(%q) = %q, want %q", model, got, fam)
		}
	}
	if IsAGYModel("gemini-2.5-pro") {
		t.Error("gemini-2.5-pro is not offered by AGY and must not be allowed")
	}
	if IsAGYModel("claude-opus-5-5-high") {
		t.Error("Claude models on AGY are opt-in")
	}
	t.Setenv("DENDRITE_AGY_MODELS", "gemini-3.8-flash-high, claude-opus-5-5-high")
	if !IsAGYModel("claude-opus-5-5-high") || IsAGYModel("gemini-4-argon-medium") {
		t.Errorf("DENDRITE_AGY_MODELS must replace the allow-list, got %v", AllowedAGYModels())
	}
}

func TestPickAGYModel_SpreadsAcrossFamilies(t *testing.T) {
	ledger := NewLedger(t.TempDir())
	d := NewDispatcher(ledger)
	fakeNow := time.Date(2026, 10, 18, 12, 0, 0, 0, time.UTC)
	d.Now = func() time.Time { return fakeNow }

	if m, _ := d.PickAGYModel(2); m != "gemini-3.8-flash-medium" {
		t.Errorf("low complexity = %q, want gemini-3.8-flash-medium", m)
	}
	if m, _ := d.PickAGYModel(8); m != "gemini-4-argon-high" {
		t.Errorf("high complexity = %q, want gemini-4-argon-high", m)
	}
	first, _ := d.PickAGYModel(5)
	if first != "gemini-3.8-flash-high" {
		t.Fatalf("first medium job = %q, want gemini-3.8-flash-high", first)
	}
	_, _ = ledger.Create(&Job{Ticket: "DEN-1", Agent: "agy", Model: first, Status: StatusRunning})
	second, reason := d.PickAGYModel(5)
	if second != "gemini-4-argon-medium" {
		t.Errorf("second concurrent medium job = %q, want gemini-4-argon-medium", second)
	}
	if !strings.Contains(reason, "gemini-4-argon") {
		t.Errorf("reason should name the family, got %q", reason)
	}
	// Unknown complexity is treated as medium.
	if m, _ := d.PickAGYModel(0); m != "gemini-4-argon-medium" {
		t.Errorf("unknown complexity = %q, want the least-loaded medium model", m)
	}

	// A cooling-down family is skipped even when it is less loaded.
	_ = d.markProviderExhausted("agy:gemini-4-argon", fakeNow.Add(time.Hour))
	if m, _ := d.PickAGYModel(8); m != "gemini-3.8-flash-high" {
		t.Errorf("argon cooling down: high complexity = %q, want gemini-3.8-flash-high", m)
	}
}

func TestSubmit_AutoModelAndValidation(t *testing.T) {
	worktree := createGitRepo(t)
	ledger := NewLedger(t.TempDir())
	d := NewDispatcher(ledger)
	d.Executable = installExecutableScript(t, t.TempDir(), "dummy_runner", "exit 0\n")

	id, err := d.Submit(SubmitSpec{Agent: "agy", Model: AutoModel, Worktree: worktree, Prompt: "p", Complexity: 2})
	if err != nil {
		t.Fatalf("Submit auto failed: %v", err)
	}
	job, _ := ledger.Load(id)
	if job.Model != "gemini-3.8-flash-medium" {
		t.Errorf("auto model = %q, want gemini-3.8-flash-medium", job.Model)
	}
	events, _ := ledger.Events(id)
	var selected bool
	for _, e := range events {
		if e.Type == "model_selected" && e.Data["model"] == job.Model && e.Data["reason"] != "" {
			selected = true
		}
	}
	if !selected {
		t.Error("missing model_selected event")
	}

	if _, err := d.Submit(SubmitSpec{Agent: "agy", Model: "gemini-2.5-pro", Worktree: worktree, Prompt: "p"}); err == nil ||
		!strings.Contains(err.Error(), "not an allowed AGY model") {
		t.Errorf("unknown AGY model must be rejected, got %v", err)
	}

	// A fresh `agy models` cache that lacks the model rejects it too.
	d.AgyPath = installExecutableScript(t, t.TempDir(), "agy", "printf 'gemini-3.8-flash-high\\tGemini 3.8 Flash (High)\\n'\n")
	models, err := d.RefreshAGYModels(context.Background())
	if err != nil || fmt.Sprint(models) != "[gemini-3.8-flash-high]" {
		t.Fatalf("RefreshAGYModels = %v, %v", models, err)
	}
	if _, err := d.Submit(SubmitSpec{Agent: "agy", Model: "gemini-4-argon-medium", Worktree: worktree, Prompt: "p"}); err == nil ||
		!strings.Contains(err.Error(), "does not offer") {
		t.Errorf("model missing from agy models must be rejected, got %v", err)
	}
	if _, err := d.Submit(SubmitSpec{Agent: "agy", Model: "gemini-3.8-flash-high", Worktree: worktree, Prompt: "p"}); err != nil {
		t.Errorf("listed model must be accepted, got %v", err)
	}
}

func TestRefreshAGYModels_Errors(t *testing.T) {
	d := NewDispatcher(NewLedger(t.TempDir()))
	d.AgyPath = installExecutableScript(t, t.TempDir(), "agy", "echo 'Fetching available models...'\n")
	if _, err := d.RefreshAGYModels(context.Background()); err == nil {
		t.Error("expected error when no models are listed")
	}
	d.AgyPath = installExecutableScript(t, t.TempDir(), "agy", "exit 2\n")
	if _, err := d.RefreshAGYModels(context.Background()); err == nil {
		t.Error("expected error when agy models fails")
	}
	if d.cachedAGYModels() != nil {
		t.Error("no cache must be written on failure")
	}
}

// transientAgy fails `failures` times with a stream EOF that reports `tokens` input tokens, then
// edits the scope and succeeds. Each invocation's arguments are appended to args.log.
func transientAgy(t *testing.T, worktree string, failures, tokens int) (string, string) {
	t.Helper()
	binDir := t.TempDir()
	counter := filepath.Join(binDir, "counter")
	argsLog := filepath.Join(binDir, "args.log")
	_ = os.WriteFile(counter, []byte("0"), 0o644)
	script := installExecutableScript(t, binDir, "agy", fmt.Sprintf(`
echo "$@" >> "%[2]s"
echo "<<CALL>>" >> "%[2]s"
CNT=$(cat "%[1]s")
echo $((CNT + 1)) > "%[1]s"
if [ "$CNT" -lt %[3]d ]; then
  echo '{"conversation_id":"conv-big","status":"ERROR","usage":{"input_tokens":%[4]d}}'
  echo 'AGY_ERROR {"short_error":"stream reading error: unexpected EOF"}'
  exit 3
fi
mkdir -p "%[5]s/pkg"
echo ok > "%[5]s/pkg/done.txt"
echo '{"conversation_id":"conv-new","status":"SUCCESS"}'
exit 0
`, counter, argsLog, failures, tokens, worktree))
	return script, argsLog
}

// agyCalls splits args.log into one entry per agy invocation (prompts span several lines).
func agyCalls(data []byte) []string {
	var calls []string
	for _, c := range strings.Split(string(data), "<<CALL>>\n") {
		if strings.TrimSpace(c) != "" {
			calls = append(calls, c)
		}
	}
	return calls
}

func TestRunJob_TransientOversizedConversationStartsFresh(t *testing.T) {
	worktree := createGitRepo(t)
	agy, argsLog := transientAgy(t, worktree, 1, 500_000)
	ledger := NewLedger(t.TempDir())
	d := NewDispatcher(ledger)
	d.Executable = installExecutableScript(t, t.TempDir(), "dummy_runner", "exit 0\n")
	d.AgyPath = agy
	d.Sleep = func(time.Duration) {}

	id, err := d.Submit(SubmitSpec{Agent: "agy", Model: "gemini-3.8-flash-high", Worktree: worktree, Scope: []string{"pkg"}, Prompt: "Original task"})
	if err != nil {
		t.Fatal(err)
	}
	if err := d.RunJob(context.Background(), id); err != nil {
		t.Fatal(err)
	}
	job, _ := ledger.Load(id)
	if job.Status != StatusSucceeded {
		t.Fatalf("status = %q, want succeeded", job.Status)
	}
	data, _ := os.ReadFile(argsLog)
	calls := agyCalls(data)
	if len(calls) != 2 {
		t.Fatalf("expected 2 agy calls, got %d", len(calls))
	}
	if strings.Contains(calls[1], "--conversation") || strings.Contains(calls[1], "continue; you were interrupted") {
		t.Errorf("an oversized conversation must not be resumed: %s", calls[1])
	}
	if !strings.Contains(calls[1], "Original task") || !strings.Contains(calls[1], "Handover capsule") {
		t.Errorf("fresh attempt must carry the prompt and the capsule: %s", calls[1])
	}
	if job.Model != "gemini-3.8-flash-high" {
		t.Errorf("first transient must keep the model, got %q", job.Model)
	}
	events, _ := ledger.Events(id)
	var reset bool
	for _, e := range events {
		if e.Type == "conversation_reset" {
			reset = true
		}
	}
	if !reset {
		t.Error("missing conversation_reset event")
	}
}

func TestRunJob_SecondTransientSwitchesFamily(t *testing.T) {
	worktree := createGitRepo(t)
	agy, argsLog := transientAgy(t, worktree, 2, 1000)
	ledger := NewLedger(t.TempDir())
	d := NewDispatcher(ledger)
	d.Executable = installExecutableScript(t, t.TempDir(), "dummy_runner", "exit 0\n")
	d.AgyPath = agy
	d.Sleep = func(time.Duration) {}

	id, _ := d.Submit(SubmitSpec{Agent: "agy", Model: "gemini-3.8-flash-high", Worktree: worktree, Scope: []string{"pkg"}, Prompt: "Task"})
	if err := d.RunJob(context.Background(), id); err != nil {
		t.Fatal(err)
	}
	job, _ := ledger.Load(id)
	if job.Status != StatusSucceeded || job.Model != "gemini-4-argon-medium" {
		t.Fatalf("job = %s on %s, want succeeded on gemini-4-argon-medium", job.Status, job.Model)
	}
	data, _ := os.ReadFile(argsLog)
	calls := agyCalls(data)
	if len(calls) != 3 {
		t.Fatalf("expected 3 agy calls, got %d", len(calls))
	}
	if !strings.Contains(calls[1], "--conversation conv-big") {
		t.Errorf("a small first transient should resume the conversation: %s", calls[1])
	}
	if !strings.Contains(calls[2], "--model gemini-4-argon-medium") || strings.Contains(calls[2], "--conversation") {
		t.Errorf("second transient must start fresh on argon: %s", calls[2])
	}
}

func TestRunJob_PreExistingDirtyFilesAreNotOffending(t *testing.T) {
	worktree := createGitRepo(t)
	if err := os.WriteFile(filepath.Join(worktree, "notes.md"), []byte("human work in progress"), 0o644); err != nil {
		t.Fatal(err)
	}
	agy := installExecutableScript(t, t.TempDir(), "agy", fmt.Sprintf(`
mkdir -p "%s/pkg"
echo ok > "%s/pkg/done.txt"
echo '{"status":"SUCCESS"}'
`, worktree, worktree))
	ledger := NewLedger(t.TempDir())
	d := NewDispatcher(ledger)
	d.Executable = installExecutableScript(t, t.TempDir(), "dummy_runner", "exit 0\n")
	d.AgyPath = agy

	id, _ := d.Submit(SubmitSpec{Agent: "agy", Model: "gemini-3.8-flash-high", Worktree: worktree, Scope: []string{"pkg"}, Prompt: "Task"})
	if err := d.RunJob(context.Background(), id); err != nil {
		t.Fatal(err)
	}
	job, _ := ledger.Load(id)
	if job.Status != StatusSucceeded {
		t.Errorf("status = %q, want succeeded (notes.md was dirty before dispatch)", job.Status)
	}
	if _, ok := job.Baseline["notes.md"]; !ok {
		t.Errorf("baseline must record notes.md, got %v", job.Baseline)
	}

	// The same file edited by the agent is offending.
	if err := os.WriteFile(filepath.Join(worktree, "notes.md"), []byte("agent edit"), 0o644); err != nil {
		t.Fatal(err)
	}
	_, offending, err := d.checkScopeAgainst(context.Background(), worktree, []string{"pkg"}, job.Baseline)
	if err != nil || fmt.Sprint(offending) != "[notes.md]" {
		t.Errorf("changed baseline file must be offending, got %v, %v", offending, err)
	}
}

func TestSharedState_SpreadsAndCoolsDownAcrossRepositories(t *testing.T) {
	shared := t.TempDir()
	repoA := NewDispatcher(NewLedger(t.TempDir()))
	repoB := NewDispatcher(NewLedger(t.TempDir()))
	repoA.SharedDir, repoB.SharedDir = shared, shared
	for _, d := range []*Dispatcher{repoA, repoB} {
		d.Executable = installExecutableScript(t, t.TempDir(), "dummy_runner", "exit 0\n")
	}

	// A job submitted in repository A counts as load for repository B.
	worktree := createGitRepo(t)
	if _, err := repoA.Submit(SubmitSpec{Agent: "agy", Worktree: worktree, Prompt: "p", Complexity: 5}); err != nil {
		t.Fatal(err)
	}
	if m, _ := repoB.PickAGYModel(5); m != "gemini-4-argon-medium" {
		t.Errorf("repository B picked %q, want gemini-4-argon-medium while A runs on gemini-3.8", m)
	}

	// A cooldown recorded by A is seen by B.
	if err := repoA.markProviderExhausted("agy:gemini-4-argon", time.Now().Add(time.Hour)); err != nil {
		t.Fatal(err)
	}
	if m, _ := repoB.PickAGYModel(8); m != "gemini-3.8-flash-high" {
		t.Errorf("repository B picked %q, want gemini-3.8-flash-high while argon cools down", m)
	}

	// Cooldowns written to a ledger root before the state was shared still count.
	legacy := NewDispatcher(NewLedger(t.TempDir()))
	_ = os.MkdirAll(legacy.Ledger.Root(), 0o755)
	_ = os.WriteFile(filepath.Join(legacy.Ledger.Root(), "_providers.json"),
		[]byte(fmt.Sprintf(`{"agy:gemini-3.8":%q}`, time.Now().Add(time.Hour).Format(time.RFC3339))), 0o644)
	legacy.SharedDir = t.TempDir()
	if !coolingDown(legacy.loadExhaustedProviders(), "agy:gemini-3.8", time.Now()) {
		t.Error("legacy per-ledger cooldown must still be honoured")
	}
}

func TestRunJob_StallWhileFamilyServesOthersIsTransient(t *testing.T) {
	worktree := createGitRepo(t)
	binDir := t.TempDir()
	counter := filepath.Join(binDir, "counter")
	_ = os.WriteFile(counter, []byte("0"), 0o644)
	agy := installExecutableScript(t, binDir, "agy", fmt.Sprintf(`
CNT=$(cat "%[1]s")
echo $((CNT + 1)) > "%[1]s"
if [ "$CNT" -eq 0 ]; then
  exec sleep 30
fi
mkdir -p "%[2]s/pkg" && echo ok > "%[2]s/pkg/done.txt"
echo '{"status":"SUCCESS"}'
`, counter, worktree))
	ledger := NewLedger(t.TempDir())
	d := NewDispatcher(ledger)
	d.Executable = installExecutableScript(t, t.TempDir(), "dummy_runner", "exit 0\n")
	d.AgyPath = agy
	d.Sleep = func(time.Duration) {}
	d.AttemptTimeout = 2 * time.Second

	id, _ := d.Submit(SubmitSpec{Agent: "agy", Model: "gemini-3.8-flash-high", Worktree: worktree, Scope: []string{"pkg"}, Prompt: "Task"})
	// Another gemini-3.8 job finishes while this one is stuck.
	go func() {
		time.Sleep(500 * time.Millisecond)
		other, _ := ledger.Create(&Job{Ticket: "DEN-2", Agent: "agy", Model: "gemini-3.8-flash-medium", Status: StatusRunning})
		other.Status = StatusSucceeded
		_ = ledger.Save(other)
	}()
	if err := d.RunJob(context.Background(), id); err != nil {
		t.Fatal(err)
	}
	job, _ := ledger.Load(id)
	if job.Status != StatusSucceeded || job.Model != "gemini-3.8-flash-high" {
		t.Errorf("job = %s on %s, want succeeded on gemini-3.8-flash-high after a stall retry", job.Status, job.Model)
	}
	if coolingDown(d.loadExhaustedProviders(), "agy:gemini-3.8", time.Now()) {
		t.Error("a stall while the family serves other jobs must not cool the family down")
	}
}

func TestLiteLLMKeyFromDotEnv(t *testing.T) {
	t.Setenv("LITELLM_MASTER_KEY", "")
	d := NewDispatcher(NewLedger(t.TempDir()))
	worktree := t.TempDir()
	if got := d.liteLLMKey(worktree); got != "" {
		t.Errorf("no key anywhere, got %q", got)
	}
	_ = os.WriteFile(filepath.Join(worktree, ".env"), []byte("FOO=1\nLITELLM_MASTER_KEY=\"sk-from-env-file\"\n"), 0o644)
	if got := d.liteLLMKey(worktree); got != "sk-from-env-file" {
		t.Errorf("key from worktree .env = %q", got)
	}
	t.Setenv("LITELLM_MASTER_KEY", "sk-process")
	if got := d.liteLLMKey(worktree); got != "sk-process" {
		t.Errorf("process env must win, got %q", got)
	}
}

func TestRunAider_AuthenticationErrorIsProviderUnavailable(t *testing.T) {
	worktree := createGitRepo(t)
	ledger := NewLedger(t.TempDir())
	d := NewDispatcher(ledger)
	d.AiderPath = installExecutableScript(t, t.TempDir(), "aider", `
echo "litellm.AuthenticationError: AuthenticationError: OpenAIException - The api_key client option must be set"
echo "The API provider is not able to authenticate you. Check your API key."
exit 0
`)
	job, _ := ledger.Create(&Job{Ticket: "DEN-1", Agent: "aider", Model: copilotModel, Worktree: worktree})
	code, out, _ := d.runAider(context.Background(), job, "prompt")
	if code == 0 || !strings.Contains(out, ProviderUnavailable) {
		t.Fatalf("auth failure must be non-zero and marked, got code=%d out=%q", code, out)
	}
	if c := Classify(Outcome{ExitCode: code, LogTail: out}); c != ClassQuota {
		t.Errorf("auth failure classified %q, want QUOTA so copilot is skipped", c)
	}
}
