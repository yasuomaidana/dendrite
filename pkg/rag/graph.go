package rag

import (
	"context"
	"errors"
	"fmt"
	"strings"
)

// CommunitiesOptions controls Leiden community listing parameters.
type CommunitiesOptions struct {
	Limit   int `json:"limit,omitempty"`
	MinSize int `json:"min_size,omitempty"`
}

// TopSymbol represents a high-in-degree symbol defined within a community or file.
type TopSymbol struct {
	Name     string `json:"name"`
	Path     string `json:"path"`
	Line     int    `json:"line"`
	InDegree int    `json:"in_degree"`
}

// Community describes a Leiden cluster of files with top symbols and summary.
type Community struct {
	ID         int         `json:"id"`
	Label      string      `json:"label"`
	Size       int         `json:"size"`
	Files      []string    `json:"files"`
	TopSymbols []TopSymbol `json:"top_symbols"`
	Summary    string      `json:"summary"`
}

// CommunityRef references a community by its stable id and label.
type CommunityRef struct {
	ID    int    `json:"id"`
	Label string `json:"label"`
}

// GodNodesOptions specifies filters for retrieving central symbols.
type GodNodesOptions struct {
	MinInDegree int `json:"min_in_degree,omitempty"`
	Limit       int `json:"limit,omitempty"`
}

// GodNode is a symbol with prominent callers and type users across the project.
type GodNode struct {
	Symbol        string        `json:"symbol"`
	Path          string        `json:"path"`
	Line          int           `json:"line"`
	InDegree      int           `json:"in_degree"`
	OutDegree     int           `json:"out_degree"`
	UsedFromFiles int           `json:"used_from_files"`
	Community     *CommunityRef `json:"community,omitempty"`
	Summary       string        `json:"summary"`
}

// PathOptions specifies search constraints for graph traversal.
type PathOptions struct {
	Kinds   []string `json:"kinds,omitempty"`
	MaxHops int      `json:"max_hops,omitempty"`
}

// PathStep represents a single node transition in a graph path.
type PathStep struct {
	Node     string `json:"node"`
	Location string `json:"location,omitempty"`
	Via      string `json:"via,omitempty"`
	Reversed bool   `json:"reversed,omitempty"`
}

// GraphPath holds the shortest path found between two symbols or files.
type GraphPath struct {
	Found    bool       `json:"found"`
	Directed bool       `json:"directed"`
	Level    string     `json:"level"`
	Steps    []PathStep `json:"steps"`
}

// Site locates where a symbol is defined.
type Site struct {
	Path  string `json:"path"`
	Line  int    `json:"line"`
	Chunk string `json:"chunk"`
}

// Neighbour describes an adjacent symbol in the code graph.
type Neighbour struct {
	Symbol string `json:"symbol"`
	Path   string `json:"path"`
	Line   int    `json:"line"`
	Kind   string `json:"kind"`
}

// Explanation describes the structural relationships of a symbol or a file.
type Explanation struct {
	Kind            string        `json:"kind"` // "symbol" or "file"
	Symbol          string        `json:"symbol,omitempty"`
	Path            string        `json:"path,omitempty"`
	Sites           []Site        `json:"sites,omitempty"`
	Community       *CommunityRef `json:"community,omitempty"`
	GodNode         bool          `json:"god_node,omitempty"`
	InDegree        int           `json:"in_degree,omitempty"`
	OutDegree       int           `json:"out_degree,omitempty"`
	Users           []Neighbour   `json:"users,omitempty"`
	Uses            []Neighbour   `json:"uses,omitempty"`
	Defines         []TopSymbol   `json:"defines,omitempty"`
	Imports         []string      `json:"imports,omitempty"`
	ImportedBy      []string      `json:"imported_by,omitempty"`
	ExternalImports []string      `json:"external_imports,omitempty"`
	DependsOn       []string      `json:"depends_on,omitempty"`
	UsedBy          []string      `json:"used_by,omitempty"`
	Summary         string        `json:"summary"`
}

// runGraph locates the sidecar, indexes projectPath if needed, and invokes query.
func (r *Retriever) runGraph(ctx context.Context, projectPath string, query func(c *conn, root string) error) error {
	if projectPath == "" {
		projectPath = "."
	}
	root, _, err := projectRoot(projectPath)
	if err != nil {
		return err
	}
	bin, err := r.Locator.Locate(SidecarBinary)
	if err != nil {
		return fmt.Errorf("%w: %v; %s", ErrSidecarUnavailable, err, InstallHint)
	}
	c, err := r.connect(ctx, bin)
	if err != nil {
		return fmt.Errorf("%w: %s: %v", ErrSidecarUnavailable, bin, err)
	}

	indexParams := map[string]interface{}{"root": root, "full": false}
	if r.EmbedBudget > 0 {
		indexParams["embed_budget_ms"] = r.EmbedBudget.Milliseconds()
	}
	var report sidecarIndexReport
	if err := c.Call(ctx, "index", indexParams, &report); err != nil {
		return fmt.Errorf("index %s: %w", root, err)
	}
	return query(c, root)
}

