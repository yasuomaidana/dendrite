package server

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"strings"
	"sync"
	"time"

	"Dendrite/pkg/inference"
	"Dendrite/pkg/rag"
	"Dendrite/pkg/statedir"
	"Dendrite/pkg/telemetry/exporter"
	"Dendrite/pkg/topology"
	"Dendrite/pkg/workspace"
)

// Server is the embedded HTTP and WebSocket control server.
type Server struct {
	cfg       ServerConfig
	registry  *workspace.Registry
	hub       *TelemetryHub
	exporter  *exporter.PrometheusExporter
	server    *http.Server
	listener  net.Listener
	activeTop *topology.TopologyConfig
	startTime time.Time
	mu        sync.RWMutex
}

// NewServer initializes a new Server instance.
func NewServer(cfg ServerConfig, reg *workspace.Registry, hub *TelemetryHub) *Server {
	if cfg.Port <= 0 {
		cfg.Port = 3456
	}
	if cfg.Host == "" {
		cfg.Host = "127.0.0.1"
	}
	if cfg.ReadTimeout == 0 {
		cfg.ReadTimeout = 15 * time.Second
	}
	if hub == nil {
		hub = NewTelemetryHub()
	}

	defaultTop, _ := topology.NewTopologyConfig(topology.ScenarioLocalHybrid, topology.TailscaleEndpoints{
		ControlPlane:   "100.84.12.10",
		ExecutionPlane: "100.84.12.25",
	})

	s := &Server{
		cfg:       cfg,
		registry:  reg,
		hub:       hub,
		exporter:  exporter.NewPrometheusExporter(nil),
		activeTop: defaultTop,
		startTime: time.Now(),
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/", s.handleDashboard)
	mux.HandleFunc("/api/v1/health", s.handleHealth)
	mux.HandleFunc("/api/v1/workspaces", s.handleWorkspaces)
	mux.HandleFunc("/api/v1/workspaces/switch", s.handleWorkspaceSwitch)
	mux.HandleFunc("/api/v1/topology", s.handleTopology)
	mux.HandleFunc("/api/v1/telemetry/snapshot", s.handleTelemetrySnapshot)
	mux.HandleFunc("/api/v1/telemetry/ws", s.hub.HandleWebSocket)
	mux.HandleFunc("/api/v1/telemetry/events", s.hub.HandleSSE)
	mux.HandleFunc("/metrics", s.handleMetrics)
	mux.HandleFunc("/api/v1/telemetry/metrics", s.handleMetrics)
	mux.HandleFunc("/api/v1/inference/zero-cost", s.handleZeroCost)
	mux.HandleFunc("/api/v1/inference/zero-cost/test", s.handleZeroCostTest)
	mux.HandleFunc("/api/v1/rag/ask", s.handleRAGAsk)
	mux.HandleFunc("/api/v1/rag/retrieve", s.handleRAGRetrieve)

	s.server = &http.Server{
		Handler:     s.authMiddleware(mux),
		ReadTimeout: cfg.ReadTimeout,
	}

	return s
}

func (s *Server) authMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, OPTIONS, PATCH, DELETE")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization, x-api-key")

		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusOK)
			return
		}

		if s.cfg.AuthToken != "" {
			authHeader := r.Header.Get("Authorization")
			token := strings.TrimPrefix(authHeader, "Bearer ")
			if token != s.cfg.AuthToken && r.URL.Query().Get("token") != s.cfg.AuthToken {
				http.Error(w, ErrUnauthorizedAccess.Error(), http.StatusUnauthorized)
				return
			}
		}

		next.ServeHTTP(w, r)
	})
}

// Start binds and starts the HTTP server.
func (s *Server) Start() error {
	addr := fmt.Sprintf("%s:%d", s.cfg.Host, s.cfg.Port)
	listener, err := net.Listen("tcp", addr)
	if err != nil {
		return err
	}
	s.listener = listener

	go func() {
		_ = s.server.Serve(listener)
	}()

	return nil
}

// Addr returns the actual listening network address.
func (s *Server) Addr() string {
	if s.listener != nil {
		return s.listener.Addr().String()
	}
	return fmt.Sprintf("%s:%d", s.cfg.Host, s.cfg.Port)
}

// Shutdown gracefully terminates the HTTP server.
func (s *Server) Shutdown(ctx context.Context) error {
	if s.server != nil {
		return s.server.Shutdown(ctx)
	}
	return nil
}

// GetHub returns the attached TelemetryHub for metric injection.
func (s *Server) GetHub() *TelemetryHub {
	return s.hub
}

// Exporter returns the attached Prometheus exporter.
func (s *Server) Exporter() *exporter.PrometheusExporter {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.exporter
}

// SetExporter binds a custom or daemon-backed Prometheus exporter to the server.
func (s *Server) SetExporter(exp *exporter.PrometheusExporter) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.exporter = exp
}

func (s *Server) handleMetrics(w http.ResponseWriter, r *http.Request) {
	s.mu.RLock()
	exp := s.exporter
	s.mu.RUnlock()
	if exp != nil {
		exp.ServeHTTP(w, r)
		return
	}
	http.Error(w, "metrics exporter unavailable", http.StatusServiceUnavailable)
}

func (s *Server) handleHealth(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	resp := map[string]interface{}{
		"status":    "healthy",
		"version":   "0.1.0-alpha",
		"uptime_s":  time.Since(s.startTime).Seconds(),
		"timestamp": time.Now().UTC(),
	}
	_ = json.NewEncoder(w).Encode(resp)
}

