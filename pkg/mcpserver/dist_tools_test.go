package mcpserver

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"testing"
)

func TestDistCodebaseMCPTools(t *testing.T) {
	srv := NewServer(ServerConfig{})
	RegisterDefaultTools(srv, Dependencies{DevStatus: "../../DEV_STATUS.md"})

	ctx := context.Background()

	callTool := func(name string, args map[string]interface{}) (*CallToolResult, error) {
		reqBytes, _ := json.Marshal(map[string]interface{}{
			"jsonrpc": "2.0",
			"id":      1,
			"method":  "tools/call",
			"params": map[string]interface{}{
				"name":      name,
				"arguments": args,
			},
		})
		resp := srv.HandleRequest(ctx, reqBytes)
		if resp.Error != nil {
			return nil, fmt.Errorf("rpc error: %s", resp.Error.Message)
		}
		res, ok := resp.Result.(*CallToolResult)
		if !ok {
			return nil, fmt.Errorf("unexpected result type: %T", resp.Result)
		}
		return res, nil
	}

	// 1. Test dendrite_dist_sources: list all
	res, err := callTool("dendrite_dist_sources", map[string]interface{}{})
	if err != nil {
		t.Fatalf("failed to call dendrite_dist_sources: %v", err)
	}
	if len(res.Content) == 0 || !strings.Contains(res.Content[0].Text, "wgpu") {
		t.Errorf("expected wgpu in dist sources output, got: %v", res.Content)
	}

	// Test query resolution: "cte" -> diesel-cte-ext
	resQuery, err := callTool("dendrite_dist_sources", map[string]interface{}{
		"query": "cte",
	})
	if err != nil {
		t.Fatalf("failed to resolve query in dendrite_dist_sources: %v", err)
	}
	if !strings.Contains(resQuery.Content[0].Text, "diesel-cte-ext") {
		t.Errorf("expected diesel-cte-ext in query resolution, got: %s", resQuery.Content[0].Text)
	}

	// 2. Test dendrite_dist_decompose
	resDecomp, err := callTool("dendrite_dist_decompose", map[string]interface{}{
		"project_name": "CarlaEngine",
		"language":     "rust",
		"modules": []interface{}{
			"render-pipeline",
			"storage-engine",
		},
	})
	if err != nil {
		t.Fatalf("failed to call dendrite_dist_decompose: %v", err)
	}
	if !strings.Contains(resDecomp.Content[0].Text, "wgpu") {
		t.Errorf("expected wgpu in render-pipeline decomposition, got: %s", resDecomp.Content[0].Text)
	}

	// 3. Test dendrite_dist_interop
	resInterop, err := callTool("dendrite_dist_interop", map[string]interface{}{
		"action":             "coordinate",
		"requester_project":  "Docula",
		"dependency_project": "Oxipix",
		"feature_requested":  "simd_blend_rgba",
		"allow_fallback":     true,
	})
	if err != nil {
		t.Fatalf("failed to coordinate interop: %v", err)
	}
	if !strings.Contains(resInterop.Content[0].Text, "TODO(interop:Oxipix)") {
		t.Errorf("expected Oxipix TODO in interop, got: %s", resInterop.Content[0].Text)
	}

	var coordMap map[string]interface{}
	_ = json.Unmarshal([]byte(resInterop.Content[0].Text), &coordMap)
	coordObj := coordMap["coordination"].(map[string]interface{})
	coordID := coordObj["id"].(string)

	// Complete dependency feature
	resComplete, err := callTool("dendrite_dist_interop", map[string]interface{}{
		"action":          "complete",
		"coordination_id": coordID,
	})
	if err != nil {
		t.Fatalf("failed to complete interop: %v", err)
	}
	if !strings.Contains(resComplete.Content[0].Text, "dependency_completed") {
		t.Errorf("expected dependency_completed, got: %s", resComplete.Content[0].Text)
	}

	// Reconcile
	resRec, err := callTool("dendrite_dist_interop", map[string]interface{}{
		"action":          "reconcile",
		"coordination_id": coordID,
	})
	if err != nil {
		t.Fatalf("failed to reconcile interop: %v", err)
	}
	if !strings.Contains(resRec.Content[0].Text, "successfully_reconciled") {
		t.Errorf("expected successfully_reconciled, got: %s", resRec.Content[0].Text)
	}

	// List
	resList, err := callTool("dendrite_dist_interop", map[string]interface{}{
		"action": "list",
	})
	if err != nil || !strings.Contains(resList.Content[0].Text, coordID) {
		t.Errorf("expected coordination in list, got: %v, err: %v", resList, err)
	}
}
