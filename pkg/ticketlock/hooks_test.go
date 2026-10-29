package ticketlock

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"Dendrite/pkg/tracker"
	"Dendrite/pkg/tracker/plane"
)

func gitIn(t *testing.T, repo string, args ...string) string {
	t.Helper()
	cmd := exec.Command("git", append([]string{"-C", repo}, args...)...)
	cmd.Env = append(os.Environ(), "GIT_AUTHOR_NAME=t", "GIT_AUTHOR_EMAIL=t@example.com", "GIT_COMMITTER_NAME=t", "GIT_COMMITTER_EMAIL=t@example.com")
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %v: %v\n%s", args, err, out)
	}
	return strings.TrimSpace(string(out))
}

// hookRepo is a repository with a develop branch and a working branch agent/den-377.
func hookRepo(t *testing.T) string {
	t.Helper()
	repo := t.TempDir()
	gitIn(t, repo, "init", "-q", "-b", "develop")
	gitIn(t, repo, "commit", "-q", "--allow-empty", "-m", "base")
	gitIn(t, repo, "checkout", "-q", "-b", "agent/den-377")
	return repo
}

// hookRig is a session working in a real repository on agent/den-377.
func newHookRig(t *testing.T, state string) *session {
	t.Helper()
	s := newSession(t, map[string]string{"DEN-377": state})
	s.Dir = hookRepo(t)
	s.Branch = func(d string) string { b, _ := gitOut(d, "symbolic-ref", "--short", "HEAD"); return b }
	return s
}

func (s *session) hookDeps() Deps {
	return Deps{
		Store: s.Store, Now: s.Now, Home: s.t.TempDir(),
		RepoTop: func(dir string) (string, error) { return gitOut(dir, "rev-parse", "--show-toplevel") },
		Branch:  func(top string) string { b, _ := gitOut(top, "symbolic-ref", "--short", "HEAD"); return b },
		Linked:  func(string) bool { return true },
		PlaneStatus: func(ctx context.Context, top, ticket string) (tracker.Status, error) {
			st, _, err := s.Board.Status(ctx, ticket)
			return st, err
		},
	}
}

func (s *session) boardFunc() func() (Board, error) {
	return func() (Board, error) { return s.Board, nil }
}

func TestParseHookInputAndTarget(t *testing.T) {
	in, ok := ParseHookInput(strings.NewReader(`{"session_id":"s1","cwd":"/work","tool_name":"Edit","tool_input":{"file_path":"/work/a.go"}}`))
	if !ok || in.SessionID != "s1" || in.Target() != "/work/a.go" {
		t.Errorf("in = %+v, ok = %v", in, ok)
	}
	for name, raw := range map[string]string{"empty": "", "garbage": "{not json", "nothing known": "{}", "array": "[]"} {
		if _, ok := ParseHookInput(strings.NewReader(raw)); ok {
			t.Errorf("%s must not be judged", name)
		}
	}
	if _, ok := ParseHookInput(strings.NewReader(`{"session_id":"only"}`)); !ok {
		t.Error("a session id alone is an event")
	}
	if _, ok := ParseHookInput(failingReader{}); ok {
		t.Error("an unreadable stream is not an event")
	}

	var rel HookInput
	rel.Cwd = "/work"
	rel.ToolInput.FilePath = "pkg/../pkg/b.go"
	if got := rel.Target(); got != "/work/pkg/b.go" {
		t.Errorf("relative target = %q", got)
	}
	var nb HookInput
	nb.ToolInput.NotebookPath = "/work/n.ipynb"
	if got := nb.Target(); got != "/work/n.ipynb" {
		t.Errorf("notebook target = %q", got)
	}
	if (HookInput{}).Target() != "" {
		t.Error("no target")
	}
}

type failingReader struct{}

func (failingReader) Read([]byte) (int, error) { return 0, errors.New("broken pipe") }

func TestCheckEditFollowsTheClaim(t *testing.T) {
	s := newHookRig(t, "To Do")
	ctx := context.Background()
	in := HookInput{Cwd: s.Dir}
	in.ToolInput.FilePath = filepath.Join(s.Dir, "pkg", "x.go")

	if err := CheckEdit(ctx, s.hookDeps(), HookInput{}); err != nil {
		t.Errorf("an event with no target is allowed: %v", err)
	}
	var lock *ErrLocked
	if err := CheckEdit(ctx, s.hookDeps(), in); !errors.As(err, &lock) {
		t.Fatalf("an unclaimed copy must be locked: %v", err)
	}
	if _, err := s.Claim(ctx, "DEN-377", ClaimOptions{}); err != nil {
		t.Fatal(err)
	}
	if err := CheckEdit(ctx, s.hookDeps(), in); err != nil {
		t.Errorf("a claimed copy is open: %v", err)
	}
}

