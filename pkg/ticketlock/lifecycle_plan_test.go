package ticketlock

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

const samplePlan = "# Plan\n- [x] read the code\n- [ ] write <b>the</b> lock & test it\n- [ ] merge\n\n```go\nfmt.Println(\"a < b\")\n```"

func TestPlanRoundTripsThroughPlaneAndTheLocalStore(t *testing.T) {
	s := newSession(t, map[string]string{"DEN-377": "In Progress"})
	ctx := context.Background()
	s.board.AddComment("DEN-377", "<p>a person says hello</p>")

	p1, err := s.UploadPlan(ctx, "DEN-377", samplePlan)
	if err != nil {
		t.Fatal(err)
	}
	if p1.Version != 1 || p1.PendingSteps != 2 || p1.NextStep != "write <b>the</b> lock & test it" || p1.Source != "plane" {
		t.Errorf("first upload = %+v", p1)
	}
	p2, err := s.UploadPlan(ctx, "DEN-377", strings.Replace(samplePlan, "- [ ] write", "- [x] write", 1))
	if err != nil {
		t.Fatal(err)
	}
	if p2.Version != 2 || p2.PendingSteps != 1 || p2.NextStep != "merge" {
		t.Errorf("second upload = %+v", p2)
	}
	if n := len(s.board.Comments("DEN-377")); n != 2 {
		t.Errorf("the plan is one comment edited in place; the board has %d comments (a person's and the plan)", n)
	}

	got, err := s.GetPlan(ctx, "DEN-377")
	if err != nil || got.Version != 2 || got.Source != "plane" || !strings.Contains(got.Markdown, "- [x] write <b>the</b> lock & test it") || !strings.HasSuffix(got.Markdown, "```") {
		t.Fatalf("plan from Plane = %+v, err = %v", got, err)
	}
	local, err := s.Plans.Load("DEN-377")
	if err != nil || local.Version != 2 || local.Markdown != got.Markdown {
		t.Errorf("local copy = %+v, err = %v", local, err)
	}

	// Plane goes away: the plan is still there.
	s.board.Close()
	down, err := s.GetPlan(ctx, "DEN-377")
	if err != nil || down.Source != "engram" || down.Markdown != got.Markdown || down.Version != 2 {
		t.Errorf("plan with Plane down = %+v, err = %v", down, err)
	}
}

func TestPlanFromAPlaneNumberIsKeyedByItsTicket(t *testing.T) {
	s := newSession(t, map[string]string{"DEN-377": "In Progress"})
	if _, err := s.UploadPlan(context.Background(), "377", "- [ ] one"); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Plans.Load("DEN-377"); err != nil {
		t.Errorf("a Plane number must resolve to IDENT-N for the local key: %v", err)
	}
	s.board.Close()
	if got := s.keyOf(context.Background(), "377"); got != "377" {
		t.Errorf("with Plane down an unresolvable reference stays as given: %q", got)
	}
}

func TestPlanErrors(t *testing.T) {
	ctx := context.Background()
	s := newSession(t, map[string]string{"DEN-377": "In Progress"})
	if _, err := s.GetPlan(ctx, "DEN-377"); !errors.Is(err, ErrNoPlan) {
		t.Errorf("no plan anywhere: %v", err)
	}
	if _, err := s.UploadPlan(ctx, "DEN-377", "  "); err == nil {
		t.Error("an empty plan is refused")
	}

	s.board.Fail("comments/", 0)
	if _, err := s.UploadPlan(ctx, "DEN-377", "- [ ] x"); err == nil || !strings.Contains(err.Error(), "read the plan comments") {
		t.Errorf("Plane failing on the read: %v", err)
	}

	s2 := newSession(t, map[string]string{"DEN-377": "In Progress"})
	s2.board.Fail("POST", 0)
	if _, err := s2.UploadPlan(ctx, "DEN-377", "- [ ] x"); err == nil || !strings.Contains(err.Error(), "saved locally") {
		t.Errorf("Plane failing on the write: %v", err)
	}
	if p, err := s2.Plans.Load("DEN-377"); err != nil || p.Version != 1 {
		t.Errorf("the plan must be kept locally when Plane fails: %+v, %v", p, err)
	}

	s3 := newSession(t, map[string]string{"DEN-377": "In Progress"})
	s3.Plans = PlanStore{Dir: filepath.Join(writeRegularFile(t), "plans")}
	if _, err := s3.UploadPlan(ctx, "DEN-377", "- [ ] x"); err == nil || !strings.Contains(err.Error(), "store plan") {
		t.Errorf("local store failing: %v", err)
	}

	s4 := newSession(t, map[string]string{"DEN-377": "In Progress"})
	if _, err := s4.UploadPlan(ctx, "DEN-377", "- [ ] x"); err != nil {
		t.Fatal(err)
	}
	s4.board.Fail("PATCH", 0)
	if _, err := s4.UploadPlan(ctx, "DEN-377", "- [ ] y"); err == nil || !strings.Contains(err.Error(), "saved locally") {
		t.Errorf("Plane failing on the edit: %v", err)
	}
}

