package mcpserver

import (
	"context"
	"strings"
	"testing"
)

func TestSchedulerTools_RegistrationAndExecution(t *testing.T) {
	srv := NewServer(ServerConfig{})
	RegisterDefaultTools(srv, Dependencies{})

	// Verify tool registration
	registered := make(map[string]bool)
	for _, tool := range srv.ListTools() {
		registered[tool.Name] = true
	}

	expectedTools := []string{
		"dendrite_analyze_policy",
		"dendrite_analyze_status",
		"dendrite_analyze_trigger",
		"dendrite_analyze_switch",
		"dendrite_tokens_status",
	}

	for _, toolName := range expectedTools {
		if !registered[toolName] {
			t.Errorf("expected tool %s to be registered", toolName)
		}
	}

	ctx := context.Background()

	// 1. Test dendrite_analyze_policy query
	policyHandler, _ := srv.Handler("dendrite_analyze_policy")
	res, err := policyHandler(ctx, map[string]interface{}{})
	if err != nil {
		t.Fatalf("policyHandler query failed: %v", err)
	}
	if len(res.Content) == 0 || !strings.Contains(res.Content[0].Text, "night_window") {
		t.Errorf("expected night_window in policy response: %+v", res.Content)
	}

	// 2. Test dendrite_analyze_policy update
	resUpdate, err := policyHandler(ctx, map[string]interface{}{
		"night_window":        "22:00-05:00",
		"weekly_reset":        "MON 00:00",
		"sprint_before_reset": "24h",
		"reserve_floor":       0.15,
		"allow_zero":          true,
		"mode":                "auto",
	})
	if err != nil {
		t.Fatalf("policyHandler update failed: %v", err)
	}
	if !strings.Contains(resUpdate.Content[0].Text, `"updated": true`) {
		t.Errorf("expected updated: true in response: %s", resUpdate.Content[0].Text)
	}

	// 3. Test dendrite_analyze_status
	statusHandler, _ := srv.Handler("dendrite_analyze_status")
	resStatus, err := statusHandler(ctx, map[string]interface{}{
		"remaining_quota": 0.85,
	})
	if err != nil {
		t.Fatalf("statusHandler failed: %v", err)
	}
	if len(resStatus.Content) == 0 || !strings.Contains(resStatus.Content[0].Text, "active_tier") {
		t.Errorf("expected active_tier in status response: %+v", resStatus.Content)
	}

	// 4. Test dendrite_analyze_trigger
	triggerHandler, _ := srv.Handler("dendrite_analyze_trigger")
	resTrigger, err := triggerHandler(ctx, map[string]interface{}{
		"chunk_id": "pkg/store/store.go",
	})
	if err != nil {
		t.Fatalf("triggerHandler failed: %v", err)
	}
	if len(resTrigger.Content) == 0 || !strings.Contains(resTrigger.Content[0].Text, "chunk-") && !strings.Contains(resTrigger.Content[0].Text, "pkg/store/store.go") {
		t.Errorf("expected chunk in trigger response: %+v", resTrigger.Content)
	}

	// 5. Test dendrite_analyze_trigger with simulated sprint
	resSprint, err := triggerHandler(ctx, map[string]interface{}{
		"chunk_id":        "pkg/ast/parser.go",
		"simulate_sprint": true,
	})
	if err != nil {
		t.Fatalf("triggerHandler sprint failed: %v", err)
	}
	if !strings.Contains(resSprint.Content[0].Text, "CLOUD_SPRINT") {
		t.Errorf("expected CLOUD_SPRINT in simulated sprint trigger response: %s", resSprint.Content[0].Text)
	}

	// 6. Test dendrite_analyze_trigger with simulated night
	resNight, err := triggerHandler(ctx, map[string]interface{}{
		"chunk_id":       "pkg/ast/parser.go",
		"simulate_night": true,
	})
	if err != nil {
		t.Fatalf("triggerHandler night failed: %v", err)
	}
	if !strings.Contains(resNight.Content[0].Text, "NIGHT_LOCAL") {
		t.Errorf("expected NIGHT_LOCAL in simulated night trigger response: %s", resNight.Content[0].Text)
	}

	// 7. Test dendrite_analyze_switch
	switchHandler, _ := srv.Handler("dendrite_analyze_switch")
	resSwitch, err := switchHandler(ctx, map[string]interface{}{
		"allow_zero": true,
		"mode":       "always",
	})
	if err != nil {
		t.Fatalf("switchHandler failed: %v", err)
	}
	if !strings.Contains(resSwitch.Content[0].Text, `"allow_zero_drain": true`) {
		t.Errorf("expected allow_zero_drain in switch response: %s", resSwitch.Content[0].Text)
	}

	// 8. Test dendrite_tokens_status
	tokensHandler, _ := srv.Handler("dendrite_tokens_status")
	resTokens, err := tokensHandler(ctx, map[string]interface{}{
		"model": "gemini-3.8-flash-high",
	})
	if err != nil {
		t.Fatalf("tokensHandler failed: %v", err)
	}
	if len(resTokens.Content) == 0 || !strings.Contains(resTokens.Content[0].Text, "day_window") {
		t.Errorf("expected day_window in tokens response: %+v", resTokens.Content)
	}
	if !strings.Contains(resTokens.Content[0].Text, "week_window") {
		t.Errorf("expected week_window in tokens response: %+v", resTokens.Content)
	}
	if !strings.Contains(resTokens.Content[0].Text, "agy_status") {
		t.Errorf("expected agy_status in tokens response: %+v", resTokens.Content)
	}
}
