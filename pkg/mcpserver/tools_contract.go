package mcpserver

import (
	"context"
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
	}, contractDriftCheckHandler())
}

// contractDriftCheckHandler handles the dendrite_contract_drift_check tool.
func contractDriftCheckHandler() ToolHandler {
	return func(ctx context.Context, args map[string]interface{}) (*CallToolResult, error) {
		schemaDirs := nonEmptyStrings(args["schema_dirs"])
		if len(schemaDirs) == 0 {
			if raw, ok := args["schema_dirs"].([]interface{}); !ok || len(raw) == 0 {
				return nil, fmt.Errorf("missing required 'schema_dirs' parameter")
			}
		}
		codeDirs := make(map[contract.TargetLanguage]string)
		cMap, _ := args["code_dirs"].(map[string]interface{})
		for langStr, pathVal := range cMap {
			if pStr, ok := pathVal.(string); ok && pStr != "" {
				codeDirs[contract.TargetLanguage(langStr)] = pStr
			}
		}
		report, err := defaultContractMonitor.CheckDrift(schemaDirs, codeDirs)
		if err != nil {
			return nil, fmt.Errorf("contract drift check failed: %w", err)
		}
		return textResult(string(indentJSON(report))), nil
	}
}
