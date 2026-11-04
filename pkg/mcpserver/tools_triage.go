package mcpserver

import (
	"context"
	"fmt"

	"Dendrite/pkg/agent/triage"
	"Dendrite/pkg/ollama"
)

func registerTriageTools(s *Server, deps Dependencies) {
	// 23. dendrite_local_agent_triage
	s.RegisterTool(Tool{
		Name:        "dendrite_local_agent_triage",
		Description: "Intercepts developer prompts, evaluates local compute across multi-provider inference (Ollama, Ollama Validated, Hugging Face, Candle/OxiLLaMa), routes chores to zero-cost local models, and enhances/compacts prompts with project skills for cloud escalation",
		InputSchema: map[string]interface{}{
			"type":     "object",
			"required": []string{"prompt"},
			"properties": map[string]interface{}{
				"prompt": map[string]interface{}{
					"type":        "string",
					"description": "Developer prompt or chat message",
				},
				"active_file": map[string]interface{}{
					"type":        "string",
					"description": "Optional active file path in the IDE editor",
				},
				"file_content": map[string]interface{}{
					"type":        "string",
					"description": "Optional source code content of active file",
				},
				"force_tier": map[string]interface{}{
					"type":        "string",
					"description": "Optional tier override: 'local', 'hybrid', 'cloud'",
				},
				"target_engine": map[string]interface{}{
					"type":        "string",
					"description": "Target inference backend: 'ollama', 'ollama-validated', 'huggingface', 'candle', 'oxillama'",
				},
				"execute_local": map[string]interface{}{
					"type":        "boolean",
					"description": "Execute chore tasks locally if local inference is available",
				},
			},
		},
	}, localAgentTriageHandler(deps))
}

// triageTiers and triageEngines name the tiers and engines a caller may force; others mean "decide".
var (
	triageTiers = map[string]triage.ExecutionTier{
		"local": triage.TierLocal, "hybrid": triage.TierHybrid, "cloud": triage.TierCloud,
	}
	triageEngines = map[string]triage.SupportedEngine{
		"ollama": triage.EngineOllama, "ollama-validated": triage.EngineOllamaValidated,
		"huggingface": triage.EngineHuggingFace, "hf": triage.EngineHuggingFace,
		"candle": triage.EngineCandle, "oxillama": triage.EngineOxiLLaMa,
	}
)

// localAgentTriageHandler handles the dendrite_local_agent_triage tool.
func localAgentTriageHandler(deps Dependencies) ToolHandler {
	return func(ctx context.Context, args map[string]interface{}) (*CallToolResult, error) {
		prompt, _ := args["prompt"].(string)
		if prompt == "" {
			return nil, fmt.Errorf("prompt is required")
		}
		activeFile, _ := args["active_file"].(string)
		fileContent, _ := args["file_content"].(string)
		forceTierStr, _ := args["force_tier"].(string)
		targetEngineStr, _ := args["target_engine"].(string)
		executeLocal, _ := args["execute_local"].(bool)

		forceTier := triageTiers[forceTierStr]
		targetEngine := triageEngines[targetEngineStr]

		var ollamaClient *ollama.Client
		if deps.OllamaURL != "" {
			ollamaClient = ollama.NewClient(ollama.WithBaseURL(deps.OllamaURL))
		}
		engine := triage.NewEngine(ollamaClient)

		req := triage.TriageRequest{
			Prompt:       prompt,
			ActiveFile:   activeFile,
			FileContent:  fileContent,
			ForceTier:    forceTier,
			TargetEngine: targetEngine,
			ExecuteLocal: executeLocal,
		}

		res, _ := engine.Triage(ctx, req) // Triage degrades to a heuristic answer instead of failing

		data := indentJSON(res)

		return textResult(string(data)), nil
	}
}
