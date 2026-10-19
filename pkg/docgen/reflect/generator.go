package reflect

import (
	"fmt"
	"os"
	"path/filepath"

	"Dendrite/pkg/mcpserver"
	"github.com/spf13/cobra"
)

// ReferenceGenerationResult details the files written by reference documentation generation.
type ReferenceGenerationResult struct {
	CLIFilesWritten []string `json:"cli_files_written"`
	MCPFilesWritten []string `json:"mcp_files_written"`
	TotalWritten    int      `json:"total_written"`
}

// GenerateReferenceDocs generates docs/cli/<command>.md and docs/mcp/<tool>.md from live registries.
func GenerateReferenceDocs(docsDir string, root *cobra.Command, srv *mcpserver.Server) (*ReferenceGenerationResult, error) {
	if docsDir == "" {
		docsDir = "docs"
	}
	if root == nil {
		return nil, fmt.Errorf("cli root command cannot be nil")
	}

	cliDir := filepath.Join(docsDir, "cli")
	mcpDir := filepath.Join(docsDir, "mcp")

	if err := os.MkdirAll(cliDir, 0755); err != nil {
		return nil, fmt.Errorf("failed to create cli doc directory '%s': %w", cliDir, err)
	}
	if err := os.MkdirAll(mcpDir, 0755); err != nil {
		return nil, fmt.Errorf("failed to create mcp doc directory '%s': %w", mcpDir, err)
	}

	res := &ReferenceGenerationResult{
		CLIFilesWritten: make([]string, 0),
		MCPFilesWritten: make([]string, 0),
	}

	// 1. Generate CLI Command reference pages
	for _, page := range CLIReferencePages(root) {
		targetFile := filepath.Join(cliDir, page.Name+".md")
		if err := os.WriteFile(targetFile, []byte(page.Content), 0644); err != nil {
			return nil, fmt.Errorf("failed to write %s: %w", targetFile, err)
		}
		res.CLIFilesWritten = append(res.CLIFilesWritten, targetFile)
	}

	// 2. Generate MCP Tool reference pages
	if srv != nil {
		mcpDocs := IntrospectMCPServer(srv)
		for _, toolDoc := range mcpDocs {
			content := RenderMCPToolMarkdown(toolDoc)
			targetFile := filepath.Join(mcpDir, toolDoc.Name+".md")
			if err := os.WriteFile(targetFile, []byte(content), 0644); err != nil {
				return nil, fmt.Errorf("failed to write %s: %w", targetFile, err)
			}
			res.MCPFilesWritten = append(res.MCPFilesWritten, targetFile)
		}
	}

	res.TotalWritten = len(res.CLIFilesWritten) + len(res.MCPFilesWritten)
	return res, nil
}

// CLIReferencePage is one rendered docs/cli page.
type CLIReferencePage struct {
	Name    string
	Content string
}

// CLIReferencePages renders the root page (global flags and command index) plus one page per top-level command.
func CLIReferencePages(root *cobra.Command) []CLIReferencePage {
	rootDoc := IntrospectCLIRoot(root)
	pages := []CLIReferencePage{{Name: rootDoc.Name, Content: RenderCLIRootMarkdown(rootDoc)}}
	for _, cmdDoc := range IntrospectCLITree(root) {
		pages = append(pages, CLIReferencePage{Name: cmdDoc.Name, Content: RenderCLICommandMarkdown(cmdDoc)})
	}
	return pages
}
