package rag

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"Dendrite/pkg/sidecar"
)

// fakeRag is the testdata/fakerag sidecar, built once for the package tests.
var fakeRag string

func TestMain(m *testing.M) {
	// Tests in this package never pick up a dendrite-rag installed on the machine unless
	// they build their Retriever explicitly.
	_ = os.Setenv(SidecarEnv, "off")
	dir, err := os.MkdirTemp("", "fakerag")
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	fakeRag = filepath.Join(dir, "dendrite-rag")
	if out, err := exec.Command("go", "build", "-o", fakeRag, "./testdata/fakerag").CombinedOutput(); err != nil {
		fmt.Fprintf(os.Stderr, "building fakerag: %v\n%s", err, out)
		os.Exit(1)
	}
	code := m.Run()
	_ = os.RemoveAll(dir)
	os.Exit(code)
}

func notFound(string) (string, error) { return "", errors.New("not on PATH") }

func ragProject(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	src := "package demo\n\n// LocateBinary finds the sidecar executable.\nfunc LocateBinary(name string) string { return name }\n"
	if err := os.WriteFile(filepath.Join(dir, "demo.go"), []byte(src), 0o644); err != nil {
		t.Fatal(err)
	}
	return dir
}

func TestRetrieverUsesSidecarAndMapsHits(t *testing.T) {
	log := filepath.Join(t.TempDir(), "calls.log")
	t.Setenv("FAKERAG_LOG", log)
	r := &Retriever{Locator: sidecar.Locator{MemberDirs: nil, LookPath: notFound}, EmbedBudget: 3 * time.Second}
	t.Setenv(sidecar.EnvVar(SidecarBinary), fakeRag)

	got, err := r.Retrieve(context.Background(), "testdata", "where is A", SearchOptions{TopK: 2}, false)
	if err != nil {
		t.Fatal(err)
	}
	if got.Outcome.Origin != sidecar.OriginSidecar || got.Outcome.Path != fakeRag {
		t.Fatalf("outcome = %+v", got.Outcome)
	}
	if got.IndexStatus != "incremental" || got.EmbedPending != 7 {
		t.Fatalf("status %q pending %d", got.IndexStatus, got.EmbedPending)
	}
	want := Chunk{ID: "c1", FilePath: filepath.Join("testdata", "pkg/a/a.go"), Heading: "impl Engine › method search", Content: "func A() {}", LineStart: 3, LineEnd: 9, Score: 0.03}
	if len(got.Chunks) != 1 || got.Chunks[0] != want {
		t.Fatalf("chunks = %+v", got.Chunks)
	}

	calls, _ := os.ReadFile(log)
	abs, _ := filepath.Abs("testdata")
	for _, frag := range []string{`index {"embed_budget_ms":3000,"full":false,"root":"` + abs + `"}`, `search {"query":"where is A","root":"` + abs + `","top_k":2}`} {
		if !strings.Contains(string(calls), frag) {
			t.Errorf("sidecar calls missing %s:\n%s", frag, calls)
		}
	}
}

func TestRetrieverFallsBack(t *testing.T) {
	dir := ragProject(t)
	cases := map[string]struct {
		r      *Retriever
		env    map[string]string
		reason string
	}{
		"binary missing": {r: &Retriever{Locator: sidecar.Locator{LookPath: notFound}}, reason: "not found"},
		"disabled":       {r: &Retriever{Disabled: true}, reason: SidecarEnv},
		"sidecar error": {
			r:      &Retriever{Locator: sidecar.Locator{LookPath: notFound}},
			env:    map[string]string{sidecar.EnvVar(SidecarBinary): fakeRag, "FAKERAG_FAIL": "search"},
			reason: "boom",
		},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			for k, v := range tc.env {
				t.Setenv(k, v)
			}
			got, err := tc.r.Retrieve(context.Background(), dir, "LocateBinary sidecar", SearchOptions{TopK: 3, MinScore: 0.01}, false)
			if err != nil {
				t.Fatal(err)
			}
			if got.Outcome.Origin != sidecar.OriginFallback || !strings.Contains(got.Outcome.Reason, tc.reason) {
				t.Fatalf("outcome = %+v", got.Outcome)
			}
			if len(got.Chunks) == 0 || !strings.HasSuffix(got.Chunks[0].FilePath, "demo.go") {
				t.Fatalf("fallback chunks = %+v", got.Chunks)
			}
		})
	}
}

func TestNewRetrieverHonoursOptOut(t *testing.T) {
	for v, disabled := range map[string]bool{"off": true, "0": true, "false": true, "on": false, "": false} {
		t.Setenv(SidecarEnv, v)
		if got := NewRetriever(nil).Disabled; got != disabled {
			t.Errorf("%s=%q: disabled = %v", SidecarEnv, v, got)
		}
	}
}
