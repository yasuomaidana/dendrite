package mcpserver

import (
	"context"
	"testing"
)

func TestStoryForkTools_RegistrationAndExecution(t *testing.T) {
	srv := NewServer(ServerConfig{})
	RegisterDefaultTools(srv, Dependencies{})

	expectedTools := []string{
		"dendrite_story_fork",
		"dendrite_story_benchmark",
		"dendrite_story_resolve",
		"dendrite_story_abort",
	}

	registered := make(map[string]bool)
	for _, tool := range srv.ListTools() {
		registered[tool.Name] = true
	}

	for _, name := range expectedTools {
		if !registered[name] {
			t.Errorf("expected MCP tool %s to be registered", name)
		}
	}

	ctx := context.Background()

	// Test validation errors on handler calls
	forkHandler, _ := srv.Handler("dendrite_story_fork")
	_, err := forkHandler(ctx, map[string]interface{}{"story_id": ""})
	if err == nil {
		t.Error("expected error on missing story_id")
	}

	benchHandler, _ := srv.Handler("dendrite_story_benchmark")
	_, err = benchHandler(ctx, map[string]interface{}{"story_id": ""})
	if err == nil {
		t.Error("expected error on missing story_id")
	}

	resolveHandler, _ := srv.Handler("dendrite_story_resolve")
	_, err = resolveHandler(ctx, map[string]interface{}{"story_id": "", "winner": ""})
	if err == nil {
		t.Error("expected error on missing parameters")
	}

	abortHandler, _ := srv.Handler("dendrite_story_abort")
	_, err = abortHandler(ctx, map[string]interface{}{"story_id": ""})
	if err == nil {
		t.Error("expected error on missing story_id")
	}

	// Abort on nonexistent story should succeed cleanly
	res, err := abortHandler(ctx, map[string]interface{}{"story_id": "nonexistent-story-123"})
	if err != nil {
		t.Fatalf("abort handler failed: %v", err)
	}
	if len(res.Content) == 0 {
		t.Error("expected content in abort response")
	}
}
