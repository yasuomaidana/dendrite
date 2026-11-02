package plane

import (
	"Dendrite/pkg/testkit"
	"context"
	"os"
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
		sp, comp := TicketScore(ParsedDevTicket{StoryPoints: c.sp, ComplexityScore: c.comp})
		if sp != c.wantSP || comp != c.wantComp {
			t.Errorf("TicketScore(%q, %q) = %d, %v; want %d, %v", c.sp, c.comp, sp, comp, c.wantSP, c.wantComp)
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
		if got := PriorityFor(c.comp, c.active); got != c.want {
			t.Errorf("PriorityFor(%v, %v) = %q, want %q", c.comp, c.active, got, c.want)
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
	ok := []rule{on("GET", "/states/", 200, statesJSON), on("GET", "/modules/", 200, `[]`), on("GET", "/issues/", 200, `[]`)}
	with := func(first rule) []rule { return append([]rule{first}, ok...) }

	for _, c := range []struct {
		name  string
		md    string
		rules []rule
		want  string
	}{
		{"markdown too long to scan", strings.Repeat("x", 70*1024), ok, "failed to parse dev status"},
		{"states", devStatusFixture, with(on("GET", "/states/", 500, "")), "failed to fetch plane states"},
		{"modules", devStatusFixture, with(on("GET", "/modules/", 500, "")), "failed to list plane modules"},
		{"issues", devStatusFixture, with(on("GET", "/issues/", 500, "")), "failed to list plane issues"},
		{"create", devStatusFixture, with(on("POST", "/issues/", 500, "")), "failed to create issue DEN-010"},
		{"update", devStatusFixture, []rule{
			on("PATCH", "/issues/", 500, ""),
			on("GET", "/issues/", 200, `[{"id":"u1","name":"[DEN-010] Base work"}]`),
			on("GET", "/states/", 200, statesJSON), on("GET", "/modules/", 200, `[]`)}, "failed to update issue DEN-010"},
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
	if err := os.WriteFile(filepath.Join(dir, "pkg", "risky", "risky.go"), []byte("package risky\n\ntype "+"Mo"+"ck struct{}\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	testkit.Git(t, dir, "init", "-q")
	testkit.Git(t, dir, "add", ".")
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
		on("GET", "/states/", 200, `[{"id":"s-todo","name":"To Do","group":"unstarted"},{"id":"s-done","name":"Done","group":"completed"}]`),
		on("GET", "/modules/", 200, `[{"id":"m1","name":"Milestone 1: Base"}]`),
		on("GET", "/issues/", 200, `[{"id":"u1","name":"[DEN-020] Review work"}]`),
		on("GET", "/pages/", 200, `[{"id":"pg","name":"Development Status"}]`),
		on("POST", "/issues/", 201, `{"id":"new-issue"}`),
		on("POST", "/modules/", 201, `{"id":"m2"}`),
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
	if sp.Called("PATCH /api/v1/workspaces/ws/projects/proj/modules/m1") != 1 {
		t.Errorf("the existing module gets its dates: %v", sp.Calls)
	}
	if sp.Called("POST /api/v1/workspaces/ws/projects/proj/modules/") != 1 {
		t.Errorf("the new module is created: %v", sp.Calls)
	}
	if sp.Called("PATCH /api/workspaces/ws/projects/proj/pages/pg") != 1 {
		t.Errorf("the existing status page is updated: %v", sp.Calls)
	}
}

func TestPushStatusCreatesTheStatusPageWhenMissing(t *testing.T) {
	sp := newScriptedPlane(t,
		on("GET", "/states/", 200, statesJSON),
		on("GET", "/modules/", 200, `[]`),
		on("GET", "/issues/", 200, `[]`),
		on("GET", "/pages/", 200, `[{"id":"other","name":"Unrelated"}]`),
		on("POST", "/issues/", 201, `{"id":"n"}`),
		on("POST", "/modules/", 500, ``),
	)
	if _, err := PushStatusToPlaneIn(context.Background(), planeAt(t, sp.URL), "### Milestone 1: A\n- [ ] **DEN-1**: x\n", t.TempDir()); err != nil {
		t.Fatal(err)
	}
	if sp.Called("POST /api/workspaces/ws/projects/proj/pages/") != 1 {
		t.Errorf("a missing status page is created: %v", sp.Calls)
	}
}
