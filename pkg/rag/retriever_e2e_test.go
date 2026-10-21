//go:build e2e

package rag

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"Dendrite/pkg/sidecar"
)

// TestRetrieverRealSidecar runs against a built dendrite-rag-rs binary:
//
//	DENDRITE_RAG_REAL_BIN=../dendrite-rag-rs/target/release/dendrite-rag go test -tags e2e ./pkg/rag -run RealSidecar
func TestRetrieverRealSidecar(t *testing.T) {
	bin := os.Getenv("DENDRITE_RAG_REAL_BIN")
	if bin == "" {
		t.Fatal("set DENDRITE_RAG_REAL_BIN to a built dendrite-rag binary")
	}
	t.Setenv(sidecar.EnvVar(SidecarBinary), bin)
	t.Setenv("DENDRITE_RAG_EMBED_BACKEND", "none")
	dir := ragProject(t)
	r := &Retriever{Locator: sidecar.Locator{LookPath: notFound}, EmbedBudget: time.Second}
	got, err := r.Retrieve(context.Background(), dir, "LocateBinary", SearchOptions{TopK: 3}, false)
	if err != nil {
		t.Fatal(err)
	}
	if got.Outcome.Origin != sidecar.OriginSidecar || len(got.Chunks) == 0 || got.Chunks[0].FilePath != filepath.Join(dir, "demo.go") {
		t.Fatalf("got %+v", got)
	}
}
