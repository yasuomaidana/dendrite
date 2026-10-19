package agentjobs

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestProviderKey(t *testing.T) {
	tests := []struct {
		agent, model string
		expected     string
	}{
		{"aider", copilotModel, providerCopilot},
		{"oxillama", oxillamaModel, providerOxillama},
		{"agy", "gemini-3.8-flash-high", "agy:gemini-3.8"},
		{"agy", "gemini-4-argon-medium", "agy:gemini-4-argon"},
		{"", "gemini-3.8-flash-low", "agy:gemini-3.8"},
		{"agy", "claude-sonnet-5-5-high", "agy:claude"},
		{"custom_agent", "unknown", providerAGY},
	}
	for _, tc := range tests {
		if got := providerKey(tc.agent, tc.model); got != tc.expected {
			t.Errorf("providerKey(%q, %q) = %q, want %q", tc.agent, tc.model, got, tc.expected)
		}
	}
}

func TestLadder(t *testing.T) {
	// 1. Complexity <= 4: requested model, other family, copilot, oxillama
	j1 := &Job{Agent: "agy", Model: "gemini-3.8-flash-high", Complexity: 3}
	l1 := ladder(j1)
	want := []tier{
		{"agy", "gemini-3.8-flash-high"}, {"agy", "gemini-4-argon-medium"},
		{"aider", copilotModel}, {"oxillama", oxillamaModel},
	}
	if fmt.Sprint(l1) != fmt.Sprint(want) {
		t.Fatalf("ladder = %+v, want %+v", l1, want)
	}

	// 2. Argon falls back to Gemini 3.8
	lArgon := ladder(&Job{Agent: "agy", Model: "gemini-4-argon-high", Complexity: 3})
	if lArgon[1].Model != "gemini-3.8-flash-high" {
		t.Errorf("argon counterpart = %+v, want gemini-3.8-flash-high", lArgon[1])
	}

	// 3. Complexity > 4 (e.g. 8) skips oxillama
	l8 := ladder(&Job{Agent: "agy", Model: "gemini-3.8-flash-high", Complexity: 8})
	if len(l8) != 3 {
		t.Fatalf("expected 3 tiers for complexity 8, got %d: %+v", len(l8), l8)
	}
	for _, tr := range l8 {
		if tr.Agent == "oxillama" {
			t.Errorf("complexity 8 must skip oxillama, got %+v", l8)
		}
	}

	// 4. Unknown model has no counterpart; agent "" defaults to agy
	lEmpty := ladder(&Job{Agent: "", Model: "default-model", Complexity: 4})
	if len(lEmpty) != 3 || lEmpty[0].Agent != "agy" {
		t.Errorf("empty agent must default to agy tier, got %+v", lEmpty)
	}

	// 5. Agent == "aider" does not include agy
	lAider := ladder(&Job{Agent: "aider", Model: copilotModel, Complexity: 4})
	if len(lAider) != 2 || lAider[0].Agent != "aider" {
		t.Errorf("aider job ladder mismatch, got %+v", lAider)
	}

	// 6. A family excluded by DENDRITE_AGY_MODELS is not used as counterpart
	t.Setenv("DENDRITE_AGY_MODELS", "gemini-3.8-flash-high")
	if l := ladder(j1); len(l) != 3 || l[1].Agent != "aider" {
		t.Errorf("disallowed counterpart must be skipped, got %+v", l)
	}
}

