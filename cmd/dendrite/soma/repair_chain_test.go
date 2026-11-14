package soma

import (
	"path/filepath"
	"strings"
	"testing"

	"Dendrite/cmd/dendrite/internal/clitest"
	"Dendrite/pkg/pullrequest/prtest"
	"Dendrite/pkg/repair"
	"Dendrite/pkg/testkit"
)

// startedRepair is a repair rig with the repair DEN-890/r1 started and a local gh.
func startedRepair(t *testing.T) (link repair.Link, gh *prtest.GitHub) {
	t.Helper()
	_, r1 := repairRig(t)
	gh = prtest.New(t)
	_, err := runRepair(t, "start", "r1", "--repo", r1)
	testkit.Check(t, err)
	ledger, err := repair.Open()
	testkit.Check(t, err)
	link, err = ledger.Get("DEN-890/r1")
	testkit.Check(t, err)
	return link, gh
}

func repairOut(t *testing.T, args ...string) string {
	t.Helper()
	out, err := runRepair(t, args...)
	testkit.Check(t, err)
	return out
}

func TestRepairStatusPrAndAdvanceFollowARepairToItsRelease(t *testing.T) {
	link, gh := startedRepair(t)
	testkit.Contains(t, repairOut(t, "status"), filepath.Base(link.Consumer)+"  (DEN-890)", "└─ r1  [repairing]  branch DEN-890-fix/main in "+link.Copy)
	testkit.Contains(t, repairOut(t, "advance"), "[=] DEN-890/r1: repairing; no pull request yet")

	testkit.Contains(t, repairOut(t, "pr"), "[✓] DEN-890/r1: https://github.example/o/r/pull/1")
	testkit.Contains(t, repairOut(t, "pr"), "Every repair has its pull request")
	testkit.Contains(t, repairOut(t, "status"), "pull request: https://github.example/o/r/pull/1")

	testkit.Git(t, link.Copy, "commit", "-q", "--allow-empty", "-m", "fix")
	testkit.Git(t, link.Copy, "push", "-q", "origin", "HEAD:main")
	sha := testkit.Git(t, link.Copy, "rev-parse", "HEAD")
	gh.Merge(link.Branch, "main", sha)
	testkit.Contains(t, repairOut(t, "advance"), "[✓] DEN-890/r1: repairing -> merged")
	testkit.Contains(t, repairOut(t, "status"), "[merged]  pinned to "+link.Upstream+" at "+sha)
	out, err := runBranch(t, "show")
	testkit.Check(t, err)
	testkit.Contains(t, out, "Repair:             r1  [merged]")

	testkit.Git(t, link.Copy, "tag", "v1.0.1")
	testkit.Git(t, link.Copy, "push", "-q", "origin", "v1.0.1")
	testkit.Contains(t, repairOut(t, "advance"), "merged -> released; released as v1.0.1")
	testkit.Contains(t, repairOut(t, "status"), "[released]  released as v1.0.1")

	clitest.JsonOutput(t, true)
	testkit.Contains(t, repairOut(t, "status"), `"depth": 0`, `"stage": "released"`)
	testkit.Equal(t, "nothing left to advance", strings.TrimSpace(repairOut(t, "advance")), "[]")
}

func TestRepairDropForgetsALinkAndAnEmptyLedgerSaysSo(t *testing.T) {
	startedRepair(t)
	if _, err := runRepair(t, "drop", "DEN-1/none"); err == nil {
		t.Fatal("dropping a link that is not there")
	}
	testkit.Contains(t, repairOut(t, "drop", "DEN-890/r1"), "Dropped DEN-890/r1")
	testkit.Contains(t, repairOut(t, "status"), "No repairs in progress")
	testkit.Contains(t, repairOut(t, "advance"), "No repairs to advance")
	clitest.JsonOutput(t, true)
	testkit.Equal(t, "empty tree", strings.TrimSpace(repairOut(t, "status")), "[]")
}

func TestRepairCommandsReportAnUnreadableLedgerOrAFailingGh(t *testing.T) {
	link, gh := startedRepair(t)
	gh.Break()
	if _, err := runRepair(t, "pr"); err == nil || !strings.Contains(err.Error(), "repair DEN-890/r1") {
		t.Fatalf("a failing gh: %v", err)
	}
	ledger, _ := repair.Open()
	testkit.WriteFile(t, ledger.Path, "not json", 0o644)
	for _, sub := range []string{"status", "pr", "advance"} {
		if _, err := runRepair(t, sub); err == nil {
			t.Errorf("%s must report an unreadable ledger", sub)
		}
	}
	if _, err := runRepair(t, "drop", link.ID); err == nil {
		t.Error("drop must report an unreadable ledger")
	}
	testkit.Equal(t, "no repairs are listed for the project", len(repairsOf(".")), 0)
	t.Setenv("DENDRITE_HOME", "")
	t.Setenv("HOME", "")
	if _, err := runRepair(t, "status"); err == nil {
		t.Error("no home directory")
	}
}
