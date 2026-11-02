package mcpserver

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestBenchToolDefaultsAndFailures(t *testing.T) {
	srv := toolsWith(t, nil, "")
	if text, isErr := callTool(t, srv, "dendrite_bench_run", `{"mode":"stress"}`); isErr {
		t.Errorf("a stress run with the default users: %s", text)
	}
	if text, isErr := callTool(t, srv, "dendrite_bench_run", `{"mode":"inference","engine_id":"ollama-metal","duration_seconds":1,"virtual_users":1}`); isErr && !strings.Contains(text, "benchmark execution failed") {
		t.Errorf("a known engine supplies its endpoint: %s", text)
	}
	dead := closedURL()
	if text, isErr := callTool(t, srv, "dendrite_bench_run", `{"mode":"inference","endpoint":"`+dead+`","duration_seconds":1,"virtual_users":1}`); isErr && !strings.Contains(text, "benchmark execution failed") {
		t.Errorf("an endpoint that is down: %s", text)
	}
}

func TestTokenStatusToolReadsItsFiles(t *testing.T) {
	dir := t.TempDir()
	spend := filepath.Join(dir, "spend.json")
	policy := filepath.Join(dir, "policy.json")
	if err := os.WriteFile(spend, []byte(`{}`), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(policy, []byte(`{broken`), 0o644); err != nil {
		t.Fatal(err)
	}
	srv := toolsWith(t, nil, "")
	text, isErr := callTool(t, srv, "dendrite_tokens_status", `{"spend_file":"`+spend+`","policy_file":"`+policy+`","model":"m"}`)
	if isErr && !strings.Contains(text, "evaluating token status") {
		t.Errorf("text = %q", text)
	}
}

func closedURL() string {
	s := httptest.NewServer(http.NotFoundHandler())
	s.Close()
	return s.URL
}

func TestBenchToolReportsAK6RunThatFails(t *testing.T) {
	bin := t.TempDir()
	if err := os.WriteFile(filepath.Join(bin, "k6"), []byte("#!/bin/sh\nexit 1\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", bin+string(os.PathListSeparator)+"/usr/bin:/bin")
	srv := toolsWith(t, nil, "")
	if text, isErr := callTool(t, srv, "dendrite_bench_run", `{"mode":"inference","endpoint":"http://127.0.0.1:1","duration_seconds":1,"virtual_users":1}`); !isErr || !strings.Contains(text, "benchmark execution failed") {
		t.Fatalf("text = %q (error %v)", text, isErr)
	}
}

// resetScheduler puts the shared scheduler back to automatic mode with a full quota.
func resetScheduler(t *testing.T, srv *Server) {
	t.Helper()
	t.Cleanup(func() {
		callTool(t, srv, "dendrite_analyze_policy", `{"mode":"auto","allow_zero":false}`)
		callTool(t, srv, "dendrite_analyze_status", `{"remaining_quota":1}`)
	})
}

func TestAnalyzeTriggerReportsWhyAChunkCannotRun(t *testing.T) {
	srv := toolsWith(t, nil, "")
	resetScheduler(t, srv)

	if text, isErr := callTool(t, srv, "dendrite_analyze_policy", `{"mode":"paused"}`); isErr {
		t.Fatalf("pausing: %s", text)
	}
	if text, isErr := callTool(t, srv, "dendrite_analyze_trigger", `{"chunk_id":"c1"}`); !isErr || !strings.Contains(text, "processing chunk") {
		t.Errorf("a paused scheduler: %q (error %v)", text, isErr)
	}

	callTool(t, srv, "dendrite_analyze_policy", `{"mode":"auto"}`)
	callTool(t, srv, "dendrite_analyze_status", `{"remaining_quota":0.01}`)
	if text, isErr := callTool(t, srv, "dendrite_analyze_trigger", `{"chunk_id":"c2","simulate_sprint":true}`); !isErr || !strings.Contains(text, "processing chunk") {
		t.Errorf("the quota governor refuses the cloud: %q (error %v)", text, isErr)
	}
}
