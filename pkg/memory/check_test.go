package memory_test

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"Dendrite/pkg/engram"
	"Dendrite/pkg/memory"
	"Dendrite/pkg/testkit"
)

var now = time.Date(2026, 11, 22, 12, 0, 0, 0, time.UTC)

// cannedStore is an Engram that answers from fields; a non-nil error makes that call fail.
type cannedStore struct {
	healthErr, obsErr, searchErr, startErr error
	observations                           []engram.Observation
	hits                                   map[string][]string // query -> titles; a missing query finds nothing
	start                                  int
}

func (f cannedStore) Healthy(context.Context) error { return f.healthErr }
func (f cannedStore) Observations(context.Context) ([]engram.Observation, error) {
	return f.observations, f.obsErr
}
func (f cannedStore) Search(_ context.Context, q string) ([]string, error) {
	return f.hits[q], f.searchErr
}
func (f cannedStore) StartBytes(context.Context) (int, error) { return f.start, f.startErr }

// memoryFolder writes Claude memories into a temp folder: name -> frontmatter description + body, all linked
// from MEMORY.md except those in unlisted. It returns the folder and a repo with the paths in repoFiles.
func memoryFolder(t *testing.T, mem map[string][2]string, unlisted ...string) (dir, repo string) {
	t.Helper()
	dir, repo = t.TempDir(), t.TempDir()
	var index strings.Builder
	for name, db := range mem {
		testkit.WriteFile(t, filepath.Join(dir, name+".md"), "---\nname: "+name+"\ndescription: "+db[0]+"\n---\n"+db[1]+"\n", 0o644)
		if !contains(unlisted, name) {
			index.WriteString("- [" + name + "](" + name + ".md) — hook\n")
		}
	}
	testkit.WriteFile(t, filepath.Join(dir, "MEMORY.md"), index.String(), 0o644)
	testkit.Check(t, os.MkdirAll(filepath.Join(repo, "pkg", "real"), 0o755))
	return dir, repo
}

func contains(list []string, s string) bool {
	for _, x := range list {
		if x == s {
			return true
		}
	}
	return false
}

func fresh(t *testing.T, dir string) {
	t.Helper()
	files, _ := filepath.Glob(filepath.Join(dir, "*.md"))
	for _, f := range files {
		testkit.Check(t, os.Chtimes(f, now.Add(-time.Hour), now.Add(-time.Hour)))
	}
}

func TestCheckScoresBothStores(t *testing.T) {
	dir, repo := memoryFolder(t, map[string][2]string{
		"alpha": {"first fact about alpha things", "see pkg/real and pkg/gone"},
		"beta":  {"", "no paths here"},
	}, "beta")
	fresh(t, dir)
	store := cannedStore{
		observations: []engram.Observation{
			{Title: "alpha", Content: "pkg/real", CreatedAt: "2026-11-22 11:00:00"}, // mirrored: not new work
			{Title: "an-engram-only-decision", Content: "x", CreatedAt: "2026-11-22 10:00:00"},
			{Title: "old", Content: "x", CreatedAt: "2026-01-01 10:00:00"},
		},
		hits:  map[string][]string{"first fact about alpha things": {"alpha"}, "beta": {"other"}},
		start: 900,
	}
	r, err := memory.Check(context.Background(), memory.CheckInput{ClaudeDir: dir, Repo: repo, Now: now, Store: store})
	testkit.Check(t, err)

	testkit.Equal(t, "claude memories", r.Claude.Memories, 2)
	testkit.Equal(t, "claude new", r.Claude.New7d, 2)
	testkit.Equal(t, "claude recall (beta is not in the index)", r.Claude.Recall, 0.5)
	testkit.Equal(t, "claude stale (1 of 2 paths)", r.Claude.Stale, 0.5)
	if r.Claude.StartBytes == 0 {
		t.Error("the index size is the cost of Claude's memory")
	}
	testkit.Equal(t, "engram memories", r.Engram.Memories, 3)
	testkit.Equal(t, "engram new (mirrored and old excluded)", r.Engram.New7d, 1)
	testkit.Equal(t, "engram recall (alpha found, beta not)", r.Engram.Recall, 0.5)
	testkit.Equal(t, "engram stale", r.Engram.Stale, 0.0)
	testkit.Equal(t, "engram start bytes", r.Engram.StartBytes, 900)
	testkit.Equal(t, "healthy", r.EngramHealthy, true)
	if !strings.Contains(memory.Summary(r), "engram ") || !strings.Contains(memory.Summary(r), "vs claude") {
		t.Errorf("summary = %q", memory.Summary(r))
	}
}

