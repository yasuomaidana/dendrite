package ast

import (
	"sync"
)

// SymbolGraph maintains the in-memory directed knowledge graph of symbols and call flows.
type SymbolGraph struct {
	mu       sync.RWMutex
	nodes    map[string]*SymbolNode
	edgesOut map[string][]*SymbolEdge
	edgesIn  map[string][]*SymbolEdge
}

// NewSymbolGraph creates an empty SymbolGraph.
func NewSymbolGraph() *SymbolGraph {
	return &SymbolGraph{
		nodes:    make(map[string]*SymbolNode),
		edgesOut: make(map[string][]*SymbolEdge),
		edgesIn:  make(map[string][]*SymbolEdge),
	}
}

// AddNode registers a symbol in the graph.
func (g *SymbolGraph) AddNode(node SymbolNode) error {
	g.mu.Lock()
	defer g.mu.Unlock()

	if _, exists := g.nodes[node.ID]; exists {
		return ErrNodeAlreadyExists
	}

	cpy := node
	g.nodes[node.ID] = &cpy
	return nil
}

// GetNode retrieves a symbol node by ID.
func (g *SymbolGraph) GetNode(id string) (*SymbolNode, bool) {
	g.mu.RLock()
	defer g.mu.RUnlock()

	node, exists := g.nodes[id]
	if !exists {
		return nil, false
	}
	cpy := *node
	return &cpy, true
}

// AddEdge registers a directed relation from source to destination.
func (g *SymbolGraph) AddEdge(edge SymbolEdge) error {
	g.mu.Lock()
	defer g.mu.Unlock()

	if _, ok := g.nodes[edge.FromID]; !ok {
		return ErrNodeNotFound
	}
	if _, ok := g.nodes[edge.ToID]; !ok {
		return ErrNodeNotFound
	}

	e := edge
	g.edgesOut[edge.FromID] = append(g.edgesOut[edge.FromID], &e)
	g.edgesIn[edge.ToID] = append(g.edgesIn[edge.ToID], &e)
	return nil
}

// GetCallers returns all symbols that directly call or depend on the given symbol.
func (g *SymbolGraph) GetCallers(symbolID string) []*SymbolNode {
	g.mu.RLock()
	defer g.mu.RUnlock()

	inEdges := g.edgesIn[symbolID]
	res := make([]*SymbolNode, 0, len(inEdges))
	for _, e := range inEdges {
		if e.Type == EdgeCalls {
			if node, ok := g.nodes[e.FromID]; ok {
				cpy := *node
				res = append(res, &cpy)
			}
		}
	}
	return res
}

// GetCallees returns all symbols directly invoked by the given symbol.
func (g *SymbolGraph) GetCallees(symbolID string) []*SymbolNode {
	g.mu.RLock()
	defer g.mu.RUnlock()

	outEdges := g.edgesOut[symbolID]
	res := make([]*SymbolNode, 0, len(outEdges))
	for _, e := range outEdges {
		if e.Type == EdgeCalls {
			if node, ok := g.nodes[e.ToID]; ok {
				cpy := *node
				res = append(res, &cpy)
			}
		}
	}
	return res
}

// QueryBlastRadius computes all transitively impacted symbols up to maxDepth with cycle detection.
func (g *SymbolGraph) QueryBlastRadius(symbolID string, maxDepth int) (*BlastRadiusReport, error) {
	if maxDepth <= 0 {
		return nil, ErrInvalidDepth
	}

	g.mu.RLock()
	defer g.mu.RUnlock()

	rootNode, exists := g.nodes[symbolID]
	if !exists {
		return nil, ErrNodeNotFound
	}

	visited := make(map[string]bool)
	visited[symbolID] = true

	queue := []string{symbolID}
	affected := make([]SymbolNode, 0)
	maxComplexity := rootNode.CyclomaticComplexity

	for depth := 0; depth < maxDepth && len(queue) > 0; depth++ {
		nextQueue := make([]string, 0)
		for _, currID := range queue {
			for _, edge := range g.edgesIn[currID] {
				callerID := edge.FromID
				if !visited[callerID] {
					visited[callerID] = true
					if callerNode, ok := g.nodes[callerID]; ok {
						affected = append(affected, *callerNode)
						if callerNode.CyclomaticComplexity > maxComplexity {
							maxComplexity = callerNode.CyclomaticComplexity
						}
						nextQueue = append(nextQueue, callerID)
					}
				}
			}
		}
		queue = nextQueue
	}

	return &BlastRadiusReport{
		RootSymbol:      symbolID,
		MaxDepth:        maxDepth,
		AffectedSymbols: affected,
		TotalAffected:   len(affected),
		MaxComplexity:   maxComplexity,
	}, nil
}

// GetAllNodes returns a slice of all SymbolNodes currently registered in the graph.
func (g *SymbolGraph) GetAllNodes() []*SymbolNode {
	g.mu.RLock()
	defer g.mu.RUnlock()

	res := make([]*SymbolNode, 0, len(g.nodes))
	for _, n := range g.nodes {
		cpy := *n
		res = append(res, &cpy)
	}
	return res
}
