package slice

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"Dendrite/pkg/ast"
)

func TestIntentSlicer_Debloating80Percent(t *testing.T) {
	graph := ast.NewSymbolGraph()

	// 10 symbols total: 2 core, 8 dead/bloated
	symbols := []string{
		"CoreAlgorithm", "MathHelper",
		"RenderWindow", "DrawButton", "HandleClick",
		"SendTelemetry", "FlushMetrics",
		"CheckUpdate", "DownloadBinary", "InstallUpdate",
	}

	for _, name := range symbols {
		_ = graph.AddNode(ast.SymbolNode{
			ID:       name,
			Name:     name,
			Kind:     ast.SymbolFunction,
			Language: ast.LangGo,
			FilePath: "cmd/bloated/main.go",
		})
	}

	// Edges: CoreAlgorithm calls MathHelper
	_ = graph.AddEdge(ast.SymbolEdge{
		FromID: "CoreAlgorithm",
		ToID:   "MathHelper",
		Type:   ast.EdgeCalls,
	})

	// Bloated edges (GUI, Telemetry, Updater all call each other, but not reachable from CoreAlgorithm)
	_ = graph.AddEdge(ast.SymbolEdge{FromID: "RenderWindow", ToID: "DrawButton", Type: ast.EdgeCalls})
	_ = graph.AddEdge(ast.SymbolEdge{FromID: "DrawButton", ToID: "HandleClick", Type: ast.EdgeCalls})
	_ = graph.AddEdge(ast.SymbolEdge{FromID: "SendTelemetry", ToID: "FlushMetrics", Type: ast.EdgeCalls})
	_ = graph.AddEdge(ast.SymbolEdge{FromID: "CheckUpdate", ToID: "DownloadBinary", Type: ast.EdgeCalls})
	_ = graph.AddEdge(ast.SymbolEdge{FromID: "DownloadBinary", ToID: "InstallUpdate", Type: ast.EdgeCalls})

	slicer := NewIntentSlicer()
	ctx := context.Background()
	tempDir := t.TempDir()
	outDir := filepath.Join(tempDir, "debloated-core")

	res, err := slicer.SliceGraph(ctx, graph, SliceRequest{
		EntryPointSymbol: "CoreAlgorithm",
		Intent:           "extract pure core math calculation without GUI or telemetry",
		Direction:        DirectionForward,
		OutputDir:        outDir,
	})
	if err != nil {
		t.Fatalf("SliceGraph failed: %v", err)
	}

	if res.OriginalSymbolsCount != 10 {
		t.Errorf("expected 10 original symbols, got %d", res.OriginalSymbolsCount)
	}
	if res.RetainedSymbolsCount != 2 {
		t.Errorf("expected 2 retained symbols, got %d", res.RetainedSymbolsCount)
	}

	// Verify >= 80% dead-code pruning ratio
	if res.PruneRatio < 0.80 {
		t.Errorf("expected prune ratio >= 0.80 (80%%), got %.2f", res.PruneRatio)
	}

	// Verify golden vectors
	if len(res.GoldenVectors) != 2 {
		t.Errorf("expected 2 golden vectors, got %d", len(res.GoldenVectors))
	}

	// Verify emitted project files
	if _, err := os.Stat(filepath.Join(outDir, "go.mod")); err != nil {
		t.Errorf("go.mod not emitted in %s: %v", outDir, err)
	}
	if _, err := os.Stat(filepath.Join(outDir, "sliced.go")); err != nil {
		t.Errorf("sliced.go not emitted in %s: %v", outDir, err)
	}
	if _, err := os.Stat(filepath.Join(outDir, "sliced_golden_test.go")); err != nil {
		t.Errorf("sliced_golden_test.go not emitted in %s: %v", outDir, err)
	}
}

func TestIntentSlicer_BackwardAndBidirectional(t *testing.T) {
	graph := ast.NewSymbolGraph()

	_ = graph.AddNode(ast.SymbolNode{ID: "CallerA", Name: "CallerA", Kind: ast.SymbolFunction})
	_ = graph.AddNode(ast.SymbolNode{ID: "TargetB", Name: "TargetB", Kind: ast.SymbolFunction})
	_ = graph.AddNode(ast.SymbolNode{ID: "CalleeC", Name: "CalleeC", Kind: ast.SymbolFunction})
	_ = graph.AddNode(ast.SymbolNode{ID: "DeadD", Name: "DeadD", Kind: ast.SymbolFunction})

	_ = graph.AddEdge(ast.SymbolEdge{FromID: "CallerA", ToID: "TargetB", Type: ast.EdgeCalls})
	_ = graph.AddEdge(ast.SymbolEdge{FromID: "TargetB", ToID: "CalleeC", Type: ast.EdgeCalls})

	slicer := NewIntentSlicer()
	ctx := context.Background()

	// 1. Backward from TargetB should find CallerA and TargetB
	resBack, err := slicer.SliceGraph(ctx, graph, SliceRequest{
		EntryPointSymbol: "TargetB",
		Direction:        DirectionBackward,
	})
	if err != nil {
		t.Fatalf("backward slice failed: %v", err)
	}
	if resBack.RetainedSymbolsCount != 2 {
		t.Errorf("expected 2 symbols in backward slice, got %d", resBack.RetainedSymbolsCount)
	}

	// 2. Bidirectional from TargetB should find CallerA, TargetB, and CalleeC
	resBi, err := slicer.SliceGraph(ctx, graph, SliceRequest{
		EntryPointSymbol: "TargetB",
		Direction:        DirectionBidirectional,
	})
	if err != nil {
		t.Fatalf("bidirectional slice failed: %v", err)
	}
	if resBi.RetainedSymbolsCount != 3 {
		t.Errorf("expected 3 symbols in bidirectional slice, got %d", resBi.RetainedSymbolsCount)
	}
}

func TestIntentSlicer_ValidationErrors(t *testing.T) {
	slicer := NewIntentSlicer()
	ctx := context.Background()

	// Missing entrypoint
	_, err := slicer.Slice(ctx, SliceRequest{
		SourcesPath: ".",
	})
	if !errors.Is(err, ErrMissingParameter) {
		t.Errorf("expected ErrMissingParameter, got %v", err)
	}

	// Missing sources path
	_, err = slicer.Slice(ctx, SliceRequest{
		EntryPointSymbol: "Main",
	})
	if !errors.Is(err, ErrMissingParameter) {
		t.Errorf("expected ErrMissingParameter, got %v", err)
	}

	// Non-existent sources path
	_, err = slicer.Slice(ctx, SliceRequest{
		SourcesPath:      "/non/existent/path/for/sure",
		EntryPointSymbol: "Main",
	})
	if !errors.Is(err, ErrSourceNotFound) {
		t.Errorf("expected ErrSourceNotFound, got %v", err)
	}

	// Entrypoint not in graph
	graph := ast.NewSymbolGraph()
	_, err = slicer.SliceGraph(ctx, graph, SliceRequest{
		EntryPointSymbol: "MissingSymbol",
	})
	if !errors.Is(err, ErrEntryPointNotFound) {
		t.Errorf("expected ErrEntryPointNotFound, got %v", err)
	}
}
