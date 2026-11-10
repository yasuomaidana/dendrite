package soma

import (
	"Dendrite/cmd/dendrite/internal/clitest"
	"Dendrite/cmd/dendrite/internal/planecli"
	"bytes"
	"context"
	"errors"
	"path/filepath"
	"strings"
	"testing"

	"Dendrite/pkg/branchpolicy"
	"Dendrite/pkg/planesvc/planetest"
	"Dendrite/pkg/testkit"
	"Dendrite/pkg/tracker/plane"
)

// targetRig is a Plane board where DEN-841 is a sub-ticket of DEN-840 and DEN-900 has no parent, and a repository
// on develop with a bare origin, which is the working directory.
type targetRig struct {
	board *planetest.Board
	repo  string
	out   bytes.Buffer
}

func newTargetRig(t *testing.T) *targetRig {
	t.Helper()
	clitest.JsonOutput(t, false)
	t.Setenv("DENDRITE_HOME", t.TempDir())
	t.Setenv("DENDRITE_STATE_DIR", t.TempDir())
	t.Setenv("DENDRITE_WORK_DIR", t.TempDir())
	board := planetest.NewBoard(t, map[string]string{"DEN-840": "To Do", "DEN-841": "To Do", "DEN-900": "To Do"})
	board.SetParent("DEN-841", "DEN-840")
	t.Setenv("PLANE_BASE_URL", board.URL)
	t.Setenv("PLANE_API_KEY", "k")
	t.Setenv("PLANE_WORKSPACE_SLUG", planetest.BoardWorkspace)
	t.Setenv("PLANE_PROJECT_ID", planetest.BoardProject)
	client, err := plane.NewClient(plane.Config{BaseURL: board.URL, WorkspaceSlug: planetest.BoardWorkspace, ProjectID: planetest.BoardProject, DirectToken: "k"}, board.Client())
	testkit.Check(t, err)
	prev := planecli.NewPlaneClient
	planecli.NewPlaneClient = func() (*plane.Client, error) { return client, nil }
	t.Cleanup(func() { planecli.NewPlaneClient = prev })

	origin := t.TempDir()
	testkit.Git(t, origin, "init", "-q", "--bare", "-b", "develop")
	repo := t.TempDir()
	testkit.GitInit(t, repo, "develop")
	testkit.Git(t, repo, "commit", "-q", "--allow-empty", "-m", "base")
	testkit.Git(t, repo, "remote", "add", "origin", origin)
	testkit.Git(t, repo, "push", "-q", "origin", "develop")
	t.Chdir(repo)
	return &targetRig{board: board, repo: repo}
}

func TestAChildTicketWorksOnItsParentsFeatureBranch(t *testing.T) {
	r := newTargetRig(t)
	wt, err := prepareWorkTarget(context.Background(), "DEN-841", "", "", true, &r.out)
	testkit.Check(t, err)
	testkit.Equal(t, "branch", wt.Branch, "DEN-840-work/main")
	testkit.Equal(t, "base", wt.Base, "DEN-840-work/main")
	testkit.Equal(t, "integration", wt.Integration, "develop")
	testkit.Contains(t, r.out.String(), "DEN-841 is a sub-ticket of DEN-840")
	testkit.Equal(t, "created", testkit.Git(t, r.repo, "rev-parse", "--verify", "refs/heads/DEN-840-work/main") != "", true)
}

func TestATicketWithoutAParentKeepsTheDefaults(t *testing.T) {
	r := newTargetRig(t)
	wt, err := prepareWorkTarget(context.Background(), "DEN-900", "", "", true, &r.out)
	testkit.Check(t, err)
	testkit.Equal(t, "no branch", wt.Branch, "")
	testkit.Equal(t, "no base", wt.Base, "")
	testkit.Equal(t, "integration known", wt.Integration, "develop")
}

func TestATargetFlagOrAnExplicitBaseWinsOverPlane(t *testing.T) {
	r := newTargetRig(t)
	wt, err := prepareWorkTarget(context.Background(), "DEN-841", "", "release/2", true, &r.out)
	testkit.Check(t, err)
	testkit.Equal(t, "target", wt.Branch, "release/2")
	testkit.Equal(t, "base follows target", wt.Base, "release/2")

	wt, err = prepareWorkTarget(context.Background(), "DEN-841", "abc123", "release/2", true, &r.out)
	testkit.Check(t, err)
	testkit.Equal(t, "base kept", wt.Base, "abc123")

	wt, err = prepareWorkTarget(context.Background(), "DEN-841", "abc123", "", true, &r.out)
	testkit.Check(t, err)
	testkit.Equal(t, "explicit base kept with a feature target", wt.Base, "abc123")
	testkit.Equal(t, "feature target still recorded", wt.Branch, "DEN-840-work/main")
}

func TestNoPlaneLookupWhenTheGuardIsOff(t *testing.T) {
	r := newTargetRig(t)
	wt, err := prepareWorkTarget(context.Background(), "DEN-841", "", "", false, &r.out)
	testkit.Check(t, err)
	testkit.Equal(t, "no branch", wt.Branch, "")
	testkit.Equal(t, "silent", r.out.String(), "")
}