func TestNextTier_AndProviderMemory(t *testing.T) {
	tempLedger := NewLedger(t.TempDir())
	d := NewDispatcher(tempLedger)
	fakeNow := time.Date(2026, 10, 18, 12, 0, 0, 0, time.UTC)
	d.Now = func() time.Time { return fakeNow }
	cooling := fakeNow.Add(30 * time.Minute)

	// 1. Gemini 3.8 -> Argon
	jAgy := &Job{Agent: "agy", Model: "gemini-3.8-flash-high", Complexity: 3}
	next, ok := d.nextTier(jAgy, nil)
	if !ok || next.Agent != "agy" || next.Model != "gemini-4-argon-medium" {
		t.Errorf("nextTier from gemini-3.8 want argon, got: %+v, ok=%v", next, ok)
	}

	// 2. A Gemini 3.8 cooldown does not block Argon
	next, ok = d.nextTier(jAgy, map[string]time.Time{"agy:gemini-3.8": cooling})
	if !ok || next.Model != "gemini-4-argon-medium" {
		t.Errorf("argon must stay available while gemini-3.8 cools down, got: %+v", next)
	}

	// 3. Argon cooling down -> copilot
	next, ok = d.nextTier(jAgy, map[string]time.Time{"agy:gemini-4-argon": cooling})
	if !ok || next.Agent != "aider" || next.Model != copilotModel {
		t.Errorf("nextTier with argon cooling want aider, got: %+v, ok=%v", next, ok)
	}

	// 4. Legacy bare "agy" cooldown covers every AGY family
	next, ok = d.nextTier(jAgy, map[string]time.Time{providerAGY: cooling})
	if !ok || next.Agent != "aider" {
		t.Errorf("legacy agy cooldown must skip argon, got: %+v", next)
	}

	// 5. Argon and copilot cooling down -> oxillama; expired cooldowns are ignored
	next, ok = d.nextTier(jAgy, map[string]time.Time{"agy:gemini-4-argon": cooling, providerCopilot: cooling})
	if !ok || next.Agent != "oxillama" || next.Model != oxillamaModel {
		t.Errorf("want oxillama, got: %+v, ok=%v", next, ok)
	}
	next, ok = d.nextTier(jAgy, map[string]time.Time{"agy:gemini-4-argon": fakeNow.Add(-time.Minute)})
	if !ok || next.Model != "gemini-4-argon-medium" {
		t.Errorf("expired cooldown must be ignored, got: %+v", next)
	}

	// 6. All remaining tiers cooling down -> false
	all := map[string]time.Time{"agy:gemini-4-argon": cooling, providerCopilot: cooling, providerOxillama: cooling}
	if _, ok := d.nextTier(jAgy, all); ok {
		t.Error("expected ok=false when all next tiers cooling down")
	}

	// 7. From argon (after an escalation), gemini-3.8 cooling down -> copilot, not back to 3.8
	jArgon := &Job{Agent: "agy", Model: "gemini-4-argon-medium", Complexity: 3}
	next, ok = d.nextTier(jArgon, map[string]time.Time{"agy:gemini-3.8": cooling})
	if !ok || next.Agent != "aider" {
		t.Errorf("argon with gemini-3.8 cooling want aider, got: %+v", next)
	}

	// 8. Last tier reached (oxillama) -> false
	if _, ok := d.nextTier(&Job{Agent: "oxillama", Model: oxillamaModel, Complexity: 3}, nil); ok {
		t.Error("expected ok=false when at last tier")
	}

	// 9. A non-AGY custom agent has no AGY tiers and goes to copilot
	next, ok = d.nextTier(&Job{Agent: "custom", Model: "gemini-3.8-flash-high", Complexity: 3}, nil)
	if !ok || next.Agent != "aider" {
		t.Errorf("custom agent want aider, got: %+v, ok=%v", next, ok)
	}
}

func TestDispatcher_Now_DefaultAndCustom(t *testing.T) {
	d := &Dispatcher{}
	t1 := d.now()
	if t1.IsZero() || time.Since(t1) > 5*time.Second {
		t.Errorf("default now() returned invalid time: %v", t1)
	}

	fixed := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	d.Now = func() time.Time { return fixed }
	if got := d.now(); !got.Equal(fixed) {
		t.Errorf("custom now() = %v, want %v", got, fixed)
	}
}

func TestRetryAfter(t *testing.T) {
	cases := []struct {
		input    string
		expected time.Duration
	}{
		{"Retry-After: 120", 120 * time.Second},
		{"retry_after=30", 30 * time.Second},
		{"retry-after: 5", 5 * time.Second},
		{"retry after 45", 45 * time.Second},
		{"\"retry_after\": 90", 90 * time.Second},
		{"Retry-After: 0", DefaultProviderCooldown},
		{"Retry-After: -10", DefaultProviderCooldown},
		{"no retry hint present", DefaultProviderCooldown},
		{`"short_error":"RESOURCE_EXHAUSTED (code 429): Individual quota reached. Resets in 140h4m53s."`, 140*time.Hour + 4*time.Minute + 53*time.Second},
		{"Resets in 45m.", 45 * time.Minute},
		{"Resets in soon", DefaultProviderCooldown},
		{"", DefaultProviderCooldown},
	}
	for _, tc := range cases {
		got := retryAfter(tc.input)
		if got != tc.expected {
			t.Errorf("retryAfter(%q) = %v, want %v", tc.input, got, tc.expected)
		}
	}
}

