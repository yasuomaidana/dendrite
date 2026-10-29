package ticketlock

import (
	"context"
	"os"
	"path/filepath"
	"reflect"
	"strconv"
	"testing"

	"Dendrite/pkg/rag"
	"Dendrite/pkg/rag/ragtest"
	"Dendrite/pkg/sidecar"
)

func docsOf(texts ...string) []rag.RankDocument {
	docs := make([]rag.RankDocument, len(texts))
	for i, t := range texts {
		docs[i] = rag.RankDocument{ID: strconv.Itoa(i), Text: t}
	}
	return docs
}

func TestLexicalRankPutsTheDocumentsSharingRareWordsFirst(t *testing.T) {
	docs := docsOf("the the the common words", "coverage threshold for the gate", "unrelated text here", "coverage coverage and gate and threshold words")
	got := lexicalRank("coverage threshold gate", docs, 2)
	if !reflect.DeepEqual(got, []string{"3", "1"}) {
		t.Errorf("ranked %v", got)
	}
	if all := lexicalRank("coverage", docs, 10); len(all) != 4 || all[0] != "3" {
		t.Errorf("topK above the count returns every document: %v", all)
	}
}

func TestLexicalRankKeepsTheOrderWhenNothingMatches(t *testing.T) {
	if got := lexicalRank("zzz qqq", docsOf("a b c", "d e f", "g h i"), 2); !reflect.DeepEqual(got, []string{"0", "1"}) {
		t.Errorf("%v", got)
	}
	if got := lexicalRank("anything", nil, 5); len(got) != 0 {
		t.Errorf("no documents: %v", got)
	}
}

func TestSidecarRankerUsesTheSidecarAndFallsBackToWords(t *testing.T) {
	bin, err := ragtest.Build(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	t.Setenv(sidecar.EnvVar("dendrite-rag"), bin)
	docs := docsOf("nothing relevant", "the coverage gate", "also nothing")
	got, err := sidecarRanker(t.TempDir())(context.Background(), "coverage gate", docs, 2)
	if err != nil || len(got) != 2 || got[0] != "1" {
		t.Fatalf("the sidecar's ranking: %v, %v", got, err)
	}

	broken := filepath.Join(t.TempDir(), "not-a-binary")
	if err := os.WriteFile(broken, []byte("not executable"), 0o644); err != nil {
		t.Fatal(err)
	}
	t.Setenv(sidecar.EnvVar("dendrite-rag"), broken)
	got, err = sidecarRanker(t.TempDir())(context.Background(), "coverage gate", docs, 2)
	if err != nil || len(got) != 2 || got[0] != "1" {
		t.Errorf("a sidecar that cannot start falls back to word ranking: %v, %v", got, err)
	}
}
