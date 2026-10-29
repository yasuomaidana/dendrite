package ticketlock

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"Dendrite/pkg/planesvc"
	"Dendrite/pkg/planesvc/planetest"
	"Dendrite/pkg/sidecar"
	"Dendrite/pkg/tracker"
	"Dendrite/pkg/tracker/plane"
)

// session is one agent working on a Plane board: the lifecycle, the board it talks to and the branch it is on.
type session struct {
	*Lifecycle
	t      *testing.T
	board  *planetest.Board
	branch string
}

func newSession(t *testing.T, tickets map[string]string) *session {
	t.Helper()
	t.Setenv(AgentEnv, "claude")
	t.Setenv(TierEnv, "7")
	t.Setenv(SessionIDEnv, "")
	board := planetest.NewBoard(t, tickets)
	client, err := plane.NewClient(plane.Config{BaseURL: board.URL, WorkspaceSlug: planetest.BoardWorkspace, ProjectID: planetest.BoardProject, DirectToken: "k"}, board.Client())
	if err != nil {
		t.Fatal(err)
	}
	svc := planesvc.New(sidecar.DefaultLocator())
	t.Cleanup(func() { _ = svc.Close() })
	s := &session{t: t, board: board, branch: "agent/den-377"}
	s.Lifecycle = &Lifecycle{
		Board: Board{Svc: svc, Client: client}, Store: &Store{Dir: t.TempDir()}, Plans: PlanStore{Dir: t.TempDir()},
		Now: func() time.Time { return t0 }, Dir: t.TempDir(),
		Branch: func(string) string { return s.branch }, Head: func(string) string { return "abc1234" },
	}
	return s
}

func texts(b *planetest.Board, ref string) []string {
	var out []string
	for _, c := range b.Comments(ref) {
		out = append(out, c.Text())
	}
	return out
}