// Communities returns the Leiden communities of the project's file graph.
func (r *Retriever) Communities(ctx context.Context, projectPath string, opts CommunitiesOptions) ([]Community, error) {
	if r == nil {
		r = NewRetriever()
	}
	var communities []Community
	err := r.runGraph(ctx, projectPath, func(c *conn, root string) error {
		params := map[string]interface{}{"root": root}
		if opts.Limit > 0 {
			params["limit"] = opts.Limit
		}
		if opts.MinSize > 0 {
			params["min_size"] = opts.MinSize
		}
		var res struct {
			Communities []Community `json:"communities"`
		}
		if err := c.Call(ctx, "graph.communities", params, &res); err != nil {
			return fmt.Errorf("graph.communities: %w", err)
		}
		communities = res.Communities
		return nil
	})
	return communities, err
}

// GodNodes returns the symbols with the most callers and type users in the project.
func (r *Retriever) GodNodes(ctx context.Context, projectPath string, opts GodNodesOptions) ([]GodNode, error) {
	if r == nil {
		r = NewRetriever()
	}
	var nodes []GodNode
	err := r.runGraph(ctx, projectPath, func(c *conn, root string) error {
		params := map[string]interface{}{"root": root}
		if opts.MinInDegree > 0 {
			params["min_in_degree"] = opts.MinInDegree
		}
		if opts.Limit > 0 {
			params["limit"] = opts.Limit
		}
		var res struct {
			Nodes []GodNode `json:"nodes"`
		}
		if err := c.Call(ctx, "graph.god_nodes", params, &res); err != nil {
			return fmt.Errorf("graph.god_nodes: %w", err)
		}
		nodes = res.Nodes
		return nil
	})
	return nodes, err
}

// Path finds the shortest path between two symbols or files in the code graph.
func (r *Retriever) Path(ctx context.Context, projectPath, from, to string, opts PathOptions) (GraphPath, error) {
	if r == nil {
		r = NewRetriever()
	}
	if strings.TrimSpace(from) == "" || strings.TrimSpace(to) == "" {
		return GraphPath{}, errors.New("from and to are required")
	}
	var path GraphPath
	err := r.runGraph(ctx, projectPath, func(c *conn, root string) error {
		params := map[string]interface{}{
			"root": root,
			"from": from,
			"to":   to,
		}
		if len(opts.Kinds) > 0 {
			params["kinds"] = opts.Kinds
		}
		if opts.MaxHops > 0 {
			params["max_hops"] = opts.MaxHops
		}
		if err := c.Call(ctx, "graph.path", params, &path); err != nil {
			return fmt.Errorf("graph.path: %w", err)
		}
		return nil
	})
	return path, err
}

// Explain explains a symbol or file: callers, callees, definitions, imports, and community membership.
func (r *Retriever) Explain(ctx context.Context, projectPath, target string) ([]Explanation, error) {
	if r == nil {
		r = NewRetriever()
	}
	if strings.TrimSpace(target) == "" {
		return nil, errors.New("target is required")
	}
	var matches []Explanation
	err := r.runGraph(ctx, projectPath, func(c *conn, root string) error {
		params := map[string]interface{}{
			"root":   root,
			"target": target,
		}
		var res struct {
			Matches []Explanation `json:"matches"`
		}
		if err := c.Call(ctx, "graph.explain", params, &res); err != nil {
			return fmt.Errorf("graph.explain: %w", err)
		}
		matches = res.Matches
		return nil
	})
	return matches, err
}

// Communities returns the Leiden communities of the project's file graph using default settings.
func Communities(ctx context.Context, projectPath string, opts CommunitiesOptions) ([]Community, error) {
	return NewRetriever().Communities(ctx, projectPath, opts)
}

// GodNodes returns the symbols with the most callers and type users using default settings.
func GodNodes(ctx context.Context, projectPath string, opts GodNodesOptions) ([]GodNode, error) {
	return NewRetriever().GodNodes(ctx, projectPath, opts)
}

// Path finds the shortest path between two symbols or files using default settings.
func Path(ctx context.Context, projectPath, from, to string, opts PathOptions) (GraphPath, error) {
	return NewRetriever().Path(ctx, projectPath, from, to, opts)
}

// Explain explains a symbol or file using default settings.
func Explain(ctx context.Context, projectPath, target string) ([]Explanation, error) {
	return NewRetriever().Explain(ctx, projectPath, target)
}
