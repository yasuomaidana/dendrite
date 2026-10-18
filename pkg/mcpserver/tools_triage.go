package mcpserver

import (
	"context"
	"encoding/json"
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
	}, func(ctx context.Context, args map[string]interface{}) (*CallToolResult, error) {
		prompt, _ := args["prompt"].(string)
		if prompt == "" {
			return nil, fmt.Errorf("prompt is required")
		}
		activeFile, _ := args["active_file"].(string)
		fileContent, _ := args["file_content"].(string)
		forceTierStr, _ := args["force_tier"].(string)
		targetEngineStr, _ := args["target_engine"].(string)
		executeLocal, _ := args["execute_local"].(bool)

		var forceTier triage.ExecutionTier
		switch forceTierStr {
		case "local":
			forceTier = triage.TierLocal
		case "hybrid":
			forceTier = triage.TierHybrid
		case "cloud":
			forceTier = triage.TierCloud
		}

		var targetEngine triage.SupportedEngine
		switch targetEngineStr {
		case "ollama":
			targetEngine = triage.EngineOllama
		case "ollama-validated":
			targetEngine = triage.EngineOllamaValidated
		case "huggingface", "hf":
			targetEngine = triage.EngineHuggingFace
		case "candle":
			targetEngine = triage.EngineCandle
		case "oxillama":
			targetEngine = triage.EngineOxiLLaMa
		}

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

		res, err := engine.Triage(ctx, req)
		if err != nil {
			return nil, fmt.Errorf("triage failed: %w", err)
		}

		data, err := json.MarshalIndent(res, "", "  ")
		if err != nil {
			return nil, err
		}

		return &CallToolResult{
			Content: []ContentItem{
				{Type: "text", Text: string(data)},
			},
		}, nil
	})
}
