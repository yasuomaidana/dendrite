package rag

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// expectSidecarCall requires the sidecar log to hold a call; %s in format stands for the project's absolute path.
func expectSidecarCall(t *testing.T, log, dir, format string) {
	t.Helper()
	calls, _ := os.ReadFile(log)
	abs, _ := filepath.Abs(dir)
	want := strings.Replace(format, "%s", abs, 1)
	if !strings.Contains(string(calls), want) {
		t.Errorf("calls lack %s:\n%s", want, calls)
	}
}

func TestGraphQueriesIndexThenCallSidecar(t *testing.T) {
	ctx := context.Background()
	cases := []struct {
		name string
		// via runs the query on the retriever and reports whether the answer was the expected one.
		via func(r *Retriever, dir string) (bool, error)
		// top runs the package-level helper and reports whether it found something.
		top   func(dir string) (bool, error)
		calls []string
	}{
		{"communities",
			func(r *Retriever, dir string) (bool, error) {
				got, err := r.Communities(ctx, dir, CommunitiesOptions{Limit: 10, MinSize: 1})
				return len(got) == 1 && got[0].ID == 1 && got[0].Label == "pkg/demo · LocateBinary", err
			},
			func(dir string) (bool, error) {
				got, err := Communities(ctx, dir, CommunitiesOptions{})
				return len(got) == 1, err
			},
			[]string{`index {"embed_budget_ms":3000,"full":false,"root":"%s"}`, `graph.communities {"limit":10,"min_size":1,"root":"%s"}`}},
		{"god nodes",
			func(r *Retriever, dir string) (bool, error) {
				got, err := r.GodNodes(ctx, dir, GodNodesOptions{MinInDegree: 21, Limit: 10})
				return len(got) == 1 && got[0].Symbol == "LocateBinary" && got[0].InDegree == 25, err
			},
			func(dir string) (bool, error) {
				got, err := GodNodes(ctx, dir, GodNodesOptions{})
				return len(got) == 1, err
			},
			[]string{`graph.god_nodes {"limit":10,"min_in_degree":21,"root":"%s"}`}},
		{"path",
			func(r *Retriever, dir string) (bool, error) {
				got, err := r.Path(ctx, dir, "Run", "LocateBinary", PathOptions{Kinds: []string{"call"}, MaxHops: 4})
				return got.Found && got.Directed && got.Level == "symbol" && len(got.Steps) == 2, err
			},
			func(dir string) (bool, error) {
				got, err := Path(ctx, dir, "Run", "LocateBinary", PathOptions{})
				return got.Found, err
			},
			[]string{`graph.path {"from":"Run","kinds":["call"],"max_hops":4,"root":"%s","to":"LocateBinary"}`}},
		{"explain",
			func(r *Retriever, dir string) (bool, error) {
				got, err := r.Explain(ctx, dir, "LocateBinary")
				return len(got) == 1 && got[0].Kind == "symbol" && got[0].Symbol == "LocateBinary", err
			},
			func(dir string) (bool, error) {
				got, err := Explain(ctx, dir, "LocateBinary")
				return len(got) == 1, err
			},
			[]string{`graph.explain {"root":"%s","target":"LocateBinary"}`}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			r, log := standInRetriever(t)
			dir := ragProject(t)
			if ok, err := c.via(r, dir); err != nil || !ok {
				t.Fatalf("unexpected answer: ok=%v err=%v", ok, err)
			}
			for _, call := range c.calls {
				expectSidecarCall(t, log, dir, call)
			}
			if ok, err := c.top(dir); err != nil || !ok {
				t.Fatalf("unexpected top-level answer: ok=%v err=%v", ok, err)
			}
		})
	}
}
