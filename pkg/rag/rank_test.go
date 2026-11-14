package rag

import (
	"context"
	"errors"
	"os"
	"strings"
	"testing"

	"Dendrite/pkg/sidecar"
)

func TestRankSendsDocumentsWithoutIndexing(t *testing.T) {
	r, log := standInRetriever(t)
	dir := t.TempDir()
	docs := []RankDocument{{ID: "a", Text: "git commits"}, {ID: "b", Text: "system design and architecture"}}

	got, err := r.Rank(context.Background(), dir, "architecture design", docs, 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 || got[0].ID != "b" || got[0].Lexical != 2 || got[0].Score <= got[1].Score || got[0].Dense != nil {
		t.Fatalf("ranked = %+v", got)
	}
	calls, _ := os.ReadFile(log)
	if strings.Contains(string(calls), "index ") || !strings.Contains(string(calls), `rank {"documents":[{"id":"a","text":"git commits"}`) {
		t.Fatalf("calls:\n%s", calls)
	}
}

func TestRankRefusesASidecarWithoutRank(t *testing.T) {
	r, _ := standInRetriever(t)
	t.Setenv("STANDIN_PROTOCOL", "1.2.0")
	if _, err := r.Rank(context.Background(), t.TempDir(), "q", []RankDocument{{ID: "a", Text: "q"}}, 0); !errors.Is(err, ErrSidecarTooOld) {
		t.Fatalf("err = %v, want ErrSidecarTooOld", err)
	}
}

func TestRankWithoutSidecar(t *testing.T) {
	t.Setenv(sidecar.EnvVar(SidecarBinary), "")
	r := &Retriever{Locator: sidecar.Locator{LookPath: notFound}}
	if _, err := r.Rank(context.Background(), t.TempDir(), "q", nil, 0); !errors.Is(err, ErrSidecarUnavailable) {
		t.Fatalf("err = %v, want ErrSidecarUnavailable", err)
	}
}
