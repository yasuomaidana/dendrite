package ast_test

import (
	"errors"
	"testing"

	"Dendrite/pkg/ast"
)

func TestSymbolGraph_BasicOperations(t *testing.T) {
	g := ast.NewSymbolGraph()

	nodeA := ast.SymbolNode{
		ID:                   "pkg/auth.Login",
		Name:                 "Login",
		Kind:                 ast.SymbolFunction,
		Language:             ast.LangGo,
		FilePath:             "pkg/auth/login.go",
		StartLine:            10,
		EndLine:              25,
		CyclomaticComplexity: 3,
		CognitiveComplexity:  2,
	}

	// 1. AddNode
	if err := g.AddNode(nodeA); err != nil {
		t.Fatalf("failed to add node: %v", err)
	}

	// 2. Duplicate AddNode error
	if err := g.AddNode(nodeA); !errors.Is(err, ast.ErrNodeAlreadyExists) {
		t.Errorf("expected ErrNodeAlreadyExists on duplicate add, got: %v", err)
	}

	// 3. GetNode
	retrieved, ok := g.GetNode("pkg/auth.Login")
	if !ok || retrieved.Name != "Login" {
		t.Errorf("expected to retrieve Login node, got ok=%v", ok)
	}

	_, okNonExistent := g.GetNode("nonexistent")
	if okNonExistent {
		t.Errorf("expected false for nonexistent node")
	}

	// 4. AddEdge errors
	badEdge := ast.SymbolEdge{FromID: "pkg/auth.Login", ToID: "missing", Type: ast.EdgeCalls}
	if err := g.AddEdge(badEdge); !errors.Is(err, ast.ErrNodeNotFound) {
		t.Errorf("expected ErrNodeNotFound for missing target node")
	}

	badEdgeFrom := ast.SymbolEdge{FromID: "missing", ToID: "pkg/auth.Login", Type: ast.EdgeCalls}
	if err := g.AddEdge(badEdgeFrom); !errors.Is(err, ast.ErrNodeNotFound) {
		t.Errorf("expected ErrNodeNotFound for missing source node")
	}
}

func TestSymbolGraph_CallersCalleesAndBlastRadius(t *testing.T) {
	g := ast.NewSymbolGraph()

	// Build chain: Controller -> Service -> Repository
	controller := ast.SymbolNode{ID: "Controller", Name: "Controller", CyclomaticComplexity: 2}
	service := ast.SymbolNode{ID: "Service", Name: "Service", CyclomaticComplexity: 4}
	repo := ast.SymbolNode{ID: "Repository", Name: "Repository", CyclomaticComplexity: 1}

	_ = g.AddNode(controller)
	_ = g.AddNode(service)
	_ = g.AddNode(repo)

	_ = g.AddEdge(ast.SymbolEdge{FromID: "Controller", ToID: "Service", Type: ast.EdgeCalls})
	_ = g.AddEdge(ast.SymbolEdge{FromID: "Service", ToID: "Repository", Type: ast.EdgeCalls})

	// Callers of Repository should be [Service]
	callers := g.GetCallers("Repository")
	if len(callers) != 1 || callers[0].ID != "Service" {
		t.Errorf("expected caller of Repository to be Service, got %v", callers)
	}

	// Callees of Controller should be [Service]
	callees := g.GetCallees("Controller")
	if len(callees) != 1 || callees[0].ID != "Service" {
		t.Errorf("expected callee of Controller to be Service, got %v", callees)
	}

	// Blast radius of Repository at depth 2 should affect Service and Controller
	report, err := g.QueryBlastRadius("Repository", 2)
	if err != nil {
		t.Fatalf("unexpected error querying blast radius: %v", err)
	}

	if report.TotalAffected != 2 {
		t.Errorf("expected 2 affected symbols, got %d", report.TotalAffected)
	}
	if report.MaxComplexity != 4 {
		t.Errorf("expected max complexity 4, got %d", report.MaxComplexity)
	}

	// Test depth limits
	shallowReport, _ := g.QueryBlastRadius("Repository", 1)
	if shallowReport.TotalAffected != 1 || shallowReport.AffectedSymbols[0].ID != "Service" {
		t.Errorf("expected shallow report (depth 1) to only contain Service, got: %v", shallowReport.AffectedSymbols)
	}

	// Test invalid depth
	if _, err := g.QueryBlastRadius("Repository", 0); !errors.Is(err, ast.ErrInvalidDepth) {
		t.Errorf("expected ErrInvalidDepth for depth 0")
	}

	// Test nonexistent node
	if _, err := g.QueryBlastRadius("unknown", 2); !errors.Is(err, ast.ErrNodeNotFound) {
		t.Errorf("expected ErrNodeNotFound for unknown node")
	}
}

func TestSymbolGraph_CircularDependencyGuardrail(t *testing.T) {
	g := ast.NewSymbolGraph()

	// Create circular loop: NodeA -> NodeB -> NodeC -> NodeA
	nodeA := ast.SymbolNode{ID: "NodeA", Name: "NodeA", CyclomaticComplexity: 2}
	nodeB := ast.SymbolNode{ID: "NodeB", Name: "NodeB", CyclomaticComplexity: 3}
	nodeC := ast.SymbolNode{ID: "NodeC", Name: "NodeC", CyclomaticComplexity: 5}

	_ = g.AddNode(nodeA)
	_ = g.AddNode(nodeB)
	_ = g.AddNode(nodeC)

	_ = g.AddEdge(ast.SymbolEdge{FromID: "NodeA", ToID: "NodeB", Type: ast.EdgeCalls})
	_ = g.AddEdge(ast.SymbolEdge{FromID: "NodeB", ToID: "NodeC", Type: ast.EdgeCalls})
	_ = g.AddEdge(ast.SymbolEdge{FromID: "NodeC", ToID: "NodeA", Type: ast.EdgeCalls})

	// Query blast radius with large depth on circular graph
	report, err := g.QueryBlastRadius("NodeA", 10)
	if err != nil {
		t.Fatalf("unexpected error on circular graph: %v", err)
	}

	// Must terminate cleanly and not duplicate nodes in visited set
	if report.TotalAffected != 2 { // NodeC and NodeB (excluding root NodeA)
		t.Errorf("expected exactly 2 affected nodes in circle, got %d", report.TotalAffected)
	}
}

func TestAnalyzeSourceComplexity(t *testing.T) {
	// Simple code: baseline CC = 1, Cog = 0
	simpleCode := `
func Add(a, b int) int {
	return a + b
}
`
	cc, cog := ast.AnalyzeSourceComplexity(simpleCode)
	if cc != 1 || cog != 0 {
		t.Errorf("expected CC=1, Cog=0 for simple code, got CC=%d, Cog=%d", cc, cog)
	}

	// Nested branching code
	nestedCode := `
func Process(items []int) int {
	total := 0
	for _, item := range items {
		if item > 0 {
			if item%2 == 0 && item < 100 {
				total += item
			}
		}
	}
	return total
}
`
	nestedCC, nestedCog := ast.AnalyzeSourceComplexity(nestedCode)
	if nestedCC <= 3 {
		t.Errorf("expected higher cyclomatic complexity for nested code, got %d", nestedCC)
	}
	if nestedCog <= 2 {
		t.Errorf("expected cognitive complexity penalty for nested conditionals, got %d", nestedCog)
	}
}
