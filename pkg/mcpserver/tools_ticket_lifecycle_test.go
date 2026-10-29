package mcpserver

import (
	"context"
	"encoding/json"
	"os/exec"
	"strings"
	"testing"

	"Dendrite/pkg/planesvc/planetest"
	"Dendrite/pkg/tracker/plane"
)

var lifecycleTools = []string{
	"dendrite_ticket_claim", "dendrite_ticket_comment", "dendrite_ticket_upload_plan",
	"dendrite_ticket_get_plan", "dendrite_ticket_transition", "dendrite_query_context",
}

func TestLifecycleToolsAreInTheCoreProfile(t *testing.T) {
	src := newDefaultServerForProfileTest()
	core := ApplyProfile(src, ProfileCore)
	listed := map[string]bool{}
	for _, tool := range core.ListTools() {
		listed[tool.Name] = true
	}
	for _, name := range append(lifecycleTools, "dendrite_ticket_smart_select") {
		if !listed[name] {
			t.Errorf("core profile lacks %s", name)
		}
		if _, ok := core.Handler(name); !ok {
			t.Errorf("core tool %s lost its handler", name)
		}
	}
}

// lifecycleRig is a server with the lifecycle tools, a Plane board and a working copy on branch agent/den-377.
type lifecycleRig struct {
	t     *testing.T
	s     *Server
	board *planetest.Board
	repo  string
}

func newLifecycleRig(t *testing.T, tickets map[string]string) *lifecycleRig {
	t.Helper()
	t.Setenv("DENDRITE_HOME", t.TempDir())
	t.Setenv("DENDRITE_STATE_DIR", t.TempDir())
	t.Setenv("DENDRITE_AGENT", "claude")
	t.Setenv("DENDRITE_MODEL_TIER", "7")
	board := planetest.NewBoard(t, tickets)
	client, err := plane.NewClient(plane.Config{BaseURL: board.URL, WorkspaceSlug: planetest.BoardWorkspace, ProjectID: planetest.BoardProject, DirectToken: "k"}, board.Client())
	if err != nil {
		t.Fatal(err)
	}
	repo := t.TempDir()
	for _, args := range [][]string{{"init", "-q"}, {"checkout", "-q", "-b", "agent/den-377"}} {
		if out, err := exec.Command("git", append([]string{"-C", repo}, args...)...).CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, out)
		}
	}
	s := NewServer(ServerConfig{})
	registerTicketLifecycleTools(s, Dependencies{PlaneClient: client})
	return &lifecycleRig{t: t, s: s, board: board, repo: repo}
}

// call runs a tool in the rig's working copy and decodes its JSON answer.
func (r *lifecycleRig) call(name string, args map[string]any) (map[string]any, error) {
	r.t.Helper()
	if _, has := args["worktree"]; !has {
		args["worktree"] = r.repo
	}
	h, ok := r.s.Handler(name)
	if !ok {
		r.t.Fatalf("%s is not registered", name)
	}
	res, err := h(context.Background(), args)
	if err != nil {
		return nil, err
	}
	var out map[string]any
	if err := json.Unmarshal([]byte(res.Content[0].Text), &out); err != nil {
		r.t.Fatalf("%s answered %q: %v", name, res.Content[0].Text, err)
	}
	return out, nil
}

