package rag

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"Dendrite/pkg/rag/ragtest"
	"Dendrite/pkg/sidecar"
)

// standIn is the path of the dendrite-rag stand-in that every test in the package uses.
var standIn string

func TestMain(m *testing.M) {
	cleanup, err := ragtest.Install()
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	standIn = os.Getenv(sidecar.EnvVar(SidecarBinary))
	code := m.Run()
	CloseSidecars()
	cleanup()
	os.Exit(code)
}

func notFound(string) (string, error) { return "", errors.New("not on PATH") }

func ragProject(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	src := "package demo\n\n// LocateBinary finds the sidecar executable.\nfunc LocateBinary(name string) string { return name }\n"
	if err := os.MkdirAll(filepath.Join(dir, "pkg", "demo"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "pkg", "demo", "demo.go"), []byte(src), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "README.md"), []byte("# Demo\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	return dir
}

func standInRetriever(t *testing.T) (*Retriever, string) {
	t.Helper()
	if standIn == "" {
		t.Fatal("TestMain did not build the stand-in")
	}
	log := filepath.Join(t.TempDir(), "calls.log")
	t.Setenv("STANDIN_LOG", log)
	if os.Getenv(sidecar.EnvVar(SidecarBinary)) == "" {
		t.Setenv(sidecar.EnvVar(SidecarBinary), standIn)
	}
	return &Retriever{Locator: sidecar.Locator{LookPath: notFound}, EmbedBudget: 3 * time.Second}, log
}

func TestRetrieveIndexesThenSearchesAndMapsHits(t *testing.T) {
	r, log := standInRetriever(t)
	dir := ragProject(t)

	got, err := r.Retrieve(context.Background(), dir, "where is LocateBinary", SearchOptions{TopK: 2}, false)
	if err != nil {
		t.Fatal(err)
	}
	if got.Sidecar != standIn || got.IndexStatus != "incremental" || got.EmbedPending != 7 {
		t.Fatalf("retrieval = %+v", got)
	}
	if len(got.Chunks) == 0 || got.Chunks[0].FilePath != filepath.Join(dir, "pkg", "demo", "demo.go") || got.Chunks[0].LineStart != 1 {
		t.Fatalf("chunks = %+v", got.Chunks)
	}

	calls, _ := os.ReadFile(log)
	abs, _ := filepath.Abs(dir)
	for _, frag := range []string{`index {"embed_budget_ms":3000,"focus":"where is LocateBinary","full":false,"root":"` + abs + `"}`, `search {"query":"where is LocateBinary","root":"` + abs + `","top_k":2}`} {
		if !strings.Contains(string(calls), frag) {
			t.Errorf("sidecar calls missing %s:\n%s", frag, calls)
		}
	}
}

func TestRetrieveSingleFileSearchesOnlyThatFile(t *testing.T) {
	r, log := standInRetriever(t)
	dir := ragProject(t)
	file := filepath.Join(dir, "pkg", "demo", "demo.go")

	got, err := r.Retrieve(context.Background(), file, "LocateBinary", SearchOptions{}, true)
	if err != nil {
		t.Fatal(err)
	}
	if got.IndexStatus != "refreshed" || len(got.Chunks) != 1 || got.Chunks[0].FilePath != file {
		t.Fatalf("retrieval = %+v", got)
	}
	calls, _ := os.ReadFile(log)
	if !strings.Contains(string(calls), `"paths":["demo.go"]`) {
		t.Errorf("search not restricted to the file:\n%s", calls)
	}
}

func TestContextReturnsPackedSpans(t *testing.T) {
	r, log := standInRetriever(t)
	dir := ragProject(t)

	got, err := r.Context(context.Background(), dir, "LocateBinary sidecar", 1200, false)
	if err != nil {
		t.Fatal(err)
	}
	if len(got.Chunks) == 0 || got.Tokens == 0 {
		t.Fatalf("context = %+v", got)
	}
	calls, _ := os.ReadFile(log)
	if !strings.Contains(string(calls), `context {"query":"LocateBinary sidecar","root":"`) || !strings.Contains(string(calls), `"token_budget":1200`) {
		t.Errorf("context call:\n%s", calls)
	}
}

func TestRetrieveReportsSidecarProblems(t *testing.T) {
	dir := ragProject(t)
	t.Run("binary missing", func(t *testing.T) {
		t.Setenv(sidecar.EnvVar(SidecarBinary), "")
		r := &Retriever{Locator: sidecar.Locator{LookPath: notFound}}
		_, err := r.Retrieve(context.Background(), dir, "LocateBinary", SearchOptions{}, false)
		if !errors.Is(err, ErrSidecarUnavailable) || !strings.Contains(err.Error(), "cargo install") {
			t.Fatalf("err = %v", err)
		}
	})
	t.Run("sidecar error", func(t *testing.T) {
		r, _ := standInRetriever(t)
		t.Setenv("STANDIN_FAIL", "search")
		_, err := r.Retrieve(context.Background(), dir, "LocateBinary", SearchOptions{}, false)
		var rpcErr *sidecar.RPCError
		if !errors.As(err, &rpcErr) || !strings.Contains(err.Error(), "boom") {
			t.Fatalf("err = %v", err)
		}
	})
	t.Run("missing project", func(t *testing.T) {
		r, _ := standInRetriever(t)
		if _, err := r.Retrieve(context.Background(), filepath.Join(dir, "nope"), "x", SearchOptions{}, false); err == nil {
			t.Fatal("expected an error for a missing project path")
		}
	})
}
