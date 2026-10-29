package main

import (
	"bytes"
	"context"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"Dendrite/pkg/inference"
)

func TestSpike_ListAndProbe(t *testing.T) {
	isolateEngines(t)
	if out, code := cliOutput(t, "spike", "list"); code != ExitOK || !strings.Contains(out, "ollama-metal") || !strings.Contains(out, "Cascade Failover") {
		t.Fatalf("list: exit %d\n%s", code, out)
	}
	if out, code := cliOutput(t, "inference", "ls", "--json"); code != ExitOK || !strings.Contains(out, `"cascade"`) {
		t.Fatalf("list --json: exit %d\n%s", code, out)
	}
	if out, code := cliOutput(t, "spike", "probe", "ollama-metal"); code != ExitOK || !strings.Contains(out, "[Offline] Engine: ollama-metal") {
		t.Fatalf("probe one: exit %d\n%s", code, out)
	}
	out, code := cliOutput(t, "spike", "probe")
	if code != ExitOK || strings.Index(out, "candle") > strings.Index(out, "ollama-metal") {
		t.Fatalf("probe all should list engines sorted: exit %d\n%s", code, out)
	}
	if out, code := cliOutput(t, "spike", "probe", "--json"); code != ExitOK || !strings.HasPrefix(strings.TrimSpace(out), "[") {
		t.Fatalf("probe --json: exit %d\n%s", code, out)
	}
	for _, args := range [][]string{
		{"spike", "probe", "no-such-engine"},
		{"spike", "probe", "ollama-metal", "--endpoint", "http://x"},
		{"spike", "switch", "no-such-engine"},
		{"spike", "switch"},
		{"spike", "migrate"},
	} {
		if _, code := cliOutput(t, args...); code != ExitUsage {
			t.Errorf("%v: exit %d, want %d", args, code, ExitUsage)
		}
	}
}

