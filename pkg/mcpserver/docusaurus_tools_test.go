package mcpserver

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestDocusaurusTools_RegistrationAndExecution(t *testing.T) {
	srv := NewServer(ServerConfig{})
	RegisterDefaultTools(srv, Dependencies{DevStatus: "../../DEV_STATUS.md"})

	expectedTools := []string{
		"dendrite_docusaurus_generate",
		"dendrite_docusaurus_share",
		"dendrite_docusaurus_audit",
	}

	tools := srv.ListTools()
	for _, expected := range expectedTools {
		found := false
		for _, tool := range tools {
			if tool.Name == expected {
				found = true
				break
			}
		}
		if !found {
			t.Fatalf("expected tool %s to be registered", expected)
		}
	}

	ctx := context.Background()

	// Setup temporary dirs
	tempSrc, err := os.MkdirTemp("", "mcp-docusaurus-src-*")
	if err != nil {
		t.Fatalf("failed to create temp src: %v", err)
	}
	defer os.RemoveAll(tempSrc)

	tempDest, err := os.MkdirTemp("", "mcp-docusaurus-dest-*")
	if err != nil {
		t.Fatalf("failed to create temp dest: %v", err)
	}
	defer os.RemoveAll(tempDest)

	_ = os.WriteFile(filepath.Join(tempSrc, "FEAT-100-test.md"), []byte("# FEAT-100\n## Acceptance Criteria\n- [x] pass\n"), 0644)

	// 1. Test dendrite_docusaurus_generate
	genHandler, _ := srv.Handler("dendrite_docusaurus_generate")
	resGen, err := genHandler(ctx, map[string]interface{}{
		"docs_dir":     tempSrc,
		"target_dir":   tempDest,
		"title":        "Test Docs Portal",
		"port":         3050,
		"sharing_tier": "federated",
	})
	if err != nil {
		t.Fatalf("dendrite_docusaurus_generate failed: %v", err)
	}
	if len(resGen.Content) == 0 || !strings.Contains(resGen.Content[0].Text, "site_directory") {
		t.Errorf("unexpected generation result: %v", resGen.Content)
	}

	// 2. Test dendrite_docusaurus_share
	shareHandler, _ := srv.Handler("dendrite_docusaurus_share")
	resShare, err := shareHandler(ctx, map[string]interface{}{
		"site_dir": tempDest,
	})
	if err != nil {
		t.Fatalf("dendrite_docusaurus_share failed: %v", err)
	}
	if len(resShare.Content) == 0 || !strings.Contains(resShare.Content[0].Text, "manifest_version") {
		t.Errorf("unexpected share result: %v", resShare.Content)
	}

	// 3. Test dendrite_docusaurus_audit
	auditHandler, _ := srv.Handler("dendrite_docusaurus_audit")
	resAudit, err := auditHandler(ctx, map[string]interface{}{
		"project_root": tempSrc,
		"docs_dir":     tempSrc,
	})
	if err != nil {
		t.Fatalf("dendrite_docusaurus_audit failed: %v", err)
	}
	if len(resAudit.Content) == 0 || !strings.Contains(resAudit.Content[0].Text, "total_features") {
		t.Errorf("unexpected audit result: %v", resAudit.Content)
	}
}