func TestProviderMemory_LoadMarkAndReset(t *testing.T) {
	tempLedger := NewLedger(t.TempDir())
	d := NewDispatcher(tempLedger)
	fakeNow := time.Date(2026, 10, 18, 12, 0, 0, 0, time.UTC)
	d.Now = func() time.Time { return fakeNow }

	// Empty providers file returns empty map
	empty := d.loadExhaustedProviders()
	if len(empty) != 0 {
		t.Fatalf("expected empty map on missing file, got %+v", empty)
	}

	// earliestReset on empty map defaults to d.now().Add(DefaultProviderCooldown)
	erDefault := d.earliestReset()
	if want := fakeNow.Add(DefaultProviderCooldown); !erDefault.Equal(want) {
		t.Errorf("earliestReset default = %v, want %v", erDefault, want)
	}

	// Mark one provider
	t1 := fakeNow.Add(2 * time.Hour)
	if err := d.markProviderExhausted("copilot", t1); err != nil {
		t.Fatalf("markProviderExhausted failed: %v", err)
	}

	erSingle := d.earliestReset()
	if !erSingle.Equal(t1) {
		t.Errorf("earliestReset with 1 provider = %v, want %v", erSingle, t1)
	}

	// Mark multiple providers: earlier and later
	tEarlier := fakeNow.Add(30 * time.Minute)
	tLater := fakeNow.Add(5 * time.Hour)
	_ = d.markProviderExhausted("oxillama", tEarlier)
	_ = d.markProviderExhausted("agy", tLater)

	erMulti := d.earliestReset()
	if !erMulti.Equal(tEarlier) {
		t.Errorf("earliestReset with multiple providers = %v, want %v", erMulti, tEarlier)
	}

	// Verify all loaded
	all := d.loadExhaustedProviders()
	if len(all) != 3 {
		t.Errorf("expected 3 providers in map, got %d", len(all))
	}

	// Error when ledger root cannot be created
	f := filepath.Join(t.TempDir(), "regular_file")
	if err := os.WriteFile(f, []byte("contents"), 0644); err != nil {
		t.Fatalf("failed to create file: %v", err)
	}
	badD := NewDispatcher(NewLedger(filepath.Join(f, "child_dir")))
	if err := badD.markProviderExhausted("agy", fakeNow); err == nil {
		t.Error("expected error when ledger root directory cannot be created")
	}

	// Error in writeFileAtomic via writeFile hook
	origWriteFile := writeFile
	writeFile = func(name string, data []byte, perm os.FileMode) error {
		return errors.New("simulated write failure")
	}
	errWrite := d.markProviderExhausted("agy", fakeNow)
	writeFile = origWriteFile
	if errWrite == nil || !strings.Contains(errWrite.Error(), "simulated write failure") {
		t.Errorf("expected simulated write failure, got %v", errWrite)
	}
}

func TestSkipExhaustedInitialTier(t *testing.T) {
	tempLedger := NewLedger(t.TempDir())
	d := NewDispatcher(tempLedger)
	fakeNow := time.Date(2026, 10, 18, 12, 0, 0, 0, time.UTC)
	d.Now = func() time.Time { return fakeNow }

	// 1. Not in exhausted map -> does nothing
	j1, _ := tempLedger.Create(&Job{Ticket: "DEN-1", Agent: "agy", Model: "gemini-3.8-flash-high"})
	d.skipExhaustedInitialTier(j1)
	if j1.Agent != "agy" || j1.Tier != 0 {
		t.Errorf("job should remain unchanged when not exhausted: %+v", j1)
	}

	// 2. In exhausted map, but expired -> does nothing
	_ = d.markProviderExhausted(providerAGY, fakeNow.Add(-5*time.Minute))
	j2, _ := tempLedger.Create(&Job{Ticket: "DEN-2", Agent: "agy", Model: "gemini-3.8-flash-high"})
	d.skipExhaustedInitialTier(j2)
	if j2.Agent != "agy" || j2.Tier != 0 {
		t.Errorf("job should remain unchanged when provider cooldown expired: %+v", j2)
	}

	// 3. In exhausted map, cooling down -> moves to next tier
	_ = d.markProviderExhausted(providerAGY, fakeNow.Add(30*time.Minute))
	j3, _ := tempLedger.Create(&Job{Ticket: "DEN-3", Agent: "agy", Model: "gemini-3.8-flash-high"})
	d.skipExhaustedInitialTier(j3)
	if j3.Agent != "aider" || j3.Model != copilotModel || j3.Tier != 1 {
		t.Errorf("job should escalate to aider: %+v", j3)
	}
	evs, _ := tempLedger.Events(j3.ID)
	var foundEscalated bool
	for _, ev := range evs {
		if ev.Type == "escalated" && ev.Data["reason"] == "provider cooling down" {
			foundEscalated = true
		}
	}
	if !foundEscalated {
		t.Error("missing escalated event for provider cooling down")
	}

	// 4. All next tiers cooling down -> nextTier returns false -> remains unchanged
	_ = d.markProviderExhausted(providerCopilot, fakeNow.Add(30*time.Minute))
	_ = d.markProviderExhausted(providerOxillama, fakeNow.Add(30*time.Minute))
	j4, _ := tempLedger.Create(&Job{Ticket: "DEN-4", Agent: "agy", Model: "gemini-3.8-flash-high"})
	d.skipExhaustedInitialTier(j4)
	if j4.Agent != "agy" || j4.Tier != 0 {
		t.Errorf("job should not escalate when no next tier is available: %+v", j4)
	}
}

