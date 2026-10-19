package main

import (
	"bytes"
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestSoma_UsageErrors(t *testing.T) {
	ctx := context.Background()
	for _, args := range [][]string{
		{"soma", "plan", "--mode", "bogus"},
		{"env", "replicate", "--mode=nope"},
		{"soma", "install", "litellm", "--mode", "x"},
		{"soma", "install"},
		{"soma", "plan", "--with-plane-ui", "--no-plane-ui"},
		{"soma", "prepare", "a", "b"},
		{"soma", "prepare", "a", "--path", "b"},
		{"soma", "status", "extra"},
	} {
		var buf bytes.Buffer
		err := runCLI(ctx, &buf, args...)
		if got := exitCodeFor(err); got != ExitUsage {
			t.Errorf("%v: exit %d, want %d (err %v)", args, got, ExitUsage, err)
		}
	}
}

func TestSoma_PlanJSON(t *testing.T) {
	var buf bytes.Buffer
	if err := runCLI(context.Background(), &buf, "soma", "plan", "--mode", "host", "--with-plane", "--no-plane-ui", "--json"); err != nil {
		t.Fatalf("%v\n%s", err, buf.String())
	}
	if !strings.HasPrefix(strings.TrimSpace(buf.String()), "{") {
		t.Fatalf("want JSON plan, got %q", buf.String())
	}
}

func TestPlaneUIFlag(t *testing.T) {
	cmd := newSomaPlanCmd()
	if planeUIFlag(cmd) != nil {
		t.Fatal("unset flags should give nil")
	}
	_ = cmd.Flags().Set("with-plane-ui", "true")
	if v := planeUIFlag(cmd); v == nil || !*v {
		t.Fatal("--with-plane-ui should give true")
	}
	cmd = newSomaPlanCmd()
	_ = cmd.Flags().Set("no-plane-ui", "true")
	if v := planeUIFlag(cmd); v == nil || *v {
		t.Fatal("--no-plane-ui should give false")
	}
}

func TestSomaDiagnose(t *testing.T) {
	ctx := context.Background()
	t.Chdir(t.TempDir())

	var buf bytes.Buffer
	if err := runSomaDiagnose(ctx, "http://127.0.0.1:1", &buf); err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"Local Ollama: UNREACHABLE", "DEV_STATUS.md not found", "diagnostics complete"} {
		if !strings.Contains(buf.String(), want) {
			t.Errorf("missing %q in %q", want, buf.String())
		}
	}

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"models":[{"name":"qwen","size":1073741824}]}`))
	}))
	defer srv.Close()
	buf.Reset()
	if err := runSomaDiagnose(ctx, srv.URL, &buf); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(buf.String(), "Local Ollama: REACHABLE") {
		t.Errorf("want reachable, got %q", buf.String())
	}
}
