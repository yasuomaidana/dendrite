package ticketlock

import (
	"Dendrite/pkg/branchpolicy"
	"Dendrite/pkg/planesvc"
	"Dendrite/pkg/quality/gate"
	"Dendrite/pkg/tracker"
	"Dendrite/pkg/tracker/plane"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
)

// SavePlan is the plan-saved hook (after ExitPlanMode): the plan goes to the ticket the working copy has
// claimed. With no claim there is no ticket to put it on, so it is kept locally under the session and the
// message tells the agent to claim one; nothing is lost. board is only opened when there is a ticket.
func SavePlan(ctx context.Context, l *Lifecycle, board func() (Board, error), in HookInput) (string, error) {
	plan := strings.TrimSpace(in.ToolInput.Plan)
	if plan == "" {
		return "", nil
	}
	claim, claimed := l.Store.ClaimOfDir(in.Cwd)
	if !claimed {
		key := hookSessionKey(in)
		if err := l.Plans.Save(key, newPlan(plan, 1)); err != nil {
			return "", err
		}
		return fmt.Sprintf("plan kept locally (%s): no ticket is claimed here. Claim one with `dendrite chronos claim <ticket>` and push it with `dendrite chronos plan push <ticket> --file <plan>`", key), nil
	}
	b, err := board()
	if err != nil {
		return "", fmt.Errorf("plan not stored on %s: %w", claim.Ticket, err)
	}
	l.Board = b
	p, err := l.UploadPlan(ctx, claim.Ticket, plan)
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("plan v%d stored on %s (%d step(s) left)", p.Version, claim.Ticket, p.PendingSteps), nil
}

// AutoHandoff is the session-end hook: a session that ends while its claimed ticket is still In Progress
// leaves the handoff it would have written, then blocks the ticket with the needs-input label so the next
// session sees where it stopped. A session that finished the ticket (To Verify, Blocked, ...) leaves nothing.
func AutoHandoff(ctx context.Context, l *Lifecycle, board func() (Board, error), in HookInput) (string, error) {
	claim, claimed := l.Store.ClaimOfDir(in.Cwd)
	if !claimed {
		return "", nil
	}
	b, err := board()
	if err != nil {
		return "", fmt.Errorf("handoff of %s not written: %w", claim.Ticket, err)
	}
	l.Board = b
	st, _, err := b.Status(ctx, claim.Ticket)
	if err != nil {
		return "", fmt.Errorf("handoff of %s not written: %w", claim.Ticket, err)
	}
	if st != tracker.StatusInProgress {
		return "", nil
	}

	h := Handoff{
		Completed:  l.completedWork(ctx, claim),
		Challenges: "The session ended before the ticket was finished; this handoff was written automatically.",
		Missing:    l.pendingWork(ctx, claim),
	}
	if _, err := l.Transition(ctx, claim.Ticket, "blocked", h); err != nil {
		return "", err
	}
	if _, _, err := b.Svc.PatchLabels(ctx, b.Client, claim.Ticket, []string{"needs-input"}, nil); err != nil {
		return "", fmt.Errorf("%s is blocked but not labelled needs-input: %w", claim.Ticket, err)
	}
	return fmt.Sprintf("%s was still In Progress: blocked with an automatic handoff (needs-input)", claim.Ticket), nil
}

// unmergedCommits describes the commits of branch that are not yet on the integration branch, or "" when there
// are none (or git cannot say).
func unmergedCommits(dir, branch string) string {
	for _, base := range branchpolicy.For(dir).Bases() {
		out, err := gitBytes(dir, "log", "--oneline", "--max-count=15", base+"..HEAD")
		if err != nil {
			continue
		}
		if s := strings.TrimSpace(string(out)); s != "" {
			return "Commits on " + branch + " not on " + base + ":\n" + s
		}
		return ""
	}
	return ""
}

// completedWork lists what the branch holds: commits not yet on the integration branch and the uncommitted changes.
func (l *Lifecycle) completedWork(ctx context.Context, c Claim) string {
	dir := firstNonEmpty(c.Worktree, l.Dir)
	var parts []string
	if s := unmergedCommits(dir, c.Branch); s != "" {
		parts = append(parts, s)
	}
	if len(parts) == 0 { // the branch is merged: its commits are the ones that name the ticket
		if _, lines := gate.TicketCommits(dir, c.Ticket); len(lines) > 0 {
			parts = append(parts, "Commits for "+c.Ticket+" (already merged):\n"+strings.Join(lines[max(0, len(lines)-15):], "\n"))
		}
	}
	if out, err := gitBytes(dir, "diff", "--stat", "HEAD"); err == nil {
		if s := strings.TrimSpace(string(out)); s != "" {
			parts = append(parts, "Uncommitted changes:\n"+s)
		}
	}
	if p, err := l.GetPlan(ctx, c.Ticket); err == nil {
		if done := checkedSteps(p.Markdown); done != "" {
			parts = append(parts, "Plan steps done:\n"+done)
		}
	}
	if len(parts) == 0 {
		return "Nothing committed or changed on the branch yet."
	}
	return strings.Join(parts, "\n\n")
}

// pendingWork lists the unchecked plan steps, or points at the acceptance criteria when there is no plan.
func (l *Lifecycle) pendingWork(ctx context.Context, c Claim) string {
	if p, err := l.GetPlan(ctx, c.Ticket); err == nil && p.PendingSteps > 0 {
		return "Plan steps left:\n" + uncheckedSteps(p.Markdown)
	}
	return "No plan was stored: compare the work with the ticket's acceptance criteria (`dendrite chronos context " + c.Ticket + "`)."
}

