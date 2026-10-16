package rag

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestTokenize(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected []string
	}{
		{
			name:     "empty text",
			input:    "",
			expected: []string{},
		},
		{
			name:     "punctuation and mixed case",
			input:    "Check Available LLMs: Ollama, OxiLLaMa & LiteLLM-v1!",
			expected: []string{"check", "available", "llms", "ollama", "oxillama", "litellm", "v1"},
		},
		{
			name:     "alphanumeric numbers and code symbols",
			input:    "dendrite dev inference list --json",
			expected: []string{"dendrite", "dev", "inference", "list", "json"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			actual := Tokenize(tt.input)
			if len(actual) != len(tt.expected) {
				t.Fatalf("expected %d tokens, got %d: %+v", len(tt.expected), len(actual), actual)
			}
			for i := range actual {
				if actual[i] != tt.expected[i] {
					t.Errorf("token %d: expected %s, got %s", i, tt.expected[i], actual[i])
				}
			}
		})
	}
}

func TestIndexer_IndexProject_And_Retrieve(t *testing.T) {
	tmpDir := t.TempDir()

	// 1. Create a markdown doc
	mdPath := filepath.Join(tmpDir, "llm-guide.md")
	mdContent := `# LLM Inference Guide

## Checking Available LLMs
To inspect and check available LLMs in Dendrite, execute:
` + "```bash" + `
dendrite inference list
dendrite dev inference zero-cost list
` + "```" + `
This queries registered Ollama, OxiLLaMa, and LiteLLM engines.

## Starting OxiLLaMa Server
You can launch pure-Rust OxiLLaMa using:
dendrite dev inference serve --port 8080
`
	if err := os.WriteFile(mdPath, []byte(mdContent), 0644); err != nil {
		t.Fatalf("failed to write test md: %v", err)
	}

	// 2. Create a source code file
	goPath := filepath.Join(tmpDir, "inference.go")
	goContent := `package main

import "fmt"

func listModels() {
    fmt.Println("Listing all registered LLM engines and models")
}
`
	if err := os.WriteFile(goPath, []byte(goContent), 0644); err != nil {
		t.Fatalf("failed to write test go file: %v", err)
	}

	// 3. Create a sub-directory that should be ignored
	ignoredDir := filepath.Join(tmpDir, "node_modules", "somepkg")
	if err := os.MkdirAll(ignoredDir, 0755); err != nil {
		t.Fatalf("failed to create ignored dir: %v", err)
	}
	_ = os.WriteFile(filepath.Join(ignoredDir, "package.json"), []byte(`{"name": "ignored"}`), 0644)

	// Create a hidden dir that should be ignored
	hiddenDir := filepath.Join(tmpDir, ".git")
	if err := os.MkdirAll(hiddenDir, 0755); err != nil {
		t.Fatalf("failed to create hidden dir: %v", err)
	}
	_ = os.WriteFile(filepath.Join(hiddenDir, "config"), []byte(`[core]`), 0644)

	ctx := context.Background()
	indexer := NewIndexer()

	// Index directory
	if err := indexer.IndexProject(ctx, tmpDir); err != nil {
		t.Fatalf("IndexProject failed: %v", err)
	}

	if indexer.ChunksCount() == 0 {
		t.Fatalf("expected non-zero indexed chunks, got 0")
	}

	// Retrieve with user's example query: "I want to know how to check available LLMs"
	query := "I want to know how to check available LLMs"
	chunks, err := indexer.Retrieve(ctx, query, SearchOptions{
		TopK:      3,
		MinScore:  0.01,
		MaxTokens: 1000,
	})
	if err != nil {
		t.Fatalf("Retrieve failed: %v", err)
	}
	if len(chunks) == 0 {
		t.Fatalf("expected chunks retrieved for LLM query, got 0")
	}

	topChunk := chunks[0]
	if !strings.Contains(topChunk.Content, "dendrite inference list") {
		t.Errorf("expected top chunk to contain 'dendrite inference list', got: %s", topChunk.Content)
	}

	// Test Retrieve edge cases
	emptyChunks, err := indexer.Retrieve(ctx, "", DefaultSearchOptions())
	if err != nil || len(emptyChunks) != 0 {
		t.Errorf("expected nil chunks on empty query, got: %+v (err: %v)", emptyChunks, err)
	}

	canceledCtx, cancel := context.WithCancel(ctx)
	cancel()
	_, cErr := indexer.Retrieve(canceledCtx, query, DefaultSearchOptions())
	if cErr == nil {
		t.Errorf("expected error on cancelled context")
	}

	// Test indexing non-existent path
	badIdx := NewIndexer()
	if err := badIdx.IndexProject(ctx, filepath.Join(tmpDir, "does-not-exist")); err == nil {
		t.Errorf("expected error when indexing non-existent path")
	}

	// Test indexing a single file directly
	singleIdx := NewIndexer()
	if err := singleIdx.IndexProject(ctx, mdPath); err != nil {
		t.Errorf("failed to index single file: %v", err)
	}
	if singleIdx.ChunksCount() == 0 {
		t.Errorf("expected chunks when indexing single file")
	}
}

