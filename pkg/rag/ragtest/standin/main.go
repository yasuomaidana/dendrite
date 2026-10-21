// Command standin speaks the dendrite-rag sidecar protocol for Dendrite's tests, which
// must not depend on a Rust build. It does no ranking of its own: search and context
// return the project's text files that contain a query word, one whole-file span each,
// most matching words first, then by path. index reports files_changed 1 and
// embed_pending 7. STANDIN_LOG names a file that records each call's method and params;
// STANDIN_FAIL=<method> makes that method return an engine error.
package main

import (
	"bufio"
	"encoding/json"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

type request struct {
	ID     int64           `json:"id"`
	Method string          `json:"method"`
	Params json.RawMessage `json:"params"`
}

type params struct {
	Root        string   `json:"root"`
	Query       string   `json:"query"`
	TopK        int      `json:"top_k"`
	Paths       []string `json:"paths"`
	Limit       int      `json:"limit"`
	MinSize     int      `json:"min_size"`
	MinInDegree int      `json:"min_in_degree"`
	From        string   `json:"from"`
	To          string   `json:"to"`
	Kinds       []string `json:"kinds"`
	MaxHops     int      `json:"max_hops"`
	Target      string   `json:"target"`
}

type hit struct {
	ChunkID   string   `json:"chunk_id"`
	Path      string   `json:"path"`
	StartLine int      `json:"start_line"`
	EndLine   int      `json:"end_line"`
	Scope     []string `json:"scope"`
	Score     float64  `json:"score"`
	Content   string   `json:"content"`
}

func main() {
	in := bufio.NewScanner(os.Stdin)
	in.Buffer(make([]byte, 1<<20), 1<<24)
	out := json.NewEncoder(os.Stdout)
	for in.Scan() {
		var req request
		if err := json.Unmarshal(in.Bytes(), &req); err != nil {
			continue
		}
		if log := os.Getenv("STANDIN_LOG"); log != "" && req.Method != "health" {
			if f, err := os.OpenFile(log, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644); err == nil {
				_, _ = f.WriteString(req.Method + " " + string(req.Params) + "\n")
				_ = f.Close()
			}
		}
		var p params
		_ = json.Unmarshal(req.Params, &p)
		resp := map[string]interface{}{"jsonrpc": "2.0", "id": req.ID}
		switch {
		case req.Method == "health":
			resp["result"] = map[string]string{"name": "dendrite-rag", "version": "0.1.0", "protocol": "1.1.0"}
		case req.Method == os.Getenv("STANDIN_FAIL"):
			resp["error"] = map[string]interface{}{"code": -32000, "message": "boom", "data": map[string]string{"kind": "index"}}
		case req.Method == "index":
			resp["result"] = map[string]int{"files_scanned": 1, "files_changed": 1, "embed_pending": 7}
		case req.Method == "search":
			resp["result"] = map[string]interface{}{"hits": matches(p)}
		case req.Method == "context":
			hits := matches(p)
			tokens := 0
			for _, h := range hits {
				tokens += len(h.Content) / 4
			}
			resp["result"] = map[string]interface{}{"spans": hits, "tokens": tokens}
		case req.Method == "graph.communities":
			resp["result"] = map[string]interface{}{
				"communities": []map[string]interface{}{
					{
						"id":    1,
						"label": "pkg/demo · LocateBinary",
						"size":  1,
						"files": []string{"pkg/demo/demo.go"},
						"top_symbols": []map[string]interface{}{
							{
								"name":      "LocateBinary",
								"path":      "pkg/demo/demo.go",
								"line":      4,
								"in_degree": 25,
							},
						},
						"summary": "Community pkg/demo · LocateBinary: 1 files in pkg/demo (1).",
					},
				},
			}
		case req.Method == "graph.god_nodes":
			resp["result"] = map[string]interface{}{
				"nodes": []map[string]interface{}{
					{
						"symbol":          "LocateBinary",
						"path":            "pkg/demo/demo.go",
						"line":            4,
						"in_degree":       25,
						"out_degree":      0,
						"used_from_files": 3,
						"community": map[string]interface{}{
							"id":    1,
							"label": "pkg/demo",
						},
						"summary": "Symbol LocateBinary in pkg/demo/demo.go:4 with 25 users.",
					},
				},
			}
		case req.Method == "graph.path":
			fromNode := p.From
			if fromNode == "" {
				fromNode = "Run"
			}
			toNode := p.To
			if toNode == "" {
				toNode = "LocateBinary"
			}
			resp["result"] = map[string]interface{}{
				"found":    true,
				"directed": true,
				"level":    "symbol",
				"steps": []map[string]interface{}{
					{
						"node":     fromNode,
						"location": "pkg/demo/from.go:1",
					},
					{
						"node":     toNode,
						"location": "pkg/demo/to.go:10",
						"via":      "call",
					},
				},
			}
		case req.Method == "graph.explain":
			target := p.Target
			if target == "" {
				target = "LocateBinary"
			}
			resp["result"] = map[string]interface{}{
				"matches": []map[string]interface{}{
					{
						"kind":   "symbol",
						"symbol": target,
						"sites": []map[string]interface{}{
							{
								"path":  "pkg/demo/demo.go",
								"line":  4,
								"chunk": "pkg/demo/demo.go#1",
							},
						},
						"community": map[string]interface{}{
							"id":    1,
							"label": "pkg/demo",
						},
						"god_node":   true,
						"in_degree":  25,
						"out_degree": 0,
						"users": []map[string]interface{}{
							{
								"symbol": "Run",
								"path":   "pkg/rag/retriever.go",
								"line":   45,
								"kind":   "call",
							},
						},
						"uses":    []map[string]interface{}{},
						"summary": "Symbol " + target + " in pkg/demo/demo.go:4.",
					},
				},
			}
		default:
			resp["error"] = map[string]interface{}{"code": -32601, "message": "method not found"}
		}
		_ = out.Encode(resp)
	}
}

func matches(p params) []hit {
	words := strings.Fields(strings.ToLower(p.Query))
	var hits []hit
	_ = filepath.WalkDir(p.Root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		if d.IsDir() {
			if path != p.Root && strings.HasPrefix(d.Name(), ".") {
				return filepath.SkipDir
			}
			return nil
		}
		rel, _ := filepath.Rel(p.Root, path)
		rel = filepath.ToSlash(rel)
		if len(p.Paths) > 0 && !contains(p.Paths, rel) {
			return nil
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return nil
		}
		text := strings.ToLower(string(data))
		n := 0
		for _, w := range words {
			if strings.Contains(text, strings.Trim(w, "?.,!`'\"")) {
				n++
			}
		}
		if n > 0 {
			hits = append(hits, hit{
				ChunkID: rel + "#1", Path: rel, StartLine: 1,
				EndLine: strings.Count(string(data), "\n") + 1,
				Score:   float64(n) / float64(len(words)), Content: string(data),
			})
		}
		return nil
	})
	sort.SliceStable(hits, func(i, j int) bool {
		if hits[i].Score != hits[j].Score {
			return hits[i].Score > hits[j].Score
		}
		return hits[i].Path < hits[j].Path
	})
	if p.TopK > 0 && len(hits) > p.TopK {
		hits = hits[:p.TopK]
	}
	return hits
}

func contains(list []string, s string) bool {
	for _, v := range list {
		if v == s {
			return true
		}
	}
	return false
}
