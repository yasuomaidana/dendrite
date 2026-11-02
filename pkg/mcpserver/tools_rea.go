package mcpserver

import (
	"context"
	"fmt"
	"strings"

	"Dendrite/pkg/rea/showcase"
)

func registerREATools(s *Server, deps Dependencies) {
	// 21. dendrite_rea_showcase
	s.RegisterTool(Tool{
		Name:        "dendrite_rea_showcase",
		Description: "Execute automated open-source REA cleanroom analysis, evidence extraction, property test verification, and ADR generation across showcase targets (aegis, dxball, ctf, th04, mattermost, all)",
		InputSchema: map[string]interface{}{
			"type": "object",
			"properties": map[string]interface{}{
				"target": map[string]interface{}{
					"type":        "string",
					"description": "Showcase target: 'all', 'aegis', 'dxball', 'ctf', 'th04', 'mattermost' (default: 'all')",
				},
				"format": map[string]interface{}{
					"type":        "string",
					"description": "Output representation: 'json', 'markdown', or 'summary' (default: 'summary')",
				},
			},
		},
	}, func(ctx context.Context, args map[string]interface{}) (*CallToolResult, error) {
		target := stringArgOr(args, "target", "all")
		format := stringArgOr(args, "format", "summary")

		report, err := showcase.RunShowcase(ctx, target)
		if err != nil {
			return nil, fmt.Errorf("showcase execution failed: %w", err)
		}

		jsonData := indentJSON(report)

		markdownScorecard := showcase.RenderMarkdownScorecard(report)

		switch strings.ToLower(format) {
		case "json":
			return textResult(string(jsonData)), nil
		case "markdown", "md":
			return textResult(markdownScorecard), nil
		default: // "summary"
			summaryText := fmt.Sprintf("[+] REA Showcase Verification (%s): %d/%d targets passed (%.2f%% aggregate parity)",
				report.OverallStatus, report.SucceededTargets, report.TotalTargets, report.OverallParityPercent)
			return &CallToolResult{
				Content: []ContentItem{
					{Type: "text", Text: summaryText},
					{Type: "text", Text: "\n" + markdownScorecard},
				},
			}, nil
		}
	})
}