func TestHandleQuotaFallback(t *testing.T) {
	tempLedger := NewLedger(t.TempDir())
	d := NewDispatcher(tempLedger)
	fakeNow := time.Date(2026, 10, 18, 12, 0, 0, 0, time.UTC)
	d.Now = func() time.Time { return fakeNow }

	ctx := context.Background()

	// 1. Quota fallback escalates from gemini-3.8 to argon
	j1, _ := tempLedger.Create(&Job{
		Ticket:         "DEN-990",
		Agent:          "agy",
		Model:          "gemini-3.8-flash-high",
		Complexity:     3,
		ConversationID: "conv-123",
	})
	ok1, prompt1, err1 := d.handleQuotaFallback(ctx, j1, "Original prompt", "HTTP 429: retry_after=30", false)
	if err1 != nil || !ok1 {
		t.Fatalf("handleQuotaFallback failed: ok=%v, err=%v", ok1, err1)
	}
	if j1.Agent != "agy" || j1.Model != "gemini-4-argon-medium" || j1.Tier != 1 || j1.ConversationID != "" || j1.Status != StatusEscalated {
		t.Errorf("job escalation state mismatch: %+v", j1)
	}
	if !strings.Contains(prompt1, "Original prompt") || !strings.Contains(prompt1, "## Handover capsule") || !strings.Contains(prompt1, "Reason: quota") {
		t.Errorf("handover prompt format mismatch: %s", prompt1)
	}
	capsuleData, cErr := os.ReadFile(tempLedger.CapsulePath(j1.ID))
	if cErr != nil || !strings.Contains(string(capsuleData), "Reason: quota") {
		t.Errorf("capsule file missing or invalid: %v, %s", cErr, string(capsuleData))
	}

	// 2. Throttled timeout quota fallback
	j2, _ := tempLedger.Create(&Job{
		Ticket:     "DEN-990",
		Agent:      "agy",
		Model:      "gemini-3.8-flash-high",
		Complexity: 3,
	})
	ok2, prompt2, err2 := d.handleQuotaFallback(ctx, j2, "Prompt", "timeout occurred", true)
	if err2 != nil || !ok2 {
		t.Fatalf("handleQuotaFallback timedOut failed: ok=%v, err=%v", ok2, err2)
	}
	if !strings.Contains(prompt2, "Reason: throttled (attempt deadline exceeded)") {
		t.Errorf("expected throttled reason in prompt: %s", prompt2)
	}

	// 3. Any quota error on gemini-3.8 -> escalates to agy/gemini-4-argon-medium
	j3, _ := tempLedger.Create(&Job{
		Ticket:     "DEN-990",
		Agent:      "agy",
		Model:      "gemini-3.8-flash-high",
		Complexity: 3,
	})
	ok3, prompt3, err3 := d.handleQuotaFallback(ctx, j3, "Prompt", "Rate limit for model reached", false)
	if err3 != nil || !ok3 {
		t.Fatalf("handleQuotaFallback modelScoped failed: ok=%v, err=%v", ok3, err3)
	}
	if j3.Agent != "agy" || j3.Model != "gemini-4-argon-medium" {
		t.Errorf("expected escalation to agy/gemini-4-argon-medium, got: %s/%s", j3.Agent, j3.Model)
	}
	if !strings.Contains(prompt3, "## Handover capsule") {
		t.Errorf("expected capsule in prompt: %s", prompt3)
	}

	// 4. No tier left -> parks job
	jPark, _ := tempLedger.Create(&Job{
		Ticket:     "DEN-990",
		Agent:      "oxillama",
		Model:      oxillamaModel,
		Complexity: 3,
	})
	okPark, promptPark, errPark := d.handleQuotaFallback(ctx, jPark, "Prompt", "quota exhausted", false)
	if errPark != nil {
		t.Fatalf("handleQuotaFallback park failed: %v", errPark)
	}
	if okPark {
		t.Error("expected ok=false when job is parked")
	}
	if promptPark != "" {
		t.Errorf("expected empty prompt when parked, got %q", promptPark)
	}
	if jPark.Status != StatusParked {
		t.Errorf("job status = %q, want %q", jPark.Status, StatusParked)
	}
	if jPark.ResumeAfter.IsZero() {
		t.Error("expected non-zero ResumeAfter for parked job")
	}

	// 5. Error writing capsule
	jCapsuleFail := &Job{
		ID:     "DEN-bad-capsule",
		Ticket: "DEN-990",
		Agent:  "agy",
		Model:  "gemini-3.8-flash-high",
	}
	_ = os.WriteFile(tempLedger.JobDir(jCapsuleFail.ID), []byte("regular file"), 0644)
	_, _, errCap := d.handleQuotaFallback(ctx, jCapsuleFail, "Prompt", "quota", false)
	if errCap == nil || !strings.Contains(errCap.Error(), "failed to write capsule") {
		t.Errorf("expected failed to write capsule error, got: %v", errCap)
	}

	// 6. Error recording exhausted provider
	badLedger := NewLedger(filepath.Join(tempLedger.JobDir(jCapsuleFail.ID), "sub_ledger"))
	dBad := NewDispatcher(badLedger)
	_, _, errProv := dBad.handleQuotaFallback(ctx, j1, "Prompt", "quota", false)
	if errProv == nil || !strings.Contains(errProv.Error(), "failed to record exhausted provider") {
		t.Errorf("expected failed to record exhausted provider error, got: %v", errProv)
	}

	// 7. Error saving job on escalation
	jSaveFail := &Job{
		ID:         "DEN-save-fail-2",
		Ticket:     "DEN-990",
		Agent:      "agy",
		Model:      "gemini-3.8-flash-high",
		Complexity: 3,
	}
	_ = os.MkdirAll(tempLedger.JobDir(jSaveFail.ID), 0755)
	origWriteFile := writeFile
	writeFile = func(name string, data []byte, perm os.FileMode) error {
		if strings.Contains(name, "job.json") {
			return errors.New("simulated save error")
		}
		return origWriteFile(name, data, perm)
	}
	_, _, errSave := d.handleQuotaFallback(ctx, jSaveFail, "Prompt", "quota", false)
	writeFile = origWriteFile
	if errSave == nil || !strings.Contains(errSave.Error(), "simulated save error") {
		t.Errorf("expected simulated save error, got: %v", errSave)
	}
}

