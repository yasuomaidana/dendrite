package insight

import (
	"context"
	"testing"

	"Dendrite/pkg/coreclient"
	"Dendrite/pkg/rea/analysis"
)

func ctx() context.Context { return context.Background() }

func TestRankReadsTheOverviewFirstThenTheBusiestDocuments(t *testing.T) {
	img := &analysis.Image{Format: "pe",
		Libraries: []coreclient.ImageLibrary{{Name: "quiet.dll", Symbols: []string{"A"}}, {Name: "busy.dll", Symbols: []string{"A", "B", "C", "D"}}, {Name: "empty.dll"}, {Name: "tied.dll", Symbols: []string{"X", "Y"}}, {Name: "also.dll", Symbols: []string{"X", "Y"}}},
		Exports:   []string{"E1", "E2"}}
	got := Rank(analysis.State{ID: "r", Binary: "/x/a.exe"}, img)
	var docs []string
	for _, r := range got {
		docs = append(docs, r.Doc)
	}
	want := []string{"overview.md", "graph.md", "libraries/busy.dll.md", "exports.md", "libraries/also.dll.md", "libraries/tied.dll.md", "libraries/quiet.dll.md"}
	if len(docs) != len(want) {
		t.Fatalf("ranked %v, want %v", docs, want)
	}
	for i := range want {
		if docs[i] != want[i] {
			t.Errorf("rank %d = %s, want %s (all: %v)", i+1, docs[i], want[i], docs)
		}
	}
	if got[2].Edges != 4 || got[2].Why == "" {
		t.Errorf("%+v", got[2])
	}
}

func TestRankOfAnEmptyTargetIsTheOverviewAlone(t *testing.T) {
	got := Rank(analysis.State{ID: "r"}, &analysis.Image{Format: "unknown"})
	if len(got) != 1 || got[0].Doc != "overview.md" {
		t.Errorf("%+v", got)
	}
}
