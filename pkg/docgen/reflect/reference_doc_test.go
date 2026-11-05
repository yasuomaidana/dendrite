package reflect

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/spf13/cobra"

	"Dendrite/pkg/mcpserver"
)

// testRoot builds a root command with one "core" help group.
func testRoot() *cobra.Command {
	root := &cobra.Command{Use: "dendrite", Short: "Test root"}
	root.AddGroup(&cobra.Group{ID: "core", Title: "Core:"})
	root.PersistentFlags().Bool("json", false, "Emit JSON")
	return root
}

// testCmd builds a runnable command in the core group.
func testCmd(name, short string) *cobra.Command {
	return &cobra.Command{Use: name, Short: short, GroupID: "core", Run: func(*cobra.Command, []string) {}}
}

func TestIntrospectCLITree_NilAndEmpty(t *testing.T) {
	if docs := IntrospectCLITree(nil); docs != nil {
		t.Fatalf("expected nil docs for nil root, got: %v", docs)
	}

	reg := testRoot()
	docs := IntrospectCLITree(reg)
	if len(docs) != 0 {
		t.Fatalf("expected 0 docs for empty registry, got: %d", len(docs))
	}
}

func TestIntrospectCLICommand_Nil(t *testing.T) {
	doc := IntrospectCLICommand(nil)
	if doc.Name != "" {
		t.Fatalf("expected empty doc for nil command, got: %v", doc)
	}
}

func TestIntrospectCLICommand_WithSubcommands(t *testing.T) {
	root := testRoot()
	cmd := testCmd("custom", "A custom test command")
	cmd.Aliases = []string{"cst", "test-c"}
	beta := testCmd("beta", "Beta subcommand")
	beta.GroupID = ""
	beta.Flags().StringP("ticket", "t", "DEN-1", "Ticket id")
	beta.Example = "dendrite custom beta -t DEN-2"
	alpha := testCmd("alpha", "Alpha subcommand")
	alpha.GroupID = ""
	nested := &cobra.Command{Use: "deep <id>", Short: "Nested", Run: func(*cobra.Command, []string) {}}
	alpha.AddCommand(nested)
	cmd.AddCommand(beta, alpha)
	root.AddCommand(cmd)

	doc := IntrospectCLICommand(cmd)
	if doc.Name != "custom" {
		t.Errorf("expected name 'custom', got: %s", doc.Name)
	}
	if len(doc.Subcommands) != 3 {
		t.Fatalf("expected 3 subcommands, got: %d", len(doc.Subcommands))
	}
	// Verify sorted, depth-first order with nested paths
	if doc.Subcommands[0].Name != "alpha" || doc.Subcommands[1].Name != "alpha deep" || doc.Subcommands[2].Name != "beta" {
		t.Errorf("expected [alpha, alpha deep, beta], got: %v", doc.Subcommands)
	}
	if doc.Category != "Core" {
		t.Errorf("expected category Core, got %q", doc.Category)
	}
	if f := doc.Subcommands[2].Flags; len(f) != 1 || f[0].Name != "--ticket, -t" || f[0].Default != "DEN-1" {
		t.Errorf("unexpected beta flags: %v", f)
	}
	md := RenderCLICommandMarkdown(doc)
	for _, want := range []string{"## Subcommand Reference", "### `dendrite custom beta`", "dendrite custom beta -t DEN-2", "`--ticket, -t`"} {
		if !strings.Contains(md, want) {
			t.Errorf("rendered markdown missing %q:\n%s", want, md)
		}
	}
	if len(doc.Examples) == 0 || doc.Examples[0] != "dendrite custom" {
		t.Errorf("expected default example 'dendrite custom', got: %v", doc.Examples)
	}
}