func TestIndexer_ASTChunking(t *testing.T) {
	tmpDir := t.TempDir()

	goPath := filepath.Join(tmpDir, "calculator.go")
	goCode := `// Package math provides high-precision arithmetic algorithms.
package math

// Calculator defines arithmetic operations contract.
type Calculator interface {
	Compute(a, b int) int
}

// Engine implements Calculator with state.
type Engine struct {
	Base int
}

// Compute executes arithmetic logic with branching.
func (e *Engine) Compute(a, b int) int {
	if a > 0 && b > 0 {
		return a + b + e.Base
	} else if a < 0 {
		return -a
	}
	return 0
}
`
	if err := os.WriteFile(goPath, []byte(goCode), 0644); err != nil {
		t.Fatalf("failed to write test code: %v", err)
	}

	idx := NewIndexer()
	if err := idx.IndexProject(context.Background(), tmpDir); err != nil {
		t.Fatalf("IndexProject failed: %v", err)
	}

	if idx.ChunksCount() < 3 {
		t.Fatalf("expected at least 3 AST chunks (interface, struct, method), got %d", idx.ChunksCount())
	}

	// Query for Calculator interface
	resInterface, err := idx.Retrieve(context.Background(), "Calculator interface contract", SearchOptions{TopK: 1, MinScore: 0.01})
	if err != nil || len(resInterface) == 0 {
		t.Fatalf("failed to retrieve interface chunk: %v", err)
	}
	if !strings.Contains(resInterface[0].Heading, "Calculator interface") {
		t.Errorf("expected Calculator interface in heading, got: %s", resInterface[0].Heading)
	}

	// Query for Compute method
	resMethod, err := idx.Retrieve(context.Background(), "func Engine Compute cyclomatic", SearchOptions{TopK: 1, MinScore: 0.01})
	if err != nil || len(resMethod) == 0 {
		t.Fatalf("failed to retrieve method chunk: %v", err)
	}
	if !strings.Contains(resMethod[0].Heading, "Compute") || !strings.Contains(resMethod[0].Heading, "cyclomatic:") {
		t.Errorf("expected Compute method with cyclomatic complexity in heading, got: %s", resMethod[0].Heading)
	}
}

func TestIndexer_EmptyCorpus(t *testing.T) {
	idx := NewIndexer()
	chunks, err := idx.Retrieve(context.Background(), "hello", DefaultSearchOptions())
	if err != nil {
		t.Fatalf("unexpected error on empty index: %v", err)
	}
	if len(chunks) != 0 {
		t.Fatalf("expected 0 chunks on empty index, got %d", len(chunks))
	}
}

func TestIndexer_ChunkCodeOrText(t *testing.T) {
	tmpDir := t.TempDir()
	txtPath := filepath.Join(tmpDir, "sample.txt")
	var lines []string
	for i := 1; i <= 65; i++ {
		lines = append(lines, fmt.Sprintf("line %d: This is sample configuration and text content for testing text chunker.", i))
	}
	_ = os.WriteFile(txtPath, []byte(strings.Join(lines, "\n")), 0644)

	idx := NewIndexer()
	err := idx.IndexProject(context.Background(), tmpDir)
	if err != nil {
		t.Fatalf("failed to index txt: %v", err)
	}
	if idx.ChunksCount() == 0 {
		t.Errorf("expected chunks from txt file")
	}
}

