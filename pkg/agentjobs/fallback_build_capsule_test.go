package agentjobs

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

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
