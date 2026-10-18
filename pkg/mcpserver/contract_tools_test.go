package mcpserver

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestContractTools_RegistrationAndExecution(t *testing.T) {
	srv := NewServer(ServerConfig{})
	RegisterDefaultTools(srv, Dependencies{})

	// Verify tool registration
	registered := false
	for _, tool := range srv.ListTools() {
		if tool.Name == "dendrite_contract_drift_check" {
			registered = true
			break
		}
	}
	if !registered {
		t.Fatalf("expected dendrite_contract_drift_check to be registered")
	}

	tmpDir, err := os.MkdirTemp("", "mcp_contract_test_*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	schemaDir := filepath.Join(tmpDir, "schemas")
	codeDir := filepath.Join(tmpDir, "models")
	_ = os.MkdirAll(schemaDir, 0755)
	_ = os.MkdirAll(codeDir, 0755)

	protoFile := filepath.Join(schemaDir, "test.proto")
	_ = os.WriteFile(protoFile, []byte("message User {\n  string id = 1;\n  int32 age = 2;\n}"), 0644)

	goFile := filepath.Join(codeDir, "user.go")
	_ = os.WriteFile(goFile, []byte("package main\ntype User struct {\n\tID string `json:\"id\"`\n\tAge string `json:\"age\"`\n}\n"), 0644)

	handler, _ := srv.Handler("dendrite_contract_drift_check")
	ctx := context.Background()

	res, err := handler(ctx, map[string]interface{}{
		"schema_dirs": []interface{}{schemaDir},
		"code_dirs": map[string]interface{}{
			"go": codeDir,
		},
	})
	if err != nil {
		t.Fatalf("handler execution failed: %v", err)
	}

	if len(res.Content) == 0 || !strings.Contains(res.Content[0].Text, "TYPE_MISMATCH") {
		t.Errorf("expected TYPE_MISMATCH in drift check response: %+v", res.Content)
	}
}
