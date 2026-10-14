package jetbrains_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"Dendrite/pkg/jetbrains"
)

func TestSetupJetBrainsToolset_Full(t *testing.T) {
	tempProject := t.TempDir()
	tempGlobal := t.TempDir()
	globalAIPath := filepath.Join(tempGlobal, ".ai", "mcp", "mcp.json")

	// Pre-create .idea folder to simulate an IDE project
	if err := os.MkdirAll(filepath.Join(tempProject, ".idea"), 0755); err != nil {
		t.Fatalf("failed to create .idea: %v", err)
	}

	opts := jetbrains.SetupOptions{
		ProjectDir:             tempProject,
		BinaryPath:             filepath.Join(tempProject, "bin", "dendrite"),
		ConfigureProjectMCP:    true,
		ConfigureGlobalMCP:     true,
		ConfigureRunConfigs:    true,
		ConfigureExternalTools: true,
		GlobalAIPathOverride:   globalAIPath,
	}

	res, err := jetbrains.SetupJetBrainsToolset(opts)
	if err != nil {
		t.Fatalf("SetupJetBrainsToolset failed: %v", err)
	}

	if len(res.FilesCreated) == 0 {
		t.Fatal("expected created files, got 0")
	}

	// 1. Verify project MCP file
	projMCPPath := filepath.Join(tempProject, ".ai", "mcp", "mcp.json")
	data, err := os.ReadFile(projMCPPath)
	if err != nil {
		t.Fatalf("failed to read project mcp.json: %v", err)
	}
	var cfg jetbrains.MCPConfig
	if err := json.Unmarshal(data, &cfg); err != nil {
		t.Fatalf("invalid json in project mcp.json: %v", err)
	}
	dendriteServer, ok := cfg.MCPServers["dendrite"]
	if !ok {
		t.Fatal("expected 'dendrite' server in mcpServers")
	}
	if dendriteServer.Command != opts.BinaryPath {
		t.Errorf("expected command %s, got %s", opts.BinaryPath, dendriteServer.Command)
	}

	// 2. Verify global MCP file
	data, err = os.ReadFile(globalAIPath)
	if err != nil {
		t.Fatalf("failed to read global mcp.json: %v", err)
	}
	if err := json.Unmarshal(data, &cfg); err != nil {
		t.Fatalf("invalid json in global mcp.json: %v", err)
	}
	if _, ok := cfg.MCPServers["dendrite"]; !ok {
		t.Fatal("expected 'dendrite' server in global mcp.json")
	}

	// 3. Verify Run Configurations
	runConfigDir := filepath.Join(tempProject, ".idea", "runConfigurations")
	entries, err := os.ReadDir(runConfigDir)
	if err != nil || len(entries) < 5 {
		t.Fatalf("expected at least 5 run configurations, found: %d", len(entries))
	}

	// 4. Verify External Tools
	extToolsPath := filepath.Join(tempProject, ".idea", "tools", "External_Tools.xml")
	if _, err := os.Stat(extToolsPath); err != nil {
		t.Fatalf("expected External_Tools.xml to exist: %v", err)
	}

	// 5. Test idempotence: running again updates existing files without errors
	res2, err := jetbrains.SetupJetBrainsToolset(opts)
	if err != nil {
		t.Fatalf("second run failed: %v", err)
	}
	if len(res2.FilesUpdated) == 0 {
		t.Fatal("expected updated files on re-run, got 0")
	}
}

func TestSetupJetBrainsToolset_DefaultPaths(t *testing.T) {
	tempProject := t.TempDir()
	opts := jetbrains.SetupOptions{
		ProjectDir:          tempProject,
		ConfigureProjectMCP: true,
	}

	res, err := jetbrains.SetupJetBrainsToolset(opts)
	if err != nil {
		t.Fatalf("setup with defaults failed: %v", err)
	}
	if len(res.FilesCreated) != 1 {
		t.Fatalf("expected 1 created file, got %d", len(res.FilesCreated))
	}
}
