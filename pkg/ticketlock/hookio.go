package ticketlock

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"Dendrite/pkg/planesvc"
	"Dendrite/pkg/tracker"
	"Dendrite/pkg/tracker/plane"
)

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

// CheckStaged is the git pre-commit hook: the same guard, applied to every staged path of the repository
// at dir. Git reports paths relative to the repository's top level.
func CheckStaged(ctx context.Context, d Deps, dir string) error {
	top, err := d.RepoTop(dir)
	if err != nil || top == "" {
		return nil
	}
	out, err := gitBytes(top, "diff", "--cached", "--name-only", "-z", "--diff-filter=ACMRD")
	if err != nil {
		return nil
	}
	for _, rel := range bytes.Split(out, []byte{0}) {
		if len(rel) == 0 {
			continue
		}
		if err := Require(ctx, d, filepath.Join(top, string(rel))); err != nil {
			return err
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

// hookSessionKey names a session's plans that belong to no ticket.
func hookSessionKey(in HookInput) string {
	return "unclaimed/" + strings.TrimSpace(firstNonEmpty(in.SessionID, "session"))
}
