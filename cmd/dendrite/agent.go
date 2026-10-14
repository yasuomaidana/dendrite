package main

import (
	"context"
	"fmt"
	"os"
	"strconv"
	"strings"

	"Dendrite/pkg/agent"
	"Dendrite/pkg/agentsearch"
)

func handleAgentSearchCommand(ctx context.Context, args []string) {
	if len(args) == 0 {
		fmt.Println("Usage: dendrite agent search <query> [--pkg <package>] [--local] [--max-cost <usd>]")
		return
	}

	var queryParts []string
	var targetPkg string
	var requireLocal bool
	var maxCost float64

	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "--pkg", "--package":
			if i+1 < len(args) {
				targetPkg = args[i+1]
				i++
			}
		case "--local":
			requireLocal = true
		case "--max-cost":
			if i+1 < len(args) {
				maxCost, _ = strconv.ParseFloat(args[i+1], 64)
				i++
			}
		default:
			queryParts = append(queryParts, args[i])
		}
	}

	qStr := strings.TrimSpace(strings.Join(queryParts, " "))
	if qStr == "" {
		fmt.Println("[-] Please provide a search query")
		return
	}

	q := agentsearch.NewQueryBuilder().
		WithText(qStr).
		WithTargetPackage(targetPkg).
		RequireLocalInference(requireLocal).
		WithMaxCost(maxCost).
		Build()

	engine := agentsearch.NewSearchEngine(nil, nil, nil)
	results, err := engine.Search(ctx, q)
	if err != nil {
		fmt.Printf("[-] Agent search error: %v\n", err)
		return
	}

	fmt.Printf("[+] Agent Search Results (%d matching candidates):\n\n", len(results))
	for i, res := range results {
		fmt.Printf("  %d. [%s] %s\n", i+1, res.Manifest.ID, res.Manifest.Name)
		fmt.Printf("     - Capability Score: %.2f (RRF: %.2f, Dense: %.2f, Sparse: %.2f)\n",
			res.FinalScore, res.RRFScore, res.DenseScore, res.SparseScore)
		fmt.Printf("     - Preferred Tier:   %s (Engine: %s, Model: %s)\n",
			res.Manifest.PreferredTier, res.Manifest.PreferredEngine, res.Manifest.PreferredModel)
		fmt.Printf("     - Zero-Cost:        %v\n", res.Manifest.ZeroCostCandidate)
		fmt.Printf("     - Match Reasoning:  %s\n\n", res.Reasoning)
	}
}

func handleAgentRouteCommand(ctx context.Context, args []string) {
	if len(args) == 0 {
		fmt.Println("Usage: dendrite agent route --task <desc> [--pkg <package>] [--exec] [--local]")
		return
	}

	var taskDesc string
	var targetPkg string
	var execute bool
	var requireLocal bool

	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "--task", "--prompt":
			if i+1 < len(args) {
				taskDesc = args[i+1]
				i++
			}
		case "--pkg", "--package":
			if i+1 < len(args) {
				targetPkg = args[i+1]
				i++
			}
		case "--exec", "--execute":
			execute = true
		case "--local":
			requireLocal = true
		default:
			if taskDesc == "" && !strings.HasPrefix(args[i], "--") {
				taskDesc = args[i]
			}
		}
	}

	taskDesc = strings.TrimSpace(taskDesc)
	if taskDesc == "" {
		fmt.Println("[-] Please provide a task description via --task \"...\"")
		return
	}

	q := agentsearch.NewQueryBuilder().
		WithText(taskDesc).
		WithTargetPackage(targetPkg).
		RequireLocalInference(requireLocal).
		Build()

	engine := agentsearch.NewSearchEngine(nil, nil, nil)
	decision, err := engine.Route(ctx, q)
	if err != nil {
		fmt.Printf("[-] Routing error: %v\n", err)
		return
	}

	fmt.Printf("[+] Optimal Agent Routing Decision:\n")
	fmt.Printf("    - Selected Agent:   %s (%s)\n", decision.SelectedAgent.ID, decision.SelectedAgent.Name)
	fmt.Printf("    - Capability Score:  %.2f\n", decision.Score)
	fmt.Printf("    - Recommended Tier:  %s\n", decision.Tier)
	fmt.Printf("    - Target Engine:    %s\n", decision.EngineID)
	fmt.Printf("    - Target Model:     %s\n", decision.Model)
	fmt.Printf("    - Virtual Key:      %s\n", decision.VirtualKeyAlias)
	fmt.Printf("    - Estimated Cost:   $%.2f\n", decision.EstimatedCost)
	fmt.Printf("    - Routing Rationale: %s\n", decision.Reasoning)

	if execute {
		fmt.Printf("\n[+] Auto-launching agent harness (%s)...\n", decision.SelectedAgent.ID)
		cwd, _ := os.Getwd()
		cfg := agent.AgentConfig{
			Type:            decision.SelectedAgent.Type,
			LiteLLMBaseURL:  "http://localhost:4000/v1",
			VirtualAPIKey:   decision.VirtualKeyAlias,
			Model:           decision.Model,
			WorkspaceDir:    cwd,
			TaskDescription: taskDesc,
		}
		runner := &agent.MockRunner{}
		summary, rErr := runner.Execute(ctx, cfg)
		if rErr != nil {
			fmt.Printf("[-] Agent execution failed: %v\n", rErr)
			return
		}
		fmt.Printf("[✓] Executed successfully (Session ID: %s, Duration: %v)\n", summary.SessionID, summary.Duration)
	}
}
