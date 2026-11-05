package rag

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"Dendrite/pkg/sidecar"
)

// operation is one sidecar call of the Retriever, as a function of the project path.
type operation struct {
	name, method string
	call         func(r *Retriever, dir string) error
}

func operations() []operation {
	ctx := context.Background()
	return []operation{
		{"Index", "index", func(r *Retriever, dir string) error {
			_, err := r.Index(ctx, dir, IndexOptions{Full: true, EmbedBudgetMS: 5, EmbedBudgetTokens: 9, Focus: "how"})
			return err
		}},
		{"Status", "status", func(r *Retriever, dir string) error { _, err := r.Status(ctx, dir); return err }},
		{"Settings", "settings", func(r *Retriever, dir string) error { _, err := r.Settings(ctx, dir); return err }},
		{"Purge", "purge", func(r *Retriever, dir string) error { return r.Purge(ctx, dir) }},
		{"Symbols", "symbols.definitions", func(r *Retriever, dir string) error { _, err := r.Symbols(ctx, dir, "LocateBinary", false); return err }},
		{"SymbolsRefs", "symbols.references", func(r *Retriever, dir string) error { _, err := r.Symbols(ctx, dir, "LocateBinary", true); return err }},
		{"Grep", "grep", func(r *Retriever, dir string) error {
			_, err := r.Grep(ctx, dir, "Locate", []string{"pkg"}, 5)
			return err
		}},
		{"History", "history.search", func(r *Retriever, dir string) error { _, err := r.History(ctx, dir, "rag", 3); return err }},
		{"Sync", "sync", func(r *Retriever, dir string) error { _, err := r.Sync(ctx, dir); return err }},
		{"Pull", "pull", func(r *Retriever, dir string) error { _, err := r.Pull(ctx, dir); return err }},
		{"Push", "push", func(r *Retriever, dir string) error { _, err := r.Push(ctx, dir, true); return err }},
		{"Rank", "rank", func(r *Retriever, dir string) error {
			_, err := r.Rank(ctx, dir, "coverage gate", []RankDocument{{ID: "1", Text: "the coverage gate"}, {ID: "2", Text: "other"}}, 1)
			return err
		}},
		{"Quality", "quality.loc", func(r *Retriever, dir string) error {
			return r.Quality(ctx, dir, "quality.loc", nil, &struct{}{})
		}},
	}
}

func TestOperationsReachTheSidecar(t *testing.T) {
	t.Setenv("STANDIN_PROTOCOL", "1.11.0")
	dir := t.TempDir()
	for _, op := range operations() {
		if err := op.call(NewRetriever(), dir); err != nil {
			t.Errorf("%s: %v", op.name, err)
		}
	}
}

func TestOperationsReportTheirFailures(t *testing.T) {
	t.Setenv("STANDIN_PROTOCOL", "1.11.0")
	dir := t.TempDir()
	for _, op := range operations() {
		if err := op.call(NewRetriever(), filepath.Join(dir, "missing")); err == nil {
			t.Errorf("%s: a project that does not exist is an error", op.name)
		}
	}
	for _, op := range operations() {
		t.Run(op.name+" fails in the sidecar", func(t *testing.T) {
			t.Setenv("STANDIN_PROTOCOL", "1.11.0")
			t.Setenv("STANDIN_FAIL", op.method)
			if err := op.call(NewRetriever(), dir); err == nil || !strings.Contains(err.Error(), "boom") {
				t.Errorf("%v", err)
			}
		})
	}
}

func TestOperationsNeedTheSidecar(t *testing.T) {
	t.Setenv(sidecar.EnvVar(SidecarBinary), filepath.Join(t.TempDir(), "no-such-binary"))
	dir := t.TempDir()
	for _, op := range operations() {
		if err := op.call(NewRetriever(), dir); !errors.Is(err, ErrSidecarUnavailable) {
			t.Errorf("%s without a sidecar: %v", op.name, err)
		}
	}
	if _, err := NewRetriever().Embedders(context.Background()); !errors.Is(err, ErrSidecarUnavailable) {
		t.Errorf("Embedders without a sidecar: %v", err)
	}
}

