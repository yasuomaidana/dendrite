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

func TestAuditReferenceDocs_ValidationAndMissingAndStale(t *testing.T) {
	// Nil registry
	_, err := AuditReferenceDocs("", nil, nil)
	if err == nil {
		t.Fatalf("expected error when root is nil")
	}

	tempDocs := filepath.Join(t.TempDir(), "docs")

	reg := testRoot()
	reg.AddCommand(testCmd("cmd1", "Command 1"))
	reg.AddCommand(testCmd("cmd2", "Command 2"))

	srv := mcpserver.NewServer(mcpserver.ServerConfig{Name: "Srv"})
	srv.RegisterTool(mcpserver.Tool{
		Name:        "tool1",
		Description: "Tool 1",
		InputSchema: map[string]interface{}{"type": "object"},
	}, func(ctx context.Context, args map[string]interface{}) (*mcpserver.CallToolResult, error) {
		return nil, nil
	})
	srv.RegisterTool(mcpserver.Tool{
		Name:        "tool2",
		Description: "Tool 2",
		InputSchema: map[string]interface{}{"type": "object"},
	}, func(ctx context.Context, args map[string]interface{}) (*mcpserver.CallToolResult, error) {
		return nil, nil
	})

	// Case 1: Empty docs directory -> all items missing
	rep, err := AuditReferenceDocs(tempDocs, reg, srv)
	if err != nil {
		t.Fatalf("AuditReferenceDocs failed: %v", err)
	}
	if rep.Passed {
		t.Errorf("expected audit to fail when files are missing")
	}
	if rep.MissingCount != 5 {
		t.Errorf("expected 5 missing files (root + 2 CLI + 2 MCP), got: %d", rep.MissingCount)
	}

	// Case 2: Generate all files properly
	_, err = GenerateReferenceDocs(tempDocs, reg, srv)
	if err != nil {
		t.Fatalf("GenerateReferenceDocs failed: %v", err)
	}

	// Verify audit passes
	rep2, err := AuditReferenceDocs(tempDocs, reg, srv)
	if err != nil || !rep2.Passed {
		t.Fatalf("expected audit to pass, got passed=%v err=%v", rep2.Passed, err)
	}

	// Case 3: Make cmd1 and tool1 stale by corrupting their contents
	cliPath := filepath.Join(tempDocs, "cli", "cmd1.md")
	if err := os.WriteFile(cliPath, []byte("Corrupted content"), 0644); err != nil {
		t.Fatal(err)
	}
	mcpPath := filepath.Join(tempDocs, "mcp", "tool1.md")
	if err := os.WriteFile(mcpPath, []byte("Outdated content"), 0644); err != nil {
		t.Fatal(err)
	}

	rep3, err := AuditReferenceDocs(tempDocs, reg, srv)
	if err != nil {
		t.Fatalf("AuditReferenceDocs failed: %v", err)
	}
	if rep3.Passed {
		t.Errorf("expected audit to fail on stale files")
	}
	if rep3.StaleCount != 2 {
		t.Errorf("expected 2 stale files, got: %d", rep3.StaleCount)
	}

	// Case 4: Audit with nil srv (audits only CLI commands)
	rep4, err := AuditReferenceDocs(tempDocs, reg, nil)
	if err != nil {
		t.Fatalf("AuditReferenceDocs failed with nil srv: %v", err)
	}
	if rep4.TotalChecked != 3 {
		t.Errorf("expected 3 CLI items checked, got: %d", rep4.TotalChecked)
	}
}

