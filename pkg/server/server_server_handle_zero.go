package server

import (
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

func (s *Server) handleRAGAsk(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req rag.AskRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, fmt.Sprintf("invalid request payload: %v", err), http.StatusBadRequest)
		return
	}

	if strings.TrimSpace(req.Question) == "" {
		http.Error(w, "question is required", http.StatusBadRequest)
		return
	}

	if req.ProjectPath == "" {
		req.ProjectPath = "."
	}
	if req.PaidModel == "" {
		req.PaidModel = "gemini-2.5-pro"
	}
	if req.TopK <= 0 {
		req.TopK = 4
	}
	req.Input = strings.NewReader("n\n")

	engine := rag.NewAskEngine(nil)
	res, err := engine.Ask(r.Context(), req)
	if err != nil {
		http.Error(w, fmt.Sprintf("rag ask failed: %v", err), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(res)
}

func (s *Server) handleRAGRetrieve(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var body struct {
		Query       string  `json:"query"`
		ProjectPath string  `json:"project_path"`
		TopK        int     `json:"top_k"`
		MinScore    float32 `json:"min_score"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		http.Error(w, fmt.Sprintf("invalid request payload: %v", err), http.StatusBadRequest)
		return
	}

	if strings.TrimSpace(body.Query) == "" {
		http.Error(w, "query is required", http.StatusBadRequest)
		return
	}

	if body.ProjectPath == "" {
		body.ProjectPath = "."
	}
	if body.TopK <= 0 {
		body.TopK = 4
	}
	retrieval, err := rag.NewRetriever().Retrieve(r.Context(), body.ProjectPath, body.Query, rag.SearchOptions{
		TopK:     body.TopK,
		MinScore: body.MinScore,
	}, false)
	if err != nil {
		http.Error(w, fmt.Sprintf("retrieval failed: %v", err), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(retrieval.Chunks)
}

func (s *Server) handleGraphCommunities(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var body struct {
		ProjectPath string `json:"project_path"`
		Limit       int    `json:"limit"`
		MinSize     int    `json:"min_size"`
	}
	if r.Body != nil {
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil && err != io.EOF {
			http.Error(w, fmt.Sprintf("invalid request payload: %v", err), http.StatusBadRequest)
			return
		}
	}
	if body.ProjectPath == "" {
		body.ProjectPath = "."
	}

	communities, err := rag.NewRetriever().Communities(r.Context(), body.ProjectPath, rag.CommunitiesOptions{
		Limit:   body.Limit,
		MinSize: body.MinSize,
	})
	if err != nil {
		http.Error(w, fmt.Sprintf("graph communities failed: %v", err), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]interface{}{
		"communities": communities,
	})
}

func (s *Server) handleGraphGodNodes(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var body struct {
		ProjectPath string `json:"project_path"`
		MinInDegree int    `json:"min_in_degree"`
		Limit       int    `json:"limit"`
	}
	if r.Body != nil {
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil && err != io.EOF {
			http.Error(w, fmt.Sprintf("invalid request payload: %v", err), http.StatusBadRequest)
			return
		}
	}
	if body.ProjectPath == "" {
		body.ProjectPath = "."
	}

	nodes, err := rag.NewRetriever().GodNodes(r.Context(), body.ProjectPath, rag.GodNodesOptions{
		MinInDegree: body.MinInDegree,
		Limit:       body.Limit,
	})
	if err != nil {
		http.Error(w, fmt.Sprintf("graph god nodes failed: %v", err), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]interface{}{
		"nodes": nodes,
	})
}

func (s *Server) handleGraphPath(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var body struct {
		ProjectPath string   `json:"project_path"`
		From        string   `json:"from"`
		To          string   `json:"to"`
		Kinds       []string `json:"kinds"`
		MaxHops     int      `json:"max_hops"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		http.Error(w, fmt.Sprintf("invalid request payload: %v", err), http.StatusBadRequest)
		return
	}

	if strings.TrimSpace(body.From) == "" || strings.TrimSpace(body.To) == "" {
		http.Error(w, "from and to are required", http.StatusBadRequest)
		return
	}

	if body.ProjectPath == "" {
		body.ProjectPath = "."
	}

	path, err := rag.NewRetriever().Path(r.Context(), body.ProjectPath, body.From, body.To, rag.PathOptions{
		Kinds:   body.Kinds,
		MaxHops: body.MaxHops,
	})
	if err != nil {
		http.Error(w, fmt.Sprintf("graph path failed: %v", err), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(path)
}

func (s *Server) handleGraphExplain(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var body struct {
		ProjectPath string `json:"project_path"`
		Target      string `json:"target"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		http.Error(w, fmt.Sprintf("invalid request payload: %v", err), http.StatusBadRequest)
		return
	}

	if strings.TrimSpace(body.Target) == "" {
		http.Error(w, "target is required", http.StatusBadRequest)
		return
	}

	if body.ProjectPath == "" {
		body.ProjectPath = "."
	}

	matches, err := rag.NewRetriever().Explain(r.Context(), body.ProjectPath, body.Target)
	if err != nil {
		http.Error(w, fmt.Sprintf("graph explain failed: %v", err), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]interface{}{
		"matches": matches,
	})
}
