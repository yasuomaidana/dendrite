package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"Dendrite/pkg/rag"
	"Dendrite/pkg/rag/ragtest"
	"Dendrite/pkg/sidecar"
)

func TestAsk_UsageErrors(t *testing.T) {
	for _, args := range [][]string{
		{"ask"},
		{"ask", "q", "--tier", "galactic"},
		{"ask", "q", "--top-k", "-1"},
		{"ask", "q", "--paid", "--hybrid"},
		{"ask", "q", "--corpus", "galactic"},
		{"ask", "q", "-d", "--corpus", "docs"},
		{"ask", "q", "--docs-share", "1.5"},
		{"ask", "q", "--corpus", "code", "--docs-share", "0.5"},
	} {
		if _, code := cliOutput(t, args...); code != ExitUsage {
			t.Errorf("%v: exit %d, want %d", args, code, ExitUsage)
		}
	}
}

func TestAsk_CentralServerUnreachable(t *testing.T) {
	out, code := cliOutput(t, "ask", "--server", "http://127.0.0.1:1", "--project", t.TempDir(), "--tier", "hybrid", "where", "is", "telemetry?")
	if code != ExitError || !strings.Contains(out, `Forwarding query to Central RAG Service at http://127.0.0.1:1 for: "where is telemetry?"`) {
		t.Fatalf("unreachable server: exit %d\n%s", code, out)
	}
}

func TestAsk_CorpusFlagsAndTimings(t *testing.T) {
	bin, err := ragtest.Build(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	t.Setenv(sidecar.EnvVar(rag.SidecarBinary), bin)
	t.Setenv("DENDRITE_RAG_SERVER", "")
	project := t.TempDir()
	if err := os.WriteFile(filepath.Join(project, "main.go"), []byte("package main\n\n// Locate finds the sidecar.\nfunc Locate() {}\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		flags  []string
		corpus string
		share  string
	}{
		{nil, "code", ""},
		{[]string{"-d"}, "all", ""},
		{[]string{"--docs"}, "all", ""},
		{[]string{"--corpus", "docs"}, "docs", ""},
		{[]string{"--corpus", "ALL"}, "all", ""},
		{[]string{"-d", "--docs-share", "0.5"}, "all", "0.5"},
		{[]string{"--docs-share", "0"}, "all", "0"},
	} {
		log := filepath.Join(t.TempDir(), "calls.log")
		t.Setenv("STANDIN_LOG", log)
		args := append([]string{"ask", "--project", project}, tc.flags...)
		out, code := cliOutput(t, append(args, "where", "is", "Locate")...)
		if code != 0 || !strings.Contains(out, "[i] Timing: sidecar start") || !strings.Contains(out, "local model 120 ms") {
			t.Fatalf("%v: exit %d\n%s", tc.flags, code, out)
		}
		calls, _ := os.ReadFile(log)
		if !strings.Contains(string(calls), `"corpus":"`+tc.corpus+`"`) {
			t.Errorf("%v: ask did not send corpus %s:\n%s", tc.flags, tc.corpus, calls)
		}
		if sent := strings.Contains(string(calls), `"docs_share":`); sent != (tc.share != "") ||
			(tc.share != "" && !strings.Contains(string(calls), `"docs_share":`+tc.share)) {
			t.Errorf("%v: docs_share sent = %v, want %q:\n%s", tc.flags, sent, tc.share, calls)
		}
	}
}
