package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"Dendrite/pkg/planesvc/planetest"
	"Dendrite/pkg/tracker/plane"
)

// lifecycleCLI points `dendrite chronos` at a stateful Plane board and a working copy on branch agent/den-377.
type lifecycleCLI struct {
	t     *testing.T
	board *planetest.Board
	repo  string
}

func newLifecycleCLI(t *testing.T, tickets map[string]string) *lifecycleCLI {
	t.Helper()
	jsonOutput(t, false)
	t.Setenv("DENDRITE_HOME", t.TempDir())
	t.Setenv("DENDRITE_STATE_DIR", t.TempDir())
	t.Setenv("DENDRITE_AGENT", "claude")
	t.Setenv("DENDRITE_MODEL_TIER", "7")
	board := planetest.NewBoard(t, tickets)
	client, err := plane.NewClient(plane.Config{BaseURL: board.URL, WorkspaceSlug: planetest.BoardWorkspace, ProjectID: planetest.BoardProject, DirectToken: "k"}, board.Client())
	if err != nil {
		t.Fatal(err)
	}
	prev := newPlaneClient
	newPlaneClient = func() (*plane.Client, error) { return client, nil }
	t.Cleanup(func() { newPlaneClient = prev })
	repo := t.TempDir()
	for _, args := range [][]string{{"init", "-q"}, {"checkout", "-q", "-b", "agent/den-377"}} {
		if out, err := exec.Command("git", append([]string{"-C", repo}, args...)...).CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, out)
		}
	}
	return &lifecycleCLI{t: t, board: board, repo: repo}
}

// run executes `dendrite chronos <args>`.
func (c *lifecycleCLI) run(args ...string) (string, error) {
	c.t.Helper()
	var buf bytes.Buffer
	err := runCLI(context.Background(), &buf, append([]string{"chronos"}, args...)...)
	return buf.String(), err
}

// runJSON executes `dendrite chronos <args> --json`.
func (c *lifecycleCLI) runJSON(args ...string) (string, error) {
	c.t.Helper()
	return c.run(append(args, "--json")...)
}

func TestChronosClaimPrintsAndRecords(t *testing.T) {
	c := newLifecycleCLI(t, map[string]string{"DEN-377": "To Do"})
	out, err := c.run("claim", "DEN-377", "--worktree", c.repo)
	if err != nil {
		t.Fatalf("claim: %v\n%s", err, out)
	}
	if !strings.Contains(out, "[✓] Claimed DEN-377: In Progress on branch agent/den-377 (session s-") {
		t.Errorf("output:\n%s", out)
	}
	if c.board.State("DEN-377") != "In Progress" || strings.Join(c.board.LabelNames("DEN-377"), ",") != "agent:claude,tier:7" {
		t.Errorf("state = %s, labels = %v", c.board.State("DEN-377"), c.board.LabelNames("DEN-377"))
	}

	if out, err = c.run("plan", "push", "DEN-377", "--worktree", c.repo, "--file", writeFile(t, "- [ ] one\n- [ ] two")); err != nil {
		t.Fatalf("plan push: %v\n%s", err, out)
	}
	out, err = c.runJSON("claim", "DEN-377", "--worktree", c.repo, "--agent", "agy", "--tier", "5")
	if err != nil {
		t.Fatal(err)
	}
	var res struct {
		Ticket string `json:"ticket"`
		Plan   struct {
			NextStep string `json:"next_step"`
		} `json:"plan"`
	}
	if json.Unmarshal([]byte(out), &res) != nil || res.Ticket != "DEN-377" || res.Plan.NextStep != "one" {
		t.Errorf("JSON claim = %s", out)
	}
	out, _ = c.run("claim", "DEN-377", "--worktree", c.repo)
	if !strings.Contains(out, "Plan v1 from plane: 2 step(s) left, next: one") {
		t.Errorf("a claim shows the plan:\n%s", out)
	}
}

func TestChronosClaimRefusalsAndPlaneErrors(t *testing.T) {
	c := newLifecycleCLI(t, map[string]string{"DEN-377": "Done"})
	if _, err := c.run("claim", "DEN-377", "--worktree", c.repo); err == nil || !strings.Contains(err.Error(), "it is finished") {
		t.Errorf("claim of a finished ticket: %v", err)
	}
	prev := newPlaneClient
	newPlaneClient = func() (*plane.Client, error) { return nil, errors.New("no Plane configured") }
	defer func() { newPlaneClient = prev }()
	for _, args := range [][]string{{"claim", "DEN-1"}, {"comment", "DEN-1", "--body", "x"}, {"plan", "get", "DEN-1"}, {"plan", "push", "DEN-1", "--file", writeFile(t, "- [ ] x")}, {"handoff", "DEN-1"}, {"context", "DEN-1"}} {
		if _, err := c.run(args...); err == nil || !strings.Contains(err.Error(), "initialize Plane client") {
			t.Errorf("%v: %v", args, err)
		}
	}
}