func TestCandidateFollowsTheScoreGap(t *testing.T) {
	dir, repo := memoryFolder(t, map[string][2]string{"alpha": {"alpha fact", "no paths"}})
	cases := map[string]struct {
		hits map[string][]string
		want memory.Mode
	}{
		"engram finds nothing, so claude alone": {nil, memory.ClaudeOnly},
		"engram finds it, scores close, both":   {map[string][]string{"alpha fact": {"alpha"}}, memory.Both},
	}
	for name, c := range cases {
		r, err := memory.Check(context.Background(), memory.CheckInput{ClaudeDir: dir, Repo: repo, Now: now, Store: cannedStore{hits: c.hits}})
		testkit.Check(t, err)
		testkit.Equal(t, name, r.Candidate, c.want)
	}

	// Claude's index lists nothing and Engram recalls everything: Engram alone.
	dir2, repo2 := memoryFolder(t, map[string][2]string{"alpha": {"alpha fact", "no paths"}}, "alpha")
	r, _ := memory.Check(context.Background(), memory.CheckInput{ClaudeDir: dir2, Repo: repo2, Now: now, Store: cannedStore{hits: map[string][]string{"alpha fact": {"alpha"}}}})
	testkit.Equal(t, "engram clearly better", r.Candidate, memory.EngramOnly)
}

func TestAnUnusableEngramPointsToClaudeAlone(t *testing.T) {
	dir, repo := memoryFolder(t, map[string][2]string{"alpha": {"alpha fact", "x"}})
	boom := errors.New("boom")
	for name, c := range map[string]struct {
		store memory.Store
		note  string
	}{
		"not installed": {nil, "not installed"},
		"unhealthy":     {cannedStore{healthErr: boom}, "unhealthy: boom"},
		"unreadable":    {cannedStore{obsErr: boom}, "cannot be read: boom"},
	} {
		r, err := memory.Check(context.Background(), memory.CheckInput{ClaudeDir: dir, Repo: repo, Now: now, Store: c.store})
		testkit.Check(t, err)
		if r.EngramHealthy || r.Candidate != memory.ClaudeOnly || !strings.Contains(r.Note, c.note) {
			t.Errorf("%s: %+v", name, r)
		}
	}
}

func TestASearchOrSnapshotFailureCostsRecallNotTheWholeCheck(t *testing.T) {
	dir, repo := memoryFolder(t, map[string][2]string{"alpha": {"alpha fact", "x"}})
	boom := errors.New("boom")
	r, err := memory.Check(context.Background(), memory.CheckInput{ClaudeDir: dir, Repo: repo, Now: now, Store: cannedStore{searchErr: boom, startErr: boom}})
	testkit.Check(t, err)
	if !r.EngramHealthy || r.Engram.Recall != 0 || r.Engram.StartBytes != 0 {
		t.Fatalf("report = %+v", r)
	}
}

func TestCheckReportsAMissingMemoryFolder(t *testing.T) {
	if _, err := memory.Check(context.Background(), memory.CheckInput{ClaudeDir: filepath.Join(t.TempDir(), "absent"), Now: now}); err == nil {
		t.Fatal("expected an error")
	}
}

func TestAMemoryWithoutADescriptionIsSearchedByItsName(t *testing.T) {
	dir, repo := memoryFolder(t, map[string][2]string{"my-fact": {"", "x"}})
	store := cannedStore{hits: map[string][]string{"my fact": {"my-fact"}}}
	r, _ := memory.Check(context.Background(), memory.CheckInput{ClaudeDir: dir, Repo: repo, Now: now, Store: store})
	testkit.Equal(t, "recall", r.Engram.Recall, 1.0)
}

func TestEngramStoreReadsThroughTheBinary(t *testing.T) {
	testkit.StubEngram(t, `case "$1" in
  doctor) echo '{"status":"ok","summary":{"total":10,"ok":10,"warnings":0,"blocked":0,"errors":0}}' ;;
  export) printf '{"observations":[{"title":"x","content":"c","created_at":"2026-11-22 10:00:00"}]}' > "$2" ;;
  search) echo "[1] #1 (note) — fact-a" ;;
  context) echo "## Memory"; echo "- thing" ;;
esac`)
	bin, err := engram.Locate()
	testkit.Check(t, err)
	s := memory.EngramStore{Bin: bin, Dir: t.TempDir()}
	ctx := context.Background()

	testkit.Check(t, s.Healthy(ctx))
	obs, err := s.Observations(ctx)
	testkit.Check(t, err)
	testkit.Equal(t, "observations", len(obs), 1)
	titles, err := s.Search(ctx, "q")
	testkit.Check(t, err)
	testkit.Equal(t, "titles", strings.Join(titles, ","), "fact-a")
	n, err := s.StartBytes(ctx)
	testkit.Check(t, err)
	if n == 0 {
		t.Fatal("the snapshot has content")
	}
}

func TestEngramStoreIsUnhealthyWhenDoctorFailsOrReportsErrors(t *testing.T) {
	testkit.StubEngram(t, `echo '{"status":"error","summary":{"total":10,"ok":8,"warnings":0,"blocked":1,"errors":1}}'; exit 1`)
	bin, _ := engram.Locate()
	if err := (memory.EngramStore{Bin: bin}).Healthy(context.Background()); err == nil || !strings.Contains(err.Error(), "1 blocked and 1 failed") {
		t.Fatalf("err = %v", err)
	}
	testkit.StubEngram(t, `exit 1`)
	bin, _ = engram.Locate()
	if err := (memory.EngramStore{Bin: bin}).Healthy(context.Background()); err == nil {
		t.Fatal("an unreadable doctor report must be unhealthy")
	}
}
