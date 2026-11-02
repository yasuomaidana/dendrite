package plane

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"Dendrite/pkg/tracker/dag"
)

const statesJSON = `[
 {"id":"s-todo","name":"To Do","group":"unstarted"},
 {"id":"s-prog","name":"In Progress","group":"started"},
 {"id":"s-done","name":"Done","group":"completed"}]`

func TestTicketScoreAndPriority(t *testing.T) {
	for _, c := range []struct {
		sp, comp string
		wantSP   int
		wantComp float64
	}{{"", "", 5, 5.0}, {"8", "7.5", 8, 7.5}, {"x", "y", 5, 5.0}} {
		sp, comp := ticketScore(ParsedDevTicket{StoryPoints: c.sp, ComplexityScore: c.comp})
		if sp != c.wantSP || comp != c.wantComp {
			t.Errorf("ticketScore(%q, %q) = %d, %v; want %d, %v", c.sp, c.comp, sp, comp, c.wantSP, c.wantComp)
		}
	}
	for _, c := range []struct {
		comp   float64
		active bool
		want   string
	}{
		{9.5, false, dag.PriorityUrgent}, {7.0, false, dag.PriorityHigh}, {5.0, false, dag.PriorityMedium},
		{5.0, true, dag.PriorityHigh}, {2.0, true, dag.PriorityLow},
	} {
		if got := priorityFor(c.comp, c.active); got != c.want {
			t.Errorf("priorityFor(%v, %v) = %q, want %q", c.comp, c.active, got, c.want)
		}
	}
	for in, want := range map[int]int{-3: 0, 0: 0, 7: 7, 12: 12, 40: 12} {
		if got := clampEstimate(in); got != want {
			t.Errorf("clampEstimate(%d) = %d, want %d", in, got, want)
		}
	}
}

func TestPushStatusReportsEveryFailure(t *testing.T) {
	ctx := context.Background()
	ok := []rule{{"GET", "/states/", 200, statesJSON}, {"GET", "/modules/", 200, `[]`}, {"GET", "/issues/", 200, `[]`}}
	with := func(first rule) []rule { return append([]rule{first}, ok...) }

	for _, c := range []struct {
		name  string
		md    string
		rules []rule
		want  string
	}{
		{"markdown too long to scan", strings.Repeat("x", 70*1024), ok, "failed to parse dev status"},
		{"states", devStatusFixture, with(rule{"GET", "/states/", 500, ""}), "failed to fetch plane states"},
		{"modules", devStatusFixture, with(rule{"GET", "/modules/", 500, ""}), "failed to list plane modules"},
		{"issues", devStatusFixture, with(rule{"GET", "/issues/", 500, ""}), "failed to list plane issues"},
		{"create", devStatusFixture, with(rule{"POST", "/issues/", 500, ""}), "failed to create issue DEN-010"},
		{"update", devStatusFixture, []rule{
			{"PATCH", "/issues/", 500, ""},
			{"GET", "/issues/", 200, `[{"id":"u1","name":"[DEN-010] Base work"}]`},
			{"GET", "/states/", 200, statesJSON}, {"GET", "/modules/", 200, `[]`}}, "failed to update issue DEN-010"},
	} {
		t.Run(c.name, func(t *testing.T) {
			sp := newScriptedPlane(t, c.rules...)
			_, err := PushStatusToPlaneIn(ctx, planeAt(t, sp.URL), c.md, t.TempDir())
			if err == nil || !strings.Contains(err.Error(), c.want) {
				t.Fatalf("err = %v, want it to mention %q", err, c.want)
			}
		})
	}
}

func gitRepoWithMock(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, "pkg", "risky"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "pkg", "risky", "risky.go"), []byte("package risky\n\ntype " + "Mo" + "ck struct{}\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	for _, args := range [][]string{{"init", "-q"}, {"add", "."}} {
		cmd := exec.Command("git", args...)
		cmd.Dir = dir
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, out)
		}
	}
	return dir
}

func TestPushStatusRoutesEveryTicketKind(t *testing.T) {
	md := `- **Active Milestone**: Milestone 1: Base
- **Current Focus**: DEN-020

| Rank | Ticket | Milestone | Feature | Points | Score |
|:--:|:--|:--|:--|:--:|:--:|
| **1** | **DEN-010** | Milestone 1 | FEAT-001 | 20 SP | 9.5 / 10 |

### Milestone 1: Base
- [x] **DEN-010**: Shipped work pkg/risky [State: Done]
- [ ] **DEN-020**: Review work [State: To Verify]
- [ ] **DEN-030**: Parked work [State: Backlog]
- [ ] **DEN-040**: Plain work

### Milestone 2: Next
- [ ] **DEN-050**: Later work
`
	sp := newScriptedPlane(t,
		rule{"GET", "/states/", 200, `[{"id":"s-todo","name":"To Do","group":"unstarted"},{"id":"s-done","name":"Done","group":"completed"}]`},
		rule{"GET", "/modules/", 200, `[{"id":"m1","name":"Milestone 1: Base"}]`},
		rule{"GET", "/issues/", 200, `[{"id":"u1","name":"[DEN-020] Review work"}]`},
		rule{"GET", "/pages/", 200, `[{"id":"pg","name":"Development Status"}]`},
		rule{"POST", "/issues/", 201, `{"id":"new-issue"}`},
		rule{"POST", "/modules/", 201, `{"id":"m2"}`},
	)
	report, err := PushStatusToPlaneIn(context.Background(), planeAt(t, sp.URL), md, gitRepoWithMock(t))
	if err != nil {
		t.Fatal(err)
	}
	if report.TotalTickets != 5 {
		t.Fatalf("report = %+v", report)
	}
	// DEN-010 is Done but its package holds a mock, so the gate sends it back to To Do.
	if report.DoneTickets != 0 || report.ToVerifyTickets != 1 || report.BacklogTickets != 1 || report.InProgressTickets != 0 || report.TodoTickets != 3 {
		t.Errorf("counts = %+v", report)
	}
	if sp.called("PATCH /api/v1/workspaces/ws/projects/proj/modules/m1") != 1 {
		t.Errorf("the existing module gets its dates: %v", sp.Calls)
	}
	if sp.called("POST /api/v1/workspaces/ws/projects/proj/modules/") != 1 {
		t.Errorf("the new module is created: %v", sp.Calls)
	}
	if sp.called("PATCH /api/workspaces/ws/projects/proj/pages/pg") != 1 {
		t.Errorf("the existing status page is updated: %v", sp.Calls)
	}
}

func TestPushStatusCreatesTheStatusPageWhenMissing(t *testing.T) {
	sp := newScriptedPlane(t,
		rule{"GET", "/states/", 200, statesJSON},
		rule{"GET", "/modules/", 200, `[]`},
		rule{"GET", "/issues/", 200, `[]`},
		rule{"GET", "/pages/", 200, `[{"id":"other","name":"Unrelated"}]`},
		rule{"POST", "/issues/", 201, `{"id":"n"}`},
		rule{"POST", "/modules/", 500, ``},
	)
	if _, err := PushStatusToPlaneIn(context.Background(), planeAt(t, sp.URL), "### Milestone 1: A\n- [ ] **DEN-1**: x\n", t.TempDir()); err != nil {
		t.Fatal(err)
	}
	if sp.called("POST /api/workspaces/ws/projects/proj/pages/") != 1 {
		t.Errorf("a missing status page is created: %v", sp.Calls)
	}
}
