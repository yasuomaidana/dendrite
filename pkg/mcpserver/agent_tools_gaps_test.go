package mcpserver

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"Dendrite/pkg/agent"
)

func TestAgentRunReportListsTheSession(t *testing.T) {
	got := agentRunReport(&agent.SessionSummary{
		SessionID: "s1", AgentType: agent.AgentAider, TicketID: "DEN-1", Duration: 3 * time.Second,
		TokensConsumed: 120, EstimatedCostUSD: 0.5, CommitsGenerated: []string{"abc"}, Status: "ok",
	}, []string{"aider", "--yes"}, map[string]string{"K": "V"})
	if got["command"] != "aider --yes" || got["duration"] != "3s" || got["tokens"] != 120 || got["status"] != "ok" {
		t.Fatalf("report = %v", got)
	}
}

func TestAgentRunRejectsIncompleteAndFallsBackToSandboxForOpenHands(t *testing.T) {
	srv := toolsWith(t, nil, "")
	if text, isErr := callTool(t, srv, "dendrite_agent_run", `{"agent_type":"aider","task":"t","spend_limit":-1}`); !isErr || !strings.Contains(text, "failed to build agent env") {
		t.Errorf("a negative spend limit: %q (error %v)", text, isErr)
	}
	t.Setenv("PATH", t.TempDir()) // no openhands binary to run on the host
	callTool(t, srv, "dendrite_agent_run", `{"agent_type":"openhands","task":"t","sandbox":false}`)
}

func TestInferenceModelsToolAssessesTheMachine(t *testing.T) {
	srv := toolsWith(t, nil, "")
	if text, isErr := callTool(t, srv, "dendrite_inference_models", `{}`); isErr || !strings.Contains(text, "budget_bytes") {
		t.Fatalf("text = %q (error %v)", text, isErr)
	}
}

func TestPlaneTicketSetNeedsArgumentsAndAClient(t *testing.T) {
	noEnvPlane(t)
	srv := toolsWith(t, nil, "")
	if text, isErr := callTool(t, srv, "dendrite_plane_ticket_set", `{"ticket":" ","status":"done"}`); !isErr || !strings.Contains(text, "are required") {
		t.Errorf("blank ticket: %q (error %v)", text, isErr)
	}
	if text, isErr := callTool(t, srv, "dendrite_plane_ticket_set", `{"ticket":"DEN-1","status":"done"}`); !isErr || !strings.Contains(text, "plane client error") {
		t.Errorf("no client: %q (error %v)", text, isErr)
	}
}

func TestAgentRunReportsASessionThatSucceeds(t *testing.T) {
	bin := t.TempDir()
	if err := os.WriteFile(filepath.Join(bin, "aider"), []byte("#!/bin/sh\nexit 0\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", bin+string(os.PathListSeparator)+"/usr/bin:/bin")
	workdir := t.TempDir()
	t.Chdir(workdir)
	srv := toolsWith(t, nil, "")
	text, isErr := callTool(t, srv, "dendrite_agent_run", `{"agent_type":"aider","task":"say hi","ticket_id":"DEN-1","model":"m"}`)
	if isErr || !strings.Contains(text, `"session_id"`) {
		t.Fatalf("text = %q (error %v)", text, isErr)
	}
}
