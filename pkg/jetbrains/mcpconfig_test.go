package jetbrains

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestUpsertMCPServer_NewFile(t *testing.T) {
	dir := t.TempDir()
	targetPath := filepath.Join(dir, "sub", "mcp.json")

	entry := MCPServerEntry{
		Command: "/usr/local/bin/dendrite",
		Args:    []string{"mcp", "serve"},
		Env: map[string]string{
			"DENDRITE_ENV": "development",
		},
	}

	created, err := UpsertMCPServer(targetPath, "dendrite", entry)
	if err != nil {
		t.Fatalf("expected UpsertMCPServer to succeed, got %v", err)
	}
	if !created {
		t.Fatalf("expected created to be true for new file")
	}

	data, err := os.ReadFile(targetPath)
	if err != nil {
		t.Fatalf("failed to read created file: %v", err)
	}
	if !strings.HasSuffix(string(data), "\n") {
		t.Errorf("expected trailing newline")
	}

	var topLevel map[string]json.RawMessage
	if err := json.Unmarshal(data, &topLevel); err != nil {
		t.Fatalf("invalid json: %v", err)
	}
	rawServers, ok := topLevel["mcpServers"]
	if !ok {
		t.Fatalf("missing mcpServers in topLevel")
	}

	var servers map[string]MCPServerEntry
	if err := json.Unmarshal(rawServers, &servers); err != nil {
		t.Fatalf("invalid servers: %v", err)
	}
	srv, ok := servers["dendrite"]
	if !ok {
		t.Fatalf("missing dendrite in servers")
	}
	if srv.Command != entry.Command {
		t.Errorf("expected command %s, got %s", entry.Command, srv.Command)
	}
	if len(srv.Args) != len(entry.Args) || srv.Args[0] != "mcp" {
		t.Errorf("unexpected args: %v", srv.Args)
	}
	if srv.Env["DENDRITE_ENV"] != "development" {
		t.Errorf("unexpected env: %v", srv.Env)
	}
}

func TestUpsertMCPServer_ExistingFile_CreatedFalse(t *testing.T) {
	dir := t.TempDir()
	targetPath := filepath.Join(dir, "mcp.json")

	entry1 := MCPServerEntry{
		Command: "bin/v1",
	}
	created1, err := UpsertMCPServer(targetPath, "dendrite", entry1)
	if err != nil || !created1 {
		t.Fatalf("first upsert failed: created=%v, err=%v", created1, err)
	}

	entry2 := MCPServerEntry{
		Command: "bin/v2",
	}
	created2, err := UpsertMCPServer(targetPath, "dendrite", entry2)
	if err != nil {
		t.Fatalf("second upsert failed: %v", err)
	}
	if created2 {
		t.Fatalf("expected created=false on existing file")
	}

	data, err := os.ReadFile(targetPath)
	if err != nil {
		t.Fatalf("read failed: %v", err)
	}
	var cfg MCPConfig
	if err := json.Unmarshal(data, &cfg); err != nil {
		t.Fatalf("invalid json: %v", err)
	}
	if cfg.MCPServers["dendrite"].Command != "bin/v2" {
		t.Errorf("expected updated command 'bin/v2', got '%s'", cfg.MCPServers["dendrite"].Command)
	}
}