func TestClaimMovesAssignsLabelsRecordsAndGreets(t *testing.T) {
	s := newSession(t, map[string]string{"DEN-377": "To Do"})
	res, err := s.Claim(context.Background(), "DEN-377", ClaimOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if res.Ticket != "DEN-377" || res.State != "In Progress" || res.Branch != "agent/den-377" || res.SessionID == "" || res.Plan != nil {
		t.Errorf("result = %+v", res)
	}
	if got := s.board.State("DEN-377"); got != "In Progress" {
		t.Errorf("Plane state = %q", got)
	}
	if got := s.board.Assignees("DEN-377"); !reflect.DeepEqual(got, []string{"u-1"}) {
		t.Errorf("assignees = %v, want the API key's user", got)
	}
	if got := s.board.LabelNames("DEN-377"); !reflect.DeepEqual(got, []string{"agent:claude", "tier:7"}) {
		t.Errorf("labels = %v", got)
	}
	c, err := s.Store.ClaimFor("agent/den-377")
	if err != nil || c.Ticket != "DEN-377" || c.SessionID != res.SessionID || c.PlaneState != tracker.StatusInProgress || !c.CheckedAt.Equal(t0) {
		t.Errorf("claim = %+v, err = %v", c, err)
	}
	got := texts(s.board, "DEN-377")
	if len(got) != 1 || !strings.HasPrefix(got[0], "[dendrite:claim] Claimed by claude") || !strings.Contains(got[0], "branch agent/den-377") || !strings.Contains(got[0], "commit abc1234") {
		t.Errorf("comments = %q", got)
	}
}

func TestReClaimReplacesAgentAndTierLabels(t *testing.T) {
	s := newSession(t, map[string]string{"DEN-377": "In Progress"})
	if _, err := s.Claim(context.Background(), "DEN-377", ClaimOptions{}); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Claim(context.Background(), "DEN-377", ClaimOptions{Agent: "agy", Tier: "5"}); err != nil {
		t.Fatal(err)
	}
	if got := s.board.LabelNames("DEN-377"); !reflect.DeepEqual(got, []string{"agent:agy", "tier:5"}) {
		t.Errorf("labels after a re-claim = %v: the old agent and tier labels must go", got)
	}
	if got := s.board.State("DEN-377"); got != "In Progress" {
		t.Errorf("state = %q", got)
	}
}

func TestClaimWithoutATierAddsOnlyTheAgentLabel(t *testing.T) {
	s := newSession(t, map[string]string{"DEN-377": "Backlog"})
	t.Setenv(TierEnv, "")
	if _, err := s.Claim(context.Background(), "377", ClaimOptions{}); err != nil {
		t.Fatal(err)
	}
	if got := s.board.LabelNames("DEN-377"); !reflect.DeepEqual(got, []string{"agent:claude"}) {
		t.Errorf("labels = %v", got)
	}
}

func TestClaimResumesABlockedTicketAndReturnsItsPlan(t *testing.T) {
	s := newSession(t, map[string]string{"DEN-377": "Blocked"})
	if err := s.Plans.Save("DEN-377", newPlan("- [x] design\n- [ ] build the thing\n- [ ] test it", 3)); err != nil {
		t.Fatal(err)
	}
	res, err := s.Claim(context.Background(), "DEN-377", ClaimOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if s.board.State("DEN-377") != "In Progress" || res.Plan == nil || res.Plan.NextStep != "build the thing" || res.Plan.PendingSteps != 2 || res.Plan.Source != "engram" {
		t.Errorf("state = %s, plan = %+v", s.board.State("DEN-377"), res.Plan)
	}
}

func TestClaimRefusals(t *testing.T) {
	cases := []struct {
		name    string
		tickets map[string]string
		ref     string
		setup   func(s *session)
		want    string
	}{
		{"empty reference", map[string]string{"DEN-377": "To Do"}, "  ", nil, "a ticket is required"},
		{"unknown ticket", map[string]string{"DEN-377": "To Do"}, "DEN-999", nil, "no ticket matches"},
		{"finished ticket", map[string]string{"DEN-377": "Done"}, "DEN-377", nil, "it is finished"},
		{"awaiting verification", map[string]string{"DEN-377": "To Verify"}, "DEN-377", nil, "awaiting verification"},
		{"not a git working copy", map[string]string{"DEN-377": "To Do"}, "DEN-377", func(s *session) { s.branch = "" }, "not a git working copy"},
		{"branch of another ticket", map[string]string{"DEN-377": "To Do"}, "DEN-377", func(s *session) { s.branch = "agent/den-1" }, "belongs to den-1"},
		{"live claim of another session", map[string]string{"DEN-377": "To Do"}, "DEN-377", func(s *session) {
			_ = s.Store.WriteClaim(Claim{Ticket: "DEN-377", SessionID: "s-other", Agent: "agy", Branch: "feature/other", ClaimedAt: t0.Add(-time.Hour), CheckedAt: t0.Add(-time.Hour)})
		}, "claimed by agy (session s-other) on branch feature/other"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			s := newSession(t, c.tickets)
			if c.setup != nil {
				c.setup(s)
			}
			_, err := s.Claim(context.Background(), c.ref, ClaimOptions{})
			if err == nil || !strings.Contains(err.Error(), c.want) {
				t.Errorf("err = %v, want it to contain %q", err, c.want)
			}
			if s.board.State("DEN-377") == "In Progress" {
				t.Error("a refused claim must not start the ticket")
			}
		})
	}
}

func TestStealTakesOverAndAStaleClaimIsIgnored(t *testing.T) {
	s := newSession(t, map[string]string{"DEN-377": "To Do"})
	_ = s.Store.WriteClaim(Claim{Ticket: "DEN-377", SessionID: "s-old", Branch: "feature/stale", ClaimedAt: t0.Add(-5 * time.Hour), CheckedAt: t0.Add(-5 * time.Hour)})
	_ = s.Store.WriteClaim(Claim{Ticket: "DEN-377", SessionID: "s-live", Branch: "feature/live", ClaimedAt: t0.Add(-10 * time.Minute), CheckedAt: t0.Add(-10 * time.Minute)})

	if _, err := s.Claim(context.Background(), "DEN-377", ClaimOptions{}); err == nil {
		t.Fatal("the live claim must be respected")
	}
	if _, err := s.Claim(context.Background(), "DEN-377", ClaimOptions{Steal: true}); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Store.ClaimFor("feature/live"); err != ErrNoClaim {
		t.Errorf("the stolen claim must be released: %v", err)
	}
	if _, err := s.Store.ClaimFor("feature/stale"); err != nil {
		t.Errorf("a stale claim of someone else is left alone: %v", err)
	}
}

func TestStealFailsWhenTheOldClaimCannotBeReleased(t *testing.T) {
	s := newSession(t, map[string]string{"DEN-377": "To Do"})
	_ = s.Store.WriteClaim(Claim{Ticket: "DEN-377", SessionID: "s-live", Branch: "feature/live", ClaimedAt: t0, CheckedAt: t0})
	dir := filepath.Join(s.Store.Dir, "claims")
	if err := os.Chmod(dir, 0o500); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(dir, 0o755) })
	canWrite := os.WriteFile(filepath.Join(dir, "probe"), nil, 0o644) == nil // root ignores permissions
	_, err := s.Claim(context.Background(), "DEN-377", ClaimOptions{Steal: true})
	if canWrite == (err != nil) {
		t.Errorf("writable = %v but err = %v", canWrite, err)
	}
}

func TestClaimStopsAtEachPlaneFailure(t *testing.T) {
	cases := []struct {
		name  string
		match string
		after int
		want  string
	}{
		{"cannot start the ticket", "PATCH", 0, "start DEN-377"},
		{"cannot find the user", "users/me", 0, "find the Plane user"},
		{"cannot assign", "PATCH", 1, "assign DEN-377"},
		{"cannot label", "labels/", 0, "label DEN-377"},
		{"cannot comment", "comments/", 0, "comment on DEN-377"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			s := newSession(t, map[string]string{"DEN-377": "To Do"})
			s.board.Fail(c.match, c.after)
			if _, err := s.Claim(context.Background(), "DEN-377", ClaimOptions{}); err == nil || !strings.Contains(err.Error(), c.want) {
				t.Errorf("err = %v, want it to contain %q", err, c.want)
			}
		})
	}
}