func TestRenderCLICommandMarkdown_Variations(t *testing.T) {
	// Full command doc
	fullDoc := CLICommandDoc{
		Name:        "testcmd",
		Category:    "DevOps",
		Description: "Comprehensive test command",
		Aliases:     []string{"tcmd", "tc"},
		Subcommands: []SubcommandDoc{
			{Name: "sub1", Description: "Subcommand 1"},
		},
		Flags: []FlagDoc{
			{Name: "--flag1", Description: "Boolean flag"},
			{Name: "--flag2", Description: "String flag", Default: "val"},
		},
		Examples: []string{
			"dendrite testcmd sub1 --flag1",
			"dendrite testcmd --flag2 val",
		},
	}

	md := RenderCLICommandMarkdown(fullDoc)
	if !strings.Contains(md, "id: testcmd") {
		t.Errorf("missing frontmatter id")
	}
	if !strings.Contains(md, "# dendrite testcmd") {
		t.Errorf("missing title")
	}
	if !strings.Contains(md, "Comprehensive test command") {
		t.Errorf("missing description")
	}
	if !strings.Contains(md, "DevOps") {
		t.Errorf("missing category")
	}
	if !strings.Contains(md, "- `tcmd`") || !strings.Contains(md, "- `tc`") {
		t.Errorf("missing aliases")
	}
	if !strings.Contains(md, "| `sub1` | Subcommand 1 |") {
		t.Errorf("missing subcommand table row")
	}
	if !strings.Contains(md, "| `--flag2` | String flag (default: val) |") {
		t.Errorf("missing flag default table row")
	}
	if !strings.Contains(md, "dendrite testcmd sub1 --flag1") {
		t.Errorf("missing example")
	}

	// Minimal command doc with empty fields
	minDoc := CLICommandDoc{
		Name: "bare",
	}
	minMD := RenderCLICommandMarkdown(minDoc)
	if !strings.Contains(minMD, "No description available.") {
		t.Errorf("expected default description message")
	}
	if !strings.Contains(minMD, "General") {
		t.Errorf("expected default General category")
	}
	if !strings.Contains(minMD, "## Aliases\n\nNone") {
		t.Errorf("expected None for empty aliases")
	}
	if !strings.Contains(minMD, "## Subcommands\n\nNone") {
		t.Errorf("expected None for empty subcommands")
	}
	if !strings.Contains(minMD, "## Flags\n\nNone") {
		t.Errorf("expected None for empty flags")
	}
	if !strings.Contains(minMD, "dendrite bare") {
		t.Errorf("expected default bare example")
	}
}

func TestDefaultExample(t *testing.T) {
	doc := CLICommandDoc{Name: "soma"}
	defaultExample(&doc)
	if len(doc.Examples) != 1 || doc.Examples[0] != "dendrite soma" {
		t.Fatalf("missing default example: %v", doc.Examples)
	}
	kept := CLICommandDoc{Name: "soma", Examples: []string{"dendrite soma prepare"}}
	defaultExample(&kept)
	if len(kept.Examples) != 1 || kept.Examples[0] != "dendrite soma prepare" {
		t.Fatalf("explicit examples must be kept: %v", kept.Examples)
	}
}

func TestIntrospectMCPServer_NilAndTools(t *testing.T) {
	if docs := IntrospectMCPServer(nil); docs != nil {
		t.Fatalf("expected nil docs for nil server, got: %v", docs)
	}

	srv := mcpserver.NewServer(mcpserver.ServerConfig{
		Name:    "Test MCP Server",
		Version: "1.0.0",
	})

	srv.RegisterTool(mcpserver.Tool{
		Name:        "zebra_tool",
		Description: "Tool Z",
		InputSchema: map[string]interface{}{"type": "object"},
	}, func(ctx context.Context, args map[string]interface{}) (*mcpserver.CallToolResult, error) {
		return nil, nil
	})

	srv.RegisterTool(mcpserver.Tool{
		Name:        "alpha_tool",
		Description: "Tool A",
		InputSchema: map[string]interface{}{"type": "object"},
	}, func(ctx context.Context, args map[string]interface{}) (*mcpserver.CallToolResult, error) {
		return nil, nil
	})

	docs := IntrospectMCPServer(srv)
	if len(docs) != 2 {
		t.Fatalf("expected 2 tools, got: %d", len(docs))
	}
	if docs[0].Name != "alpha_tool" || docs[1].Name != "zebra_tool" {
		t.Errorf("expected sorted tools [alpha_tool, zebra_tool], got: [%s, %s]", docs[0].Name, docs[1].Name)
	}
}

