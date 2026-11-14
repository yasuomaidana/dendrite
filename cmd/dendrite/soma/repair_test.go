package soma

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"Dendrite/cmd/dendrite/internal/clitest"
	"Dendrite/pkg/repair"
	"Dendrite/pkg/testkit"
	"Dendrite/pkg/workcopy"
)

// repairRig is a consumer repository (the working directory, on DEN-890-fix/main) whose Cargo.toml uses r1,
// and the bare upstream repository of r1.
func repairRig(t *testing.T) (consumer, r1 string) {
	t.Helper()
	clitest.JsonOutput(t, false)
	t.Setenv("DENDRITE_HOME", t.TempDir())
	t.Setenv(workcopy.EnvWorkDir, t.TempDir())
	r1 = filepath.Join(t.TempDir(), "r1.git")
	testkit.Git(t, filepath.Dir(r1), "init", "-q", "--bare", "-b", "main", r1)
	seed := t.TempDir()
	testkit.GitInit(t, seed, "main")
	testkit.Git(t, seed, "commit", "-q", "--allow-empty", "-m", "init")
	testkit.Git(t, seed, "push", "-q", r1, "main")
	consumer = workcopy.Canon(t.TempDir())
	testkit.GitInit(t, consumer, "main")
	testkit.WriteFile(t, filepath.Join(consumer, "Cargo.toml"), "[dependencies]\nr1 = \"1.0\"\n", 0o644)
	testkit.Git(t, consumer, "add", "-A")
	testkit.Git(t, consumer, "commit", "-q", "-m", "init")
	testkit.Git(t, consumer, "checkout", "-q", "-b", "DEN-890-fix/main")
	t.Chdir(consumer)
	return consumer, r1
}

func runRepair(t *testing.T, args ...string) (string, error) {
	t.Helper()
	return runCommand(t, NewRepairCmd(), args...)
}

func TestRepairStartTakesTheTicketFromTheBranchAndReportsTheLink(t *testing.T) {
	consumer, r1 := repairRig(t)
	out, err := runRepair(t, "start", "r1", "--repo", r1)
	testkit.Check(t, err)
	testkit.Contains(t, out, "Repair DEN-890/r1 started", "Branch:       DEN-890-fix/main  (pushed to "+r1+")", `Was:          r1 = "1.0"  (Cargo.toml, published)`, "cd ")
	testkit.NotContains(t, out, "Part of:", "[!]")
	data, _ := os.ReadFile(filepath.Join(consumer, "Cargo.toml"))
	testkit.Contains(t, string(data), `branch = "DEN-890-fix/main"`)

	ledger, err := repair.Open()
	testkit.Check(t, err)
	link, err := ledger.Get("DEN-890/r1")
	testkit.Check(t, err)

	// Inside the repair copy, with JSON output: the next link names its parent.
	testkit.WriteFile(t, filepath.Join(link.Copy, "Cargo.toml"), "[dependencies]\nr2 = \"0.1\"\n", 0o644)
	r2 := filepath.Join(t.TempDir(), "r2.git")
	testkit.Git(t, filepath.Dir(r2), "clone", "-q", "--bare", r1, r2)
	t.Chdir(link.Copy)
	out, err = runRepair(t, "start", "r2", "--repo", r2, "--ticket", "DEN-890")
	testkit.Check(t, err)
	testkit.Contains(t, out, "Part of:      DEN-890/r1")
	clitest.JsonOutput(t, true)
	testkit.Git(t, link.Copy, "checkout", "-q", "-b", "DEN-891-more/main")
	out, err = runRepair(t, "start", "r2", "--repo", r2)
	testkit.Check(t, err)
	testkit.Contains(t, out, `"id": "DEN-891/r2"`, `"Warnings"`)
}

func TestRepairStartSaysWhatItNeeds(t *testing.T) {
	consumer, r1 := repairRig(t)
	for name, c := range map[string]struct {
		args []string
		want string
	}{
		"not a peer and no repository": {[]string{"start", "r1"}, "is not a synapse peer of this project: pass --repo"},
		"a dependency no manifest has": {[]string{"start", "nope", "--repo", r1}, "not found"},
	} {
		if _, err := runRepair(t, c.args...); err == nil || !strings.Contains(err.Error(), c.want) {
			t.Errorf("%s: %v", name, err)
		}
	}
	testkit.Git(t, consumer, "checkout", "-q", "main")
	if _, err := runRepair(t, "start", "r1", "--repo", r1); err == nil || !strings.Contains(err.Error(), "pass --ticket") {
		t.Errorf("a branch with no ticket: %v", err)
	}
}