func TestAnUnreachablePlaneOnlyCostsTheFeatureBranch(t *testing.T) {
	r := newTargetRig(t)
	prev := planecli.NewPlaneClient
	t.Cleanup(func() { planecli.NewPlaneClient = prev })
	planecli.NewPlaneClient = func() (*plane.Client, error) { return nil, errors.New("PLANE_API_KEY is not set\nsecond line") }
	wt, err := prepareWorkTarget(context.Background(), "DEN-841", "", "", true, &r.out)
	testkit.Check(t, err)
	testkit.Equal(t, "default", wt.Branch, "")
	testkit.Contains(t, r.out.String(), "Plane is not configured (PLANE_API_KEY is not set)")
	testkit.NotContains(t, r.out.String(), "second line")

	planecli.NewPlaneClient = prev
	r.board.Close()
	r.out.Reset()
	wt, err = prepareWorkTarget(context.Background(), "DEN-841", "", "", true, &r.out)
	testkit.Check(t, err)
	testkit.Equal(t, "default", wt.Branch, "")
	testkit.Contains(t, r.out.String(), "feature branch not looked up")
}

func TestAFeatureBranchThatCannotBeCreatedStopsTheCopy(t *testing.T) {
	r := newTargetRig(t)
	empty := t.TempDir()
	testkit.GitInit(t, empty, "develop")
	t.Chdir(empty)
	if _, err := prepareWorkTarget(context.Background(), "DEN-841", "", "", true, &r.out); err == nil || !strings.Contains(err.Error(), "DEN-840-work/main") {
		t.Fatalf("err = %v, want the failed feature branch named", err)
	}
}

func TestAFeatureBranchThatCannotBePushedIsOnlyAWarning(t *testing.T) {
	r := newTargetRig(t)
	testkit.Git(t, r.repo, "remote", "set-url", "origin", filepath.Join(t.TempDir(), "gone.git"))
	wt, err := prepareWorkTarget(context.Background(), "DEN-841", "", "", true, &r.out)
	testkit.Check(t, err)
	testkit.Equal(t, "branch", wt.Branch, "DEN-840-work/main")
	testkit.Contains(t, r.out.String(), "exists locally but was not pushed")
}

func TestRecordWorkTargetRemembersTheTarget(t *testing.T) {
	r := newTargetRig(t)
	recordWorkTarget("DEN-840-work/DEN-841-work", workTarget{Branch: "DEN-840-work/main", Integration: "develop"}, &r.out)
	testkit.Equal(t, "recorded", branchpolicy.ForBranch(r.repo, "DEN-840-work/DEN-841-work").Branch, "DEN-840-work/main")

	recordWorkTarget("custom/five", workTarget{Branch: "release/2", Integration: "develop"}, &r.out)
	testkit.Equal(t, "custom branch", branchpolicy.ForBranch(r.repo, "custom/five").Branch, "release/2")

	recordWorkTarget("DEN-6/main", workTarget{}, &r.out)
	testkit.Equal(t, "nothing to record", r.out.String(), "")

	t.Chdir(t.TempDir())
	recordWorkTarget("DEN-7/main", workTarget{Branch: "x", Integration: "develop"}, &r.out)
	testkit.Contains(t, r.out.String(), "was not recorded")
}

func TestWorkNewStartsAChildOnItsParentsFeatureBranchAndRecordsTheTarget(t *testing.T) {
	r := newTargetRig(t)
	t.Setenv("DENDRITE_AGENT", "claude")
	testkit.Check(t, runWorkNew(context.Background(), "DEN-841", "", "", "", false, &r.out))
	got := branchpolicy.ForBranch(r.repo, "DEN-840-work/DEN-841-work")
	testkit.Equal(t, "target", got.Branch, "DEN-840-work/main")
	testkit.Equal(t, "source", got.Source, branchpolicy.SourceTicket)
	testkit.Equal(t, "base commit is the feature branch's", testkit.Git(t, r.repo, "merge-base", "DEN-840-work/DEN-841-work", "DEN-840-work/main"), testkit.Git(t, r.repo, "rev-parse", "DEN-840-work/main"))
	testkit.Contains(t, r.out.String(), "Working copy created")
}

func TestWorkNewNamesALoneTicketsTrunkAndCopyAfterItsTitle(t *testing.T) {
	r := newTargetRig(t)
	t.Setenv("DENDRITE_AGENT", "claude")
	wt, err := prepareWorkTarget(context.Background(), "DEN-900", "", "", true, &r.out)
	testkit.Check(t, err)
	testkit.Equal(t, "nothing to record", wt.Branch, "")
	testkit.Check(t, runWorkNew(context.Background(), "DEN-900", "", "", "", false, &r.out))
	testkit.Equal(t, "branch", testkit.Git(t, r.repo, "rev-parse", "--verify", "refs/heads/"+wt.Work) != "", true)
	if !strings.HasPrefix(wt.Work, "DEN-900") || !strings.HasSuffix(wt.Work, "/main") {
		t.Fatalf("work branch = %q, want DEN-900[-slug]/main", wt.Work)
	}
	testkit.Contains(t, r.out.String(), "-"+wt.Copy)
	testkit.Equal(t, "merges into the integration branch", branchpolicy.ForBranch(r.repo, wt.Work).Branch, "develop")
}

func TestWorkNewWithoutPlaneUsesTheDefaultBranchOfTheName(t *testing.T) {
	r := newTargetRig(t)
	testkit.Check(t, runWorkNew(context.Background(), "spike", "", "", "", true, &r.out))
	testkit.Equal(t, "branch", testkit.Git(t, r.repo, "rev-parse", "--verify", "refs/heads/spike/main") != "", true)
	testkit.Check(t, runWorkNew(context.Background(), "named", "custom/branch", "", "", true, &r.out))
	testkit.Equal(t, "--branch wins", testkit.Git(t, r.repo, "rev-parse", "--verify", "refs/heads/custom/branch") != "", true)
}
