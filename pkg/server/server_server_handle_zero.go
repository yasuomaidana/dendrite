package server

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"

	"Dendrite/pkg/inference"
	"Dendrite/pkg/rag"
)

func (s *Server) handleZeroCostTest(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	reg := inference.GetZeroCostRegistry()

	model := r.URL.Query().Get("model")
	if model == "" && r.Method == http.MethodPost {
		var req struct {
			Model string `json:"model"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err == nil {
			model = req.Model
		}
	}

	if strings.TrimSpace(model) == "" {
		http.Error(w, "query param 'model' or json body 'model' required", http.StatusBadRequest)
		return
	}

	isZeroCost, reason := reg.Test(model)
	_ = json.NewEncoder(w).Encode(map[string]interface{}{
		"model":     model,
		"zero_cost": isZeroCost,
		"reason":    reason,
	})
}

func (s *Server) handleDashboard(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/" {
		http.NotFound(w, r)
		return
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	html := `<!DOCTYPE html>
<html lang="en">
<head>
  <meta charset="UTF-8">
  <title>Dendrite Control Plane Dashboard</title>
  <style>
    body { font-family: -apple-system, BlinkMacSystemFont, "Segoe UI", Roboto, sans-serif; background: #0f172a; color: #f8fafc; margin: 0; padding: 2rem; }
    h1 { font-size: 1.5rem; color: #38bdf8; margin-bottom: 1.5rem; }
    .grid { display: grid; grid-template-columns: repeat(auto-fit, minmax(300px, 1fr)); gap: 1.5rem; }
    .card { background: #1e293b; border-radius: 0.5rem; padding: 1.25rem; border: 1px solid #334155; }
    .card h2 { font-size: 1.1rem; margin-top: 0; color: #94a3b8; }
    .gauge { font-size: 2rem; font-weight: bold; color: #10b981; }
    .alert { color: #ef4444; }
  </style>
</head>
<body>
  <h1>Dendrite Control Plane</h1>
  <div class="grid">
    <div class="card">
      <h2>SoC Die Temperature</h2>
      <div id="soc-temp" class="gauge">48.5°C</div>
    </div>
    <div class="card">
      <h2>24h Token Spend</h2>
      <div id="token-spend" class="gauge">$12.45</div>
    </div>
    <div class="card">
      <h2>Active Scenario</h2>
      <div id="active-scenario" class="gauge" style="font-size: 1.5rem;">local-hybrid</div>
    </div>
  </div>
</body>
</html>`
	_, _ = w.Write([]byte(html))
}

// decodePost accepts POST only and decodes the JSON body into out. With allowEmpty a missing or
// empty body leaves out untouched. It answers the client itself and returns false when the
// request cannot be used.
func decodePost(w http.ResponseWriter, r *http.Request, out interface{}, allowEmpty bool) bool {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return false
	}
	if allowEmpty && r.Body == nil {
		return true
	}
	if err := json.NewDecoder(r.Body).Decode(out); err != nil && !(allowEmpty && err == io.EOF) {
		http.Error(w, fmt.Sprintf("invalid request payload: %v", err), http.StatusBadRequest)
		return false
	}
	return true
}

// writeJSON answers 200 with v as JSON.
func writeJSON(w http.ResponseWriter, v interface{}) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(v)
}

// failInternal answers 500 with "<what> failed: <err>".
func failInternal(w http.ResponseWriter, what string, err error) {
	http.Error(w, fmt.Sprintf("%s failed: %v", what, err), http.StatusInternalServerError)
}

// requireField answers 400 and returns false when value is blank.
func requireField(w http.ResponseWriter, value, message string) bool {
	if strings.TrimSpace(value) == "" {
		http.Error(w, message, http.StatusBadRequest)
		return false
	}
	return true
}

// projectOrDot is the project path to use: "." when none was given.
func projectOrDot(path string) string {
	if path == "" {
		return "."
	}
	return path
}

func (s *Server) handleRAGAsk(w http.ResponseWriter, r *http.Request) {
	var req rag.AskRequest
	if !decodePost(w, r, &req, false) || !requireField(w, req.Question, "question is required") {
		return
	}

	req.ProjectPath = projectOrDot(req.ProjectPath)
	if req.PaidModel == "" {
		req.PaidModel = "gemini-2.5-pro"
	}
	if req.TopK <= 0 {
		req.TopK = 4
	}
	req.Input = strings.NewReader("n\n")

	res, err := rag.NewAskEngine().Ask(r.Context(), req)
	if err != nil {
		failInternal(w, "rag ask", err)
		return
	}
	writeJSON(w, res)
}

func (s *Server) handleRAGRetrieve(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Query       string  `json:"query"`
		ProjectPath string  `json:"project_path"`
		TopK        int     `json:"top_k"`
		MinScore    float32 `json:"min_score"`
	}
	if !decodePost(w, r, &body, false) || !requireField(w, body.Query, "query is required") {
		return
	}

	if body.TopK <= 0 {
		body.TopK = 4
	}
	retrieval, err := rag.NewRetriever().Retrieve(r.Context(), projectOrDot(body.ProjectPath), body.Query, rag.SearchOptions{
		TopK:     body.TopK,
		MinScore: body.MinScore,
	}, false)
	if err != nil {
		failInternal(w, "retrieval", err)
		return
	}
	writeJSON(w, retrieval.Chunks)
}

// graphListBody is the request shared by the graph list endpoints; each reads the fields it needs.
type graphListBody struct {
	ProjectPath string `json:"project_path"`
	Limit       int    `json:"limit"`
	MinSize     int    `json:"min_size"`
	MinInDegree int    `json:"min_in_degree"`
}

// serveGraphList decodes a graph list request, runs query and answers {key: result}.
func serveGraphList(w http.ResponseWriter, r *http.Request, what, key string, query func(ctx context.Context, b graphListBody) (interface{}, error)) {
	var body graphListBody
	if !decodePost(w, r, &body, true) {
		return
	}
	res, err := query(r.Context(), body)
	if err != nil {
		failInternal(w, what, err)
		return
	}
	writeJSON(w, map[string]interface{}{key: res})
}

func (s *Server) handleGraphCommunities(w http.ResponseWriter, r *http.Request) {
	serveGraphList(w, r, "graph communities", "communities", func(ctx context.Context, b graphListBody) (interface{}, error) {
		return rag.NewRetriever().Communities(ctx, projectOrDot(b.ProjectPath), rag.CommunitiesOptions{Limit: b.Limit, MinSize: b.MinSize})
	})
}

func (s *Server) handleGraphGodNodes(w http.ResponseWriter, r *http.Request) {
	serveGraphList(w, r, "graph god nodes", "nodes", func(ctx context.Context, b graphListBody) (interface{}, error) {
		return rag.NewRetriever().GodNodes(ctx, projectOrDot(b.ProjectPath), rag.GodNodesOptions{MinInDegree: b.MinInDegree, Limit: b.Limit})
	})
}

func (s *Server) handleGraphPath(w http.ResponseWriter, r *http.Request) {
	var body struct {
		ProjectPath string   `json:"project_path"`
		From        string   `json:"from"`
		To          string   `json:"to"`
		Kinds       []string `json:"kinds"`
		MaxHops     int      `json:"max_hops"`
	}
	if !decodePost(w, r, &body, false) {
		return
	}
	if strings.TrimSpace(body.From) == "" || strings.TrimSpace(body.To) == "" {
		http.Error(w, "from and to are required", http.StatusBadRequest)
		return
	}

	path, err := rag.NewRetriever().Path(r.Context(), projectOrDot(body.ProjectPath), body.From, body.To, rag.PathOptions{
		Kinds:   body.Kinds,
		MaxHops: body.MaxHops,
	})
	if err != nil {
		failInternal(w, "graph path", err)
		return
	}
	writeJSON(w, path)
}

func (s *Server) handleGraphExplain(w http.ResponseWriter, r *http.Request) {
	var body struct {
		ProjectPath string `json:"project_path"`
		Target      string `json:"target"`
	}
	if !decodePost(w, r, &body, false) || !requireField(w, body.Target, "target is required") {
		return
	}

	matches, err := rag.NewRetriever().Explain(r.Context(), projectOrDot(body.ProjectPath), body.Target)
	if err != nil {
		failInternal(w, "graph explain", err)
		return
	}
	writeJSON(w, map[string]interface{}{"matches": matches})
}