func TestCheckStagedAppliesTheGuardToEveryStagedPath(t *testing.T) {
	s := newHookRig(t, "To Do")
	ctx := context.Background()
	d := s.hookDeps()

	if err := CheckStaged(ctx, d, s.Dir); err != nil {
		t.Errorf("nothing staged: %v", err)
	}
	if err := os.WriteFile(filepath.Join(s.Dir, "a.go"), []byte("package a"), 0o644); err != nil {
		t.Fatal(err)
	}
	gitIn(t, s.Dir, "add", "a.go")
	var lock *ErrLocked
	if err := CheckStaged(ctx, d, s.Dir); !errors.As(err, &lock) {
		t.Fatalf("staged work without a claim must be refused: %v", err)
	}
	if _, err := s.Claim(ctx, "DEN-377", ClaimOptions{}); err != nil {
		t.Fatal(err)
	}
	if err := CheckStaged(ctx, d, s.Dir); err != nil {
		t.Errorf("staged work with a claim: %v", err)
	}

	notARepo := t.TempDir()
	if err := CheckStaged(ctx, d, notARepo); err != nil {
		t.Errorf("outside git: %v", err)
	}
	d.RepoTop = func(string) (string, error) { return notARepo, nil }
	if err := CheckStaged(ctx, d, s.Dir); err != nil {
		t.Errorf("a top level git cannot read: %v", err)
	}
}

func TestPlaneStatusForUsesTheEnvironmentAndTheRepositorysProjectLink(t *testing.T) {
	s := newSession(t, map[string]string{"DEN-377": "Blocked"})
	// Nothing of the developer's own project may leak in through the current directory or Dendrite's home.
	t.Chdir(t.TempDir())
	t.Setenv("DENDRITE_HOME", t.TempDir())
	t.Setenv("PLANE_API_KEY", "k")
	t.Setenv("PLANE_BASE_URL", s.board.URL)
	t.Setenv("PLANE_WORKSPACE_SLUG", "ws")
	t.Setenv("PLANE_PROJECT_ID", "proj")
	ctx := context.Background()

	if st, err := PlaneStatusFor(ctx, t.TempDir(), "DEN-377"); err != nil || st != tracker.StatusBlocked {
		t.Errorf("from the environment: %v, %v", st, err)
	}

	// Without project settings in the environment the repository's own link decides.
	t.Setenv("PLANE_WORKSPACE_SLUG", "")
	t.Setenv("PLANE_PROJECT_ID", "")
	repo := t.TempDir()
	gitIn(t, repo, "init", "-q")
	// Nothing names a project, so ConfigFromEnv falls back to its built-in default one, which this board does not serve.
	if _, err := PlaneStatusFor(ctx, repo, "DEN-377"); err == nil {
		t.Error("a project the board does not serve must be an error")
	}
	if err := plane.WriteLink(repo, plane.ProjectLink{Identifier: "DEN", ProjectID: "proj", Workspace: "ws"}); err != nil {
		t.Fatal(err)
	}
	if st, err := PlaneStatusFor(ctx, repo, "DEN-377"); err != nil || st != tracker.StatusBlocked {
		t.Errorf("from the link: %v, %v", st, err)
	}

	t.Setenv("PLANE_API_KEY", "")
	if _, err := PlaneStatusFor(ctx, repo, "DEN-377"); err == nil {
		t.Error("no API key is an error")
	}
	if _, err := BoardFor(repo); err == nil {
		t.Error("BoardFor without credentials is an error")
	}
	if d := HookDeps(); d.PlaneStatus == nil || d.Store == nil || d.RepoTop == nil {
		t.Errorf("HookDeps = %+v", d)
	}
}