func checkedSteps(markdown string) string   { return stepsMatching(markdown, true) }
func uncheckedSteps(markdown string) string { return stepsMatching(markdown, false) }

var stepLine = regexp.MustCompile(`^[-*]\s+\[([ xX])\]`)

// stepsMatching lists the checked (done) or unchecked plan steps, one trimmed line each.
func stepsMatching(markdown string, done bool) string {
	var out []string
	for _, line := range strings.Split(markdown, "\n") {
		t := strings.TrimSpace(line)
		if m := stepLine.FindStringSubmatch(t); m != nil && (m[1] != " ") == done {
			out = append(out, t)
		}
	}
	return strings.Join(out, "\n")
}

// HookInput is the part of a Claude Code hook event Dendrite reads.
type HookInput struct {
	SessionID string `json:"session_id"`
	Cwd       string `json:"cwd"`
	ToolName  string `json:"tool_name"`
	ToolInput struct {
		FilePath     string `json:"file_path"`
		NotebookPath string `json:"notebook_path"`
		Plan         string `json:"plan"`
		// Questions are what an AskUserQuestion call put to the user.
		Questions []struct {
			Question string `json:"question"`
			Options  []struct {
				Label string `json:"label"`
			} `json:"options"`
		} `json:"questions"`
	} `json:"tool_input"`
	// ToolResponse is what the tool answered: for AskUserQuestion an object with the user's answers.
	ToolResponse json.RawMessage `json:"tool_response"`
}

// ParseHookInput reads one hook event from r; ok is false for empty or malformed input, which a hook must
// treat as "nothing to judge" rather than as a reason to stop the agent.
func ParseHookInput(r io.Reader) (HookInput, bool) {
	data, err := io.ReadAll(io.LimitReader(r, 8<<20))
	var in HookInput
	if err != nil || json.Unmarshal(data, &in) != nil {
		return HookInput{}, false
	}
	return in, in.ToolName != "" || in.Cwd != "" || in.SessionID != ""
}

// Target is the file a write tool is about to change, made absolute against the session's directory.
func (in HookInput) Target() string {
	t := in.ToolInput.FilePath
	if t == "" {
		t = in.ToolInput.NotebookPath
	}
	if t == "" {
		return ""
	}
	if !filepath.IsAbs(t) {
		t = filepath.Join(in.Cwd, t)
	}
	return filepath.Clean(t)
}

// CheckEdit is the pre-edit hook: nil lets the write through, an *ErrLocked stops it. An event with no
// target (a tool that writes nothing) is let through.
func CheckEdit(ctx context.Context, d Deps, in HookInput) error {
	target := in.Target()
	if target == "" {
		return nil
	}
	return Require(ctx, d, target)
}

// CheckStaged is the git pre-commit hook: the same guard, applied to the repository at dir when a path outside
// Dendrite's own state is staged. The guard decides on the repository, its branch and its claim, never on the
// file, so it runs once however many files are staged. It fails closed: staged files that git cannot list are
// refused, because the commit cannot be judged.
func CheckStaged(ctx context.Context, d Deps, dir string) error {
	top, err := d.RepoTop(dir)
	if err != nil || top == "" {
		return nil
	}
	out, err := gitBytes(top, "diff", "--cached", "--name-only", "-z", "--diff-filter=ACMRD")
	if err != nil {
		return locked("git cannot list the staged files (%v), so the commit cannot be judged", err)
	}
	for _, rel := range bytes.Split(out, []byte{0}) {
		if len(rel) > 0 && !within(filepath.Join(top, string(rel)), filepath.Join(top, ".dendrite")) {
			return Require(ctx, d, top)
		}
	}
	return nil
}

func gitCommand(dir string, args ...string) *exec.Cmd {
	return exec.Command("git", append([]string{"-C", dir}, args...)...)
}

func gitBytes(dir string, args ...string) ([]byte, error) {
	return gitCommand(dir, args...).Output()
}

// planeClientFor opens the Plane client of the repository at top: the environment's credentials with the
// project named by the repository's .dendrite/plane.json. A seam for tests.
var planeClientFor = func(top string) (*plane.Client, error) {
	cfg, err := plane.ConfigFromEnv()
	if err != nil {
		return nil, err
	}
	if link, _, lerr := plane.LinkForDir(top); lerr == nil {
		if os.Getenv("PLANE_PROJECT_ID") == "" {
			cfg.ProjectID = link.ProjectID
		}
		if link.Workspace != "" && os.Getenv("PLANE_WORKSPACE_SLUG") == "" {
			cfg.WorkspaceSlug = link.Workspace
		}
	}
	return plane.NewClient(cfg, nil)
}

// BoardFor is the Plane board of the repository at top.
func BoardFor(top string) (Board, error) {
	client, err := planeClientFor(top)
	if err != nil {
		return Board{}, err
	}
	return Board{Svc: planesvc.Default(), Client: client}, nil
}

// PlaneStatusFor asks Plane for the lifecycle state of a ticket of the repository at top; it is what the
// hooks give Deps.PlaneStatus.
func PlaneStatusFor(ctx context.Context, top, ticket string) (tracker.Status, error) {
	board, err := BoardFor(top)
	if err != nil {
		return "", err
	}
	st, _, err := board.Status(ctx, ticket)
	return st, err
}

// HookDeps are the real dependencies of the hooks.
func HookDeps() Deps { return DefaultDeps(PlaneStatusFor) }

// hookSessionKey names a session's plans that belong to no ticket, within the repository of its working directory.
func hookSessionKey(in HookInput) string {
	return unclaimedKey(repoScope(in.Cwd), strings.TrimSpace(firstNonEmpty(in.SessionID, "session")))
}