func writeFile(t *testing.T, content string) string {
	t.Helper()
	p := filepath.Join(t.TempDir(), "f.md")
	if err := os.WriteFile(p, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	return p
}

func TestChronosCommentFromFlagAndFile(t *testing.T) {
	c := newLifecycleCLI(t, map[string]string{"DEN-377": "In Progress"})
	out, err := c.run("comment", "DEN-377", "--worktree", c.repo, "--kind", "milestone", "--body", "lock done")
	if err != nil || !strings.Contains(out, "Posted a milestone comment on DEN-377") {
		t.Fatalf("comment: %v\n%s", err, out)
	}
	if _, err := c.run("comment", "DEN-377", "--body-file", writeFile(t, "from a file\nsecond line")); err != nil {
		t.Fatal(err)
	}
	got := texts(c.board, "DEN-377")
	if len(got) != 2 || !strings.HasPrefix(got[0], "[dendrite:milestone] lock done") || !strings.HasPrefix(got[1], "[dendrite:progress] from a file second line") {
		t.Errorf("comments = %q", got)
	}
	if _, err := c.run("comment", "DEN-377", "--body-file", filepath.Join(t.TempDir(), "missing.md")); err == nil || !strings.Contains(err.Error(), "read ") {
		t.Errorf("missing body file: %v", err)
	}
	if _, err := c.run("comment", "DEN-377", "--kind", "progress"); err == nil {
		t.Error("a comment with no text is refused")
	}
}

func texts(b *planetest.Board, ref string) []string {
	var out []string
	for _, c := range b.Comments(ref) {
		out = append(out, c.Text())
	}
	return out
}

func TestChronosPlanPushAndGet(t *testing.T) {
	c := newLifecycleCLI(t, map[string]string{"DEN-377": "In Progress"})
	if _, err := c.run("plan", "get", "DEN-377"); err == nil || !strings.Contains(err.Error(), "no plan") {
		t.Errorf("plan get before any plan: %v", err)
	}
	if _, err := c.run("plan", "push", "DEN-377"); err == nil || !strings.Contains(err.Error(), "--file is required") {
		t.Errorf("plan push without --file: %v", err)
	}
	if _, err := c.run("plan", "push", "DEN-377", "--file", filepath.Join(t.TempDir(), "none.md")); err == nil {
		t.Error("plan push of a missing file")
	}
	out, err := c.run("plan", "push", "DEN-377", "--file", writeFile(t, "- [x] a\n- [ ] b\n"))
	if err != nil || !strings.Contains(out, "Stored plan v1 of DEN-377: 1 step(s) left, next: b") {
		t.Fatalf("push: %v\n%s", err, out)
	}
	out, err = c.run("plan", "get", "DEN-377")
	if err != nil || !strings.Contains(out, "- [ ] b") || !strings.Contains(out, "[i] v1 from plane, 1 step(s) left, next: b") {
		t.Errorf("get: %v\n%s", err, out)
	}
	out, _ = c.runJSON("plan", "get", "DEN-377")
	var p struct {
		Version int    `json:"version"`
		Source  string `json:"source"`
	}
	if json.Unmarshal([]byte(out), &p) != nil || p.Version != 1 || p.Source != "plane" {
		t.Errorf("JSON get = %s", out)
	}
	out, _ = c.runJSON("plan", "push", "DEN-377", "--file", writeFile(t, "- [ ] again"))
	if json.Unmarshal([]byte(out), &p) != nil || p.Version != 2 {
		t.Errorf("JSON push = %s", out)
	}
}

func TestChronosHandoffNeedsAllThreeSections(t *testing.T) {
	c := newLifecycleCLI(t, map[string]string{"DEN-377": "In Progress"})
	if _, err := c.run("handoff", "DEN-377", "--worktree", c.repo, "--completed", "a"); err == nil || !strings.Contains(err.Error(), "needs a handoff") {
		t.Errorf("incomplete handoff: %v", err)
	}
	out, err := c.run("handoff", "DEN-377", "--worktree", c.repo, "--completed", "a", "--challenges", "b", "--missing", "c")
	if err != nil || !strings.Contains(out, "DEN-377 is now Blocked, with its handoff") || c.board.State("DEN-377") != "Blocked" {
		t.Errorf("handoff: %v\n%s", err, out)
	}
	c2 := newLifecycleCLI(t, map[string]string{"DEN-377": "In Progress"})
	out, err = c2.runJSON("handoff", "DEN-377", "--worktree", c2.repo, "--completed", "a", "--challenges", "b", "--missing", "c")
	var res struct{ State string }
	if err != nil || json.Unmarshal([]byte(out), &res) != nil || res.State != "Blocked" {
		t.Errorf("JSON handoff = %s, err = %v", out, err)
	}
}

func TestChronosContext(t *testing.T) {
	c := newLifecycleCLI(t, map[string]string{"DEN-377": "In Progress"})
	c.board.SetDescription("DEN-377", "<p>edit pkg/ticketlock/plan.go</p><ul><li>[AC-1] listed</li></ul>")
	if _, err := c.run("plan", "push", "DEN-377", "--file", writeFile(t, "- [ ] step one")); err != nil {
		t.Fatal(err)
	}
	out, err := c.run("context", "DEN-377")
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"[DEN-377] work [In Progress]", "- [AC-1] listed", "Plan v1 (1 step(s) left), next: step one", "Files named: pkg/ticketlock/plan.go"} {
		if !strings.Contains(out, want) {
			t.Errorf("context output lacks %q:\n%s", want, out)
		}
	}
	out, _ = c.runJSON("context", "DEN-377")
	var got struct {
		Ticket string `json:"ticket"`
	}
	if json.Unmarshal([]byte(out), &got) != nil || got.Ticket != "DEN-377" {
		t.Errorf("JSON context = %s", out)
	}
	if _, err := c.run("context", "DEN-999"); err == nil {
		t.Error("an unknown ticket is an error")
	}
}

