package mcpserver

import (
	"Dendrite/pkg/testkit"
	"context"
	"os"
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
func storyRepo(t *testing.T) string {
	t.Helper()
	repo, stories := t.TempDir(), filepath.Join(t.TempDir(), "stories")
	testkit.Git(t, repo, "init", "-q")
	testkit.Git(t, repo, "-c", "user.name=t", "-c", "user.email=t@t", "commit", "--allow-empty", "-m", "base")
	old := defaultForkController
	defaultForkController = fork.NewForkController(repo, stories, fork.NewOSGitExecutor(), timedHarness{}, store.NewStore(filepath.Join(repo, "store"), filepath.Join(repo, "adr")))
	t.Cleanup(func() { defaultForkController = old })
	t.Setenv("GIT_AUTHOR_NAME", "t")
	t.Setenv("GIT_AUTHOR_EMAIL", "t@t")
	t.Setenv("GIT_COMMITTER_NAME", "t")
	t.Setenv("GIT_COMMITTER_EMAIL", "t@t")
	return stories
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
	runToolFailures(t, srv, []toolFailure{
		{"fork with one variant", "dendrite_story_fork", `{"story_id":"s","variants":["only-one"]}`, nil, "at least 2"},
		{"fork with a repeated variant", "dendrite_story_fork", `{"story_id":"s","variants":["a","a"]}`, nil, "story fork failed"},
		{"benchmark of an unknown story", "dendrite_story_benchmark", `{"story_id":"never-forked"}`, nil, "story benchmark failed"},
		{"resolve of an unknown story", "dendrite_story_resolve", `{"story_id":"never-forked","winner":"v1"}`, nil, "story resolve failed"},
	})
}

func TestStoryAbortReportsADirectoryItCannotRemove(t *testing.T) {
	stories := storyRepo(t)
	srv := NewServer(ServerConfig{})
	RegisterDefaultTools(srv, Dependencies{})
	// A story with a manifest is aborted by removing its directory.
	if _, isErr := callTool(t, srv, "dendrite_story_fork", `{"story_id":"stuck2","variants":["a","b"]}`); isErr {
		t.Fatal("fork failed")
	}
	if err := os.Chmod(filepath.Join(stories, "stuck2"), 0o500); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(filepath.Join(stories, "stuck2"), 0o755) })
	if text, isErr := callTool(t, srv, "dendrite_story_abort", `{"story_id":"stuck2"}`); !isErr || !strings.Contains(text, "story abort failed") {
		t.Fatalf("text = %q (error %v)", text, isErr)
	}
}
