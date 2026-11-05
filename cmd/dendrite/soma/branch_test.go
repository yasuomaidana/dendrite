package soma

import (
	"Dendrite/cmd/dendrite/internal/clitest"
	"bytes"
	"context"
	"strings"
	"testing"

	"Dendrite/pkg/testkit"
)

func runBranch(t *testing.T, args ...string) (string, error) {
	t.Helper()
	cmd := NewBranchCmd()
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	cmd.SetArgs(args)
	err := cmd.ExecuteContext(context.Background())
	return out.String(), err
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
	testkit.Contains(t, out, "Merges into:        feature/DEN-840-work  (ticket target recorded for this branch)", "Feature of:         DEN-840  (work)")

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
	testkit.Contains(t, out, `"target"`, `"plan"`, `"Target": "feature/DEN-840-work"`)
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