func TestUpsertMCPServer_PreservesForeignServerAndTopLevelInputs(t *testing.T) {
	dir := t.TempDir()
	targetPath := filepath.Join(dir, "mcp.json")

	initialJSON := `{
  "inputs": {
    "token_limit": 4000,
    "mode": "auto"
  },
  "mcpServers": {
    "foreign-sse": {
      "url": "http://x",
      "type": "sse",
      "headers": {
        "Authorization": "Bearer token123"
      }
    }
  }
}`
	if err := os.WriteFile(targetPath, []byte(initialJSON), 0644); err != nil {
		t.Fatalf("failed to write initial JSON: %v", err)
	}

	newEntry := MCPServerEntry{
		Command: "/bin/dendrite",
		Args:    []string{"mcp", "serve"},
		Env: map[string]string{
			"KEY": "VAL",
		},
	}

	created, err := UpsertMCPServer(targetPath, "dendrite", newEntry)
	if err != nil {
		t.Fatalf("UpsertMCPServer failed: %v", err)
	}
	if created {
		t.Fatalf("expected created to be false for existing file")
	}

	data, err := os.ReadFile(targetPath)
	if err != nil {
		t.Fatalf("failed to read updated file: %v", err)
	}

	var topLevel map[string]json.RawMessage
	if err := json.Unmarshal(data, &topLevel); err != nil {
		t.Fatalf("failed to parse top level: %v", err)
	}

	// Verify top-level "inputs" key is intact
	inputsRaw, ok := topLevel["inputs"]
	if !ok {
		t.Fatalf("top-level key 'inputs' was dropped")
	}
	var inputsMap map[string]interface{}
	if err := json.Unmarshal(inputsRaw, &inputsMap); err != nil {
		t.Fatalf("failed to parse inputs: %v", err)
	}
	if inputsMap["mode"] != "auto" || int(inputsMap["token_limit"].(float64)) != 4000 {
		t.Errorf("inputs content corrupted: %+v", inputsMap)
	}

	// Verify mcpServers has both "foreign-sse" and "dendrite"
	serversRaw, ok := topLevel["mcpServers"]
	if !ok {
		t.Fatalf("missing mcpServers")
	}
	var serversMap map[string]json.RawMessage
	if err := json.Unmarshal(serversRaw, &serversMap); err != nil {
		t.Fatalf("failed to parse mcpServers: %v", err)
	}

	foreignRaw, ok := serversMap["foreign-sse"]
	if !ok {
		t.Fatalf("foreign-sse server was dropped")
	}
	var foreignMap map[string]interface{}
	if err := json.Unmarshal(foreignRaw, &foreignMap); err != nil {
		t.Fatalf("failed to parse foreign server: %v", err)
	}
	if foreignMap["url"] != "http://x" {
		t.Errorf("foreign server url lost or changed: %v", foreignMap["url"])
	}
	if foreignMap["type"] != "sse" {
		t.Errorf("foreign server type lost or changed: %v", foreignMap["type"])
	}

	dendriteRaw, ok := serversMap["dendrite"]
	if !ok {
		t.Fatalf("dendrite server not found")
	}
	var dendriteEntry MCPServerEntry
	if err := json.Unmarshal(dendriteRaw, &dendriteEntry); err != nil {
		t.Fatalf("failed to parse dendrite entry: %v", err)
	}
	if dendriteEntry.Command != "/bin/dendrite" {
		t.Errorf("unexpected dendrite command: %s", dendriteEntry.Command)
	}
}

func TestUpsertMCPServer_EmptyAndWhitespaceFile(t *testing.T) {
	dir := t.TempDir()

	entry := MCPServerEntry{Command: "/bin/tool"}

	// Empty file
	emptyPath := filepath.Join(dir, "empty.json")
	if err := os.WriteFile(emptyPath, []byte(""), 0644); err != nil {
		t.Fatalf("failed to write empty file: %v", err)
	}
	created, err := UpsertMCPServer(emptyPath, "srv", entry)
	if err != nil {
		t.Fatalf("UpsertMCPServer on empty file failed: %v", err)
	}
	if created {
		t.Errorf("expected created=false because empty file existed")
	}
	data, _ := os.ReadFile(emptyPath)
	var top map[string]json.RawMessage
	if err := json.Unmarshal(data, &top); err != nil || top["mcpServers"] == nil {
		t.Fatalf("failed to create valid json in empty file: %s", string(data))
	}

	// Whitespace file
	wsPath := filepath.Join(dir, "whitespace.json")
	if err := os.WriteFile(wsPath, []byte("  \t\r\n  \n"), 0644); err != nil {
		t.Fatalf("failed to write whitespace file: %v", err)
	}
	created, err = UpsertMCPServer(wsPath, "srv", entry)
	if err != nil {
		t.Fatalf("UpsertMCPServer on whitespace file failed: %v", err)
	}
	if created {
		t.Errorf("expected created=false because whitespace file existed")
	}
}

func TestUpsertMCPServer_InvalidJSON_Untouched(t *testing.T) {
	dir := t.TempDir()
	targetPath := filepath.Join(dir, "corrupt.json")
	invalidContent := "{\n  \"invalid\": json,\n  corrupt\n"

	if err := os.WriteFile(targetPath, []byte(invalidContent), 0644); err != nil {
		t.Fatalf("failed to write invalid file: %v", err)
	}

	entry := MCPServerEntry{Command: "bin/test"}
	created, err := UpsertMCPServer(targetPath, "dendrite", entry)
	if err == nil {
		t.Fatalf("expected error on invalid JSON, got nil")
	}
	if created {
		t.Errorf("expected created=false on error")
	}

	data, readErr := os.ReadFile(targetPath)
	if readErr != nil {
		t.Fatalf("failed to read back corrupt file: %v", readErr)
	}
	if string(data) != invalidContent {
		t.Fatalf("file content was modified despite invalid JSON:\n%s", string(data))
	}
}