func TestSynthesizeExampleArguments_Types(t *testing.T) {
	// Nil schema
	if res := synthesizeExampleArguments(nil); len(res) != 0 {
		t.Errorf("expected empty map for nil schema")
	}

	// Schema with empty properties
	if res := synthesizeExampleArguments(map[string]interface{}{"type": "object"}); len(res) != 0 {
		t.Errorf("expected empty map for empty properties")
	}

	schema := map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"str_default": map[string]interface{}{
				"type":        "string",
				"description": "Default test (default: my-def)",
			},
			"str_ticket": map[string]interface{}{
				"type":        "string",
				"description": "Ticket id",
			},
			"str_file": map[string]interface{}{
				"type":        "string",
				"description": "Target file path",
			},
			"str_status": map[string]interface{}{
				"type":        "string",
				"description": "New status",
			},
			"str_other": map[string]interface{}{
				"type":        "string",
				"description": "Normal string",
			},
			"bool_flag": map[string]interface{}{
				"type": "boolean",
			},
			"int_count": map[string]interface{}{
				"type": "integer",
			},
			"num_val": map[string]interface{}{
				"type": "number",
			},
			"arr_list": map[string]interface{}{
				"type": "array",
			},
			"obj_nested": map[string]interface{}{
				"type": "object",
			},
			"unknown_prop": map[string]interface{}{
				"type": "binary",
			},
			"invalid_prop": "not-a-map",
		},
	}

	args := synthesizeExampleArguments(schema)
	if args["str_default"] != "my-def" {
		t.Errorf("expected my-def, got: %v", args["str_default"])
	}
	if args["str_ticket"] != "DEN-985" {
		t.Errorf("expected DEN-985, got: %v", args["str_ticket"])
	}
	if args["str_file"] != "DEV_STATUS.md" {
		t.Errorf("expected DEV_STATUS.md, got: %v", args["str_file"])
	}
	if args["str_status"] != "DONE" {
		t.Errorf("expected DONE, got: %v", args["str_status"])
	}
	if args["str_other"] != "sample" {
		t.Errorf("expected sample, got: %v", args["str_other"])
	}
	if args["bool_flag"] != false {
		t.Errorf("expected false, got: %v", args["bool_flag"])
	}
	if args["int_count"] != 1 || args["num_val"] != 1 {
		t.Errorf("expected 1, got: %v, %v", args["int_count"], args["num_val"])
	}
	if len(args["arr_list"].([]string)) != 1 {
		t.Errorf("expected array with 1 item")
	}
	if args["unknown_prop"] != "value" {
		t.Errorf("expected value, got: %v", args["unknown_prop"])
	}
	if args["invalid_prop"] != "example" {
		t.Errorf("expected example, got: %v", args["invalid_prop"])
	}
}

func TestRenderMCPToolMarkdown(t *testing.T) {
	doc := MCPToolDoc{
		Name:        "test_mcp_tool",
		Description: "A test tool",
		InputSchema: map[string]interface{}{
			"type": "object",
			"properties": map[string]interface{}{
				"action": map[string]interface{}{"type": "string"},
			},
		},
		ExampleArgs: map[string]interface{}{
			"action": "run",
		},
	}

	md := RenderMCPToolMarkdown(doc)
	if !strings.Contains(md, "id: test_mcp_tool") {
		t.Errorf("missing frontmatter id")
	}
	if !strings.Contains(md, "# test_mcp_tool") {
		t.Errorf("missing tool title")
	}
	if !strings.Contains(md, "A test tool") {
		t.Errorf("missing description")
	}
	if !strings.Contains(md, "## Input Schema") {
		t.Errorf("missing input schema section")
	}
	if !strings.Contains(md, "dendrite mcp call test_mcp_tool") {
		t.Errorf("missing cli call example")
	}

	// Tool with empty description and empty arguments
	emptyDoc := MCPToolDoc{
		Name:        "empty_tool",
		ExampleArgs: map[string]interface{}{},
	}
	emptyMD := RenderMCPToolMarkdown(emptyDoc)
	if !strings.Contains(emptyMD, "No description available.") {
		t.Errorf("missing empty description fallback")
	}
	if !strings.Contains(emptyMD, "dendrite mcp call empty_tool\n") {
		t.Errorf("missing plain CLI invocation for empty args")
	}
}

