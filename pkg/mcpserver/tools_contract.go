package mcpserver

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/yasuomaidana/dendrite-mcp-hub/pkg/polyglot/contract"
)

var defaultContractMonitor = contract.NewContractMonitor(nil)

func registerContractTools(s *Server, deps Dependencies) {
	// 45. dendrite_contract_drift_check
	s.RegisterTool(Tool{
		Name:        "dendrite_contract_drift_check",
		Description: "Analyze cross-language schema and IPC contract drift across Protobuf, OpenAPI, and JSON-Schema definitions against Go, Rust, TS, and Python models in Quad-IDE projects",
		InputSchema: map[string]interface{}{
			"type": "object",
			"properties": map[string]interface{}{
				"schema_dirs": map[string]interface{}{
					"type": "array",
					"items": map[string]interface{}{
						"type": "string",
					},
					"description": "List of directories containing .proto, openapi.yaml, or .json schema definitions",
				},
				"code_dirs": map[string]interface{}{
					"type":        "object",
					"description": "Map of target languages ('go', 'rust', 'typescript', 'python') to repository source directories",
				},
			},
			"required": []interface{}{"schema_dirs"},
		},
	}, func(ctx context.Context, args map[string]interface{}) (*CallToolResult, error) {
		schemaDirsRaw, ok := args["schema_dirs"].([]interface{})
		if !ok || len(schemaDirsRaw) == 0 {
			return nil, fmt.Errorf("missing required 'schema_dirs' parameter")
		}

		schemaDirs := make([]string, 0, len(schemaDirsRaw))
		for _, s := range schemaDirsRaw {
			if sStr, ok := s.(string); ok && sStr != "" {
				schemaDirs = append(schemaDirs, sStr)
			}
		}

		codeDirs := make(map[contract.TargetLanguage]string)
		if cMap, ok := args["code_dirs"].(map[string]interface{}); ok {
			for langStr, pathVal := range cMap {
				if pStr, ok := pathVal.(string); ok && pStr != "" {
					codeDirs[contract.TargetLanguage(langStr)] = pStr
				}
			}
		}

		report, err := defaultContractMonitor.CheckDrift(schemaDirs, codeDirs)
		if err != nil {
			return nil, fmt.Errorf("contract drift check failed: %w", err)
		}

		data, err := json.MarshalIndent(report, "", "  ")
		if err != nil {
			return nil, err
		}

		return &CallToolResult{
			Content: []ContentItem{
				{
					Type: "text",
					Text: string(data),
				},
			},
		}, nil
	})
}
