package rag

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestCommunitiesIndexesThenCallsSidecar(t *testing.T) {
	r, log := standInRetriever(t)
	dir := ragProject(t)

	got, err := r.Communities(context.Background(), dir, CommunitiesOptions{Limit: 10, MinSize: 1})
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || got[0].ID != 1 || got[0].Label != "pkg/demo · LocateBinary" {
		t.Fatalf("unexpected communities: %+v", got)
	}
	calls, _ := os.ReadFile(log)
	abs, _ := filepath.Abs(dir)
	if !strings.Contains(string(calls), `index {"embed_budget_ms":3000,"full":false,"root":"`+abs+`"}`) {
		t.Errorf("calls missing index: %s", calls)
	}
	if !strings.Contains(string(calls), `graph.communities {"limit":10,"min_size":1,"root":"`+abs+`"}`) {
		t.Errorf("calls missing graph.communities: %s", calls)
	}

	// Also test top-level helper
	topGot, err := Communities(context.Background(), dir, CommunitiesOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if len(topGot) != 1 {
		t.Fatalf("unexpected top-level communities: %+v", topGot)
	}
}

func TestGodNodesIndexesThenCallsSidecar(t *testing.T) {
	r, log := standInRetriever(t)
	dir := ragProject(t)

	got, err := r.GodNodes(context.Background(), dir, GodNodesOptions{MinInDegree: 21, Limit: 10})
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || got[0].Symbol != "LocateBinary" || got[0].InDegree != 25 {
		t.Fatalf("unexpected god nodes: %+v", got)
	}
	calls, _ := os.ReadFile(log)
	abs, _ := filepath.Abs(dir)
	if !strings.Contains(string(calls), `graph.god_nodes {"limit":10,"min_in_degree":21,"root":"`+abs+`"}`) {
		t.Errorf("calls missing graph.god_nodes: %s", calls)
	}

	// Top-level helper
	topGot, err := GodNodes(context.Background(), dir, GodNodesOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if len(topGot) != 1 {
		t.Fatalf("unexpected top-level god nodes: %+v", topGot)
	}
}

func TestPathIndexesThenCallsSidecar(t *testing.T) {
	r, log := standInRetriever(t)
	dir := ragProject(t)

	got, err := r.Path(context.Background(), dir, "Run", "LocateBinary", PathOptions{Kinds: []string{"call"}, MaxHops: 4})
	if err != nil {
		t.Fatal(err)
	}
	if !got.Found || !got.Directed || got.Level != "symbol" || len(got.Steps) != 2 {
		t.Fatalf("unexpected path: %+v", got)
	}
	calls, _ := os.ReadFile(log)
	abs, _ := filepath.Abs(dir)
	if !strings.Contains(string(calls), `graph.path {"from":"Run","kinds":["call"],"max_hops":4,"root":"`+abs+`","to":"LocateBinary"}`) {
		t.Errorf("calls missing graph.path: %s", calls)
	}

	// Top-level helper
	topGot, err := Path(context.Background(), dir, "Run", "LocateBinary", PathOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if !topGot.Found {
		t.Fatalf("unexpected top-level path: %+v", topGot)
	}
}

func TestExplainIndexesThenCallsSidecar(t *testing.T) {
	r, log := standInRetriever(t)
	dir := ragProject(t)

	got, err := r.Explain(context.Background(), dir, "LocateBinary")
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || got[0].Kind != "symbol" || got[0].Symbol != "LocateBinary" {
		t.Fatalf("unexpected explain: %+v", got)
	}
	calls, _ := os.ReadFile(log)
	abs, _ := filepath.Abs(dir)
	if !strings.Contains(string(calls), `graph.explain {"root":"`+abs+`","target":"LocateBinary"}`) {
		t.Errorf("calls missing graph.explain: %s", calls)
	}

	// Top-level helper
	topGot, err := Explain(context.Background(), dir, "LocateBinary")
	if err != nil {
		t.Fatal(err)
	}
	if len(topGot) != 1 {
		t.Fatalf("unexpected top-level explain: %+v", topGot)
	}
}

func TestGraphValidationErrors(t *testing.T) {
	r, _ := standInRetriever(t)
	dir := ragProject(t)

	// Path missing from or to
	if _, err := r.Path(context.Background(), dir, "", "B", PathOptions{}); err == nil {
		t.Error("expected error for empty from")
	}
	if _, err := r.Path(context.Background(), dir, "A", "", PathOptions{}); err == nil {
		t.Error("expected error for empty to")
	}

	// Explain missing target
	if _, err := r.Explain(context.Background(), dir, ""); err == nil {
		t.Error("expected error for empty target")
	}

	// Invalid directory
	if _, err := r.Communities(context.Background(), "/nonexistent/directory/here", CommunitiesOptions{}); err == nil {
		t.Error("expected error for nonexistent directory")
	}
}

func TestGraphSidecarFailure(t *testing.T) {
	r, _ := standInRetriever(t)
	dir := ragProject(t)

	t.Setenv("STANDIN_FAIL", "graph.communities")
	if _, err := r.Communities(context.Background(), dir, CommunitiesOptions{}); err == nil {
		t.Error("expected error when sidecar fails")
	}
}