func TestSavePlanOnTheClaimedTicketOrLocally(t *testing.T) {
	ctx := context.Background()
	s := newHookRig(t, "To Do")
	in := HookInput{SessionID: "s-9", Cwd: s.Dir}

	if msg, err := SavePlan(ctx, s.Lifecycle, s.boardFunc(), in); err != nil || msg != "" {
		t.Errorf("no plan text: %q, %v", msg, err)
	}
	in.ToolInput.Plan = "- [ ] one\n- [ ] two"
	msg, err := SavePlan(ctx, s.Lifecycle, s.boardFunc(), in)
	if err != nil || !strings.Contains(msg, "kept locally (unclaimed/s-9)") || !strings.Contains(msg, "dendrite chronos claim") {
		t.Fatalf("unclaimed: %q, %v", msg, err)
	}
	if p, err := s.Plans.Load("unclaimed/s-9"); err != nil || p.PendingSteps != 2 {
		t.Errorf("local plan = %+v, %v", p, err)
	}
	in.SessionID = ""
	if msg, _ := SavePlan(ctx, s.Lifecycle, s.boardFunc(), in); !strings.Contains(msg, "unclaimed/session") {
		t.Errorf("a session id is not required: %q", msg)
	}

	if _, err := s.Claim(ctx, "DEN-377", ClaimOptions{}); err != nil {
		t.Fatal(err)
	}
	msg, err = SavePlan(ctx, s.Lifecycle, s.boardFunc(), in)
	if err != nil || msg != "plan v1 stored on DEN-377 (2 step(s) left)" {
		t.Errorf("claimed: %q, %v", msg, err)
	}
	if got, err := s.GetPlan(ctx, "DEN-377"); err != nil || got.Version != 1 {
		t.Errorf("plan on the ticket = %+v, %v", got, err)
	}

	failing := func() (Board, error) { return Board{}, errors.New("Plane is not configured") }
	if _, err := SavePlan(ctx, s.Lifecycle, failing, in); err == nil || !strings.Contains(err.Error(), "plan not stored on DEN-377") {
		t.Errorf("no board: %v", err)
	}
	s.board.Fail("comments/", 0)
	if _, err := SavePlan(ctx, s.Lifecycle, s.boardFunc(), in); err == nil {
		t.Error("Plane failing on the upload is reported")
	}
}

func TestSavePlanReportsALocalStoreThatCannotBeWritten(t *testing.T) {
	s := newHookRig(t, "To Do")
	s.Plans = PlanStore{Dir: filepath.Join(writeRegularFile(t), "plans")}
	in := HookInput{SessionID: "s-1", Cwd: s.Dir}
	in.ToolInput.Plan = "- [ ] one"
	if _, err := SavePlan(context.Background(), s.Lifecycle, s.boardFunc(), in); err == nil {
		t.Error("a plan that cannot be kept anywhere is an error")
	}
}

func TestAutoHandoffBlocksATicketLeftInProgress(t *testing.T) {
	ctx := context.Background()
	s := newHookRig(t, "To Do")
	in := HookInput{SessionID: "s-1", Cwd: s.Dir}

	if msg, err := AutoHandoff(ctx, s.Lifecycle, s.boardFunc(), in); err != nil || msg != "" {
		t.Errorf("no claim, nothing to hand off: %q, %v", msg, err)
	}
	if _, err := s.Claim(ctx, "DEN-377", ClaimOptions{}); err != nil {
		t.Fatal(err)
	}
	if _, err := s.UploadPlan(ctx, "DEN-377", "- [x] design\n- [ ] build\n- [ ] test"); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(s.Dir, "work.go"), []byte("package w"), 0o644); err != nil {
		t.Fatal(err)
	}
	gitIn(t, s.Dir, "add", "work.go")
	gitIn(t, s.Dir, "commit", "-q", "-m", "first slice")
	if err := os.WriteFile(filepath.Join(s.Dir, "work.go"), []byte("package w // edited"), 0o644); err != nil {
		t.Fatal(err)
	}

	msg, err := AutoHandoff(ctx, s.Lifecycle, s.boardFunc(), in)
	if err != nil || !strings.Contains(msg, "DEN-377 was still In Progress: blocked with an automatic handoff") {
		t.Fatalf("msg = %q, err = %v", msg, err)
	}
	if s.board.State("DEN-377") != "Blocked" {
		t.Errorf("state = %s", s.board.State("DEN-377"))
	}
	if labels := strings.Join(s.board.LabelNames("DEN-377"), ","); !strings.Contains(labels, "needs-input") {
		t.Errorf("labels = %s", labels)
	}
	all := texts(s.board, "DEN-377")
	handoff := all[len(all)-1]
	for _, want := range []string{"[dendrite:handoff] Completed:", "Commits on agent/den-377 not on develop:", "first slice", "Uncommitted changes:", "work.go", "Plan steps done:", "- [x] design", "written automatically", "Plan steps left:", "- [ ] build", "- [ ] test"} {
		if !strings.Contains(handoff, want) {
			t.Errorf("handoff lacks %q:\n%s", want, handoff)
		}
	}
	if _, err := s.Store.ClaimFor("agent/den-377"); err != nil {
		t.Errorf("the claim stays for whoever resumes: %v", err)
	}
}