func TestPlanHTMLAndStore(t *testing.T) {
	p := newPlan(samplePlan, 4)
	back, ok := parsePlanHTML(planHTML(p))
	if !ok || back.Version != 4 || back.Markdown != samplePlan || back.PendingSteps != 2 {
		t.Errorf("round trip = %+v, ok = %v", back, ok)
	}
	for _, other := range []string{"<p>[dendrite:claim] hi</p>", "<p>[dendrite:plan v=1]</p>", "<pre>x</pre>", ""} {
		if _, ok := parsePlanHTML(other); ok {
			t.Errorf("%q is not a plan comment", other)
		}
	}
	if p := newPlan("no steps here", 1); p.PendingSteps != 0 || p.NextStep != "" {
		t.Errorf("plan without steps = %+v", p)
	}

	store := PlanStore{Dir: t.TempDir()}
	if _, err := store.Load("DEN-1"); !errors.Is(err, ErrNoPlan) {
		t.Errorf("empty store: %v", err)
	}
	if err := store.Save("den-1", p); err != nil {
		t.Fatal(err)
	}
	if got, err := store.Load("DEN-1"); err != nil || got.Markdown != samplePlan || got.Version != 4 || got.Source != "engram" {
		t.Errorf("case does not matter: %+v, %v", got, err)
	}

	_ = os.WriteFile(store.indexPath("DEN-2"), []byte("{"), 0o644)
	if _, err := store.Load("DEN-2"); err == nil || errors.Is(err, ErrNoPlan) {
		t.Errorf("a corrupt index is an error: %v", err)
	}
	_ = os.WriteFile(store.indexPath("DEN-3"), []byte(`{"version":1,"hash":"deadbeef"}`), 0o644)
	if _, err := store.Load("DEN-3"); err == nil || !strings.Contains(err.Error(), "read plan") {
		t.Errorf("a missing blob is an error: %v", err)
	}
	if err := (PlanStore{Dir: filepath.Join(writeRegularFile(t), "p")}).Save("DEN-4", p); err == nil {
		t.Error("blobs that cannot be written must fail")
	}
	blockedIndex := PlanStore{Dir: t.TempDir()}
	_ = os.WriteFile(filepath.Join(blockedIndex.Dir, "index"), nil, 0o644)
	if err := blockedIndex.Save("DEN-5", p); err == nil || !strings.Contains(err.Error(), "index plan") {
		t.Errorf("an index that cannot be written must fail: %v", err)
	}
}

