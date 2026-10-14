package docusaurus

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"sync"
	"time"
)

// ServerInstance represents a running embedded preview and sharing HTTP server.
type ServerInstance struct {
	server   *http.Server
	listener net.Listener
	addr     string
	siteDir  string
	mu       sync.Mutex
	running  bool
}

// StartPreviewServer launches the embedded preview server asynchronously.
func StartPreviewServer(siteDir string, port int) (*ServerInstance, error) {
	if siteDir == "" {
		return nil, ErrEmptyOutputDir
	}
	if port <= 0 {
		port = 3000
	}

	addr := fmt.Sprintf(":%d", port)
	ln, err := net.Listen("tcp", addr)
	if err != nil {
		return nil, fmt.Errorf("%w: failed to listen on %s: %v", ErrServerStartFailed, addr, err)
	}

	mux := http.NewServeMux()
	registerServerRoutes(mux, siteDir)

	srv := &http.Server{
		Handler:      mux,
		ReadTimeout:  10 * time.Second,
		WriteTimeout: 10 * time.Second,
	}

	inst := &ServerInstance{
		server:   srv,
		listener: ln,
		addr:     ln.Addr().String(),
		siteDir:  siteDir,
		running:  true,
	}

	go func() {
		_ = srv.Serve(ln)
	}()

	return inst, nil
}

func registerServerRoutes(mux *http.ServeMux, siteDir string) {
	mux.HandleFunc("/api/v1/health", handleHealth)
	mux.HandleFunc("/api/v1/sharing-manifest", handleManifest(siteDir))
	mux.HandleFunc("/api/v1/download-bundle", handleBundleDownload(siteDir))

	buildDir := filepath.Join(siteDir, "build")
	serveDir := siteDir
	if info, err := os.Stat(buildDir); err == nil && info.IsDir() {
		serveDir = buildDir
	}
	mux.Handle("/", http.FileServer(http.Dir(serveDir)))
}

func handleHealth(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]interface{}{
		"status":    "healthy",
		"service":   "dendrite-docusaurus-sharing-portal",
		"timestamp": time.Now().UTC(),
	})
}

func handleManifest(siteDir string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		manifestPath := filepath.Join(siteDir, "sharing-manifest.json")
		data, err := os.ReadFile(manifestPath)
		if err != nil {
			http.Error(w, "Sharing manifest not found", http.StatusNotFound)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write(data)
	}
}

func handleBundleDownload(siteDir string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		bundlePath := filepath.Join(siteDir, "dendrite-docs.tar.gz")
		if _, err := os.Stat(bundlePath); err != nil {
			http.Error(w, "Documentation bundle archive not found", http.StatusNotFound)
			return
		}
		w.Header().Set("Content-Disposition", "attachment; filename=dendrite-docs.tar.gz")
		w.Header().Set("Content-Type", "application/gzip")
		http.ServeFile(w, r, bundlePath)
	}
}

// Addr returns the network address the server is listening on.
func (s *ServerInstance) Addr() string {
	return s.addr
}

// Shutdown gracefully terminates the HTTP server and releases resources.
func (s *ServerInstance) Shutdown(ctx context.Context) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if !s.running {
		return nil
	}
	s.running = false
	return s.server.Shutdown(ctx)
}
