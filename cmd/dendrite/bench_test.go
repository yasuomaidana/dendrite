package main

import (
	"strings"
	"testing"
	"time"

	"Dendrite/pkg/perf"
)

func TestVelocity_UsageErrors(t *testing.T) {
	for _, args := range [][]string{
		{"velocity", "gateway", "--fault", "meteor"},
		{"velocity", "inference", "--vus", "0"},
		{"velocity", "matrix", "--duration", "0s"},
		{"bench", "stress", "--spends", "0"},
		{"velocity", "inference", "--duration", "later"},
	} {
		if _, code := cliOutput(t, args...); code != ExitUsage {
			t.Errorf("%v: exit %d, want %d", args, code, ExitUsage)
		}
	}
}

func TestParseFault(t *testing.T) {
	for in, want := range map[string]perf.FaultInjectionType{
		"none": perf.FaultNone, "": perf.FaultNone, "429": perf.FaultRateLimit429,
		"RATE-LIMIT-429": perf.FaultRateLimit429, "503": perf.FaultNodeGlitch503, "node-glitch-503": perf.FaultNodeGlitch503,
	} {
		if got, err := parseFault(in); err != nil || got != want {
			t.Errorf("parseFault(%q) = %v, %v", in, got, err)
		}
	}
}

func TestVelocity_Stress(t *testing.T) {
	if out, code := cliOutput(t, "velocity", "stress", "--vus", "2", "--spends", "20"); code != ExitOK || !strings.Contains(out, "Allowed Spends") {
		t.Fatalf("stress: exit %d\n%s", code, out)
	}
	if out, code := cliOutput(t, "velocity", "stress", "--vus", "2", "--spends", "20", "--json"); code != ExitOK || !strings.HasPrefix(strings.TrimSpace(out[strings.Index(out, "\n")+1:]), "{") {
		t.Fatalf("stress --json: exit %d\n%s", code, out)
	}
}

func TestFormatMatrixAndScorecard(t *testing.T) {
	md := formatMatrix([]*perf.BenchmarkSummary{
		{EngineID: "a"},
		{EngineID: "b", FailoverMetrics: &perf.FailoverMetrics{PreFlightSwitched: true, CascadedEngineID: "c"}},
		{EngineID: "d", FailoverMetrics: &perf.FailoverMetrics{FailoverTriggered: true}},
	})
	for _, want := range []string{"| `a` |", "Cascaded -> c", "Recovered", "Direct"} {
		if !strings.Contains(md, want) {
			t.Errorf("missing %q in\n%s", want, md)
		}
	}

	var sb strings.Builder
	if err := writeScorecard(benchLoad{}, "card", &sb); err != nil || !strings.Contains(sb.String(), "card") {
		t.Fatalf("stdout scorecard: %v %q", err, sb.String())
	}
	file := t.TempDir() + "/card.md"
	sb.Reset()
	if err := writeScorecard(benchLoad{Output: file}, "card", &sb); err != nil || !strings.Contains(sb.String(), "saved to") {
		t.Fatalf("file scorecard: %v %q", err, sb.String())
	}
	if err := writeScorecard(benchLoad{Output: t.TempDir() + "/missing/dir/card.md"}, "card", &sb); err == nil {
		t.Fatal("unwritable output should fail")
	}
	if (benchLoad{VUs: 1, Duration: time.Second}).validate() != nil {
		t.Fatal("valid load rejected")
	}
}
