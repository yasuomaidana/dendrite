package reflect

import (
	"fmt"
	"os"
	"path/filepath"

	"Dendrite/pkg/cli"
	"Dendrite/pkg/mcpserver"
)

// ReferenceGenerationResult details the files written by reference documentation generation.
type ReferenceGenerationResult struct {
	CLIFilesWritten []string `json:"cli_files_written"`
	MCPFilesWritten []string `json:"mcp_files_written"`
	TotalWritten    int      `json:"total_written"`
}

// GenerateReferenceDocs generates docs/cli/<command>.md and docs/mcp/<tool>.md from live registries.
func GenerateReferenceDocs(docsDir string, reg *cli.Registry, srv *mcpserver.Server) (*ReferenceGenerationResult, error) {
	if docsDir == "" {
		docsDir = "docs"
	}
	if reg == nil {
		return nil, fmt.Errorf("cli registry cannot be nil")
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
	cliDocs := IntrospectCLIRegistry(reg)
	for _, cmdDoc := range cliDocs {
		content := RenderCLICommandMarkdown(cmdDoc)
		targetFile := filepath.Join(cliDir, cmdDoc.Name+".md")
		if err := os.WriteFile(targetFile, []byte(content), 0644); err != nil {
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