func TestGenerateReferenceDocs_FilesystemErrors(t *testing.T) {
	reg := testRoot()
	reg.AddCommand(testCmd("alpha", "Alpha command"))
	srv := mcpserver.NewServer(mcpserver.ServerConfig{Name: "Srv"})
	srv.RegisterTool(mcpserver.Tool{
		Name:        "mcp_one",
		Description: "First tool",
		InputSchema: map[string]interface{}{"type": "object"},
	}, func(ctx context.Context, args map[string]interface{}) (*mcpserver.CallToolResult, error) {
		return nil, nil
	})

	cases := map[string]func(dir string) error{
		"mcp dir is a file": func(dir string) error {
			return os.WriteFile(filepath.Join(dir, "mcp"), nil, 0o644)
		},
		"cli page path is a directory": func(dir string) error {
			return os.MkdirAll(filepath.Join(dir, "cli", "alpha.md"), 0o755)
		},
		"mcp page path is a directory": func(dir string) error {
			return os.MkdirAll(filepath.Join(dir, "mcp", "mcp_one.md"), 0o755)
		},
	}
	for name, prepare := range cases {
		t.Run(name, func(t *testing.T) {
			dir := t.TempDir()
			if err := prepare(dir); err != nil {
				t.Fatal(err)
			}
			if _, err := GenerateReferenceDocs(dir, reg, srv); err == nil {
				t.Fatalf("expected an error when %s", name)
			}
		})
	}
}

func TestRenderMCPToolMarkdown_UnmarshalableSchemaFallsBack(t *testing.T) {
	out := RenderMCPToolMarkdown(MCPToolDoc{
		Name:        "odd_tool",
		InputSchema: map[string]interface{}{"bad": make(chan int)},
	})
	if !strings.Contains(out, "\"properties\": {}") {
		t.Fatalf("expected empty-object schema fallback, got:\n%s", out)
	}
}

func TestIntrospectCLIRoot_GlobalFlagsAndGroups(t *testing.T) {
	if doc := IntrospectCLIRoot(nil); doc.Name != "" {
		t.Fatalf("expected empty doc for nil root")
	}
	root := testRoot()
	root.AddCommand(testCmd("alpha", "Alpha command"))
	other := testCmd("loose", "No group")
	other.GroupID = ""
	root.AddCommand(other)
	doc := IntrospectCLIRoot(root)
	if len(doc.GlobalFlags) != 1 || doc.GlobalFlags[0].Name != "--json" {
		t.Fatalf("unexpected global flags: %v", doc.GlobalFlags)
	}
	if len(doc.Groups) != 2 || doc.Groups[0].Title != "Core" || doc.Groups[1].Title != "Other" {
		t.Fatalf("unexpected groups: %v", doc.Groups)
	}
	md := RenderCLIRootMarkdown(doc)
	for _, want := range []string{"## Global Flags", "`--json`", "### Core", "[`alpha`](alpha.md)"} {
		if !strings.Contains(md, want) {
			t.Errorf("root markdown missing %q:\n%s", want, md)
		}
	}
}

func TestIntrospectCLICommand_EdgeCases(t *testing.T) {
	root := testRoot()
	cmd := testCmd("edge", "Edge command")
	cmd.Aliases = []string{"plane", "chronos-tracker"}
	cmd.Flags().String("secret", "", "Hidden flag")
	if err := cmd.Flags().MarkHidden("secret"); err != nil {
		t.Fatal(err)
	}
	sub := &cobra.Command{Use: "list", Aliases: []string{"ls"}, Short: "List", Run: func(*cobra.Command, []string) {}}
	hidden := &cobra.Command{Use: "internal", Hidden: true, Run: func(*cobra.Command, []string) {}}
	cmd.AddCommand(sub, hidden)
	stray := testCmd("stray", "Unknown group")
	stray.GroupID = "missing"
	root.AddCommand(cmd)

	doc := IntrospectCLICommand(cmd)
	if len(doc.Subcommands) != 1 || doc.Subcommands[0].Name != "list" {
		t.Fatalf("hidden subcommand should be skipped: %v", doc.Subcommands)
	}
	if len(doc.Flags) != 0 {
		t.Fatalf("hidden flag should be skipped: %v", doc.Flags)
	}
	if md := RenderCLICommandMarkdown(doc); !strings.Contains(md, "Aliases: `ls`") {
		t.Errorf("subcommand aliases not rendered:\n%s", md)
	}
	if got := categoryOf(stray); got != "" {
		t.Errorf("command without parent should have no category, got %q", got)
	}
	root.AddGroup(&cobra.Group{ID: "other", Title: "Other:"})
	root.AddCommand(stray)
	if got := categoryOf(stray); got != "" {
		t.Errorf("unknown group should have no category, got %q", got)
	}
	if got := formatAliases([]string{"a", "b"}); got != "`a`, `b`" {
		t.Errorf("formatAliases = %q", got)
	}
}