func (s *Server) handleWorkspaces(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	if r.Method != http.MethodGet && r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	if s.registry == nil {
		_ = json.NewEncoder(w).Encode(map[string]interface{}{"workspaces": []interface{}{}})
		return
	}

	if r.Method == http.MethodGet {
		list := s.registry.ListProjects()
		active, _ := s.registry.GetActiveProject()
		activeName := ""
		if active != nil {
			activeName = active.Name
		}
		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"active":     activeName,
			"workspaces": list,
		})
		return
	}

	if r.Method == http.MethodPost {
		var body struct {
			Name string `json:"name"`
			Path string `json:"path"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		entry, err := s.registry.AddProject(body.Name, body.Path)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		w.WriteHeader(http.StatusCreated)
		_ = json.NewEncoder(w).Encode(entry)
		return
	}
}

func (s *Server) handleWorkspaceSwitch(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var body struct {
		Name string `json:"name"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	if s.registry == nil {
		http.Error(w, "workspace registry unavailable", http.StatusInternalServerError)
		return
	}

	entry, err := s.registry.SwitchProject(body.Name)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(entry)
}

func (s *Server) handleTopology(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	s.mu.Lock()
	defer s.mu.Unlock()

	if r.Method == http.MethodGet {
		_ = json.NewEncoder(w).Encode(s.activeTop)
		return
	}

	if r.Method == http.MethodPost {
		var body struct {
			Scenario string `json:"scenario"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		sc, err := topology.ParseScenario(body.Scenario)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		top, err := topology.NewTopologyConfig(sc, s.activeTop.TailscaleEndpoints)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		s.activeTop = top
		_ = json.NewEncoder(w).Encode(s.activeTop)
		return
	}

	http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
}

func (s *Server) handleTelemetrySnapshot(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	s.mu.RLock()
	topName := string(s.activeTop.Scenario)
	s.mu.RUnlock()

	activeWs := ""
	if s.registry != nil {
		if act, err := s.registry.GetActiveProject(); err == nil && act != nil {
			activeWs = act.Name
		}
	}

	frame := TelemetryFrame{
		Timestamp:       time.Now().UTC(),
		ActiveScenario:  topName,
		ActiveWorkspace: activeWs,
		SoCTemperatureC: 48.5,
		ThermalAlert:    false,
		TokenSpend24h:   12.45,
		TokenSpend7d:    54.20,
		DailyBudgetUSD:  50.00,
		NodeHealth: map[string]string{
			"Debian":  "healthy",
			"MacBook": "healthy",
			"Azure":   "idle",
			"AWS":     "standby",
		},
	}

	_ = json.NewEncoder(w).Encode(frame)
}

func (s *Server) handleZeroCost(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	reg := inference.GetZeroCostRegistry()

	switch r.Method {
	case http.MethodGet:
		details := reg.GetDetails()
		_ = json.NewEncoder(w).Encode(details)

	case http.MethodPost:
		var req struct {
			Name     string `json:"name"`
			IsPrefix bool   `json:"is_prefix"`
			SyncEnv  bool   `json:"sync_env"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		if strings.TrimSpace(req.Name) == "" {
			http.Error(w, "name cannot be empty", http.StatusBadRequest)
			return
		}

		var ok bool
		if req.IsPrefix {
			ok = reg.AddPrefix(req.Name)
		} else {
			ok, _ = reg.Add(req.Name)
		}

		if !ok {
			http.Error(w, "failed to add zero-cost entry", http.StatusInternalServerError)
			return
		}

		_ = reg.SaveToFile(statedir.Resolve(inference.DefaultZeroCostConfigFile))
		if req.SyncEnv {
			_ = reg.SaveToEnvFile(".env")
		}

		w.WriteHeader(http.StatusCreated)
		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"success": true,
			"name":    req.Name,
			"details": reg.GetDetails(),
		})

	case http.MethodDelete:
		var req struct {
			Name     string `json:"name"`
			IsPrefix bool   `json:"is_prefix"`
			SyncEnv  bool   `json:"sync_env"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		if strings.TrimSpace(req.Name) == "" {
			http.Error(w, "name cannot be empty", http.StatusBadRequest)
			return
		}

		var ok bool
		if req.IsPrefix {
			ok = reg.RemovePrefix(req.Name)
		} else {
			ok, _ = reg.Remove(req.Name)
		}

		if !ok {
			http.Error(w, "failed to remove zero-cost entry", http.StatusInternalServerError)
			return
		}

		_ = reg.SaveToFile(statedir.Resolve(inference.DefaultZeroCostConfigFile))
		if req.SyncEnv {
			_ = reg.SaveToEnvFile(".env")
		}

		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"success": true,
			"name":    req.Name,
			"details": reg.GetDetails(),
		})

	case http.MethodPatch:
		// Reset to defaults
		reg.Reset()
		reg.AutoLoad()
		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"success": true,
			"message": "reset to defaults and active .env",
			"details": reg.GetDetails(),
		})

	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

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
	if body.MinScore <= 0 {
		body.MinScore = 0.01
	}

	retrieval, err := rag.NewRetriever(rag.NewIndexer()).Retrieve(r.Context(), body.ProjectPath, body.Query, rag.SearchOptions{
		TopK:      body.TopK,
		MinScore:  body.MinScore,
		MaxTokens: 2500,
	}, false)
	if err != nil {
		http.Error(w, fmt.Sprintf("retrieval failed: %v", err), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(retrieval.Chunks)
}
