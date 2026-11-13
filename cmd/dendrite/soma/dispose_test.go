package soma

import (
	"bytes"
	"context"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"Dendrite/cmd/dendrite/internal/clitest"
	"Dendrite/pkg/projectid"
	"Dendrite/pkg/repair"
	"Dendrite/pkg/synapse"
	"Dendrite/pkg/testkit"
)

// leftovers is the retiring repository (main, a merged develop nothing targets) with a finished feature
// DEN-1-done and an open one DEN-3-open; it is the working directory.
func leftovers(t *testing.T) string {
	t.Helper()
	repo, _ := retiring(t)
	testkit.Git(t, repo, "branch", "-D", "agent/den-1", "DEN-2-new/main")
	testkit.Git(t, repo, "merge", "-q", "--ff-only", "develop")
	testkit.Git(t, repo, "checkout", "-q", "-b", "DEN-1-done/main")
	testkit.Git(t, repo, "commit", "-q", "--allow-empty", "-m", "done work")
	testkit.Git(t, repo, "checkout", "-q", "main")
	testkit.Git(t, repo, "merge", "-q", "--no-ff", "-m", "merge DEN-1-done", "DEN-1-done/main")
	testkit.Git(t, repo, "push", "-q", "origin", "main", "DEN-1-done/main")
	testkit.Git(t, repo, "checkout", "-q", "-b", "DEN-3-open/main")
	testkit.Git(t, repo, "commit", "-q", "--allow-empty", "-m", "open work")
	testkit.Git(t, repo, "checkout", "-q", "main")
	return repo
}

// dispose runs the disposal the way the command does, with answers typed by a person when interactive.
func disposeOut(t *testing.T, d disposal, answers string) string {
	t.Helper()
	var out bytes.Buffer
	testkit.Check(t, runWorkDispose(context.Background(), d, strings.NewReader(answers), &out))
	return out.String()
}

func localBranches(t *testing.T, repo string) string {
	t.Helper()
	return testkit.Git(t, repo, "branch", "-a", "--format=%(refname:short)")
}

func TestWorkDisposeListsThenDeletesOnlyWhenToldTo(t *testing.T) {
	repo := leftovers(t)
	out, err := runCommand(t, NewWorkCmd(), "dispose", "--dry-run")
	testkit.Check(t, err)
	testkit.Contains(t, out, "[+] finished feature (1 disposable, 0 kept):", "- DEN-1-done  (merged into main)", "[+] retired branch (1 disposable, 0 kept):", "- develop  (merged into main)")
	testkit.NotContains(t, out, "DEN-3-open", "Nothing was deleted", "deleted ")

	out, err = runCommand(t, NewWorkCmd(), "dispose")
	testkit.Check(t, err)
	testkit.Contains(t, out, "Nothing was deleted: run `dendrite work dispose` in a terminal")
	testkit.Contains(t, localBranches(t, repo), "DEN-1-done/main", "develop")

	out, err = runCommand(t, NewWorkCmd(), "dispose", "--yes")
	testkit.Check(t, err)
	testkit.Contains(t, out, "[✓] deleted DEN-1-done", "[✓] deleted develop")
	left := localBranches(t, repo)
	testkit.NotContains(t, left, "DEN-1-done", "develop")
	testkit.Contains(t, left, "DEN-3-open/main")

	out, err = runCommand(t, NewWorkCmd(), "dispose")
	testkit.Check(t, err)
	testkit.Contains(t, out, "Nothing to dispose of")
	testkit.Equal(t, "the quiet offer says nothing", disposeOut(t, disposal{Quiet: true}, ""), "")
}

func TestWorkDisposeAsksOncePerGroup(t *testing.T) {
	repo := leftovers(t)
	out := disposeOut(t, disposal{Interactive: true}, "no\nYES\n")
	testkit.Contains(t, out, "[?] Delete these 1 (finished feature)? [y/N] ", "[i] Left as they are.", "[?] Delete these 1 (retired branch)? [y/N] ", "[✓] deleted develop")
	testkit.NotContains(t, out, "Nothing was deleted")
	left := localBranches(t, repo)
	testkit.Contains(t, left, "DEN-1-done/main")
	testkit.NotContains(t, left, "develop")
}

func TestWorkDisposeKeepsWhatIsStillUsedAndReportsWhatGitRefuses(t *testing.T) {
	repo := leftovers(t)
	id, err := projectid.Ensure(repo)
	testkit.Check(t, err)
	st, err := synapse.DefaultStore()
	testkit.Check(t, err)
	eco := &synapse.Synapse{ID: "eco", Name: "eco", State: synapse.StateActive, Channel: synapse.ChannelDev, CreatedAt: time.Now(), UpdatedAt: time.Now(), Members: []synapse.Member{
		{ProjectID: id, Alias: "control", Role: synapse.RoleConsumer, PathHint: repo},
		{ProjectID: "dpid_other", Alias: "other", Role: synapse.RoleProducer, PathHint: t.TempDir()},
	}}
	testkit.Check(t, st.Save(eco))

	out := disposeOut(t, disposal{Yes: true}, "")
	testkit.Contains(t, out, "= develop  kept: synapse eco still tracks it for this project: set the member's branch to main first", "[✓] deleted DEN-1-done")
	testkit.Contains(t, localBranches(t, repo), "develop")
	testkit.Equal(t, "the quiet offer skips a group with nothing to delete", disposeOut(t, disposal{Quiet: true}, ""), "")

	eco.Members[0].Branch = "main"
	testkit.Check(t, st.Save(eco))
	testkit.Git(t, repo, "remote", "set-url", "origin", filepath.Join(repo, "gone.git"))
	testkit.Contains(t, disposeOut(t, disposal{Yes: true}, ""), "[!] develop not deleted:")
}

func TestWorkDisposeAsJSONAndWithAnUnreadableLedger(t *testing.T) {
	leftovers(t)
	clitest.JsonOutput(t, true)
	testkit.Contains(t, disposeOut(t, disposal{}, ""), `"kind": "finished feature"`, `"name": "DEN-1-done"`)
	testkit.Git(t, ".", "branch", "-D", "DEN-1-done/main", "develop")
	testkit.Equal(t, "empty", strings.TrimSpace(disposeOut(t, disposal{}, "")), "[]")
	var quiet bytes.Buffer
	OfferDisposal(context.Background(), &quiet)
	testkit.Equal(t, "no offer inside JSON output", quiet.String(), "")
	clitest.JsonOutput(t, false)

	ledger, err := repair.Open()
	testkit.Check(t, err)
	testkit.WriteFile(t, ledger.Path, "not json", 0o644)
	if _, err := runCommand(t, NewWorkCmd(), "dispose"); err == nil {
		t.Fatal("an unreadable ledger must be reported")
	}
	var out bytes.Buffer
	OfferDisposal(context.Background(), &out)
	testkit.Contains(t, out.String(), "[!] Disposal not offered:")
	t.Setenv("DENDRITE_HOME", "")
	t.Setenv("HOME", "")
	if runWorkDispose(context.Background(), disposal{}, strings.NewReader(""), &out) == nil {
		t.Fatal("no home directory")
	}
	testkit.Equal(t, "nothing tracks a branch without a home", trackedBy("develop"), "")
}