func TestLifecycleToolsRunATicketFromClaimToHandoff(t *testing.T) {
	r := newLifecycleRig(t, map[string]string{"DEN-377": "To Do"})
	r.board.SetDescription("DEN-377", "<p>edit pkg/ticketlock/lifecycle.go</p><ul><li>[AC-1] tools listed</li></ul>")

	claim, err := r.call("dendrite_ticket_claim", map[string]any{"ticket": "DEN-377", "agent": "claude", "model_tier": "9"})
	if err != nil {
		t.Fatal(err)
	}
	if claim["ticket"] != "DEN-377" || claim["state"] != "In Progress" || claim["branch"] != "agent/den-377" {
		t.Errorf("claim = %v", claim)
	}
	if got := r.board.LabelNames("DEN-377"); strings.Join(got, ",") != "agent:claude,tier:9" {
		t.Errorf("labels = %v", got)
	}

	if _, err := r.call("dendrite_ticket_comment", map[string]any{"ticket": "DEN-377", "kind": "progress", "body": "lock done"}); err != nil {
		t.Fatal(err)
	}
	if _, err := r.call("dendrite_ticket_comment", map[string]any{"ticket": "DEN-377", "kind": "shout", "body": "x"}); err == nil {
		t.Error("an unknown comment kind is refused")
	}

	plan, err := r.call("dendrite_ticket_upload_plan", map[string]any{"ticket": "DEN-377", "markdown": "- [x] lock\n- [ ] hooks"})
	if err != nil {
		t.Fatal(err)
	}
	if plan["version"] != float64(1) || plan["pending_steps"] != float64(1) || plan["next_step"] != "hooks" {
		t.Errorf("upload = %v", plan)
	}
	got, err := r.call("dendrite_ticket_get_plan", map[string]any{"ticket": "DEN-377"})
	if err != nil || got["markdown"] != "- [x] lock\n- [ ] hooks" || got["source"] != "plane" {
		t.Errorf("get_plan = %v, err = %v", got, err)
	}

	ctx, err := r.call("dendrite_query_context", map[string]any{"ticket": "DEN-377"})
	if err != nil {
		t.Fatal(err)
	}
	if acs, _ := ctx["acceptance_criteria"].([]any); len(acs) != 1 || acs[0] != "[AC-1] tools listed" {
		t.Errorf("context = %v", ctx)
	}

	// AC-2: blocked without the three sections is refused and the ticket stays put.
	if _, err := r.call("dendrite_ticket_transition", map[string]any{"ticket": "DEN-377", "outcome": "blocked"}); err == nil || !strings.Contains(err.Error(), "needs a handoff") {
		t.Errorf("blocked without a handoff: %v", err)
	}
	if _, err := r.call("dendrite_ticket_transition", map[string]any{"ticket": "DEN-377", "outcome": "blocked", "handoff": map[string]any{"completed": "a", "challenges": "b"}}); err == nil || !strings.Contains(err.Error(), "missing") {
		t.Errorf("blocked with two sections: %v", err)
	}
	if r.board.State("DEN-377") != "In Progress" {
		t.Errorf("state = %s", r.board.State("DEN-377"))
	}
	res, err := r.call("dendrite_ticket_transition", map[string]any{"ticket": "DEN-377", "outcome": "blocked",
		"handoff": map[string]any{"completed": "lock", "challenges": "none", "missing": "hooks"}})
	if err != nil || res["state"] != "Blocked" {
		t.Errorf("blocked with a handoff: %v, %v", res, err)
	}
	if _, err := r.call("dendrite_ticket_transition", map[string]any{"ticket": "DEN-377", "outcome": "done"}); err == nil || !strings.Contains(err.Error(), "chronos finish") {
		t.Errorf("done must point to finish: %v", err)
	}
}

func TestLifecycleToolsValidateTheirArguments(t *testing.T) {
	r := newLifecycleRig(t, map[string]string{"DEN-377": "To Do"})
	for _, name := range lifecycleTools {
		if _, err := r.call(name, map[string]any{}); err == nil || !strings.Contains(err.Error(), "ticket is required") {
			t.Errorf("%s without a ticket: %v", name, err)
		}
	}
	if _, err := r.call("dendrite_ticket_claim", map[string]any{"ticket": "DEN-999"}); err == nil {
		t.Error("an unknown ticket is an error")
	}
	if _, err := r.call("dendrite_ticket_get_plan", map[string]any{"ticket": "DEN-377"}); err == nil || !strings.Contains(err.Error(), "no plan") {
		t.Errorf("a ticket without a plan: %v", err)
	}
	if _, err := r.call("dendrite_ticket_claim", map[string]any{"ticket": "DEN-377", "steal": true}); err != nil {
		t.Errorf("steal=true on a free ticket: %v", err)
	}
}

func TestLifecycleToolsUseTheServersDirectoryByDefault(t *testing.T) {
	r := newLifecycleRig(t, map[string]string{"DEN-377": "To Do"})
	h, _ := r.s.Handler("dendrite_ticket_get_plan")
	if _, err := h(context.Background(), map[string]any{"ticket": "DEN-377", "worktree": "  "}); err == nil || !strings.Contains(err.Error(), "no plan") {
		t.Errorf("err = %v: a blank worktree falls back to the current directory", err)
	}
}

func TestLifecycleToolsNeedAPlaneClient(t *testing.T) {
	t.Setenv("PLANE_API_KEY", "")
	t.Setenv("PLANE_BASE_URL", "")
	s := NewServer(ServerConfig{})
	registerTicketLifecycleTools(s, Dependencies{})
	h, _ := s.Handler("dendrite_ticket_get_plan")
	if _, err := h(context.Background(), map[string]any{"ticket": "DEN-1"}); err == nil || !strings.Contains(err.Error(), "plane client error") {
		t.Errorf("err = %v", err)
	}
}