func TestBuildCapsule(t *testing.T) {
	worktree := createGitRepo(t)
	targetFile := filepath.Join(worktree, "scoped.txt")
	if err := os.WriteFile(targetFile, []byte("initial line\n"), 0644); err != nil {
		t.Fatalf("failed to write initial file: %v", err)
	}

	cmdAdd := exec.Command("git", "add", "scoped.txt")
	cmdAdd.Dir = worktree
	_ = cmdAdd.Run()
	cmdCommit := exec.Command("git", "commit", "-m", "add scoped.txt")
	cmdCommit.Dir = worktree
	_ = cmdCommit.Run()

	// Modify scoped.txt so git diff has content
	if err := os.WriteFile(targetFile, []byte("initial line\nadded second line\n"), 0644); err != nil {
		t.Fatalf("failed to modify scoped.txt: %v", err)
	}

	tempLedger := NewLedger(t.TempDir())
	d := NewDispatcher(tempLedger)

	job := &Job{
		Ticket:   "DEN-990",
		Agent:    "agy",
		Model:    "gemini-3.8-flash-high",
		Worktree: worktree,
		Scope:    []string{"scoped.txt"},
	}

	ctx := context.Background()
	capsule := d.buildCapsule(ctx, job, "Agent error output tail", "quota")

	if !strings.Contains(capsule, "- Ticket: DEN-990") {
		t.Errorf("capsule missing ticket: %s", capsule)
	}
	if !strings.Contains(capsule, "- Previous tier: agy/gemini-3.8-flash-high") {
		t.Errorf("capsule missing previous tier: %s", capsule)
	}
	if !strings.Contains(capsule, "- Reason: quota") {
		t.Errorf("capsule missing reason: %s", capsule)
	}
	if !strings.Contains(capsule, "### Partial work (git diff of the scope)") {
		t.Errorf("capsule missing git diff header: %s", capsule)
	}
	if !strings.Contains(capsule, "+added second line") {
		t.Errorf("capsule missing git diff content: %s", capsule)
	}
	if !strings.Contains(capsule, "### Last agent output") {
		t.Errorf("capsule missing last agent output header: %s", capsule)
	}
	if !strings.Contains(capsule, "Agent error output tail") {
		t.Errorf("capsule missing agent output: %s", capsule)
	}

	// Large diff and large output to exercise tail() truncation inside buildCapsule
	hugeOutput := strings.Repeat("O", 5000)
	capsuleHuge := d.buildCapsule(ctx, job, hugeOutput, "quota")
	if !strings.Contains(capsuleHuge, strings.Repeat("O", 4000)) {
		t.Error("expected 4000 chars of output in capsule")
	}
	if strings.Contains(capsuleHuge, strings.Repeat("O", 4001)) {
		t.Error("output in capsule should be truncated to 4000 chars")
	}

	// Empty worktree and empty scope and empty output
	emptyJob := &Job{Ticket: "DEN-EMPTY", Agent: "agy", Model: "default"}
	capsuleEmpty := d.buildCapsule(ctx, emptyJob, "   ", "quota")
	if strings.Contains(capsuleEmpty, "### Partial work") {
		t.Error("empty job should not have partial work section")
	}
	if strings.Contains(capsuleEmpty, "### Last agent output") {
		t.Error("empty output should not have last output section")
	}

	// Git failure (invalid git path)
	dInvalidGit := NewDispatcher(tempLedger)
	dInvalidGit.GitPath = "/nonexistent/git/bin"
	capsuleGitFail := dInvalidGit.buildCapsule(ctx, job, "", "quota")
	if strings.Contains(capsuleGitFail, "### Partial work") {
		t.Error("failed git command should omit partial work section")
	}
}