func TestOperationsReportASidecarThatCannotStart(t *testing.T) {
	notRunnable := filepath.Join(t.TempDir(), "dendrite-rag")
	if err := os.WriteFile(notRunnable, []byte("not a program"), 0o644); err != nil {
		t.Fatal(err)
	}
	t.Setenv(sidecar.EnvVar(SidecarBinary), notRunnable)
	dir := t.TempDir()
	for _, op := range operations() {
		if err := op.call(NewRetriever(), dir); !errors.Is(err, ErrSidecarUnavailable) || !strings.Contains(err.Error(), notRunnable) {
			t.Errorf("%s with a sidecar that does not start: %v", op.name, err)
		}
	}
	if _, err := NewRetriever().Embedders(context.Background()); !errors.Is(err, ErrSidecarUnavailable) {
		t.Errorf("Embedders with a sidecar that does not start: %v", err)
	}
}

func TestShuttingTheSidecarsDownLeavesRoomForNewOnes(t *testing.T) {
	dir := t.TempDir()
	if err := NewRetriever().Purge(context.Background(), dir); err != nil {
		t.Fatal(err)
	}
	CloseSidecars()
	if err := NewRetriever().Purge(context.Background(), dir); err != nil {
		t.Errorf("a fresh sidecar is started after the pool was closed: %v", err)
	}
	if err := (&conn{}).Close(); err != nil {
		t.Error(err)
	}
}

func TestOperationsUseTheCurrentDirectoryWhenGivenNone(t *testing.T) {
	t.Setenv("STANDIN_PROTOCOL", "1.11.0")
	t.Chdir(t.TempDir())
	for _, op := range operations() {
		if err := op.call(NewRetriever(), ""); err != nil {
			t.Errorf("%s: %v", op.name, err)
		}
	}
}

func TestEmbeddersListsTheBackendsAndReportsFailures(t *testing.T) {
	backends, err := NewRetriever().Embedders(context.Background())
	if err != nil || len(backends) != 1 {
		t.Fatalf("%+v, %v", backends, err)
	}
	t.Setenv("STANDIN_FAIL", "embedders")
	if _, err := NewRetriever().Embedders(context.Background()); err == nil || !strings.Contains(err.Error(), "boom") {
		t.Errorf("%v", err)
	}
}

func TestProtocolMinorReadsTheMinorVersion(t *testing.T) {
	for in, want := range map[string]int{"1.11.0": 11, "1.9": 9, "1": 0, "": 0, "1.x.0": 0, "2.3.4": 3} {
		if got := protocolMinor(in); got != want {
			t.Errorf("protocolMinor(%q) = %d, want %d", in, got, want)
		}
	}
}

func TestSingleFileIsTheNameOfAFileProjectAndNilOtherwise(t *testing.T) {
	dir := t.TempDir()
	file := filepath.Join(dir, "a.go")
	if err := os.WriteFile(file, []byte("package a\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if got := singleFile(file); len(got) != 1 || got[0] != "a.go" {
		t.Errorf("a file: %v", got)
	}
	if singleFile(dir) != nil || singleFile(filepath.Join(dir, "missing")) != nil {
		t.Error("a directory and a missing path restrict nothing")
	}
}

func TestContextWithSendsItsOptionsAndChecksTheProtocolForADocsShare(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	share := 0.4
	opts := ContextOptions{Corpus: "code", Detail: "map", DocsShare: &share}
	if _, err := NewRetriever().ContextWith(ctx, dir, "how does it work", 500, opts, false); err != nil {
		t.Fatalf("a sidecar at 1.9 knows the docs share: %v", err)
	}
	if _, err := NewRetriever().ContextWith(ctx, dir, "how", 0, ContextOptions{}, false); err != nil {
		t.Errorf("no options: %v", err)
	}
	t.Setenv("STANDIN_PROTOCOL", "1.6.0")
	if _, err := NewRetriever().ContextWith(ctx, dir, "how", 500, opts, false); !errors.Is(err, ErrSidecarTooOld) {
		t.Errorf("a sidecar that predates the docs share: %v", err)
	}
	t.Setenv("STANDIN_PROTOCOL", "1.9.0")
	file := filepath.Join(dir, "a.go")
	if err := os.WriteFile(file, []byte("package a\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := NewRetriever().ContextWith(ctx, file, "how", 500, ContextOptions{Corpus: "code"}, false); err != nil {
		t.Errorf("a single-file project is searched, not packed: %v", err)
	}
}