func TestUpsertMCPServer_InvalidMCPServers(t *testing.T) {
	dir := t.TempDir()
	targetPath := filepath.Join(dir, "bad_servers.json")
	content := "{\"mcpServers\": \"not-an-object\"}"
	if err := os.WriteFile(targetPath, []byte(content), 0644); err != nil {
		t.Fatalf("failed to write file: %v", err)
	}

	entry := MCPServerEntry{Command: "bin/test"}
	_, err := UpsertMCPServer(targetPath, "dendrite", entry)
	if err == nil {
		t.Fatalf("expected error when mcpServers is not an object")
	}

	data, _ := os.ReadFile(targetPath)
	if string(data) != content {
		t.Fatalf("file was modified on error")
	}
}

func TestUpsertMCPServer_EmptyServerName(t *testing.T) {
	dir := t.TempDir()
	targetPath := filepath.Join(dir, "test.json")
	_, err := UpsertMCPServer(targetPath, "", MCPServerEntry{Command: "x"})
	if err == nil {
		t.Fatalf("expected error for empty server name")
	}
}

func TestUpsertMCPServer_DirectoryTargetError(t *testing.T) {
	dir := t.TempDir()
	_, err := UpsertMCPServer(dir, "srv", MCPServerEntry{Command: "x"})
	if err == nil {
		t.Fatalf("expected error targeting a directory")
	}
}

func TestRemoveMCPServer_Present(t *testing.T) {
	dir := t.TempDir()
	targetPath := filepath.Join(dir, "mcp.json")

	content := `{
  "inputs": "preserved",
  "mcpServers": {
    "serverA": {
      "command": "cmdA"
    },
    "serverB": {
      "command": "cmdB"
    }
  }
}`
	if err := os.WriteFile(targetPath, []byte(content), 0644); err != nil {
		t.Fatalf("failed to write file: %v", err)
	}

	removed, err := RemoveMCPServer(targetPath, "serverA")
	if err != nil {
		t.Fatalf("RemoveMCPServer failed: %v", err)
	}
	if !removed {
		t.Fatalf("expected removed=true")
	}

	data, err := os.ReadFile(targetPath)
	if err != nil {
		t.Fatalf("read failed: %v", err)
	}

	var topLevel map[string]json.RawMessage
	if err := json.Unmarshal(data, &topLevel); err != nil {
		t.Fatalf("parse failed: %v", err)
	}
	if string(topLevel["inputs"]) != `"preserved"` {
		t.Errorf("inputs lost or altered: %s", string(topLevel["inputs"]))
	}

	var servers map[string]json.RawMessage
	if err := json.Unmarshal(topLevel["mcpServers"], &servers); err != nil {
		t.Fatalf("parse servers failed: %v", err)
	}
	if _, ok := servers["serverA"]; ok {
		t.Errorf("serverA was not removed")
	}
	if _, ok := servers["serverB"]; !ok {
		t.Errorf("serverB was unintentionally removed")
	}
}

func TestRemoveMCPServer_Absent(t *testing.T) {
	dir := t.TempDir()
	targetPath := filepath.Join(dir, "mcp.json")

	content := `{
  "mcpServers": {
    "serverB": {
      "command": "cmdB"
    }
  }
}`
	if err := os.WriteFile(targetPath, []byte(content), 0644); err != nil {
		t.Fatalf("failed to write file: %v", err)
	}

	removed, err := RemoveMCPServer(targetPath, "serverA")
	if err != nil {
		t.Fatalf("RemoveMCPServer failed: %v", err)
	}
	if removed {
		t.Fatalf("expected removed=false for absent server")
	}
}

func TestRemoveMCPServer_MissingFile(t *testing.T) {
	dir := t.TempDir()
	targetPath := filepath.Join(dir, "nonexistent.json")

	removed, err := RemoveMCPServer(targetPath, "serverA")
	if err != nil {
		t.Fatalf("RemoveMCPServer returned error on missing file: %v", err)
	}
	if removed {
		t.Fatalf("expected removed=false for missing file")
	}
}

func TestRemoveMCPServer_EmptyFile(t *testing.T) {
	dir := t.TempDir()
	targetPath := filepath.Join(dir, "empty.json")
	if err := os.WriteFile(targetPath, []byte(""), 0644); err != nil {
		t.Fatalf("failed to write file: %v", err)
	}

	removed, err := RemoveMCPServer(targetPath, "serverA")
	if err != nil {
		t.Fatalf("RemoveMCPServer failed on empty file: %v", err)
	}
	if removed {
		t.Fatalf("expected removed=false for empty file")
	}
}

