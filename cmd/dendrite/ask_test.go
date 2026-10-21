package main

import (
	"strings"
	"testing"
)

func TestAsk_UsageErrors(t *testing.T) {
	for _, args := range [][]string{
		{"ask"},
		{"ask", "q", "--tier", "galactic"},
		{"ask", "q", "--top-k", "-1"},
		{"ask", "q", "--paid", "--hybrid"},
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