func TestIndexer_IncrementalCaching_And_FileModification(t *testing.T) {
	tmpDir := t.TempDir()

	file1 := filepath.Join(tmpDir, "doc1.md")
	content1 := "# Architecture Guide\nCentral RAG server coordinates shared environments and multiproject indexing.\n"
	_ = os.WriteFile(file1, []byte(content1), 0644)

	ctx := context.Background()
	idx := NewIndexer()

	// 1. Initial run: should index doc1 and create cache
	status1, err := idx.IndexProjectIncremental(ctx, tmpDir, false)
	if err != nil {
		t.Fatalf("first indexing failed: %v", err)
	}
	if status1 != "incremental" && status1 != "refreshed" {
		t.Errorf("expected status 'incremental' or 'refreshed' on first run, got: %s", status1)
	}

	// 2. Second run without changes: should be "cached"
	idx2 := NewIndexer()
	status2, err := idx2.IndexProjectIncremental(ctx, tmpDir, false)
	if err != nil {
		t.Fatalf("second indexing failed: %v", err)
	}
	if status2 != "cached" {
		t.Errorf("expected status 'cached' on unchanged directory, got: %s", status2)
	}

	// 3. Add file2: should be incremental
	file2 := filepath.Join(tmpDir, "doc2.md")
	content2 := "# Hybrid Mode\nLocal first draft with paid cloud refinement eliminates hallucinations.\n"
	_ = os.WriteFile(file2, []byte(content2), 0644)

	status3, err := idx2.IndexProjectIncremental(ctx, tmpDir, false)
	if err != nil {
		t.Fatalf("third indexing failed: %v", err)
	}
	if status3 != "incremental" {
		t.Errorf("expected status 'incremental' when new file added, got: %s", status3)
	}

	// Verify file2 chunk can be retrieved
	chunks, err := idx2.Retrieve(ctx, "Hybrid Mode paid refinement", DefaultSearchOptions())
	if err != nil || len(chunks) == 0 {
		t.Fatalf("failed to retrieve chunk from new file: %v", err)
	}

	// 4. Delete file1: should prune chunks incrementally
	_ = os.Remove(file1)
	status4, err := idx2.IndexProjectIncremental(ctx, tmpDir, false)
	if err != nil {
		t.Fatalf("fourth indexing failed: %v", err)
	}
	if status4 != "incremental" {
		t.Errorf("expected status 'incremental' on file deletion, got: %s", status4)
	}

	// 5. Force refresh: should be "refreshed"
	status5, err := idx2.IndexProjectIncremental(ctx, tmpDir, true)
	if err != nil {
		t.Fatalf("force refresh failed: %v", err)
	}
	if status5 != "refreshed" {
		t.Errorf("expected status 'refreshed' on forceRefresh, got: %s", status5)
	}
}

func TestIndexer_SaveAndLoadCache(t *testing.T) {
	idx := NewIndexer()
	idx.AddChunk(Chunk{
		ID:       "c1",
		FilePath: "foo.md",
		Heading:  "Foo Header",
		Content:  "Foo content for testing",
	})

	cachePath := filepath.Join(t.TempDir(), "rag_cache.json")
	if err := idx.SaveCache(cachePath); err != nil {
		t.Fatalf("SaveCache failed: %v", err)
	}

	idxLoaded := NewIndexer()
	if err := idxLoaded.LoadCache(cachePath); err != nil {
		t.Fatalf("LoadCache failed: %v", err)
	}

	if idxLoaded.ChunksCount() != 1 {
		t.Fatalf("expected 1 chunk loaded, got %d", idxLoaded.ChunksCount())
	}

	// Test invalid cache load
	badIdx := NewIndexer()
	if err := badIdx.LoadCache(filepath.Join(t.TempDir(), "non-existent.json")); err == nil {
		t.Errorf("expected error on non-existent cache file")
	}
}

func TestIndexer_RemoveFile(t *testing.T) {
	idx := NewIndexer()
	idx.AddChunk(Chunk{
		ID:       "c1",
		FilePath: "fileA.md",
		Content:  "alpha beta gamma",
	})
	idx.AddChunk(Chunk{
		ID:       "c2",
		FilePath: "fileB.md",
		Content:  "delta epsilon zeta",
	})

	if idx.ChunksCount() != 2 {
		t.Fatalf("expected 2 chunks, got %d", idx.ChunksCount())
	}

	idx.RemoveFile("fileA.md")
	if idx.ChunksCount() != 1 {
		t.Errorf("expected 1 chunk after removing fileA, got %d", idx.ChunksCount())
	}

	// Removing non-existent file should be a no-op
	idx.RemoveFile("non-existent.md")
	if idx.ChunksCount() != 1 {
		t.Errorf("expected 1 chunk, got %d", idx.ChunksCount())
	}
}
