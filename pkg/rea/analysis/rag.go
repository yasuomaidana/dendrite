package analysis

import (
	"context"
	"path/filepath"
	"strings"

	"Dendrite/pkg/rag"
)

// RagIndex is the Indexer over the RAG sidecar: each analysis workspace is its own corpus.
type RagIndex struct{ Retriever *rag.Retriever }

// NewRagIndex searches the default sidecar locations.
func NewRagIndex() *RagIndex { return &RagIndex{Retriever: rag.NewRetriever()} }

// Index brings the index of the workspace up to date.
func (r *RagIndex) Index(ctx context.Context, dir string) error {
	_, err := r.Retriever.Index(ctx, dir, rag.IndexOptions{})
	return err
}

// Search returns the k passages of the workspace that best match query.
func (r *RagIndex) Search(ctx context.Context, dir, query string, k int) ([]Hit, error) {
	got, err := r.Retriever.Retrieve(ctx, dir, query, rag.SearchOptions{TopK: k}, false)
	if err != nil {
		return nil, err
	}
	hits := make([]Hit, 0, len(got.Chunks))
	for _, c := range got.Chunks {
		hits = append(hits, Hit{File: relativeTo(dir, c.FilePath), Heading: c.Heading, Text: c.Content, Score: c.Score})
	}
	return hits, nil
}

// Purge deletes the index of the workspace.
func (r *RagIndex) Purge(ctx context.Context, dir string) error {
	return r.Retriever.Purge(ctx, dir)
}

// relativeTo names path relative to dir when it lies inside it, so a hit reads imports/kernel32.dll.md.
func relativeTo(dir, path string) string {
	if rel, err := filepath.Rel(dir, path); err == nil && !strings.HasPrefix(rel, "..") {
		return filepath.ToSlash(rel)
	}
	return path
}
