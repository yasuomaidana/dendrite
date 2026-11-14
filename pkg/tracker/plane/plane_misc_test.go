package plane

import (
	"context"
	"errors"
	"strings"
	"testing"

	"Dendrite/pkg/tracker"
)

func TestMilestoneForPrefersTheMostSpecificSource(t *testing.T) {
	names := map[string]string{"m1": "Milestone 5: From module"}
	of := map[string]string{"listed": "Milestone 6: Listed"}
	cases := []struct {
		name string
		iss  PlaneIssue
		id   string
		want string
	}{
		{"its first module", PlaneIssue{ID: "a", ModuleIDs: []string{"m1"}}, "DEN-9", "Milestone 5: From module"},
		{"a module that lists it", PlaneIssue{ID: "listed", ModuleIDs: []string{"unknown"}}, "DEN-9", "Milestone 6: Listed"},
		{"its description", PlaneIssue{ID: "b", DescriptionStripped: "Milestone: Milestone 7: Written"}, "DEN-9", "Milestone 7: Written"},
		{"the built-in default", PlaneIssue{ID: "c"}, "DEN-030", defaultTicketMilestones["DEN-030"]},
		{"nothing", PlaneIssue{ID: "d"}, "DEN-999", ""},
	}
	for _, c := range cases {
		if got := milestoneFor(c.iss, c.id, names, of); got != c.want {
			t.Errorf("%s: milestoneFor = %q, want %q", c.name, got, c.want)
		}
	}
}

func TestPullStatusReportsStatesThatCannotBeRead(t *testing.T) {
	sp := newScriptedPlane(t, on("GET", "/states/", 500, ""), on("GET", "/issues/", 200, `[]`))
	if _, _, err := PullStatusFromPlane(context.Background(), planeAt(t, sp.URL)); err == nil || !strings.Contains(err.Error(), "failed to query states") {
		t.Fatalf("err = %v", err)
	}
}

func TestPullStatusIgnoresOtherProjects(t *testing.T) {
	sp := newScriptedPlane(t,
		on("GET", "/projects/x/states/", 200, `[{"id":"xs","name":"Blocked","group":"started"}]`),
		on("GET", "/projects/x/issues/", 200, `[{"id":"a1","name":"[X-1] shared","state":"xs","sequence_id":1},{"id":"x2","name":"Plain title","state":"xs","sequence_id":2}]`),
		on("GET", "/projects/proj/states/", 200, `[{"id":"s1","name":"Done","group":"completed"}]`),
		on("GET", "/projects/proj/issues/", 200, `[{"id":"a1","name":"[DEN-001] first","state":"s1","sequence_id":1},{"id":"copy","name":"[X-7] copy of a peer ticket","state":"s1","sequence_id":9},{"id":"nostate","name":"No state","sequence_id":3,"state_detail":{"name":"To Verify","group":"started"}},{"id":"","name":"No id","sequence_id":4}]`),
		on("GET", "/projects/proj/modules/", 200, `[{"id":"m","name":"Milestone 1: Base"}]`),
		on("GET", "/module-issues/", 200, `[{"id":"a1"}]`),
		on("GET", "/workspaces/ws/projects/", 200, `[{"id":"proj","identifier":"DEN"},{"id":"x","identifier":"X"},{"id":"blank","identifier":""}]`),
	)
	md, report, err := PullStatusFromPlane(context.Background(), planeAt(t, sp.URL))
	if err != nil {
		t.Fatal(err)
	}
	if report.TotalTickets != 3 || report.DoneTickets != 1 || report.ToVerifyTickets != 1 || report.BacklogTickets != 1 {
		t.Errorf("report = %+v", report)
	}
	if !strings.Contains(md, "DEN-001") {
		t.Errorf("missing DEN-001 in:\n%s", md)
	}
	for _, foreign := range []string{"X-002", "Plain title", "X-001", "X-7", "copy of a peer ticket"} {
		if strings.Contains(md, foreign) {
			t.Errorf("a ticket of another project (%q) leaked into:\n%s", foreign, md)
		}
	}
	if strings.Count(md, "first") != 1 {
		t.Errorf("an issue must appear once:\n%s", md)
	}
}

func TestPullStatusNeverGuessesTheProject(t *testing.T) {
	for name, projects := range map[string]rule{
		"the workspace does not list it":  on("GET", "/workspaces/ws/projects/", 200, `[{"id":"other","identifier":"DCORE"}]`),
		"the project has no identifier":   on("GET", "/workspaces/ws/projects/", 200, `[{"id":"proj","identifier":" "}]`),
		"the project list cannot be read": on("GET", "/workspaces/ws/projects/", 500, ""),
	} {
		// The project list rule goes last: its path is a prefix of every other path.
		sp := newScriptedPlane(t, on("GET", "/states/", 200, `[]`), on("GET", "/issues/", 200, `[]`), projects)
		if _, _, err := PullStatusFromPlane(context.Background(), planeAt(t, sp.URL)); err == nil {
			t.Errorf("%s: want an error, not a DEN file written from a peer's board", name)
		}
	}
}

func TestPullStatusOfAPeerHasItsOwnHeaderAndNoDendrite(t *testing.T) {
	var board []rule
	for _, r := range [][2]string{
		{"/projects/proj/states/", `[{"id":"s","name":"Done","group":"completed"}]`},
		{"/projects/proj/issues/", `[{"id":"c1","name":"sidecar","state":"s","sequence_id":1}]`},
		{"/projects/den/issues/", `[{"id":"d1","name":"[DEN-001] not mine","state":"s","sequence_id":1}]`},
		{"/workspaces/ws/projects/", `[{"id":"proj","identifier":"DCORE"},{"id":"den","identifier":"DEN"}]`},
	} {
		board = append(board, on("GET", r[0], 200, r[1]))
	}
	md, report, err := PullStatusFromPlane(context.Background(), planeAt(t, newScriptedPlane(t, board...).URL))
	if err != nil {
		t.Fatal(err)
	}
	if report.ProjectIdentifier != "DCORE" || report.TotalTickets != 1 || report.ActiveMilestone != "" {
		t.Errorf("report = %+v", report)
	}
	expectIn(t, md, "# DCORE Project Development Status", "DCORE-001")
	expectNotIn(t, md, "not mine", "Complexity Analysis")
}