func TestChronosContextWithoutAPlanOrFiles(t *testing.T) {
	c := newLifecycleCLI(t, map[string]string{"DEN-377": "To Do"})
	out, err := c.run("context", "DEN-377")
	if err != nil || strings.Contains(out, "Plan v") || strings.Contains(out, "Files named") {
		t.Errorf("err = %v\n%s", err, out)
	}
}

func TestReadTextArgAndFirstOr(t *testing.T) {
	if got, err := readTextArg("text", "ignored.md", nil); err != nil || got != "text" {
		t.Errorf("text wins: %q, %v", got, err)
	}
	if got, err := readTextArg("", "", nil); err != nil || got != "" {
		t.Errorf("nothing given: %q, %v", got, err)
	}
	if got, err := readTextArg("", "-", strings.NewReader("from stdin")); err != nil || got != "from stdin" {
		t.Errorf("stdin: %q, %v", got, err)
	}
	if got, err := readTextArg("", writeFile(t, "from file"), nil); err != nil || got != "from file" {
		t.Errorf("file: %q, %v", got, err)
	}
	if firstOr(" ", "x") != "x" || firstOr("y", "x") != "y" {
		t.Error("firstOr")
	}
}

func TestChronosPlanPushReportsAPlaneFailure(t *testing.T) {
	c := newLifecycleCLI(t, map[string]string{"DEN-377": "In Progress"})
	c.board.Fail("POST", 0)
	if _, err := c.run("plan", "push", "DEN-377", "--file", writeFile(t, "- [ ] x")); err == nil || !strings.Contains(err.Error(), "saved locally") {
		t.Errorf("err = %v", err)
	}
}

func TestWorkNewClaimsTheTicketForTheNewCopy(t *testing.T) {
	c := newLifecycleCLI(t, map[string]string{"DEN-377": "To Do", "DEN-378": "Done"})
	var buf bytes.Buffer
	claimInCopy(context.Background(), "DEN-377", c.repo, &buf)
	if !strings.Contains(buf.String(), "[✓] Claimed DEN-377 for this copy (session s-") || c.board.State("DEN-377") != "In Progress" {
		t.Errorf("success output:\n%s", buf.String())
	}

	buf.Reset()
	claimInCopy(context.Background(), "DEN-378", c.repo, &buf)
	if !strings.Contains(buf.String(), "[!] DEN-378 is not claimed") || !strings.Contains(buf.String(), "dendrite chronos claim DEN-378 --worktree "+c.repo) {
		t.Errorf("a refused claim warns and says how to claim by hand:\n%s", buf.String())
	}

	buf.Reset()
	newPlaneClient = func() (*plane.Client, error) { return nil, errors.New("no Plane configured") }
	claimInCopy(context.Background(), "DEN-377", c.repo, &buf)
	if !strings.Contains(buf.String(), "[!] DEN-377 is not claimed: no Plane configured") {
		t.Errorf("no Plane:\n%s", buf.String())
	}
}
