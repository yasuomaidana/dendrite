package analysis

import (
	"errors"
	"os"
	"strings"
	"testing"

	"Dendrite/pkg/rea"
)

func finished(t *testing.T, r *rig, id string) {
	t.Helper()
	if _, err := r.Analyze(ctx(), r.binary, Options{ID: id}); err != nil {
		t.Fatal(err)
	}
}

func TestSearchReturnsThePassagesOfAFinishedAnalysis(t *testing.T) {
	r := newRig(t)
	finished(t, r, "pe-s")
	hits, err := r.Search(ctx(), "pe-s", "CreateFileW", 0)
	if err != nil || len(hits) == 0 || hits[0].File != "imports/kernel32.dll.md" || !strings.Contains(hits[0].Text, "CreateFileW") {
		t.Errorf("hits %+v, %v", hits, err)
	}
	if hits, err := r.Search(ctx(), "pe-s", "CreateFileW", 1000); err != nil || len(hits) > maxHits {
		t.Errorf("k is capped at %d: %d hits, %v", maxHits, len(hits), err)
	}
}

func TestSearchRefusesWhatItCannotAnswer(t *testing.T) {
	r := newRig(t)
	if _, err := r.Search(ctx(), "pe-none", "x", 1); !errors.Is(err, ErrNotFound) {
		t.Errorf("an unknown analysis: %v", err)
	}
	p := &pausingParser{inner: r.Parser, svc: r.Service, id: "pe-early"}
	r.Parser = p
	if _, err := r.Analyze(ctx(), r.binary, Options{ID: "pe-early"}); !errors.Is(err, ErrPaused) {
		t.Fatal(err)
	}
	if _, err := r.Search(ctx(), "pe-early", "x", 1); !errors.Is(err, ErrNotDone) {
		t.Errorf("an unfinished analysis: %v", err)
	}
	r.Parser = p.inner
	finished(t, r, "pe-late")
	t.Setenv("STANDIN_FAIL", "search")
	if _, err := r.Search(ctx(), "pe-late", "x", 1); err == nil {
		t.Error("a failing RAG sidecar is an error")
	}
}

func TestWithinBudgetCutsLongPassagesAndStopsAtTheTokenBoundary(t *testing.T) {
	long := strings.Repeat("a", maxHitChars*3)
	got := withinBudget([]Hit{{Text: long}})
	if len(got) != 1 || len(got[0].Text) != maxHitChars+3 || !strings.HasSuffix(got[0].Text, "...") {
		t.Errorf("a long passage is cut: %d chars", len(got[0].Text))
	}
	many := make([]Hit, 40)
	for i := range many {
		many[i] = Hit{Text: long}
	}
	kept := withinBudget(many)
	total := 0
	for _, h := range kept {
		total += rea.EstimateTokens(h.Text)
	}
	if len(kept) == 0 || len(kept) == len(many) || total > rea.MaxEvidenceTokens {
		t.Errorf("kept %d of %d hits with %d tokens", len(kept), len(many), total)
	}
}

func TestPurgeDeletesTheAnalysisAndItsIndex(t *testing.T) {
	r := newRig(t)
	finished(t, r, "pe-p")
	if err := r.Purge(ctx(), "pe-p", false); !errors.Is(err, ErrNeedsForce) {
		t.Errorf("purge without force: %v", err)
	}
	if _, err := os.Stat(r.Store.Dir("pe-p")); err != nil {
		t.Fatalf("the analysis survives a refused purge: %v", err)
	}
	if err := r.Purge(ctx(), "pe-p", true); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(r.Store.Dir("pe-p")); !os.IsNotExist(err) {
		t.Errorf("the analysis directory must be gone: %v", err)
	}
	if err := r.Purge(ctx(), "pe-p", true); !errors.Is(err, ErrNotFound) {
		t.Errorf("purging twice: %v", err)
	}
}

func TestPurgeReportsAnIndexThatCouldNotBePurgedButStillDeletes(t *testing.T) {
	r := newRig(t)
	finished(t, r, "pe-q")
	t.Setenv("STANDIN_FAIL", "purge")
	err := r.Purge(ctx(), "pe-q", true)
	if err == nil || !strings.Contains(err.Error(), "deleted but its index was not purged") {
		t.Errorf("err = %v", err)
	}
	if _, statErr := os.Stat(r.Store.Dir("pe-q")); !os.IsNotExist(statErr) {
		t.Errorf("the directory is deleted even when the index purge fails: %v", statErr)
	}
}

func TestPurgeReportsADirectoryThatCannotBeDeleted(t *testing.T) {
	r := newRig(t)
	finished(t, r, "pe-r")
	stops := readOnly(t, r.Store.Root) // permission bits do not stop root
	if err := r.Purge(ctx(), "pe-r", true); (err != nil) != stops {
		t.Errorf("permissions stop deleting = %t but err = %v", stops, err)
	}
}