func TestRemoveMCPServer_InvalidJSON(t *testing.T) {
	dir := t.TempDir()
	targetPath := filepath.Join(dir, "invalid.json")
	if err := os.WriteFile(targetPath, []byte("{bad"), 0644); err != nil {
		t.Fatalf("failed to write file: %v", err)
	}

	removed, err := RemoveMCPServer(targetPath, "serverA")
	if err == nil {
		t.Fatalf("expected error on invalid JSON")
	}
	if removed {
		t.Fatalf("expected removed=false on error")
	}
}

func TestRemoveMCPServer_InvalidMCPServers(t *testing.T) {
	dir := t.TempDir()
	targetPath := filepath.Join(dir, "invalid_servers.json")
	if err := os.WriteFile(targetPath, []byte("{\"mcpServers\": 123}"), 0644); err != nil {
		t.Fatalf("failed to write file: %v", err)
	}

	removed, err := RemoveMCPServer(targetPath, "serverA")
	if err == nil {
		t.Fatalf("expected error when mcpServers is invalid")
	}
	if removed {
		t.Fatalf("expected removed=false on error")
	}
}

func TestRemoveMCPServer_NoMCPServersKey(t *testing.T) {
	dir := t.TempDir()
	targetPath := filepath.Join(dir, "no_servers.json")
	if err := os.WriteFile(targetPath, []byte("{\"other\": true}"), 0644); err != nil {
		t.Fatalf("failed to write file: %v", err)
	}

	removed, err := RemoveMCPServer(targetPath, "serverA")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if removed {
		t.Fatalf("expected removed=false")
	}
}

func TestRemoveMCPServer_EmptyName(t *testing.T) {
	dir := t.TempDir()
	targetPath := filepath.Join(dir, "mcp.json")
	_ = os.WriteFile(targetPath, []byte("{}"), 0644)

	removed, err := RemoveMCPServer(targetPath, "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if removed {
		t.Fatalf("expected removed=false for empty name")
	}
}

func TestRemoveMCPServer_DirectoryTargetError(t *testing.T) {
	dir := t.TempDir()
	_, err := RemoveMCPServer(dir, "srv")
	if err == nil {
		t.Fatalf("expected error targeting a directory")
	}
}

func TestConfigureMCPFile_RecordsCreatedVsUpdated(t *testing.T) {
	dir := t.TempDir()
	targetPath := filepath.Join(dir, ".ai", "mcp", "mcp.json")

	res := &SetupResult{
		FilesCreated: []string{},
		FilesUpdated: []string{},
	}

	// 1. First run creates the file
	err := configureMCPFile(targetPath, "/bin/dendrite", res)
	if err != nil {
		t.Fatalf("first configureMCPFile failed: %v", err)
	}
	if len(res.FilesCreated) != 1 || res.FilesCreated[0] != targetPath {
		t.Fatalf("expected FilesCreated to have targetPath, got %+v", res.FilesCreated)
	}
	if len(res.FilesUpdated) != 0 {
		t.Fatalf("expected FilesUpdated to be empty, got %+v", res.FilesUpdated)
	}

	// 2. Second run updates the file
	err = configureMCPFile(targetPath, "/bin/dendrite", res)
	if err != nil {
		t.Fatalf("second configureMCPFile failed: %v", err)
	}
	if len(res.FilesCreated) != 1 {
		t.Fatalf("FilesCreated should still have 1 entry, got %d", len(res.FilesCreated))
	}
	if len(res.FilesUpdated) != 1 || res.FilesUpdated[0] != targetPath {
		t.Fatalf("expected FilesUpdated to have targetPath, got %+v", res.FilesUpdated)
	}

	// Verify file content has dendrite server
	data, err := os.ReadFile(targetPath)
	if err != nil {
		t.Fatalf("failed to read mcp.json: %v", err)
	}
	var cfg MCPConfig
	if err := json.Unmarshal(data, &cfg); err != nil {
		t.Fatalf("invalid json: %v", err)
	}
	srv, ok := cfg.MCPServers["dendrite"]
	if !ok {
		t.Fatalf("missing dendrite server in mcpServers")
	}
	if srv.Command != "/bin/dendrite" {
		t.Errorf("expected command '/bin/dendrite', got '%s'", srv.Command)
	}
	if strings.Join(srv.Args, " ") != "inter serve" || len(srv.Env) != 0 {
		t.Errorf("expected 'inter serve' with no env, got %v %v", srv.Args, srv.Env)
	}
}
