package mcpserver

import (
	"Dendrite/pkg/synapsesearch"
	"context"
	"encoding/json"
	"fmt"
	"path/filepath"
	"strings"

	"Dendrite/pkg/synapse"
)

// SynapseScope binds a scoped MCP server to one synapse manifest.
type SynapseScope struct {
	Store *synapse.Store
	ID    string
}

// load re-reads the manifest so every call sees the latest path hints and state.
func (sc SynapseScope) load() (*synapse.Synapse, error) {
	return sc.Store.Load(sc.ID)
}

// NewSynapseServer builds an MCP server exposing only synapse_* tools bound to one synapse.
// Unlike the global control-plane server it never registers the default tool set, keeping
// the agent's tool list (and context window) limited to the relationship.
func NewSynapseServer(sc SynapseScope, version string) (*Server, error) {
	syn, err := sc.load()
	if err != nil {
		return nil, fmt.Errorf("loading synapse %q: %w", sc.ID, err)
	}
	s := NewServer(ServerConfig{
		Name:    "Dendrite Synapse MCP Server: " + syn.ID,
		Version: version,
	})
	registerSynapseTools(s, sc)
	return s, nil
}

// ConfineToMember resolves rel inside the root of the named member and rejects any
// path that escapes it (absolute paths, "..", or symlink-free lexical escapes).
func ConfineToMember(syn *synapse.Synapse, alias, rel string) (string, error) {
	m, ok := syn.Member(alias)
	if !ok {
		return "", fmt.Errorf("%q is not a member of synapse %s", alias, syn.ID)
	}
	root, err := synapse.Resolve(*m)
	if err != nil {
		return "", fmt.Errorf("member %s unresolved (%v); run `dendrite synapse heal`", alias, err)
	}
	if rel == "" {
		return root, nil
	}
	if filepath.IsAbs(rel) {
		return "", fmt.Errorf("path %q must be relative to member %s", rel, alias)
	}
	full := filepath.Join(root, rel)
	back, err := filepath.Rel(root, full)
	if err != nil || back == ".." || strings.HasPrefix(back, ".."+string(filepath.Separator)) {
		return "", fmt.Errorf("path %q escapes member %s", rel, alias)
	}
	return full, nil
}

// jsonResult renders v as an indented JSON text result.
func jsonResult(v interface{}) (*CallToolResult, error) {
	data, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return nil, err
	}
	return &CallToolResult{Content: []ContentItem{{Type: "text", Text: string(data)}}}, nil
}

// errorResult renders a tool-level failure the agent can read and react to.
func errorResult(format string, a ...interface{}) *CallToolResult {
	return &CallToolResult{IsError: true, Content: []ContentItem{{Type: "text", Text: fmt.Sprintf(format, a...)}}}
}

func registerSynapseTools(s *Server, sc SynapseScope) {
	s.RegisterTool(Tool{
		Name:        "synapse_info",
		Description: "Describes this synapse: members, roles, contracts, sync strategy and per-member path health",
		InputSchema: map[string]interface{}{"type": "object", "properties": map[string]interface{}{}},
	}, func(ctx context.Context, args map[string]interface{}) (*CallToolResult, error) {
		syn, err := sc.load()
		if err != nil {
			return errorResult("loading synapse: %v", err), nil
		}
		type memberView struct {
			synapse.Member
			Status synapse.MemberStatus `json:"status"`
		}
		members := make([]memberView, 0, len(syn.Members))
		for _, m := range syn.Members {
			members = append(members, memberView{Member: m, Status: synapse.StatusOf(m)})
		}
		return jsonResult(map[string]interface{}{
			"id":        syn.ID,
			"name":      syn.Name,
			"state":     syn.State,
			"members":   members,
			"contracts": syn.Contracts,
			"sync":      syn.Sync,
			"parity":    syn.Parity,
		})
	})

	s.RegisterTool(Tool{
		Name: "synapse_sync_status",
		Description: "Reports where every member of this synapse stands against its tracked ref: source (local/remote), " +
			"locked commit, ahead/behind, dirty files, remote branches with changes the checkout lacks, and drift. " +
			"Read-only: never pulls, links or rewrites the manifest (use `dendrite synapse sync` for that)",
		InputSchema: map[string]interface{}{"type": "object", "properties": map[string]interface{}{
			"fetch": map[string]interface{}{"type": "boolean", "description": "Fetch remotes and resolve remote refs first (network); default false"},
		}},
	}, func(ctx context.Context, args map[string]interface{}) (*CallToolResult, error) {
		syn, err := sc.load()
		if err != nil {
			return errorResult("loading synapse: %v", err), nil
		}
		fetch, _ := args["fetch"].(bool)
		return jsonResult(synapse.SyncSynapse(ctx, syn, synapse.SyncOptions{NoFetch: !fetch}))
	})

	s.RegisterTool(Tool{
		Name: "synapse_search",
		Description: "Searches the code and documentation of every member of this synapse with one query and merges the answers " +
			"by rank, each hit tagged with its member. Use it when a question crosses projects (who calls this API, where is " +
			"this contract implemented). Hits are chunk outlines with paths relative to the member's root",
		InputSchema: map[string]interface{}{"type": "object", "properties": map[string]interface{}{
			"query":   map[string]interface{}{"type": "string", "description": "What to look for"},
			"top_k":   map[string]interface{}{"type": "integer", "description": "Merged hits to return (default 10)"},
			"corpus":  map[string]interface{}{"type": "string", "enum": []interface{}{"all", "code", "docs"}, "description": "Corpora searched (default all)"},
			"members": map[string]interface{}{"type": "array", "items": map[string]interface{}{"type": "string"}, "description": "Only these member aliases (default: all)"},
		}, "required": []interface{}{"query"}},
	}, func(ctx context.Context, args map[string]interface{}) (*CallToolResult, error) {
		query, _ := args["query"].(string)
		syn, err := sc.load()
		if err != nil {
			return errorResult("loading synapse: %v", err), nil
		}
		opts := synapsesearch.Options{}
		if v, ok := args["top_k"].(float64); ok && v > 0 {
			opts.TopK = int(v)
		}
		opts.Corpus, _ = args["corpus"].(string)
		if list, ok := args["members"].([]interface{}); ok {
			for _, m := range list {
				if alias, ok := m.(string); ok {
					opts.Members = append(opts.Members, alias)
				}
			}
		}
		hits, notes, err := synapsesearch.Search(ctx, syn, query, opts, nil)
		if err != nil {
			return errorResult("%v", err), nil
		}
		return jsonResult(map[string]interface{}{"hits": hits, "skipped_members": notes})
	})
}
