package mcpserver

import (
	"context"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"Dendrite/pkg/store"
	"Dendrite/pkg/story/fork"
)

// timedHarness measures a variant by returning the same fixed numbers for each.
type timedHarness struct{}

func (timedHarness) BenchmarkVariant(_ context.Context, v *fork.Variant, _ fork.BenchmarkOptions) (*fork.VariantMetrics, error) {
	return &fork.VariantMetrics{Variant: v.Name, TTFT: time.Millisecond, TPS: 10, MemoryMB: 5}, nil
}

// storyRepo points the story tools at a temporary git repository and its own worktree directory.
func storyRepo(t *testing.T) {
	t.Helper()
	repo, stories := t.TempDir(), filepath.Join(t.TempDir(), "stories")
	for _, args := range [][]string{
		{"init", "-q"},
		{"-c", "user.name=t", "-c", "user.email=t@t", "commit", "--allow-empty", "-m", "base"},
	} {
		cmd := exec.Command("git", args...)
		cmd.Dir = repo
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, out)
		}
	}
	old := defaultForkController
	defaultForkController = fork.NewForkController(repo, stories, fork.NewOSGitExecutor(), timedHarness{}, store.NewStore(filepath.Join(repo, "store"), filepath.Join(repo, "adr")))
	t.Cleanup(func() { defaultForkController = old })
	t.Setenv("GIT_AUTHOR_NAME", "t")
	t.Setenv("GIT_AUTHOR_EMAIL", "t@t")
	t.Setenv("GIT_COMMITTER_NAME", "t")
	t.Setenv("GIT_COMMITTER_EMAIL", "t@t")
}

func TestStoryLifecycleThroughTheTools(t *testing.T) {
	storyRepo(t)
	srv := NewServer(ServerConfig{})
	RegisterDefaultTools(srv, Dependencies{})

	text, isErr := callTool(t, srv, "dendrite_story_fork", `{"story_id":"s1","variants":["v1","","v2",3]}`)
	if isErr || !strings.Contains(text, "v1") || !strings.Contains(text, "v2") {
		t.Fatalf("fork: %s (error %v)", text, isErr)
	}
	if text, isErr = callTool(t, srv, "dendrite_story_benchmark", `{"story_id":"s1","iterations":5,"virtual_users":2}`); isErr || !strings.Contains(text, "recommended_winner") {
		t.Fatalf("benchmark: %s (error %v)", text, isErr)
	}
	if text, isErr = callTool(t, srv, "dendrite_story_resolve", `{"story_id":"s1","winner":"v1"}`); isErr {
		t.Fatalf("resolve: %s", text)
	}
}

func TestStoryToolsReportWhatTheControllerRefuses(t *testing.T) {
	storyRepo(t)
	srv := NewServer(ServerConfig{})
	RegisterDefaultTools(srv, Dependencies{})
	for _, c := range []struct{ tool, args, want string }{
		{"dendrite_story_fork", `{"story_id":"s","variants":["only-one"]}`, "at least 2"},
		{"dendrite_story_fork", `{"story_id":"s","variants":["a","a"]}`, "story fork failed"},
		{"dendrite_story_benchmark", `{"story_id":"never-forked"}`, "story benchmark failed"},
		{"dendrite_story_resolve", `{"story_id":"never-forked","winner":"v1"}`, "story resolve failed"},
	} {
		if text, isErr := callTool(t, srv, c.tool, c.args); !isErr || !strings.Contains(text, c.want) {
			t.Errorf("%s %s: %q (error %v), want %q", c.tool, c.args, text, isErr, c.want)
		}
	}
}
