package mcpserver

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"Dendrite/pkg/sidecar"
)

func TestProfilesOtherThanCoreExposeEverything(t *testing.T) {
	if !ProfileFull.InProfile("any_tool_at_all") {
		t.Error("the full profile exposes every tool")
	}
	if ProfileCore.InProfile("a_tool_outside_core") {
		t.Error("the core profile hides tools outside its list")
	}
}

func TestProbeReportsAHandlerThatReturnsNothing(t *testing.T) {
	got := probeOne(context.Background(), ProbeSpec{Tool: "silent"}, func(context.Context, map[string]interface{}) (*CallToolResult, error) {
		return nil, nil
	}, time.Second)
	if got.OK || got.Detail != "nil result" {
		t.Fatalf("probe = %+v", got)
	}
}

func TestTriageTierAndEngineNames(t *testing.T) {
	srv := toolsWith(t, nil, "")
	for _, engine := range []string{"ollama", "ollama-validated", "huggingface", "hf", "candle", "oxillama", ""} {
		for _, tier := range []string{"local", "hybrid", "cloud", ""} {
			callTool(t, srv, "dendrite_local_agent_triage", `{"prompt":"fix a typo","target_engine":"`+engine+`","force_tier":"`+tier+`"}`)
		}
	}
	if text, isErr := callTool(t, srv, "dendrite_local_agent_triage", `{"prompt":"x","execute_local":true}`); isErr && !strings.Contains(text, "triage failed") {
		t.Errorf("text = %q", text)
	}
}

func TestSmallToolsReportTheirFailures(t *testing.T) {
	srv := toolsWith(t, nil, "")
	missing := filepath.Join(t.TempDir(), "missing")
	file := filepath.Join(t.TempDir(), "file")
	if err := os.WriteFile(file, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	runToolFailures(t, srv, []toolFailure{
		{"env prepare on a file", "dendrite_env_prepare", `{"project_path":"` + file + `","dry_run":false}`, nil, "environment preparation failed"},
		{"migrate to an unreachable host", "dendrite_service_move", `{"target_host":"127.0.0.1","ssh_port":1,"timeout_seconds":1}`, nil, "migration failed"},
		{"test synthesis for a missing file", "dendrite_test_auto", `{"file":"` + missing + `"}`, nil, "test synthesis error"},
		{"inference migration without a model", "dendrite_inference_migrate_model", `{"model":" "}`, nil, "model name is required"},
		{"inference migration of a model that is not there", "dendrite_inference_migrate_model", `{"model":"no-such-model:1b","dry_run":true}`, nil, "model not found"},
		{"penpot sync of a missing file", "dendrite_ui_penpot_sync", `{"file":"` + missing + `","dry_run":true}`, nil, ""},
	})
}

func TestAgentSearchFailsWithoutTheSidecar(t *testing.T) {
	t.Setenv(sidecar.EnvVar("dendrite-rag"), filepath.Join(t.TempDir(), "no-such-binary"))
	srv := toolsWith(t, nil, "")
	runToolFailures(t, srv, []toolFailure{
		{"search", "dendrite_agent_search", `{"query":"find the parser"}`, nil, "agent search failed"},
		{"route", "dendrite_agent_search", `{"query":"find the parser","route":true}`, nil, "agent routing failed"},
	})
}
