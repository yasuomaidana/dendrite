package mcpserver

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestDendriteLocalAgentTriageTool(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"models":[{"name":"qwen2.5-coder:7b"}]}`))
	}))
	defer ts.Close()

	srv := NewServer(ServerConfig{Name: "dendrite", Version: "1.0.0"})
	RegisterDefaultTools(srv, Dependencies{
		DevStatus: "DEV_STATUS.md",
		OllamaURL: ts.URL,
	})

	// 1. Test chore task with forced local tier
	reqJSON := `{
		"jsonrpc": "2.0",
		"id": 101,
		"method": "tools/call",
		"params": {
			"name": "dendrite_local_agent_triage",
			"arguments": {
				"prompt": "Fix linter errors and format this file",
				"force_tier": "local",
				"execute_local": true
			}
		}
	}`

	resp := srv.HandleRequest(context.Background(), []byte(reqJSON))
	if resp.Error != nil {
		t.Fatalf("unexpected error from dendrite_local_agent_triage: %v", resp.Error)
	}

	resultMap, ok := resp.Result.(*CallToolResult)
	if !ok {
		t.Fatalf("expected *CallToolResult, got %T", resp.Result)
	}
	if len(resultMap.Content) == 0 {
		t.Fatalf("expected non-empty content list")
	}

	text := resultMap.Content[0].Text
	if !strings.Contains(text, `"tier": "local"`) {
		t.Errorf("expected tier 'local' in output, got: %s", text)
	}
	if !strings.Contains(text, `"token_savings_pct": 100`) {
		t.Errorf("expected 100%% token savings in output, got: %s", text)
	}

	// 2. Test hybrid task with skill matching
	hybridReq := `{
		"jsonrpc": "2.0",
		"id": 102,
		"method": "tools/call",
		"params": {
			"name": "dendrite_local_agent_triage",
			"arguments": {
				"prompt": "Evaluate ticket DEN-050 and check cyclomatic complexity before refactoring",
				"force_tier": "hybrid"
			}
		}
	}`

	hybridResp := srv.HandleRequest(context.Background(), []byte(hybridReq))
	if hybridResp.Error != nil {
		t.Fatalf("unexpected error from hybrid triage: %v", hybridResp.Error)
	}

	var parsed struct {
		Tier           string   `json:"tier"`
		SelectedSkills []string `json:"selected_skills"`
		EnhancedPrompt string   `json:"enhanced_prompt"`
	}
	hResultMap := hybridResp.Result.(*CallToolResult)
	hContent := hResultMap.Content[0].Text
	if err := json.Unmarshal([]byte(hContent), &parsed); err != nil {
		t.Fatalf("failed to parse triage result json: %v", err)
	}

	if parsed.Tier != "hybrid" {
		t.Errorf("expected tier 'hybrid', got %s", parsed.Tier)
	}
	if len(parsed.SelectedSkills) == 0 {
		t.Errorf("expected selected skills, got empty list")
	}
}