func TestSpike_ProbeEndpoint(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"models":[{"name":"qwen"}]}`))
	}))
	defer srv.Close()
	if out, code := cliOutput(t, "spike", "probe", "--endpoint", srv.URL); code != ExitOK || !strings.Contains(out, "- qwen") {
		t.Fatalf("probe --endpoint: exit %d\n%s", code, out)
	}
	if _, code := cliOutput(t, "spike", "probe", "--endpoint", "http://127.0.0.1:1"); code != ExitError {
		t.Fatalf("unreachable endpoint: exit %d", code)
	}
}

func TestSpike_SwitchAndRender(t *testing.T) {
	isolateEngines(t)
	if out, code := cliOutput(t, "spike", "switch", "oxillama-rust"); code != ExitOK || !strings.Contains(out, "New cascade: oxillama-rust ->") {
		t.Fatalf("switch: exit %d\n%s", code, out)
	}
	if out, code := cliOutput(t, "spike", "render"); code != ExitOK || !strings.Contains(out, "Rendered LiteLLM Configuration") {
		t.Fatalf("render: exit %d\n%s", code, out)
	}
}

func TestSpike_MigrateMissingModelFails(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	if _, code := cliOutput(t, "spike", "migrate", "no-such-model:1b", "--dry-run"); code != ExitError {
		t.Fatalf("migrate of a missing model: exit %d", code)
	}
}

func TestSpike_ServeUntilCancelled(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	port := ln.Addr().(*net.TCPAddr).Port
	_ = ln.Close()

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	var buf bytes.Buffer
	if err := runSpikeServe(ctx, port, &buf); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(buf.String(), "listening at http://127.0.0.1:") {
		t.Fatalf("unexpected output %q", buf.String())
	}

	// A taken port is a runtime error.
	busy, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer busy.Close()
	if err := runSpikeServe(ctx, busy.Addr().(*net.TCPAddr).Port, &buf); err == nil {
		t.Fatal("serving on a busy port should fail")
	}
}

func TestSpike_ZeroCost(t *testing.T) {
	t.Chdir(t.TempDir())
	// Zero-cost rules are machine-wide: they sync to $DENDRITE_HOME/.env, never to the current directory.
	home := t.TempDir()
	t.Setenv("DENDRITE_HOME", home)
	if err := os.WriteFile(filepath.Join(home, ".env"), []byte("FOO=bar\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(".env", []byte("UNTOUCHED=1\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _, _ = cliOutput(t, "spike", "zero-cost", "reset") })

	steps := []struct {
		args []string
		code int
		want string
	}{
		{[]string{"spike", "zero-cost", "add", "acme-local-7b"}, ExitOK, "as zero-cost exact model"},
		{[]string{"spike", "zero-cost", "add", "acme", "--prefix"}, ExitOK, "as zero-cost prefix rule"},
		{[]string{"spike", "zero-cost", "test", "acme-local-7b"}, ExitOK, "Zero-Cost: TRUE"},
		{[]string{"spike", "zerocost", "check", "gpt-9-ultra", "--json"}, ExitOK, `"zero_cost": false`},
		{[]string{"spike", "zero-cost", "list"}, ExitOK, "acme-local-7b"},
		{[]string{"spike", "zero-cost", "ls", "--json"}, ExitOK, `"acme-local-7b"`},
		{[]string{"spike", "zero-cost", "rm", "acme-local-7b"}, ExitOK, "Synchronized with environment file: " + filepath.Join(home, ".env")},
		{[]string{"spike", "zero-cost", "remove", "never-added"}, ExitOK, "Removed 'never-added'"},
		{[]string{"spike", "zero-cost", "remove", " "}, ExitUsage, ""},
		{[]string{"spike", "zero-cost", "add", " "}, ExitUsage, ""},
		{[]string{"spike", "zero-cost", "reset"}, ExitOK, "reset zero-cost models"},
	}
	for _, s := range steps {
		out, code := cliOutput(t, s.args...)
		if code != s.code || !strings.Contains(out, s.want) {
			t.Errorf("%v: exit %d (want %d)\n%s", s.args, code, s.code, out)
		}
	}
	if ok, _ := inference.GetZeroCostRegistry().Test("acme-local-7b"); ok {
		t.Error("reset should drop the added model")
	}
	if raw, _ := os.ReadFile(".env"); string(raw) != "UNTOUCHED=1\n" {
		t.Errorf("the working directory's .env was modified: %q", raw)
	}
	if raw, _ := os.ReadFile(filepath.Join(home, ".env")); !strings.Contains(string(raw), "FOO=bar") {
		t.Errorf("the global .env lost its other settings: %q", raw)
	}
}

func TestPrintEngineHealth(t *testing.T) {
	var buf bytes.Buffer
	printEngineHealth(&buf, inference.EngineHealth{EngineID: "e1", Reachable: true, ZeroCost: true, Models: []string{"m"}, Error: "warn"})
	for _, want := range []string{"[Online]", "zero-cost", "Models: m", "Error:  warn"} {
		if !strings.Contains(buf.String(), want) {
			t.Errorf("missing %q in %q", want, buf.String())
		}
	}
}

func TestSpike_ModelsFlagsWhatDoesNotFit(t *testing.T) {
	t.Setenv("DENDRITE_MODEL_MEM_BUDGET_GB", "12")
	t.Setenv("HOME", t.TempDir())
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/tags":
			_, _ = w.Write([]byte(`{"models":[{"name":"huge:30b","size":19327352832},{"name":"small:4b","size":2684354560}]}`))
		case "/api/ps":
			_, _ = w.Write([]byte(`{"models":[]}`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()
	out, code := cliOutput(t, "spike", "models", "--ollama", srv.URL, "--oxillama", "http://127.0.0.1:1")
	if code != ExitOK {
		t.Fatalf("exit %d\n%s", code, out)
	}
	for _, want := range []string{"12.0 GB budget", "huge:30b", "TOO LARGE", "small:4b", "Best available now: small:4b on ollama", "OxiLLaMa is not reachable"} {
		if !strings.Contains(out, want) {
			t.Errorf("output lacks %q:\n%s", want, out)
		}
	}
}