func TestGitPath_DefaultAndCustom(t *testing.T) {
	d := &Dispatcher{}
	if got := d.gitPath(); got != "git" {
		t.Errorf("default gitPath() = %q, want 'git'", got)
	}
	d.GitPath = "/usr/bin/git"
	if got := d.gitPath(); got != "/usr/bin/git" {
		t.Errorf("custom gitPath() = %q, want '/usr/bin/git'", got)
	}
}

func TestTail(t *testing.T) {
	if got := tail("hello", 10); got != "hello" {
		t.Errorf("tail <= n failed: %q", got)
	}
	if got := tail("hello", 5); got != "hello" {
		t.Errorf("tail == n failed: %q", got)
	}
	if got := tail("hello world", 5); got != "world" {
		t.Errorf("tail > n failed: %q", got)
	}
}

func TestEnvOr(t *testing.T) {
	t.Setenv("TEST_ENV_OR_VAR", "set_value")
	if got := envOr("TEST_ENV_OR_VAR", "fallback"); got != "set_value" {
		t.Errorf("envOr with set env = %q, want 'set_value'", got)
	}
	if got := envOr("TEST_ENV_OR_VAR_UNSET_XYZ", "fallback"); got != "fallback" {
		t.Errorf("envOr with unset env = %q, want 'fallback'", got)
	}
}

