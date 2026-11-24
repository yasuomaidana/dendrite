package soma

import (
	"Dendrite/cmd/dendrite/internal/clitest"
	"bytes"
	"context"
	"strings"
	"testing"

	"Dendrite/pkg/planesvc"
	"Dendrite/pkg/testkit"

	"github.com/spf13/cobra"
)

// runCommand runs a command group with args and returns what it printed.
func runCommand(t *testing.T, cmd *cobra.Command, args ...string) (string, error) {
	t.Helper()
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	cmd.SetArgs(args)
	err := cmd.ExecuteContext(context.Background())
	return out.String(), err
}

func runBranch(t *testing.T, args ...string) (string, error) {
	t.Helper()
	return runCommand(t, NewBranchCmd(), args...)
}

func TestBranchShowExplainsTheCurrentTarget(t *testing.T) {
	newTargetRig(t)
	out, err := runBranch(t, "show")
	testkit.Check(t, err)
	testkit.Contains(t, out, "Merges into:        develop", "Integration branch: develop", "Target remote:      origin  (pushes to origin)", "Feature prefix:     feature/")
	testkit.NotContains(t, out, "Feature of:")
}

func TestBranchShowForATicketNamesItsFeatureBranch(t *testing.T) {
	newTargetRig(t)
	out, err := runBranch(t, "show", "--ticket", "DEN-841")
	testkit.Check(t, err)
	testkit.Contains(t, out, "Merges into:        DEN-840-work/main  (ticket target recorded for this branch)", "Feature of:         DEN-840  (work)")

	out, err = runBranch(t, "show", "--ticket", "DEN-900")
	testkit.Check(t, err)
	testkit.Contains(t, out, "Merges into:        develop")
	testkit.NotContains(t, out, "Feature of:")

	if _, err := runBranch(t, "show", "--ticket", "DEN-404"); err == nil {
		t.Fatal("an unknown ticket is an error")
	}
}

func TestBranchShowAsJSON(t *testing.T) {
	newTargetRig(t)
	clitest.JsonOutput(t, true)
	out, err := runBranch(t, "show", "--ticket", "DEN-841")
	testkit.Check(t, err)
	testkit.Contains(t, out, `"target"`, `"plan"`, `"Target": "DEN-840-work/main"`)
}

func TestBranchParentLinksTheTicketsInPlane(t *testing.T) {
	r := newTargetRig(t)
	out, err := runBranch(t, "parent", "DEN-900", "DEN-840")
	testkit.Check(t, err)
	testkit.Contains(t, out, "DEN-900 is now a sub-ticket of DEN-840")
	testkit.Equal(t, "parent set", r.board.ParentOf("DEN-900"), r.board.IDOf("DEN-840"))
}

func TestBranchParentRefusesLoopsAndUnknownTickets(t *testing.T) {
	newTargetRig(t)
	for name, args := range map[string][]string{
		"itself":         {"parent", "DEN-840", "DEN-840"},
		"its descendant": {"parent", "DEN-840", "DEN-841"},
		"unknown parent": {"parent", "DEN-900", "DEN-404"},
		"unknown child":  {"parent", "DEN-404", "DEN-840"},
	} {
		if _, err := runBranch(t, args...); err == nil {
			t.Errorf("%s: expected an error", name)
		}
	}
	_, err := runBranch(t, "parent", "DEN-840", "DEN-841")
	if err == nil || !strings.Contains(err.Error(), "loop") {
		t.Errorf("the loop is explained: %v", err)
	}
}

// A sidecar older than protocol 1.5 refuses to clear a parent and leaves it; a newer one clears it.
func TestBranchParentClearRemovesTheParent(t *testing.T) {
	r := newTargetRig(t)
	out, err := runBranch(t, "parent", "DEN-841", "--clear")
	if !r.board.ClearSupported(t, "DEN-841", err, planesvc.Default().CanClearParent(context.Background()) == nil) {
		return
	}
	testkit.Check(t, err)
	testkit.Contains(t, out, "DEN-841 is no longer a sub-ticket of DEN-840")
	testkit.Equal(t, "parent cleared", r.board.ParentOf("DEN-841"), "")
}

func TestBranchParentClearWithoutAParentChangesNothing(t *testing.T) {
	newTargetRig(t)
	out, err := runBranch(t, "parent", "DEN-900", "--clear")
	testkit.Check(t, err)
	testkit.Contains(t, out, "DEN-900 has no parent")
}

func TestBranchParentClearNeedsExactlyOneForm(t *testing.T) {
	newTargetRig(t)
	for name, args := range map[string][]string{
		"clear with a parent": {"parent", "DEN-841", "DEN-840", "--clear"},
		"neither":             {"parent", "DEN-841"},
	} {
		if _, err := runBranch(t, args...); err == nil {
			t.Errorf("%s: expected a usage error", name)
		}
	}
}

func TestBranchParentClearOfAnUnknownTicketIsAnError(t *testing.T) {
	newTargetRig(t)
	if _, err := runBranch(t, "parent", "DEN-404", "--clear"); err == nil {
		t.Fatal("an unknown ticket is an error")
	}
}
