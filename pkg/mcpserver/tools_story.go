package mcpserver

import (
	"Dendrite/pkg/statedir"
	"context"
	"encoding/json"
	"fmt"

	"Dendrite/pkg/store"
	"Dendrite/pkg/story/fork"
)

var defaultForkController = fork.NewForkController(
	".",
	statedir.Resolve(".dendrite/worktrees"),
	fork.NewOSGitExecutor(),
	fork.NewDefaultBenchmarkHarness(),
	store.NewStore(".dendrite/store", "docs/adr"),
)

func (s *Server) registerStoryForkTools(deps Dependencies) {
	// 47. dendrite_story_fork
	s.RegisterTool(Tool{
		Name:        "dendrite_story_fork",
		Description: "Fork a story into isolated Git worktrees and branches for competing implementation variants",
		InputSchema: map[string]interface{}{
			"type": "object",
			"properties": map[string]interface{}{
				"story_id": map[string]interface{}{
					"type":        "string",
					"description": "Identifier or slug of the story (e.g. 'DEN-064' or 'astar-search')",
				},
				"variants": map[string]interface{}{
					"type": "array",
					"items": map[string]interface{}{
						"type": "string",
					},
					"description": "List of variant names to fork (at least 2 required, e.g. ['v1', 'v2'])",
				},
				"base_commit": map[string]interface{}{
					"type":        "string",
					"description": "Optional base commit or branch to branch off (default: current HEAD)",
				},
			},
			"required": []interface{}{"story_id", "variants"},
		},
	}, func(ctx context.Context, args map[string]interface{}) (*CallToolResult, error) {
		storyID, _ := args["story_id"].(string)
		if storyID == "" {
			return nil, fmt.Errorf("missing required 'story_id' parameter")
		}

		variantsRaw, ok := args["variants"].([]interface{})
		if !ok || len(variantsRaw) < 2 {
			return nil, fmt.Errorf("'variants' must be an array of at least 2 variant names")
		}

		variants := make([]string, 0, len(variantsRaw))
		for _, v := range variantsRaw {
			if sVal, ok := v.(string); ok && sVal != "" {
				variants = append(variants, sVal)
			}
		}

		baseCommit, _ := args["base_commit"].(string)

		manifest, guard, err := defaultForkController.Fork(ctx, storyID, variants, baseCommit)
		if err != nil {
			return nil, fmt.Errorf("story fork failed: %w", err)
		}
		guard.Dismiss() // Keep worktrees alive for developer/agent use

		data, err := json.MarshalIndent(manifest, "", "  ")
		if err != nil {
			return nil, err
		}
		return &CallToolResult{
			Content: []ContentItem{{Type: "text", Text: string(data)}},
		}, nil
	})

	// 48. dendrite_story_benchmark
	s.RegisterTool(Tool{
		Name:        "dendrite_story_benchmark",
		Description: "Run comparative differential performance benchmarks (TTFT, TPS, memory, CPU) across variant worktrees",
		InputSchema: map[string]interface{}{
			"type": "object",
			"properties": map[string]interface{}{
				"story_id": map[string]interface{}{
					"type":        "string",
					"description": "Target story identifier",
				},
				"iterations": map[string]interface{}{
					"type":        "integer",
					"description": "Number of benchmark iterations per variant (default: 100)",
				},
				"virtual_users": map[string]interface{}{
					"type":        "integer",
					"description": "Number of simulated concurrent users (default: 1)",
				},
			},
			"required": []interface{}{"story_id"},
		},
	}, func(ctx context.Context, args map[string]interface{}) (*CallToolResult, error) {
		storyID, _ := args["story_id"].(string)
		if storyID == "" {
			return nil, fmt.Errorf("missing required 'story_id' parameter")
		}

		iterations := 100
		if it, ok := args["iterations"].(float64); ok && it > 0 {
			iterations = int(it)
		}

		vus := 1
		if v, ok := args["virtual_users"].(float64); ok && v > 0 {
			vus = int(v)
		}

		opts := fork.BenchmarkOptions{
			Iterations:   iterations,
			VirtualUsers: vus,
		}

		scorecard, err := defaultForkController.Benchmark(ctx, storyID, opts)
		if err != nil {
			return nil, fmt.Errorf("story benchmark failed: %w", err)
		}

		data, err := json.MarshalIndent(scorecard, "", "  ")
		if err != nil {
			return nil, err
		}
		return &CallToolResult{
			Content: []ContentItem{{Type: "text", Text: string(data)}},
		}, nil
	})

	// 49. dendrite_story_resolve
	s.RegisterTool(Tool{
		Name:        "dendrite_story_resolve",
		Description: "Merge winning variant worktree, persist benchmark metrics into store, and purge alternate worktrees",
		InputSchema: map[string]interface{}{
			"type": "object",
			"properties": map[string]interface{}{
				"story_id": map[string]interface{}{
					"type":        "string",
					"description": "Target story identifier",
				},
				"winner": map[string]interface{}{
					"type":        "string",
					"description": "Name of the winning variant to merge (e.g. 'v2' or 'astar-bitvec')",
				},
			},
			"required": []interface{}{"story_id", "winner"},
		},
	}, func(ctx context.Context, args map[string]interface{}) (*CallToolResult, error) {
		storyID, _ := args["story_id"].(string)
		winner, _ := args["winner"].(string)
		if storyID == "" || winner == "" {
			return nil, fmt.Errorf("missing required 'story_id' or 'winner' parameters")
		}

		res, err := defaultForkController.Resolve(ctx, storyID, winner)
		if err != nil {
			return nil, fmt.Errorf("story resolve failed: %w", err)
		}

		data, err := json.MarshalIndent(res, "", "  ")
		if err != nil {
			return nil, err
		}
		return &CallToolResult{
			Content: []ContentItem{{Type: "text", Text: string(data)}},
		}, nil
	})

	// 50. dendrite_story_abort
	s.RegisterTool(Tool{
		Name:        "dendrite_story_abort",
		Description: "Safely abort and purge all speculative worktrees and branches for a story without merging",
		InputSchema: map[string]interface{}{
			"type": "object",
			"properties": map[string]interface{}{
				"story_id": map[string]interface{}{
					"type":        "string",
					"description": "Target story identifier to clean up",
				},
			},
			"required": []interface{}{"story_id"},
		},
	}, func(ctx context.Context, args map[string]interface{}) (*CallToolResult, error) {
		storyID, _ := args["story_id"].(string)
		if storyID == "" {
			return nil, fmt.Errorf("missing required 'story_id' parameter")
		}

		if err := defaultForkController.Abort(ctx, storyID); err != nil {
			return nil, fmt.Errorf("story abort failed: %w", err)
		}

		return &CallToolResult{
			Content: []ContentItem{{Type: "text", Text: fmt.Sprintf("[✓] Speculative story '%s' successfully aborted and cleaned.", storyID)}},
		}, nil
	})
}