func TestTransitionBlockedNeedsAllThreeHandoffSections(t *testing.T) {
	s := newSession(t, map[string]string{"DEN-377": "To Do"})
	ctx := context.Background()
	if _, err := s.Claim(ctx, "DEN-377", ClaimOptions{}); err != nil {
		t.Fatal(err)
	}
	for name, h := range map[string]Handoff{
		"nothing":        {},
		"no challenges":  {Completed: "a", Missing: "c"},
		"blank sections": {Completed: " ", Challenges: "\n", Missing: "\t"},
	} {
		_, err := s.Transition(ctx, "DEN-377", "blocked", h)
		if !errors.Is(err, ErrHandoffIncomplete) {
			t.Errorf("%s: err = %v", name, err)
		}
	}
	if _, err := s.Transition(ctx, "DEN-377", "blocked", Handoff{Completed: "a", Missing: "c"}); err == nil || !strings.Contains(err.Error(), "write challenges") || strings.Contains(err.Error(), "completed") {
		t.Errorf("the error must name only the missing sections: %v", err)
	}
	if s.board.State("DEN-377") != "In Progress" {
		t.Errorf("a refused handoff must not move the ticket: %s", s.board.State("DEN-377"))
	}

	res, err := s.Transition(ctx, "den-377", "blocked", Handoff{Completed: "lock done", Challenges: "Plane strips <comments>", Missing: "hooks (AC-1)"})
	if err != nil || res.State != "Blocked" || res.Ticket != "DEN-377" {
		t.Fatalf("res = %+v, err = %v", res, err)
	}
	all := texts(s.board, "DEN-377")
	last := all[len(all)-1]
	for _, want := range []string{"[dendrite:handoff] Completed: lock done", "Challenges: Plane strips <comments>", "Missing: hooks (AC-1)"} {
		if !strings.Contains(last, want) {
			t.Errorf("handoff comment %q lacks %q", last, want)
		}
	}
	if _, err := s.Store.ClaimFor("agent/den-377"); err != nil {
		t.Errorf("a blocked ticket keeps its claim for whoever resumes: %v", err)
	}
}

func TestTransitionOutcomes(t *testing.T) {
	for outcome, want := range map[string]string{"verify": "To Verify", "cancelled": "Cancelled", "backlog": "Backlog"} {
		t.Run(outcome, func(t *testing.T) {
			s := newSession(t, map[string]string{"DEN-377": "To Do"})
			ctx := context.Background()
			if _, err := s.Claim(ctx, "DEN-377", ClaimOptions{}); err != nil {
				t.Fatal(err)
			}
			res, err := s.Transition(ctx, "DEN-377", outcome, Handoff{})
			if err != nil || res.State != want || s.board.State("DEN-377") != want {
				t.Errorf("res = %+v, state = %s, err = %v", res, s.board.State("DEN-377"), err)
			}
			if _, err := s.Store.ClaimFor("agent/den-377"); err != ErrNoClaim {
				t.Errorf("the claim must be released: %v", err)
			}
		})
	}
}

func TestTransitionKeepsAnotherTicketsClaim(t *testing.T) {
	s := newSession(t, map[string]string{"DEN-377": "In Progress", "DEN-378": "In Progress"})
	_ = s.Store.WriteClaim(Claim{Ticket: "DEN-378", Branch: "agent/den-377", ClaimedAt: t0, CheckedAt: t0})
	if _, err := s.Transition(context.Background(), "DEN-377", "backlog", Handoff{}); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Store.ClaimFor("agent/den-377"); err != nil {
		t.Errorf("a claim for a different ticket must stay: %v", err)
	}
}

func TestTransitionRefusals(t *testing.T) {
	s := newSession(t, map[string]string{"DEN-377": "In Progress"})
	ctx := context.Background()
	if _, err := s.Transition(ctx, "DEN-377", "done", Handoff{}); !errors.Is(err, ErrUseFinish) {
		t.Errorf("done: %v", err)
	}
	if _, err := s.Transition(ctx, "DEN-377", "finished", Handoff{}); err == nil || !strings.Contains(err.Error(), "unknown outcome") {
		t.Errorf("unknown outcome: %v", err)
	}
	if _, err := s.Transition(ctx, "DEN-999", "verify", Handoff{}); err == nil {
		t.Error("an unknown ticket is an error")
	}
	s.board.Fail("PATCH", 0)
	if _, err := s.Transition(ctx, "DEN-377", "verify", Handoff{}); err == nil || !strings.Contains(err.Error(), "set DEN-377 to To Verify") {
		t.Errorf("Plane failing: %v", err)
	}
	s2 := newSession(t, map[string]string{"DEN-377": "In Progress"})
	s2.board.Fail("comments/", 0)
	if _, err := s2.Transition(ctx, "DEN-377", "blocked", Handoff{Completed: "a", Challenges: "b", Missing: "c"}); err == nil {
		t.Error("a handoff that cannot be posted must stop the transition")
	}
	if s2.board.State("DEN-377") != "In Progress" {
		t.Error("the ticket moved although the handoff was not posted")
	}
}

