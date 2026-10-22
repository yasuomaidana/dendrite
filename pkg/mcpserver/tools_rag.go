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
					"description": "Number of best chunks to answer from (default: 4); 0 answers from a token-budgeted context pack instead",
				},
				"corpus": map[string]interface{}{
					"type":        "string",
					"enum":        []interface{}{"all", "code", "docs"},
					"description": "Corpora searched: all (default: code and documentation), code (implementation questions), or docs (documentation only: design and conceptual questions)",
				},
				"docs_share": map[string]interface{}{
					"type":        "number",
					"minimum":     0,
					"maximum":     1,
					"description": "With corpus all: share of the context pack kept for documentation, 0 to 1 (default: the project's [ask] docs_share, else 0.2). Lower leans on code, higher on documentation",
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
		// An explicit 0 asks for the context pack, which the documentation share applies to.
		if tk, ok := args["top_k"].(float64); ok && tk >= 0 {
			topK = int(tk)
		} else if tkInt, ok := args["top_k"].(int); ok && tkInt >= 0 {
			topK = tkInt
		}

		corpus, _ := args["corpus"].(string)
		docsShare, err := docsShareArg(args)
		if err != nil {
			return nil, err
		}
		if _, set := args["top_k"]; docsShare != nil && !set {
			topK = 0 // a share only applies to the context pack
		}
		engine := rag.NewAskEngine()
		req := rag.AskRequest{
			Corpus:          corpus,
			DocsShare:       docsShare,
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
		Description: "Retrieve code and documentation chunks from the project RAG index. With detail \"map\" it returns outlines (doc comment or heading and declaration) of a token-budgeted context pack, a few dozen tokens each: read the map, then fetch the bodies you need with dendrite_rag_show",
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
				"corpus": map[string]interface{}{
					"type":        "string",
					"enum":        []interface{}{"all", "code", "docs"},
					"description": "Corpora searched: all (default: code and documentation in one ranking), code, or docs",
				},
				"detail": map[string]interface{}{
					"type":        "string",
					"enum":        []interface{}{"auto", "full", "map"},
					"description": "Return a context pack instead of top hits: auto (text of the best spans, outlines of their graph neighbours), full, or map (outlines only)",
				},
				"token_budget": map[string]interface{}{
					"type":        "integer",
					"description": "Token budget of the context pack when detail is set (default: 2500)",
				},
				"docs_share": map[string]interface{}{
					"type":        "number",
					"minimum":     0,
					"maximum":     1,
					"description": "With corpus all: share of the context pack (when detail is set) kept for documentation, 0 to 1 (default: the project's [ask] docs_share, else 0.2). Lower leans on code, higher on documentation",
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

		corpus, _ := args["corpus"].(string)
		var retrieval rag.Retrieval
		var err error
		if detail, _ := args["detail"].(string); detail != "" {
			budget := rag.AskContextTokens
			if b, ok := args["token_budget"].(float64); ok && b > 0 {
				budget = int(b)
			}
			docsShare, shareErr := docsShareArg(args)
			if shareErr != nil {
				return nil, shareErr
			}
			retrieval, err = rag.NewRetriever().ContextWith(ctx, projectPath, query, budget, rag.ContextOptions{Corpus: corpus, Detail: detail, DocsShare: docsShare}, false)
		} else {
			retrieval, err = rag.NewRetriever().Retrieve(ctx, projectPath, query, rag.SearchOptions{
				TopK:     topK,
				MinScore: minScore,
				Corpus:   corpus,
			}, false)
		}
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

	// 48. dendrite_rag_show
	s.RegisterTool(Tool{
		Name:        "dendrite_rag_show",
		Description: "Full text of chunks by id: the bodies behind the outlines dendrite_rag_retrieve returns with detail \"map\"",
		InputSchema: map[string]interface{}{
			"type": "object",
			"properties": map[string]interface{}{
				"ids": map[string]interface{}{
					"type":        "array",
					"items":       map[string]interface{}{"type": "string"},
					"description": "Chunk ids from dendrite_rag_retrieve",
				},
				"project_path": map[string]interface{}{
					"type":        "string",
					"description": "Target project directory (default: '.')",
				},
			},
			"required": []interface{}{"ids"},
		},
	}, func(ctx context.Context, args map[string]interface{}) (*CallToolResult, error) {
		raw, _ := args["ids"].([]interface{})
		var ids []string
		for _, v := range raw {
			if id, ok := v.(string); ok && id != "" {
				ids = append(ids, id)
			}
		}
		if len(ids) == 0 {
			return nil, fmt.Errorf("ids is required")
		}
		projectPath := "."
		if p, ok := args["project_path"].(string); ok && p != "" {
			projectPath = p
		}
		chunks, err := rag.NewRetriever().Show(ctx, projectPath, ids)
		if err != nil {
			return nil, fmt.Errorf("rag show failed for %s: %w", projectPath, err)
		}
		data, err := json.MarshalIndent(chunks, "", "  ")
		if err != nil {
			return nil, err
		}
		return &CallToolResult{Content: []ContentItem{{Type: "text", Text: string(data)}}}, nil
	})
}

// docsShareArg reads the optional "docs_share" argument: nil when absent, an error outside
// 0 to 1.
func docsShareArg(args map[string]interface{}) (*float64, error) {
	v, ok := args["docs_share"].(float64)
	if !ok {
		return nil, nil
	}
	if v < 0 || v > 1 {
		return nil, fmt.Errorf("docs_share must be from 0 to 1, got %v", v)
	}
	return &v, nil
}
