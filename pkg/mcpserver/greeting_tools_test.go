package mcpserver

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"Dendrite/pkg/personalcontext/greeting"
)

func TestMCP_GreetingTools(t *testing.T) {
	srv := NewServer(ServerConfig{Name: "dendrite", Version: "1.0.0"})
	RegisterDefaultTools(srv, Dependencies{
		DevStatus: "DEV_STATUS.md",
	})
	ctx := context.Background()

	t.Run("generate greeting via MCP", func(t *testing.T) {
		reqJSON := `{
			"jsonrpc": "2.0",
			"id": 101,
			"method": "tools/call",
			"params": {
				"name": "dendrite_pcontext_greeting_generate",
				"arguments": {
					"topic": "Distributed Mesh",
					"milestone": "Phase 1 Complete",
					"milestone_achieved": true
				}
			}
		}`

		resp := srv.HandleRequest(ctx, []byte(reqJSON))
		if resp.Error != nil {
			t.Fatalf("HandleRequest failed: %v", resp.Error)
		}

		res, ok := resp.Result.(*CallToolResult)
		if !ok || len(res.Content) == 0 {
			t.Fatalf("expected *CallToolResult with content")
		}

		var gen greeting.GeneratedGreeting
		if err := json.Unmarshal([]byte(res.Content[0].Text), &gen); err != nil {
			t.Fatalf("failed to unmarshal output: %v", err)
		}

		if gen.StyleID != greeting.StyleProgressCelebration {
			t.Errorf("expected G-02, got %s", gen.StyleID)
		}
		if !strings.HasPrefix(gen.Headline, "# ") {
			t.Errorf("expected headline to start with '# ', got %s", gen.Headline)
		}
	})

	t.Run("suppressed style via MCP", func(t *testing.T) {
		reqJSON := `{
			"jsonrpc": "2.0",
			"id": 102,
			"method": "tools/call",
			"params": {
				"name": "dendrite_pcontext_greeting_generate",
				"arguments": {
					"topic": "Distributed Mesh",
					"milestone": "Phase 1 Complete",
					"milestone_achieved": true,
					"recent_styles": ["G-02"]
				}
			}
		}`

		resp := srv.HandleRequest(ctx, []byte(reqJSON))
		if resp.Error != nil {
			t.Fatalf("HandleRequest failed: %v", resp.Error)
		}

		res, ok := resp.Result.(*CallToolResult)
		if !ok || len(res.Content) == 0 {
			t.Fatalf("expected *CallToolResult with content")
		}

		var gen greeting.GeneratedGreeting
		if err := json.Unmarshal([]byte(res.Content[0].Text), &gen); err != nil {
			t.Fatalf("failed to unmarshal output: %v", err)
		}

		if gen.StyleID == greeting.StyleProgressCelebration {
			t.Errorf("expected G-02 to be suppressed, got %s", gen.StyleID)
		}
	})
}