func TestClaimReportsStoresThatCannotBeWritten(t *testing.T) {
	t.Run("no token can be minted", func(t *testing.T) {
		s := newSession(t, map[string]string{"DEN-377": "To Do"})
		s.Store = brokenStore(t)
		if _, err := s.Claim(context.Background(), "DEN-377", ClaimOptions{}); err == nil || !strings.Contains(err.Error(), "session token") {
			t.Errorf("err = %v", err)
		}
	})
	t.Run("the claim cannot be recorded", func(t *testing.T) {
		s := newSession(t, map[string]string{"DEN-377": "To Do"})
		_, _ = s.Store.MintToken(TokenOptions{Agent: "claude"}, t0)
		_ = os.WriteFile(filepath.Join(s.Store.Dir, "claims"), nil, 0o644) // a file where the directory belongs
		if _, err := s.Claim(context.Background(), "DEN-377", ClaimOptions{}); err == nil || !strings.Contains(err.Error(), "claim") {
			t.Errorf("err = %v", err)
		}
	})
}

func TestCommentKindsBodiesAndEscaping(t *testing.T) {
	s := newSession(t, map[string]string{"DEN-377": "In Progress"})
	_, _ = s.Claim(context.Background(), "DEN-377", ClaimOptions{})
	ctx := context.Background()

	if err := s.Comment(ctx, "DEN-377", "milestone", "tests <green>\nall of them & more"); err != nil {
		t.Fatal(err)
	}
	last := s.board.Comments("DEN-377")
	html := last[len(last)-1].HTML
	if !strings.Contains(html, "[dendrite:milestone] tests &lt;green&gt;<br>all of them &amp; more") || !strings.Contains(html, "<sub>session ") {
		t.Errorf("stored HTML = %s", html)
	}
	for _, kind := range CommentKinds {
		if err := s.Comment(ctx, "DEN-377", kind, "x"); err != nil {
			t.Errorf("kind %s: %v", kind, err)
		}
	}
	if err := s.Comment(ctx, "DEN-377", "shout", "x"); err == nil || !strings.Contains(err.Error(), "unknown comment kind") {
		t.Errorf("unknown kind: %v", err)
	}
	if err := s.Comment(ctx, "DEN-377", "note", "  "); err == nil {
		t.Error("an empty comment is refused")
	}
	s.board.Fail("comments/", 0)
	if err := s.Comment(ctx, "DEN-377", "note", "x"); err == nil {
		t.Error("a Plane failure is reported")
	}
}

func TestFooterWithoutAClaimOrAHead(t *testing.T) {
	s := newSession(t, map[string]string{"DEN-377": "In Progress"})
	s.Head = func(string) string { return "" }
	got := s.footer(Claim{})
	if got != "session - · agent - · tier - · branch agent/den-377" {
		t.Errorf("footer = %q", got)
	}
}

func TestNewLifecycleAsksGit(t *testing.T) {
	t.Setenv("DENDRITE_HOME", t.TempDir())
	t.Setenv("DENDRITE_STATE_DIR", t.TempDir())
	repo := t.TempDir()
	run := func(args ...string) {
		t.Helper()
		if out, err := exec.Command("git", append([]string{"-C", repo}, args...)...).CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, out)
		}
	}
	run("init", "-q")
	run("checkout", "-q", "-b", "agent/den-377")
	l := NewLifecycle(Board{}, repo)
	if got := l.Branch(repo); got != "agent/den-377" {
		t.Errorf("Branch = %q", got)
	}
	if got := l.Head(repo); got != "" {
		t.Errorf("Head of a repository without commits = %q", got)
	}
	run("-c", "user.name=t", "-c", "user.email=t@example.com", "commit", "-q", "--allow-empty", "-m", "first")
	if got := l.Head(repo); len(got) < 7 {
		t.Errorf("Head = %q", got)
	}
	if !strings.HasSuffix(l.Plans.Dir, filepath.Join("engram", "plans")) || l.Store == nil || l.Now == nil || l.Dir != repo {
		t.Errorf("lifecycle = %+v", l)
	}
}

func TestCanonicalAndFirstNonEmpty(t *testing.T) {
	if got := canonical(&plane.PlaneIssue{Name: " [den-9] nine"}, "9"); got != "DEN-9" {
		t.Errorf("from the title = %q", got)
	}
	if got := canonical(&plane.PlaneIssue{Name: "no id"}, " den-4 "); got != "DEN-4" {
		t.Errorf("from the reference = %q", got)
	}
	if got := firstNonEmpty("", "  ", "x", "y"); got != "x" {
		t.Errorf("firstNonEmpty = %q", got)
	}
	if got := firstNonEmpty(); got != "" {
		t.Errorf("firstNonEmpty() = %q", got)
	}
}
