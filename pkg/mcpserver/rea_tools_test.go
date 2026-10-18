package mcpserver

import (
	"context"
	"strings"
	"testing"
)

func TestREATools_RegistrationAndExecution(t *testing.T) {
	srv := NewServer(ServerConfig{})
	RegisterDefaultTools(srv, Dependencies{DevStatus: "../../DEV_STATUS.md"})

	tools := srv.ListTools()
	found := false
	for _, tool := range tools {
		if tool.Name == "dendrite_rea_showcase" {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("expected dendrite_rea_showcase to be registered among tools")
	}

	handler, exists := srv.Handler("dendrite_rea_showcase")
	if !exists {
		t.Fatalf("handler for dendrite_rea_showcase not found")
	}

	ctx := context.Background()

	// 1. Execute summary mode
	res, err := handler(ctx, map[string]interface{}{
		"target": "all",
		"format": "summary",
	})
	if err != nil {
		t.Fatalf("dendrite_rea_showcase failed: %v", err)
	}
	if len(res.Content) != 2 {
		t.Fatalf("expected 2 content items in summary mode, got %d", len(res.Content))
	}
	if !strings.Contains(res.Content[0].Text, "REA Showcase Verification") {
		t.Errorf("expected summary text, got: %s", res.Content[0].Text)
	}

	// 2. Execute json mode
	resJSON, err := handler(ctx, map[string]interface{}{
		"target": "th04",
		"format": "json",
	})
	if err != nil {
		t.Fatalf("dendrite_rea_showcase json failed: %v", err)
	}
	if len(resJSON.Content) != 1 || !strings.Contains(resJSON.Content[0].Text, `"target": "th04"`) {
		t.Errorf("unexpected json result: %+v", resJSON.Content)
	}

	// 3. Execute markdown mode
	resMD, err := handler(ctx, map[string]interface{}{
		"target": "dxball",
		"format": "markdown",
	})
	if err != nil {
		t.Fatalf("dendrite_rea_showcase markdown failed: %v", err)
	}
	if len(resMD.Content) != 1 || !strings.Contains(resMD.Content[0].Text, "# Open-Source REA Showcase") {
		t.Errorf("unexpected markdown result: %+v", resMD.Content)
	}

	// 4. Unknown target error handling
	_, err = handler(ctx, map[string]interface{}{
		"target": "nonexistent_target_xyz",
	})
	if err == nil {
		t.Fatalf("expected error on unknown target, got nil")
	}
}
