package synapsesearch

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"Dendrite/pkg/projectid"
	"Dendrite/pkg/rag"
	"Dendrite/pkg/synapse"
)

// members makes real project directories (with project.id) and the synapse that links them.
func members(t *testing.T, aliases ...string) (*synapse.Synapse, map[string]string) {
	t.Helper()
	t.Setenv("DENDRITE_HOME", t.TempDir())
	root := t.TempDir()
	dirs := map[string]string{}
	syn := &synapse.Synapse{ID: "s", Name: "s", State: synapse.StateActive}
	for _, a := range aliases {
		dir := filepath.Join(root, a)
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatal(err)
		}
		id, err := projectid.Ensure(dir)
		if err != nil {
			t.Fatal(err)
		}
		dirs[a] = dir
		syn.Members = append(syn.Members, synapse.Member{ProjectID: id, Alias: a, Role: synapse.RolePeer, PathHint: dir})
	}
	return syn, dirs
}

// perMember answers each member's search with its own scripted ranked list.
func perMember(dirs map[string]string, lists map[string][]rag.Chunk) Searcher {
	return func(_ context.Context, dir, _ string, _ int, _ string) ([]rag.Chunk, error) {
		for alias, d := range dirs {
			if d == dir {
				if lists[alias] == nil {
					return nil, errors.New("sidecar unavailable\nsecond line")
				}
				out := append([]rag.Chunk(nil), lists[alias]...)
				for i := range out {
					out[i].FilePath = filepath.Join(d, out[i].FilePath)
				}
				return out, nil
			}
		}
		return nil, errors.New("unknown dir")
	}
}

func chunk(path string, score float32) rag.Chunk {
	return rag.Chunk{FilePath: path, Score: score, LineStart: 1, LineEnd: 5}
}

func TestMergeIsByRankNotByScoreAndEveryHitNamesItsMember(t *testing.T) {
	syn, dirs := members(t, "api", "web")
	// api's scores are on a 0.01-0.03 scale, web's on 0-100: comparing them would let web swamp api.
	lists := map[string][]rag.Chunk{
		"api": {chunk("a1.go", 0.03), chunk("a2.go", 0.02), chunk("a3.go", 0.01)},
		"web": {chunk("w1.ts", 90), chunk("w2.ts", 80), chunk("w3.ts", 10)},
	}
	hits, notes, err := Search(context.Background(), syn, "q", Options{TopK: 6}, perMember(dirs, lists))
	if err != nil || len(notes) != 0 {
		t.Fatalf("err=%v notes=%v", err, notes)
	}
	var order []string
	for _, h := range hits {
		order = append(order, h.Member+":"+h.Path)
	}
	// Rank decides: both #1s, then both #2s, then both #3s, however different the score scales are.
	// Within a rank, the hit closer to its own list's best goes first (#1s tie at 100% and fall back
	// to the alias; at #2 web is at 89% of its best and api at 67%; at #3 api is at 33% and web at 11%).
	want := []string{"api:a1.go", "web:w1.ts", "web:w2.ts", "api:a2.go", "api:a3.go", "web:w3.ts"}
	for i := range want {
		if i >= len(order) || order[i] != want[i] {
			t.Fatalf("order = %v, want %v", order, want)
		}
	}
	if hits[0].Rank != 1 || hits[2].Rank != 2 {
		t.Errorf("ranks = %d %d", hits[0].Rank, hits[2].Rank)
	}
}

func TestTopKMemberFilterAndRelativePaths(t *testing.T) {
	syn, dirs := members(t, "api", "web")
	lists := map[string][]rag.Chunk{
		"api": {chunk("pkg/x.go", 1), chunk("pkg/y.go", 0.5)},
		"web": {chunk("src/a.ts", 1)},
	}
	hits, _, err := Search(context.Background(), syn, "q", Options{TopK: 2}, perMember(dirs, lists))
	if err != nil || len(hits) != 2 {
		t.Fatalf("hits = %d, err = %v", len(hits), err)
	}
	if hits[0].Path != "pkg/x.go" && hits[0].Path != "src/a.ts" {
		t.Errorf("paths are not relative to the member: %q", hits[0].Path)
	}
	only, _, err := Search(context.Background(), syn, "q", Options{Members: []string{"web"}}, perMember(dirs, lists))
	if err != nil || len(only) != 1 || only[0].Member != "web" {
		t.Errorf("member filter: %+v, %v", only, err)
	}
	if _, _, err := Search(context.Background(), syn, "q", Options{Members: []string{"nope"}}, perMember(dirs, lists)); err == nil {
		t.Error("an unknown member alias was accepted")
	}
}

func TestAMemberThatFailsOrIsAbsentIsNotedAndTheOthersAnswer(t *testing.T) {
	syn, dirs := members(t, "api", "web", "lib")
	syn.Members = append(syn.Members, synapse.Member{Alias: "far", ProjectID: "dpid_far", Source: synapse.SourceRemote, Remote: "https://example.invalid/far.git", PathHint: filepath.Join(t.TempDir(), "none")})
	lists := map[string][]rag.Chunk{"api": {chunk("a.go", 1)}, "lib": {chunk("l.go", 1)}} // web has no list: its search fails
	hits, notes, err := Search(context.Background(), syn, "q", Options{}, perMember(dirs, lists))
	if err != nil {
		t.Fatal(err)
	}
	if len(hits) != 2 {
		t.Errorf("hits = %+v, want api and lib", hits)
	}
	reasons := map[string]string{}
	for _, n := range notes {
		reasons[n.Member] = n.Reason
	}
	if reasons["web"] != "sidecar unavailable" || reasons["far"] != "no local checkout" || len(reasons) != 2 {
		t.Errorf("notes = %v", reasons)
	}
}

func TestEmptyQueryAndNilSynapseAreRefused(t *testing.T) {
	syn, _ := members(t, "api")
	if _, _, err := Search(context.Background(), syn, "  ", Options{}, nil); err == nil {
		t.Error("empty query accepted")
	}
	if _, _, err := Search(context.Background(), nil, "q", Options{}, nil); err == nil {
		t.Error("nil synapse accepted")
	}
}
