package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"Dendrite/pkg/jetbrains"
)

// serveIDE runs a minimal JetBrains MCP endpoint over HTTP and points the bridge at it.
func serveIDE(t *testing.T, buildOK bool) {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req jetbrains.JSONRPCRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		var result any
		switch req.Method {
		case "initialize":
			result = jetbrains.InitializeResult{
				ProtocolVersion: jetbrains.DefaultProtocolVersion,
				ServerInfo:      jetbrains.ServerInfo{Name: "GoLand", Version: "2026.2"},
			}
		case "tools/list":
			result = jetbrains.ListToolsResult{Tools: []jetbrains.Tool{{Name: "open_file_in_editor", Description: "Open a file"}}}
		case "tools/call":
			raw, _ := json.Marshal(req.Params)
			var p jetbrains.CallToolParams
			_ = json.Unmarshal(raw, &p)
			text := "ok"
			switch p.Name {
			case "get_file_problems":
				b, _ := json.Marshal([]jetbrains.Problem{{File: "a.go", Line: 3, Column: 1, Severity: "warning", Description: "unused"}})
				text = string(b)
			case "search_symbol":
				b, _ := json.Marshal([]jetbrains.Symbol{{Name: "newRootCmd", Kind: "function", File: "root.go", Line: 45}})
				text = string(b)
			case "build_project":
				text = "Build completed successfully"
				if !buildOK {
					text = "Build failed with 2 errors"
				}
			}
			result = jetbrains.CallToolResult{
				Content: []jetbrains.ContentItem{{Type: "text", Text: text}},
				IsError: p.Name == "build_project" && !buildOK,
			}
		default:
			result = map[string]any{}
		}
		b, _ := json.Marshal(result)
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(jetbrains.JSONRPCResponse{JSONRPC: "2.0", ID: req.ID, Result: b})
	}))
	t.Cleanup(srv.Close)
	prev := jetbrainsMCPURL
	jetbrainsMCPURL = srv.URL
	t.Cleanup(func() { jetbrainsMCPURL = prev })
}

func TestBridge_WithIDE(t *testing.T) {
	serveIDE(t, true)
	steps := []struct {
		args []string
		want string
	}{
		{[]string{"bridge", "probe"}, "Connected: GoLand (v2026.2)"},
		{[]string{"jetbrains", "open", "root.go", "--line", "40"}, "Opened root.go at line 40"},
		{[]string{"bridge", "problems", "a.go"}, "[warning] Line 3:1 - unused"},
		{[]string{"bridge", "problems", "a.go", "--json"}, `"unused"`},
		{[]string{"bridge", "symbols", "newRoot"}, "newRootCmd (function) -> root.go:45"},
		{[]string{"bridge", "symbols", "--json"}, `"newRootCmd"`},
		{[]string{"bridge", "build"}, "Project build succeeded"},
		{[]string{"bridge", "run", "All Tests"}, "Running IDE configuration 'All Tests'"},
	}
	for _, s := range steps {
		out, code := cliOutput(t, s.args...)
		if code != ExitOK || !strings.Contains(out, s.want) {
			t.Errorf("%v: exit %d\n%s", s.args, code, out)
		}
	}
	if _, code := cliOutput(t, "bridge", "open", "x.go", "--line", "0"); code != ExitUsage {
		t.Errorf("--line 0: exit %d", code)
	}
}

func TestBridge_BuildFailureExits1(t *testing.T) {
	serveIDE(t, false)
	if out, code := cliOutput(t, "bridge", "build"); code != ExitError || !strings.Contains(out, "Build failed with 2 errors") {
		t.Fatalf("failed build: exit %d\n%s", code, out)
	}
}

func TestBridge_NoIDE(t *testing.T) {
	prev := jetbrainsMCPURL
	jetbrainsMCPURL = "http://127.0.0.1:1/api/mcp"
	t.Cleanup(func() { jetbrainsMCPURL = prev })
	for _, args := range [][]string{{"bridge", "probe"}, {"bridge", "open", "a.go"}, {"bridge", "problems", "a.go"}, {"bridge", "symbols"}, {"bridge", "build"}, {"bridge", "run", "x"}} {
		if out, code := cliOutput(t, args...); code != ExitError || !strings.Contains(out, "is the IDE running?") {
			t.Errorf("%v: exit %d\n%s", args, code, out)
		}
	}
	for _, args := range [][]string{{"bridge", "open"}, {"bridge", "run"}, {"bridge", "setup", "--project-only", "--global-only"}} {
		if _, code := cliOutput(t, args...); code != ExitUsage {
			t.Errorf("%v: exit %d, want %d", args, code, ExitUsage)
		}
	}
}

func TestBridge_SetupNeedsNoIDE(t *testing.T) {
	prev := jetbrainsMCPURL
	jetbrainsMCPURL = "http://127.0.0.1:1/api/mcp"
	t.Cleanup(func() { jetbrainsMCPURL = prev })
	t.Setenv("HOME", t.TempDir())
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, ".idea"), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Chdir(dir)
	if out, code := cliOutput(t, "bridge", "setup", "--project-only"); code != ExitOK || !strings.Contains(out, "configured successfully") {
		t.Fatalf("setup: exit %d\n%s", code, out)
	}
}