func TestGenerateReferenceDocs_SuccessAndAuditPass(t *testing.T) {
	tempDocs := filepath.Join(t.TempDir(), "docs")

	reg := testRoot()
	reg.AddCommand(testCmd("alpha", "Alpha command"))
	reg.AddCommand(testCmd("beta", "Beta command"))

	srv := mcpserver.NewServer(mcpserver.ServerConfig{Name: "Srv"})
	srv.RegisterTool(mcpserver.Tool{
		Name:        "mcp_one",
		Description: "First tool",
		InputSchema: map[string]interface{}{"type": "object"},
	}, func(ctx context.Context, args map[string]interface{}) (*mcpserver.CallToolResult, error) {
		return nil, nil
	})

	// 1. Generate reference docs
	res, err := GenerateReferenceDocs(tempDocs, reg, srv)
	if err != nil {
		t.Fatalf("GenerateReferenceDocs failed: %v", err)
	}

	if res.TotalWritten != 4 {
		t.Fatalf("expected 4 files written (root + 2 CLI + 1 MCP), got: %d", res.TotalWritten)
	}

	// Verify files exist on disk
	alphaPath := filepath.Join(tempDocs, "cli", "alpha.md")
	betaPath := filepath.Join(tempDocs, "cli", "beta.md")
	mcpPath := filepath.Join(tempDocs, "mcp", "mcp_one.md")

	for _, p := range []string{alphaPath, betaPath, mcpPath} {
		if _, statErr := os.Stat(p); statErr != nil {
			t.Errorf("file was not written: %s", p)
		}
	}

	// 2. Audit should pass with 0 missing and 0 stale
	report, err := AuditReferenceDocs(tempDocs, reg, srv)
	if err != nil {
		t.Fatalf("AuditReferenceDocs failed: %v", err)
	}
	if !report.Passed {
		t.Errorf("expected audit to pass, got items: %v", report.Items)
	}
	if report.MissingCount != 0 || report.StaleCount != 0 {
		t.Errorf("expected 0 missing and 0 stale, got missing=%d stale=%d", report.MissingCount, report.StaleCount)
	}

	// 3. Re-running generation produces no diff
	alphaContent1, _ := os.ReadFile(alphaPath)
	res2, err := GenerateReferenceDocs(tempDocs, reg, srv)
	if err != nil {
		t.Fatalf("second GenerateReferenceDocs failed: %v", err)
	}
	if res2.TotalWritten != 4 {
		t.Errorf("expected 4 files written on second run")
	}
	alphaContent2, _ := os.ReadFile(alphaPath)
	if string(alphaContent1) != string(alphaContent2) {
		t.Errorf("expected re-running generator to produce no diff")
	}
}

func TestGenerateReferenceDocs_DefaultsAndNilServer(t *testing.T) {
	// Nil registry returns error
	_, err := GenerateReferenceDocs("", nil, nil)
	if err == nil {
		t.Fatalf("expected error for nil root")
	}

	// Nil server generates only CLI docs into temp directory
	tempDir := t.TempDir()
	reg := testRoot()
	reg.AddCommand(testCmd("ping", "Ping command"))

	res, err := GenerateReferenceDocs(tempDir, reg, nil)
	if err != nil {
		t.Fatalf("GenerateReferenceDocs failed with nil server: %v", err)
	}
	if res.TotalWritten != 2 || len(res.MCPFilesWritten) != 0 {
		t.Errorf("expected exactly 2 CLI docs (root + ping) and 0 MCP docs, got: %d", res.TotalWritten)
	}
}

func TestGenerateReferenceDocs_DirCreationError(t *testing.T) {
	// Attempt to create docs inside a regular file path (causes MkdirAll error)
	tempFile := filepath.Join(t.TempDir(), "blocker")
	if err := os.WriteFile(tempFile, []byte("blocker"), 0644); err != nil {
		t.Fatal(err)
	}

	reg := testRoot()
	_, err := GenerateReferenceDocs(tempFile, reg, nil)
	if err == nil {
		t.Errorf("expected error when target docs path is blocked by file")
	}
}