func TestRunAider(t *testing.T) {
	worktree := t.TempDir()
	binDir := t.TempDir()

	// Existing file
	existingFile := filepath.Join(worktree, "exist.go")
	if err := os.WriteFile(existingFile, []byte("package exist"), 0644); err != nil {
		t.Fatalf("failed to create existing file: %v", err)
	}
	// Subdirectory
	subDir := filepath.Join(worktree, "subdir")
	if err := os.MkdirAll(subDir, 0755); err != nil {
		t.Fatalf("failed to create subdir: %v", err)
	}

	argsLog := filepath.Join(binDir, "aider_args.txt")
	t.Setenv("AIDER_ARGS_OUT", argsLog)

	aiderScript := installExecutableScript(t, binDir, "aider_dummy", fmt.Sprintf(`
for arg in "$@"; do
  echo "$arg" >> "%s"
done
echo "aider ran successfully"
exit 0
`, argsLog))

	tempLedger := NewLedger(t.TempDir())
	d := NewDispatcher(tempLedger)
	d.AiderPath = aiderScript
	d.LiteLLMURL = "http://127.0.0.1:1/v1"
	d.LiteLLMKey = "test-copilot-key"

	job := &Job{
		ID:         "DEN-990-aider-test",
		Ticket:     "DEN-990",
		Agent:      "aider",
		Model:      copilotModel,
		Worktree:   worktree,
		Scope:      []string{"exist.go", "subdir", "nonexistent.go"},
		Complexity: 3,
	}
	if err := os.MkdirAll(tempLedger.JobDir(job.ID), 0755); err != nil {
		t.Fatalf("failed to create job dir: %v", err)
	}

	ctx := context.Background()
	code, out, err := d.runAider(ctx, job, "Implement feature in exist.go")
	if err != nil {
		t.Fatalf("runAider failed: %v", err)
	}
	if code != 0 {
		t.Errorf("expected exit code 0, got %d", code)
	}
	if !strings.Contains(out, "aider ran successfully") {
		t.Errorf("output mismatch: %q", out)
	}

	// Verify prompt-aider.md was written
	promptContent, pErr := os.ReadFile(filepath.Join(tempLedger.JobDir(job.ID), "prompt-aider.md"))
	if pErr != nil || string(promptContent) != "Implement feature in exist.go" {
		t.Errorf("prompt-aider.md content = %q, err=%v", string(promptContent), pErr)
	}

	// Verify args passed to aider
	rawArgs, aErr := os.ReadFile(argsLog)
	if aErr != nil {
		t.Fatalf("failed to read aider args log: %v", aErr)
	}
	argLines := strings.Split(strings.TrimSpace(string(rawArgs)), "\n")
	hasArg := func(target string) bool {
		for _, a := range argLines {
			if a == target {
				return true
			}
		}
		return false
	}

	if !hasArg("--model") || !hasArg("openai/"+copilotModel) {
		t.Errorf("missing model arg in %v", argLines)
	}
	if !hasArg("--openai-api-base") || !hasArg("http://127.0.0.1:1/v1") {
		t.Errorf("missing base url in %v", argLines)
	}
	if !hasArg("--openai-api-key") || !hasArg("test-copilot-key") {
		t.Errorf("missing api key in %v", argLines)
	}
	if !hasArg("--message-file") || !hasArg(filepath.Join(tempLedger.JobDir(job.ID), "prompt-aider.md")) {
		t.Errorf("missing message-file in %v", argLines)
	}
	// Assert it receives --file ONLY for existing scope files:
	if !hasArg("exist.go") {
		t.Errorf("missing --file exist.go in %v", argLines)
	}
	if hasArg("subdir") {
		t.Errorf("should NOT contain directory 'subdir' in %v", argLines)
	}
	if hasArg("nonexistent.go") {
		t.Errorf("should NOT contain nonexistent file in %v", argLines)
	}

	// Test attempt deadline kills the process group
	sleepScript := installExecutableScript(t, binDir, "aider_hang", `
trap 'exit 130' TERM INT
sleep 30
exit 0
`)
	dHang := NewDispatcher(tempLedger)
	dHang.AiderPath = sleepScript
	dHang.LiteLLMURL = "http://127.0.0.1:1/v1"

	ctxTimeout, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()

	hangCode, _, hangErr := dHang.runAider(ctxTimeout, job, "hang prompt")
	if hangErr == nil {
		t.Error("expected deadline exceeded error from hang script")
	}
	if hangCode == 0 {
		t.Errorf("expected non-zero exit code on cancellation, got %d", hangCode)
	}

	// Test non-zero exit code and tail truncation
	failScript := installExecutableScript(t, binDir, "aider_fail", `
for i in $(seq 1 100); do
  printf "01234567890123456789012345678901234567890123456789"
done
printf "END_OF_AIDER_ERROR\n"
exit 42
`)
	dFail := NewDispatcher(tempLedger)
	dFail.AiderPath = failScript
	dFail.LiteLLMURL = "http://127.0.0.1:1/v1"

	failCode, failOut, failErr := dFail.runAider(context.Background(), job, "fail prompt")
	if failErr == nil {
		t.Error("expected exit error")
	}
	if failCode != 42 {
		t.Errorf("expected exit code 42, got %d", failCode)
	}
	if len(failOut) != maxCapsuleOutput {
		t.Errorf("expected tail output length %d, got %d", maxCapsuleOutput, len(failOut))
	}
	if !strings.HasSuffix(failOut, "END_OF_AIDER_ERROR\n") {
		t.Errorf("expected tail to preserve suffix, got %q", failOut)
	}

	// Test prompt write failure
	jobBad := &Job{ID: "unwritable_job_id"}
	dBad := NewDispatcher(tempLedger)
	bCode, _, bErr := dBad.runAider(context.Background(), jobBad, "prompt")
	if bErr == nil || bCode != 1 {
		t.Errorf("expected write error and code 1, got code=%d, err=%v", bCode, bErr)
	}

	// Test LiteLLMURL and LiteLLMKey env var fallbacks
	t.Setenv("DENDRITE_LITELLM_URL", "http://127.0.0.1:9999/v1")
	t.Setenv("LITELLM_MASTER_KEY", "env-key-secret")
	_ = os.Remove(argsLog)
	dEnv := NewDispatcher(tempLedger)
	dEnv.AiderPath = aiderScript
	dEnv.LiteLLMURL = ""
	dEnv.LiteLLMKey = ""
	_, _, _ = dEnv.runAider(context.Background(), job, "prompt")
	envArgs, _ := os.ReadFile(argsLog)
	if !strings.Contains(string(envArgs), "http://127.0.0.1:9999/v1") {
		t.Errorf("env fallback URL not used: %s", string(envArgs))
	}
	if !strings.Contains(string(envArgs), "env-key-secret") {
		t.Errorf("env fallback Key not used: %s", string(envArgs))
	}

	// Test default AiderPath ("aider") fallback resolved from PATH
	tempBinDir := t.TempDir()
	installExecutableScript(t, tempBinDir, "aider", "echo 'default aider run'; exit 0\n")
	t.Setenv("PATH", tempBinDir+":"+os.Getenv("PATH"))

	dDefaultBin := NewDispatcher(tempLedger)
	dDefaultBin.AiderPath = ""
	dDefaultBin.LiteLLMURL = "http://127.0.0.1:1/v1"
	codeDef, outDef, errDef := dDefaultBin.runAider(context.Background(), job, "prompt")
	if errDef != nil || codeDef != 0 || !strings.Contains(outDef, "default aider run") {
		t.Errorf("default aider in PATH failed: code=%d, out=%q, err=%v", codeDef, outDef, errDef)
	}
}

