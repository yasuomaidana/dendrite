package agent

import (
	"bytes"
	"context"
	"strings"
	"testing"
)

func TestGetAgentInstallSpec(t *testing.T) {
	tests := []struct {
		agentType AgentType
		wantBin   string
	}{
		{AgentAider, "aider"},
		{AgentCopilot, "copilot"},
		{AgentGitHubCopilot, "copilot"},
		{AgentOpenHands, "openhands"},
		{AgentOpenCode, "opencode"},
		{AgentAGY, "agy"},
	}

	for _, tt := range tests {
		t.Run(string(tt.agentType), func(t *testing.T) {
			spec, err := GetAgentInstallSpec(tt.agentType)
			if err != nil {
				t.Fatalf("unexpected error for %s: %v", tt.agentType, err)
			}
			if spec.Binary != tt.wantBin {
				t.Errorf("expected binary %s, got %s", tt.wantBin, spec.Binary)
			}
			if spec.Command == "" || len(spec.Args) == 0 {
				t.Errorf("expected command and args to be populated, got %s %v", spec.Command, spec.Args)
			}
		})
	}
}

func TestPromptAndInstallAgent_Decline(t *testing.T) {
	ctx := context.Background()
	in := bytes.NewBufferString("n\n")
	var out bytes.Buffer

	// Use non-existent binary spec
	err := PromptAndInstallAgent(ctx, "nonexistent-agent", false, in, &out)
	if err == nil {
		t.Fatal("expected error for unsupported agent")
	}

	// For Aider if we pretend it's missing by testing with a decline response
	inDecline := bytes.NewBufferString("no\n")
	var outDecline bytes.Buffer
	errDecline := PromptAndInstallAgent(ctx, AgentOpenCode, false, inDecline, &outDecline)
	if errDecline == nil {
		t.Fatal("expected error when user declines")
	}
	if !strings.Contains(outDecline.String(), "Installation cancelled by user") {
		t.Errorf("expected cancellation message, got: %s", outDecline.String())
	}
}
