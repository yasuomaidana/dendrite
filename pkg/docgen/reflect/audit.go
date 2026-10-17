package reflect

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"Dendrite/pkg/cli"
	"Dendrite/pkg/mcpserver"
)

// AuditItem represents a missing or stale documentation page.
type AuditItem struct {
	Type   string `json:"type"`   // "cli" or "mcp"
	Name   string `json:"name"`   // command or tool name
	Path   string `json:"path"`   // expected file path
	Reason string `json:"reason"` // "missing" or "stale"
}

// AuditReport details the findings of a reference documentation audit.
type AuditReport struct {
	TotalChecked int         `json:"total_checked"`
	MissingCount int         `json:"missing_count"`
	StaleCount   int         `json:"stale_count"`
	Items        []AuditItem `json:"items"`
	Passed       bool        `json:"passed"`
}

// AuditReferenceDocs audits docs/cli/ and docs/mcp/ against live registries.
func AuditReferenceDocs(docsDir string, reg *cli.Registry, srv *mcpserver.Server) (*AuditReport, error) {
	if docsDir == "" {
		docsDir = "docs"
	}
	if reg == nil {
		return nil, fmt.Errorf("cli registry cannot be nil")
	}

	report := &AuditReport{
		Items:  make([]AuditItem, 0),
		Passed: true,
	}

	cliDir := filepath.Join(docsDir, "cli")
	mcpDir := filepath.Join(docsDir, "mcp")

	// 1. Audit CLI commands
	cliDocs := IntrospectCLIRegistry(reg)
	for _, cmdDoc := range cliDocs {
		report.TotalChecked++
		targetFile := filepath.Join(cliDir, cmdDoc.Name+".md")
		data, err := os.ReadFile(targetFile)
		if err != nil {
			report.MissingCount++
			report.Items = append(report.Items, AuditItem{
				Type:   "cli",
				Name:   cmdDoc.Name,
				Path:   targetFile,
				Reason: "missing",
			})
			continue
		}

		expected := RenderCLICommandMarkdown(cmdDoc)
		if strings.TrimSpace(string(data)) != strings.TrimSpace(expected) {
			report.StaleCount++
			report.Items = append(report.Items, AuditItem{
				Type:   "cli",
				Name:   cmdDoc.Name,
				Path:   targetFile,
				Reason: "stale",
			})
		}
	}

	// 2. Audit MCP tools
	if srv != nil {
		mcpDocs := IntrospectMCPServer(srv)
		for _, toolDoc := range mcpDocs {
			report.TotalChecked++
			targetFile := filepath.Join(mcpDir, toolDoc.Name+".md")
			data, err := os.ReadFile(targetFile)
			if err != nil {
				report.MissingCount++
				report.Items = append(report.Items, AuditItem{
					Type:   "mcp",
					Name:   toolDoc.Name,
					Path:   targetFile,
					Reason: "missing",
				})
				continue
			}

			expected := RenderMCPToolMarkdown(toolDoc)
			if strings.TrimSpace(string(data)) != strings.TrimSpace(expected) {
				report.StaleCount++
				report.Items = append(report.Items, AuditItem{
					Type:   "mcp",
					Name:   toolDoc.Name,
					Path:   targetFile,
					Reason: "stale",
				})
			}
		}
	}

	if report.MissingCount > 0 || report.StaleCount > 0 {
		report.Passed = false
	}

	return report, nil
}
