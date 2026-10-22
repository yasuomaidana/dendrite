// Command standin speaks the dendrite-rag sidecar protocol for Dendrite's tests, which
// must not depend on a Rust build. It does no ranking of its own: search and context
// return the project's text files that contain a query word, one whole-file span each,
// most matching words first, then by path; context with detail "map" returns each file's
// first line as its outline. index reports files_changed 1 and embed_pending 7, status an
// embed_coverage of 0.5. ask answers from those matches without a model: STANDIN_LOCAL=fail
// makes the local tier fail, so escalation is needed unless paid use is allowed.
// STANDIN_LOG names a file that records each call's method and params;
// STANDIN_FAIL=<method> makes that method return an engine error; STANDIN_PAID=fail makes the
// paid tier fail; STANDIN_PROTOCOL sets
// the protocol version the handshake reports (default 1.6.0); STANDIN_ASK_LOCAL_MODEL the
// project's [ask] local_model that settings reports.
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
	Question    string   `json:"question"`
	Mode        string   `json:"mode"`
	AllowPaid   bool     `json:"allow_paid"`
	LocalModel  string   `json:"local_model"`
	PaidModel   string   `json:"paid_model"`
	Detail      string   `json:"detail"`
	IDs         []string `json:"ids"`
}

type hit struct {
	ChunkID   string   `json:"chunk_id"`
	Path      string   `json:"path"`
	StartLine int      `json:"start_line"`
	EndLine   int      `json:"end_line"`
	Scope     []string `json:"scope"`
	Score     float64  `json:"score"`
	Content   string   `json:"content"`
	Outline   bool     `json:"outline,omitempty"`
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
			protocol := "1.6.0"
			if v := os.Getenv("STANDIN_PROTOCOL"); v != "" {
				protocol = v
			}
			resp["result"] = map[string]string{"name": "dendrite-rag", "version": "0.1.0", "protocol": protocol}
		case req.Method == os.Getenv("STANDIN_FAIL"):
			resp["error"] = map[string]interface{}{"code": -32000, "message": "boom", "data": map[string]string{"kind": "index"}}
		case req.Method == "index":
			resp["result"] = map[string]int{"files_scanned": 1, "files_changed": 1, "embed_pending": 7, "embed_ms": 40}
		case req.Method == "settings":
			resp["result"] = map[string]interface{}{"settings": map[string]interface{}{"ask": map[string]string{"local_model": os.Getenv("STANDIN_ASK_LOCAL_MODEL")}}}
		case req.Method == "status":
			resp["result"] = map[string]interface{}{"files": 1, "chunks": 14, "vectors": 7, "embed_coverage": 0.5}
		case req.Method == "ask":
			if r, failure := ask(p); failure != "" {
				resp["error"] = map[string]interface{}{"code": -32000, "message": failure, "data": map[string]string{"kind": "llm"}}
			} else {
				resp["result"] = r
			}
		case req.Method == "chunks.get":
			resp["result"] = map[string]interface{}{"hits": chunks(p)}
		case req.Method == "search":
			resp["result"] = map[string]interface{}{"hits": matches(p)}
		case req.Method == "context":
			hits := matches(p)
			if p.Detail == "map" {
				for i := range hits {
					hits[i].Content = strings.SplitN(hits[i].Content, "\n", 2)[0]
					hits[i].Outline = true
				}
			}
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

// ask answers like dendrite-rag's ask method, without a model; the second result is the
// error a failed paid model gives (STANDIN_PAID=fail).
func ask(p params) (map[string]interface{}, string) {
	topK := p.TopK
	if topK <= 0 {
		topK = 4
	}
	hits := matches(params{Root: p.Root, Query: p.Question, TopK: topK})
	local := p.LocalModel
	if local == "" {
		local = "local-coder"
	}
	out := map[string]interface{}{
		"hits": hits, "prompt_tokens": 100, "output_tokens": 20,
		"timings": map[string]interface{}{"retrieval": map[string]int{"total_ms": 3}, "local_ms": 120, "paid_ms": 0, "total_ms": 123},
	}
	paidFails := os.Getenv("STANDIN_PAID") == "fail"
	const paidError = "paid model gemini: quota exceeded"
	paid := func(tier string) map[string]interface{} {
		out["source_tier"], out["model"], out["escalated"] = tier, p.PaidModel, true
		out["answer"] = "paid answer for: " + p.Question
		out["timings"].(map[string]interface{})["paid_ms"] = 250
		return out
	}
	localFails := os.Getenv("STANDIN_LOCAL") == "fail"
	localAnswer := func() map[string]interface{} {
		out["source_tier"], out["model"], out["escalated"] = "local-rag", local, false
		out["answer"] = "local answer for: " + p.Question
		return out
	}
	switch {
	case (p.Mode == "paid" || (localFails && p.AllowPaid)) && paidFails:
		return nil, paidError
	case p.Mode == "paid":
		return paid("paid-cloud"), ""
	case localFails && !p.AllowPaid:
		out["escalation_needed"], out["local_error"], out["model"], out["answer"] = true, "local model down", local, ""
		return out, ""
	case localFails:
		out["local_error"] = "local model down"
		return paid("paid-cloud"), ""
	case p.Mode == "hybrid" && paidFails:
		// The draft is still an answer when refinement fails.
		out["local_error"] = "paid refinement failed: " + paidError
		return localAnswer(), ""
	case p.Mode == "hybrid":
		out["local_draft"] = "local draft"
		r := paid("hybrid")
		r["model"] = local + " -> " + p.PaidModel
		return r, ""
	}
	return localAnswer(), ""
}

// chunks returns the whole files named by `<path>#1` ids.
func chunks(p params) []hit {
	var out []hit
	for _, id := range p.IDs {
		rel := strings.TrimSuffix(id, "#1")
		data, err := os.ReadFile(filepath.Join(p.Root, filepath.FromSlash(rel)))
		if err != nil {
			continue
		}
		out = append(out, hit{ChunkID: id, Path: rel, StartLine: 1, EndLine: strings.Count(string(data), "\n") + 1, Content: string(data)})
	}
	return out
}

func contains(list []string, s string) bool {
	for _, v := range list {
		if v == s {
			return true
		}
	}
	return false
}
