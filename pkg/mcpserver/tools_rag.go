package mcpserver

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"Dendrite/pkg/rag"
)

func registerRAGTools(s *Server, deps Dependencies) {
	// 46. dendrite_rag_ask
	s.RegisterTool(Tool{
		Name:        "dendrite_rag_ask",
		Description: "Ask architectural, API, or codebase questions with local RAG retrieval and optional paid cloud LLM fallback",
		InputSchema: map[string]interface{}{
			"type": "object",
			"properties": map[string]interface{}{
				"question": map[string]interface{}{
					"type":        "string",
					"description": "Question to ask about the project",
				},
				"project_path": map[string]interface{}{
					"type":        "string",
					"description": "Target project directory to index and search (default: '.')",
				},
				"auto_approve_paid": map[string]interface{}{
					"type":        "boolean",
					"description": "Automatically escalate to paid cloud model if local LLM fails without prompt",
				},
				"paid_model": map[string]interface{}{
					"type":        "string",
					"description": "Paid cloud model fallback (default: 'gemini-2.5-pro')",
				},
				"local_model": map[string]interface{}{
					"type":        "string",
					"description": "Preferred local model",
				},
				"top_k": map[string]interface{}{
					"type":        "integer",
					"description": "Number of context chunks to retrieve (default: 4)",
				},
			},
			"required": []interface{}{"question"},
		},
	}, func(ctx context.Context, args map[string]interface{}) (*CallToolResult, error) {
		question, ok := args["question"].(string)
		if !ok || strings.TrimSpace(question) == "" {
			return nil, fmt.Errorf("question is required")
		}

		projectPath := "."
		if p, ok := args["project_path"].(string); ok && p != "" {
			projectPath = p
		}

		autoApprove, _ := args["auto_approve_paid"].(bool)
		paidModel := "gemini-2.5-pro"
		if pm, ok := args["paid_model"].(string); ok && pm != "" {
			paidModel = pm
		}

		localModel, _ := args["local_model"].(string)
		topK := 4
		if tk, ok := args["top_k"].(float64); ok && tk > 0 {
			topK = int(tk)
		} else if tkInt, ok := args["top_k"].(int); ok && tkInt > 0 {
			topK = tkInt
		}

		engine := rag.NewAskEngine(nil)
		req := rag.AskRequest{
			Question:        question,
			ProjectPath:     projectPath,
			AutoApprovePaid: autoApprove,
			PaidModel:       paidModel,
			LocalModel:      localModel,
			TopK:            topK,
			Input:           strings.NewReader("n\n"),
		}

		res, err := engine.Ask(ctx, req)
		if err != nil {
			return nil, fmt.Errorf("rag ask failed: %w", err)
		}

		data, err := json.MarshalIndent(res, "", "  ")
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

	// 47. dendrite_rag_retrieve
	s.RegisterTool(Tool{
		Name:        "dendrite_rag_retrieve",
		Description: "Retrieve high-precision AST symbol declarations, complexity metrics, and documentation chunks from the project RAG index for token-efficient prompt synthesis and reasoning",
		InputSchema: map[string]interface{}{
			"type": "object",
			"properties": map[string]interface{}{
				"query": map[string]interface{}{
					"type":        "string",
					"description": "Concept, symbol, or question to search within project RAG index",
				},
				"project_path": map[string]interface{}{
					"type":        "string",
					"description": "Target project directory to index and search (default: '.')",
				},
				"top_k": map[string]interface{}{
					"type":        "integer",
					"description": "Number of context chunks to retrieve (default: 4)",
				},
				"min_score": map[string]interface{}{
					"type":        "number",
					"description": "Minimum fused relevance score (default: none)",
				},
			},
			"required": []interface{}{"query"},
		},
	}, func(ctx context.Context, args map[string]interface{}) (*CallToolResult, error) {
		query, ok := args["query"].(string)
		if !ok || strings.TrimSpace(query) == "" {
			return nil, fmt.Errorf("query is required")
		}

		projectPath := "."
		if p, ok := args["project_path"].(string); ok && p != "" {
			projectPath = p
		}

		topK := 4
		if tk, ok := args["top_k"].(float64); ok && tk > 0 {
			topK = int(tk)
		} else if tkInt, ok := args["top_k"].(int); ok && tkInt > 0 {
			topK = tkInt
		}

		var minScore float32
		if ms, ok := args["min_score"].(float64); ok && ms > 0 {
			minScore = float32(ms)
		}

		retrieval, err := rag.NewRetriever().Retrieve(ctx, projectPath, query, rag.SearchOptions{
			TopK:     topK,
			MinScore: minScore,
		}, false)
		if err != nil {
			return nil, fmt.Errorf("rag retrieval failed for %s: %w", projectPath, err)
		}

		data, err := json.MarshalIndent(retrieval.Chunks, "", "  ")
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