func TestAutoHandoffWithoutAPlanOrAnyWork(t *testing.T) {
	ctx := context.Background()
	s := newHookRig(t, "To Do")
	if _, err := s.Claim(ctx, "DEN-377", ClaimOptions{}); err != nil {
		t.Fatal(err)
	}
	if _, err := AutoHandoff(ctx, s.Lifecycle, s.boardFunc(), HookInput{Cwd: s.Dir}); err != nil {
		t.Fatal(err)
	}
	all := texts(s.board, "DEN-377")
	last := all[len(all)-1]
	if !strings.Contains(last, "Nothing committed or changed on the branch yet.") || !strings.Contains(last, "No plan was stored: compare the work with the ticket's acceptance criteria (`dendrite chronos context DEN-377`)") {
		t.Errorf("handoff:\n%s", last)
	}
}

func TestAutoHandoffLeavesFinishedAndUnreachableTicketsAlone(t *testing.T) {
	ctx := context.Background()
	s := newHookRig(t, "To Do")
	if _, err := s.Claim(ctx, "DEN-377", ClaimOptions{}); err != nil {
		t.Fatal(err)
	}
	in := HookInput{Cwd: s.Dir}
	if _, err := s.Transition(ctx, "DEN-377", "verify", Handoff{}); err != nil {
		t.Fatal(err)
	}
	_ = s.Store.WriteClaim(Claim{Ticket: "DEN-377", Branch: "agent/den-377", ClaimedAt: t0, CheckedAt: t0})
	if msg, err := AutoHandoff(ctx, s.Lifecycle, s.boardFunc(), in); err != nil || msg != "" {
		t.Errorf("a ticket already To Verify: %q, %v", msg, err)
	}

	noBoard := func() (Board, error) { return Board{}, errors.New("Plane is not configured") }
	if _, err := AutoHandoff(ctx, s.Lifecycle, noBoard, in); err == nil || !strings.Contains(err.Error(), "handoff of DEN-377 not written") {
		t.Errorf("no board: %v", err)
	}
	s.board.Fail("issues/", 0)
	if _, err := AutoHandoff(ctx, s.Lifecycle, s.boardFunc(), in); err == nil || !strings.Contains(err.Error(), "handoff of DEN-377 not written") {
		t.Errorf("Plane failing on the read: %v", err)
	}
}

func TestAutoHandoffReportsWhatPlaneRefuses(t *testing.T) {
	ctx := context.Background()
	for name, fail := range map[string]struct {
		match string
		after int
		want  string
	}{
		"the transition":        {"PATCH", 0, "set DEN-377 to Blocked"},
		"the needs-input label": {"labels/", 0, "blocked but not labelled needs-input"},
	} {
		t.Run(name, func(t *testing.T) {
			s := newHookRig(t, "To Do")
			if _, err := s.Claim(ctx, "DEN-377", ClaimOptions{}); err != nil {
				t.Fatal(err)
			}
			s.board.Fail(fail.match, fail.after)
			if _, err := AutoHandoff(ctx, s.Lifecycle, s.boardFunc(), HookInput{Cwd: s.Dir}); err == nil || !strings.Contains(err.Error(), fail.want) {
				t.Errorf("err = %v, want %q", err, fail.want)
			}
		})
	}
}

func TestStepsMatching(t *testing.T) {
	md := "- [x] a\n* [X] b\n  - [ ] c\n* [ ] d\ntext\n- [x]\n"
	if got := checkedSteps(md); got != "- [x] a\n* [X] b\n- [x]" {
		t.Errorf("checked = %q", got)
	}
	if got := uncheckedSteps(md); got != "- [ ] c\n* [ ] d" {
		t.Errorf("unchecked = %q", got)
	}
}