func TestTransitionReportsAClaimItCannotRelease(t *testing.T) {
	s := newSession(t, map[string]string{"DEN-377": "In Progress"})
	_ = s.Store.WriteClaim(Claim{Ticket: "DEN-377", Branch: "agent/den-377", ClaimedAt: t0, CheckedAt: t0})
	dir := filepath.Join(s.Store.Dir, "claims")
	if err := os.Chmod(dir, 0o500); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(dir, 0o755) })
	canWrite := os.WriteFile(filepath.Join(dir, "probe"), nil, 0o644) == nil // root ignores permissions
	_, err := s.Transition(context.Background(), "DEN-377", "verify", Handoff{})
	if canWrite == (err != nil) {
		t.Errorf("writable = %v but err = %v", canWrite, err)
	}
}

func TestQueryContextGathersWhatAnAgentNeeds(t *testing.T) {
	s := newSession(t, map[string]string{"DEN-377": "In Progress"})
	ctx := context.Background()
	s.board.SetDescription("DEN-377", "<p>Goal: edit pkg/ticketlock/lifecycle.go and docs.md</p><ul><li>[AC-1] tools are listed</li><li><p>[AC-2] a <b>blocked</b> handoff is refused</p></li><li>just a note</li></ul>")
	if _, err := s.UploadPlan(ctx, "DEN-377", "- [ ] touch cmd/dendrite/plane.go\n- [ ] done"); err != nil {
		t.Fatal(err)
	}
	got, err := s.QueryContext(ctx, "DEN-377")
	if err != nil {
		t.Fatal(err)
	}
	if got.Ticket != "DEN-377" || got.State != "In Progress" || !strings.HasPrefix(got.Name, "[DEN-377]") || !strings.Contains(got.Description, "Goal: edit") {
		t.Errorf("header = %+v", got)
	}
	if want := []string{"[AC-1] tools are listed", "[AC-2] a blocked handoff is refused"}; !reflect.DeepEqual(got.AcceptanceCriteria, want) {
		t.Errorf("acceptance criteria = %q, want %q", got.AcceptanceCriteria, want)
	}
	if got.Plan == nil || got.Plan.NextStep != "touch cmd/dendrite/plane.go" {
		t.Errorf("plan = %+v", got.Plan)
	}
	for _, f := range []string{"cmd/dendrite/plane.go", "docs.md", "pkg/ticketlock/lifecycle.go"} {
		found := false
		for _, g := range got.Files {
			found = found || g == f
		}
		if !found {
			t.Errorf("files = %v lacks %s", got.Files, f)
		}
	}
	if _, err := s.QueryContext(ctx, "DEN-999"); err == nil {
		t.Error("an unknown ticket is an error")
	}
}

func TestQueryContextCapsTheFileListAndWorksWithoutAPlan(t *testing.T) {
	s := newSession(t, map[string]string{"DEN-377": "To Do"})
	var b strings.Builder
	for i := 0; i < 40; i++ {
		b.WriteString("<p>see pkg/dir/f" + string(rune('a'+i%26)) + string(rune('a'+i/26)) + ".go</p>")
	}
	s.board.SetDescription("DEN-377", b.String())
	got, err := s.QueryContext(context.Background(), "DEN-377")
	if err != nil || got.Plan != nil || len(got.Files) != 30 || len(got.AcceptanceCriteria) != 0 {
		t.Errorf("context = %+v, err = %v", got, err)
	}
}

func TestClaimsOfListsEveryClaimOnATicket(t *testing.T) {
	s := &Store{Dir: t.TempDir()}
	_ = s.WriteClaim(Claim{Ticket: "DEN-1", Branch: "a"})
	_ = s.WriteClaim(Claim{Ticket: "den-1", Branch: "b"})
	_ = s.WriteClaim(Claim{Ticket: "DEN-2", Branch: "c"})
	_ = os.WriteFile(filepath.Join(s.Dir, "claims", "junk.json"), []byte("{"), 0o644)
	if got := s.ClaimsOf("DEN-1"); len(got) != 2 {
		t.Errorf("claims = %+v", got)
	}
	if got := (&Store{Dir: t.TempDir()}).ClaimsOf("DEN-1"); len(got) != 0 {
		t.Errorf("claims = %+v", got)
	}
}
