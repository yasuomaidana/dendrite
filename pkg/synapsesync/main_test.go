package synapsesync

import (
	"context"
	"os"
	"testing"
)

// The sync tests exercise git and Plane against real repositories; indexing code needs the
// dendrite-rag binary, so by default they get an indexer that does no work. The tests of the RAG
// step itself pass their own through Options.Indexer.
func TestMain(m *testing.M) {
	defaultIndexer = func(context.Context, string) (RAGResult, error) { return RAGResult{}, nil }
	os.Exit(m.Run())
}
