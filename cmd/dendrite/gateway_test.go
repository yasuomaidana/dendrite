package main

import (
	"bytes"
	"context"
	"fmt"
	"path/filepath"
	"strings"
	"testing"

	"Dendrite/pkg/gateway"
)

// cliOutput runs the real tree and returns what a user would see (output, then any
// error the way main prints it) and the exit code.
func cliOutput(t *testing.T, args ...string) (string, int) {
	t.Helper()
	var buf bytes.Buffer
	err := runCLI(context.Background(), &buf, args...)
	if err != nil {
		fmt.Fprintf(&buf, "[-] %v\n", err)
	}
	return buf.String(), exitCodeFor(err)
}

// isolateEngines points every inference engine at a closed port so probes fail fast.
func isolateEngines(t *testing.T) {
	t.Helper()
	for _, e := range []string{"OLLAMA", "OXILLAMA", "VLLM", "HF_TGI", "HF_TEI", "CANDLE", "LLAMACPP"} {
		t.Setenv("DENDRITE_"+e+"_ENDPOINT", "http://127.0.0.1:1")
	}
}

func TestCleft_Render(t *testing.T) {
	out, code := cliOutput(t, "cleft", "render")
	if code != ExitOK || !strings.Contains(out, "scenario 'local-hybrid'") {
		t.Fatalf("render default: exit %d\n%s", code, out)
	}
	t.Setenv("TAILSCALE_CONTROL_PLANE_IP", "100.1.1.1")
	t.Setenv("TAILSCALE_EXECUTION_PLANE_IP", "100.1.1.2")
	if out, code := cliOutput(t, "gateway", "render", "distributed"); code != ExitOK || !strings.Contains(out, "scenario 'distributed'") {
		t.Fatalf("render distributed: exit %d\n%s", code, out)
	}
	if _, code := cliOutput(t, "cleft", "render", "moon-base"); code != ExitUsage {
		t.Fatalf("bad scenario: exit %d", code)
	}
}

func TestCleft_UsageErrors(t *testing.T) {
	for _, args := range [][]string{
		{"cleft", "local-hybrid"},
		{"cleft", "zero-cost"},
		{"cleft", "schedule", "--interval", "0s"},
		{"cleft", "schedule", "--interval", "soon"},
		{"cleft", "update", "extra"},
	} {
		if _, code := cliOutput(t, args...); code != ExitUsage {
			t.Errorf("%v: exit %d, want %d", args, code, ExitUsage)
		}
	}
}

func TestCleft_UpdateDryRun(t *testing.T) {
	isolateEngines(t)
	cfg := filepath.Join(t.TempDir(), "litellm_config.yaml")
	out, code := cliOutput(t, "cleft", "update", "--dry-run", "--file", cfg)
	if code != ExitOK || !strings.Contains(out, "Simulated Update (Dry-Run)") {
		t.Fatalf("update --dry-run: exit %d\n%s", code, out)
	}
	if out, code := cliOutput(t, "cleft", "update", "--dry-run", "-f", cfg, "--json"); code != ExitOK || !strings.HasPrefix(strings.TrimSpace(out), "{") {
		t.Fatalf("update --json: exit %d\n%s", code, out)
	}
}

func TestCleft_ScheduleStopsWithContext(t *testing.T) {
	isolateEngines(t)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	opts := gateway.DefaultUpdaterOptions()
	opts.ConfigPath = filepath.Join(t.TempDir(), "litellm_config.yaml")
	opts.DryRun = true
	var buf bytes.Buffer
	if err := runCleftSchedule(ctx, opts, &buf); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(buf.String(), "Starting LiteLLM Routing Table Scheduler") {
		t.Fatalf("unexpected output %q", buf.String())
	}
}

func TestAxon_Inspect(t *testing.T) {
	out, code := cliOutput(t, "axon", "inspect")
	if code != ExitOK || !strings.Contains(out, "Active Scenario: local-hybrid") {
		t.Fatalf("inspect: exit %d\n%s", code, out)
	}
	// Placements print in a stable order.
	again, _ := cliOutput(t, "axon", "inspect")
	if again != out {
		t.Fatal("inspect output is not deterministic")
	}
	if out, code := cliOutput(t, "topology", "show", "distributed", "--json"); code != ExitOK || !strings.Contains(out, `"stack_placement"`) {
		t.Fatalf("inspect --json: exit %d\n%s", code, out)
	}
	if _, code := cliOutput(t, "axon", "inspect", "nowhere"); code != ExitUsage {
		t.Fatalf("bad scenario: exit %d", code)
	}
	if _, code := cliOutput(t, "axon", "local-hybrid"); code != ExitUsage {
		t.Fatalf("bare scenario is no longer a subcommand: exit %d", code)
	}
}

func TestAxon_AdvisorLocal(t *testing.T) {
	out, code := cliOutput(t, "axon", "advisor", "--json")
	if code != ExitOK {
		t.Fatalf("advisor --json: exit %d\n%s", code, out)
	}
	if !strings.HasPrefix(strings.TrimSpace(out), "{") || strings.Contains(out, "Probing") {
		t.Fatalf("JSON mode must print only the plan:\n%s", out)
	}
	if out, code := cliOutput(t, "axon", "advise"); code != ExitOK || !strings.Contains(out, "Probing 1 compute node") {
		t.Fatalf("advisor: exit %d\n%s", code, out)
	}
}

func TestParseNodeTargets(t *testing.T) {
	got := parseNodeTargets(" local=localhost, gpu = 10.0.0.5 ,box,,")
	if len(got) != 3 {
		t.Fatalf("got %+v", got)
	}
	if got[0].ID != "local" || !got[0].IsLocal || got[1].ID != "gpu" || got[1].Host != "10.0.0.5" || got[1].IsLocal || got[2].ID != "box" || got[2].Host != "box" {
		t.Fatalf("got %+v", got)
	}
	if def := parseNodeTargets(" , "); len(def) != 1 || def[0].ID != "local" || !def[0].IsLocal {
		t.Fatalf("empty input should default to localhost: %+v", def)
	}
}
