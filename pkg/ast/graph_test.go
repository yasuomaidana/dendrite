package ast

import (
	"errors"
	"testing"

	"Dendrite/pkg/testkit"
)

func TestSymbolGraph_Nodes(t *testing.T) {
	g := NewSymbolGraph()

	node1 := SymbolNode{
		ID:                   "node1",
		Name:                 "FuncA",
		Kind:                 SymbolFunction,
		Language:             LangGo,
		FilePath:             "pkg/a.go",
		StartLine:            1,
		EndLine:              10,
		CyclomaticComplexity: 3,
		CognitiveComplexity:  2,
	}

	// 1. AddNode success
	testkit.Check(t, g.AddNode(node1))

	// 2. AddNode duplicate -> ErrNodeAlreadyExists
	err := g.AddNode(node1)
	if !errors.Is(err, ErrNodeAlreadyExists) {
		t.Fatalf("expected ErrNodeAlreadyExists, got %v", err)
	}

	// 3. GetNode found
	got, found := g.GetNode("node1")
	testkit.Equal(t, "found", found, true)
	testkit.Equal(t, "node ID", got.ID, "node1")
	testkit.Equal(t, "node Name", got.Name, "FuncA")

	// 4. GetNode not found
	_, foundMissing := g.GetNode("missing")
	testkit.Equal(t, "missing found", foundMissing, false)

	// 5. GetAllNodes
	all := g.GetAllNodes()
	testkit.Equal(t, "all count", len(all), 1)
	testkit.Equal(t, "first ID", all[0].ID, "node1")
}

func TestSymbolGraph_EdgesAndTraversal(t *testing.T) {
	g := NewSymbolGraph()

	caller := SymbolNode{ID: "caller", Name: "CallerFunc", CyclomaticComplexity: 2}
	callee := SymbolNode{ID: "callee", Name: "CalleeFunc", CyclomaticComplexity: 5}
	unrelated := SymbolNode{ID: "unrelated", Name: "Unrelated"}

	testkit.Check(t, g.AddNode(caller))
	testkit.Check(t, g.AddNode(callee))
	testkit.Check(t, g.AddNode(unrelated))

	// 1. AddEdge with missing FromID
	errFrom := g.AddEdge(SymbolEdge{FromID: "missing", ToID: "callee", Type: EdgeCalls})
	if !errors.Is(errFrom, ErrNodeNotFound) {
		t.Fatalf("expected ErrNodeNotFound for FromID, got %v", errFrom)
	}

	// 2. AddEdge with missing ToID
	errTo := g.AddEdge(SymbolEdge{FromID: "caller", ToID: "missing", Type: EdgeCalls})
	if !errors.Is(errTo, ErrNodeNotFound) {
		t.Fatalf("expected ErrNodeNotFound for ToID, got %v", errTo)
	}

	// 3. Add valid EdgeCalls
	testkit.Check(t, g.AddEdge(SymbolEdge{FromID: "caller", ToID: "callee", Type: EdgeCalls}))
	// 4. Add non-call edge (e.g. EdgeImports) to ensure filtering works
	testkit.Check(t, g.AddEdge(SymbolEdge{FromID: "caller", ToID: "unrelated", Type: EdgeImports}))

	// 5. GetCallers on callee should return caller
	callers := g.GetCallers("callee")
	testkit.Equal(t, "callers length", len(callers), 1)
	testkit.Equal(t, "caller ID", callers[0].ID, "caller")

	// GetCallers on node with no calls
	testkit.Equal(t, "caller has no callers", len(g.GetCallers("caller")), 0)

	// 6. GetCallees on caller should return callee (ignoring EdgeImports)
	callees := g.GetCallees("caller")
	testkit.Equal(t, "callees length", len(callees), 1)
	testkit.Equal(t, "callee ID", callees[0].ID, "callee")

	// GetCallees on node with no calls
	testkit.Equal(t, "callee has no callees", len(g.GetCallees("callee")), 0)
}

func TestSymbolGraph_QueryBlastRadius(t *testing.T) {
	g := NewSymbolGraph()

	// Diamond graph with cycle:
	// root -> called by A and B -> called by C
	// cycle: C -> root
	root := SymbolNode{ID: "root", Name: "Root", CyclomaticComplexity: 2}
	a := SymbolNode{ID: "A", Name: "ServiceA", CyclomaticComplexity: 7}
	b := SymbolNode{ID: "B", Name: "ServiceB", CyclomaticComplexity: 4}
	c := SymbolNode{ID: "C", Name: "ServiceC", CyclomaticComplexity: 12}

	testkit.Check(t, g.AddNode(root))
	testkit.Check(t, g.AddNode(a))
	testkit.Check(t, g.AddNode(b))
	testkit.Check(t, g.AddNode(c))

	testkit.Check(t, g.AddEdge(SymbolEdge{FromID: "A", ToID: "root", Type: EdgeCalls}))
	testkit.Check(t, g.AddEdge(SymbolEdge{FromID: "B", ToID: "root", Type: EdgeCalls}))
	testkit.Check(t, g.AddEdge(SymbolEdge{FromID: "C", ToID: "A", Type: EdgeCalls}))
	// Cycle: A calls C
	testkit.Check(t, g.AddEdge(SymbolEdge{FromID: "A", ToID: "C", Type: EdgeCalls}))

	// 1. maxDepth <= 0 -> ErrInvalidDepth
	_, errDepth := g.QueryBlastRadius("root", 0)
	if !errors.Is(errDepth, ErrInvalidDepth) {
		t.Fatalf("expected ErrInvalidDepth for depth 0, got %v", errDepth)
	}

	// 2. Non-existent root -> ErrNodeNotFound
	_, errMissing := g.QueryBlastRadius("nonexistent", 2)
	if !errors.Is(errMissing, ErrNodeNotFound) {
		t.Fatalf("expected ErrNodeNotFound, got %v", errMissing)
	}

	// 3. Depth 1: should find direct callers A and B
	rep1, err := g.QueryBlastRadius("root", 1)
	testkit.Check(t, err)
	testkit.Equal(t, "depth 1 total", rep1.TotalAffected, 2)
	testkit.Equal(t, "depth 1 max complexity", rep1.MaxComplexity, 7)

	// 4. Depth 2+: should find A, B, and C with cycle handled cleanly
	rep2, err := g.QueryBlastRadius("root", 5)
	testkit.Check(t, err)
	testkit.Equal(t, "depth 5 total", rep2.TotalAffected, 3)
	testkit.Equal(t, "depth 5 max complexity", rep2.MaxComplexity, 12)
	testkit.Equal(t, "root symbol", rep2.RootSymbol, "root")
	testkit.Equal(t, "max depth", rep2.MaxDepth, 5)
}