func TestConfigFromEnvFallsBackToDendriteCore(t *testing.T) {
	t.Setenv(APIKeyEnv, "token")
	t.Setenv("PLANE_PROJECT_ID", "")
	t.Setenv("DENDRITE_HOME", t.TempDir())
	t.Chdir(t.TempDir())
	cfg, err := ConfigFromEnv()
	if err != nil || cfg.ProjectID != "e6380603-1428-4a66-995e-e06039915d52" {
		t.Fatalf("cfg = %+v, err = %v", cfg, err)
	}
}

func TestParseIssuePageIgnoresAnswersItCannotRead(t *testing.T) {
	if issues, next, more := parseIssuePage([]byte("not json")); issues != nil || next != "" || more {
		t.Fatalf("got %v %q %v", issues, next, more)
	}
}

func TestStatusUpdatesNeedAKnownTicket(t *testing.T) {
	ctx := context.Background()
	sp := newScriptedPlane(t,
		on("GET", "/states/", 200, `[{"id":"s","name":"In Progress","group":"started"},{"id":"d","name":"Done","group":"completed"}]`),
		on("GET", "/issues/", 200, `[]`),
	)
	c := planeAt(t, sp.URL)
	if err := c.CheckZeroMockGate(ctx, "DEN-1", tracker.StatusTodo); err != nil {
		t.Errorf("only done and review are gated, got %v", err)
	}
	if err := c.UpdateStatus(ctx, "DEN-404", tracker.StatusInProgress); !errors.Is(err, ErrIssueNotFound) {
		t.Errorf("UpdateStatus on an unknown ticket = %v, want ErrIssueNotFound", err)
	}
}

func TestAddCommentFindsTheIssueByTicketID(t *testing.T) {
	sp := newScriptedPlane(t,
		on("GET", "/issues/", 200, `[{"id":"the-uuid","name":"[DEN-5] work"}]`),
		on("POST", "/comments/", 201, `{}`),
	)
	if err := planeAt(t, sp.URL).AddComment(context.Background(), "DEN-5", "hello"); err != nil {
		t.Fatal(err)
	}
	if sp.Called("POST /api/v1/workspaces/ws/projects/proj/issues/the-uuid/comments/") != 1 {
		t.Errorf("the comment must go to the resolved issue: %v", sp.Calls)
	}
}

func TestIssueReferenceHelpers(t *testing.T) {
	issues := []PlaneIssue{{ID: "i1", Name: "untitled", SequenceID: 7}}
	if FindUnprefixedIssueBySequence(issues, "not a ref") != nil {
		t.Error("a reference without a number matches nothing")
	}
	if FindUnprefixedIssueBySequence(issues, "DEN-99999999999999999999") != nil {
		t.Error("a number too large to read matches nothing")
	}
	if ResolveIssue(issues, "41") != nil {
		t.Error("an unknown sequence number matches nothing")
	}
	if got := ResolveIssue(issues, "DEN-7"); got == nil || got.ID != "i1" {
		t.Errorf("ResolveIssue(DEN-7) = %v", got)
	}
}

func TestGetRelationsForUsesFewerWorkersThanIssues(t *testing.T) {
	sp := newScriptedPlane(t, on("GET", "/issue-relation/", 200, `{}`))
	rels, err := planeAt(t, sp.URL).GetRelationsFor(context.Background(), []string{"a", "b"})
	if err != nil || len(rels) != 2 {
		t.Fatalf("rels = %v, err = %v", rels, err)
	}
}

func TestChecklistParsingAndImportEdgeCases(t *testing.T) {
	if items := ParseChecklist("- [ ] **K-1**:\n- [ ] **K-2**: real"); len(items) != 1 || items[0].Key != "K-2" {
		t.Errorf("an item without a title is skipped, got %+v", items)
	}

	ctx := context.Background()
	items := []ChecklistItem{{Key: "K-1", Title: "one", Status: tracker.StatusTodo}}
	t.Run("states cannot be read", func(t *testing.T) {
		sp := newScriptedPlane(t, on("GET", "/states/", 500, ""))
		if _, err := ImportChecklist(ctx, planeAt(t, sp.URL), items, "STATUS.md"); err == nil {
			t.Fatal("expected the states error")
		}
	})
	t.Run("issues cannot be read", func(t *testing.T) {
		sp := newScriptedPlane(t, on("GET", "/states/", 200, `[]`), on("GET", "/issues/", 500, ""))
		if _, err := ImportChecklist(ctx, planeAt(t, sp.URL), items, "STATUS.md"); err == nil {
			t.Fatal("expected the issues error")
		}
	})
	t.Run("an issue cannot be created", func(t *testing.T) {
		sp := newScriptedPlane(t, on("GET", "/states/", 200, `[]`), on("GET", "/issues/", 200, `[]`), on("GET", "/modules/", 200, `[]`), on("POST", "/issues/", 500, ""))
		report, err := ImportChecklist(ctx, planeAt(t, sp.URL), items, "STATUS.md")
		if err == nil || !strings.Contains(err.Error(), "import K-1") || report == nil {
			t.Fatalf("report = %v, err = %v", report, err)
		}
	})
}