func TestResumeParked(t *testing.T) {
	tempLedger := NewLedger(t.TempDir())
	binDir := t.TempDir()
	dummyRunner := installExecutableScript(t, binDir, "dummy_dendrite", "exit 0\n")

	d := NewDispatcher(tempLedger)
	d.Executable = dummyRunner
	fakeNow := time.Date(2026, 10, 18, 12, 0, 0, 0, time.UTC)
	d.Now = func() time.Time { return fakeNow }

	// Mark provider "agy" as cooling down until +30 min
	if err := d.markProviderExhausted(providerAGY, fakeNow.Add(30*time.Minute)); err != nil {
		t.Fatalf("failed to mark provider exhausted: %v", err)
	}

	// Job 1: Parked, resume_after passed, provider "copilot" (aider) is NOT cooling -> should resume
	j1 := &Job{
		Ticket:      "DEN-1",
		Agent:       "aider",
		Model:       copilotModel,
		Status:      StatusParked,
		ResumeAfter: fakeNow.Add(-10 * time.Minute),
	}
	created1, err := tempLedger.Create(j1)
	if err != nil {
		t.Fatalf("failed to create j1: %v", err)
	}
	created1.Status = StatusParked
	created1.ResumeAfter = fakeNow.Add(-10 * time.Minute)
	_ = tempLedger.Save(created1)

	// Job 2: Parked, resume_after passed, but provider "agy" IS still cooling -> should NOT resume
	j2 := &Job{
		Ticket:      "DEN-2",
		Agent:       "agy",
		Model:       "gemini-3.8-flash-high",
		Status:      StatusParked,
		ResumeAfter: fakeNow.Add(-10 * time.Minute),
	}
	created2, err := tempLedger.Create(j2)
	if err != nil {
		t.Fatalf("failed to create j2: %v", err)
	}
	created2.Status = StatusParked
	created2.ResumeAfter = fakeNow.Add(-10 * time.Minute)
	_ = tempLedger.Save(created2)

	// Job 3: Parked, but resume_after in future -> should NOT resume
	j3 := &Job{
		Ticket:      "DEN-3",
		Agent:       "aider",
		Model:       copilotModel,
		Status:      StatusParked,
		ResumeAfter: fakeNow.Add(15 * time.Minute),
	}
	created3, err := tempLedger.Create(j3)
	if err != nil {
		t.Fatalf("failed to create j3: %v", err)
	}
	created3.Status = StatusParked
	created3.ResumeAfter = fakeNow.Add(15 * time.Minute)
	_ = tempLedger.Save(created3)

	// Job 4: Queued (not parked) -> should NOT resume
	j4 := &Job{
		Ticket: "DEN-4",
		Agent:  "aider",
		Model:  copilotModel,
		Status: StatusQueued,
	}
	created4, err := tempLedger.Create(j4)
	if err != nil {
		t.Fatalf("failed to create j4: %v", err)
	}

	resumed, err := d.ResumeParked()
	if err != nil {
		t.Fatalf("ResumeParked failed: %v", err)
	}

	if len(resumed) != 1 || resumed[0] != created1.ID {
		t.Fatalf("expected resumed=[%s], got %v", created1.ID, resumed)
	}

	loaded1, _ := tempLedger.Load(created1.ID)
	if loaded1.Status != StatusQueued {
		t.Errorf("j1 status = %q, want queued", loaded1.Status)
	}
	if loaded1.PGID <= 0 {
		t.Errorf("j1 PGID = %d, want > 0", loaded1.PGID)
	}

	loaded2, _ := tempLedger.Load(created2.ID)
	if loaded2.Status != StatusParked {
		t.Errorf("j2 status = %q, want parked", loaded2.Status)
	}

	loaded3, _ := tempLedger.Load(created3.ID)
	if loaded3.Status != StatusParked {
		t.Errorf("j3 status = %q, want parked", loaded3.Status)
	}

	loaded4, _ := tempLedger.Load(created4.ID)
	if loaded4.Status != StatusQueued {
		t.Errorf("j4 status = %q, want queued", loaded4.Status)
	}

	// Error branch: Ledger.List() fails
	// A missing root lists as empty by design; a root that is a regular file cannot be read.
	fileRoot := filepath.Join(t.TempDir(), "not-a-dir")
	if err := os.WriteFile(fileRoot, nil, 0o644); err != nil {
		t.Fatal(err)
	}
	badLedger := NewLedger(fileRoot)
	dBad := NewDispatcher(badLedger)
	_, listErr := dBad.ResumeParked()
	if listErr == nil {
		t.Error("expected error when Ledger.List() fails")
	}

	// Error branch: d.Retry() fails
	// Needs a still-parked, eligible job: the earlier part of this test already resumed j4.
	retryLedger := NewLedger(t.TempDir())
	parked, err := retryLedger.Create(&Job{Ticket: "DEN-990", Agent: "agy", Status: StatusParked, ResumeAfter: fakeNow.Add(-time.Minute)})
	if err != nil {
		t.Fatal(err)
	}
	parked.Status = StatusParked
	parked.ResumeAfter = fakeNow.Add(-time.Minute)
	if err := retryLedger.Save(parked); err != nil {
		t.Fatal(err)
	}
	dRetryFail := NewDispatcher(retryLedger)
	dRetryFail.Executable = "/nonexistent/path/cannot/run"
	dRetryFail.Now = func() time.Time { return fakeNow }
	_, retryErr := dRetryFail.ResumeParked()
	if retryErr == nil {
		t.Error("expected error when d.Retry fails")
	}
}
