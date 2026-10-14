package main

import (
	"context"
	"fmt"
	"os"
	"strings"

	"Dendrite/pkg/agent/triage"
	"Dendrite/pkg/ollama"
)

func handleTriageCommand(ctx context.Context, args []string) {
	prompt := ""
	filePath := ""
	forceTierStr := ""
	targetEngineStr := ""
	executeLocal := false

	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "--prompt", "-p":
			if i+1 < len(args) {
				prompt = args[i+1]
				i++
			}
		case "--file", "-f":
			if i+1 < len(args) {
				filePath = args[i+1]
				i++
			}
		case "--tier", "-t":
			if i+1 < len(args) {
				forceTierStr = args[i+1]
				i++
			}
		case "--engine", "-e":
			if i+1 < len(args) {
				targetEngineStr = args[i+1]
				i++
			}
		case "--local", "-l":
			executeLocal = true
		}
	}

	if prompt == "" && len(args) > 0 && !strings.HasPrefix(args[0], "-") {
		prompt = strings.Join(args, " ")
	}

	if prompt == "" {
		fmt.Println("Usage: dendrite triage --prompt \"<prompt>\" [--file <file>] [--tier local|hybrid|cloud] [--engine ollama|ollama-validated|huggingface|candle|oxillama] [--local]")
		return
	}

	fileContent := ""
	if filePath != "" {
		data, err := os.ReadFile(filePath)
		if err != nil {
			fmt.Printf("[-] Warning: Failed to read file %s: %v\n", filePath, err)
		} else {
			fileContent = string(data)
		}
	}

	var forceTier triage.ExecutionTier
	switch strings.ToLower(forceTierStr) {
	case "local":
		forceTier = triage.TierLocal
	case "hybrid":
		forceTier = triage.TierHybrid
	case "cloud":
		forceTier = triage.TierCloud
	}

	var targetEngine triage.SupportedEngine
	switch strings.ToLower(targetEngineStr) {
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

	engine := triage.NewEngine(ollama.NewClient())
	req := triage.TriageRequest{
		Prompt:       prompt,
		ActiveFile:   filePath,
		FileContent:  fileContent,
		ForceTier:    forceTier,
		TargetEngine: targetEngine,
		ExecuteLocal: executeLocal,
	}

	res, err := engine.Triage(ctx, req)
	if err != nil {
		fmt.Printf("[-] Triage failed: %v\n", err)
		return
	}

	fmt.Printf("[+] Local Agent Triage Decision:\n")
	fmt.Printf("    - Assigned Tier:      %s\n", strings.ToUpper(string(res.Tier)))
	fmt.Printf("    - Assigned Model:     %s\n", res.AssignedModel)
	fmt.Printf("    - Assigned Engine:    %s (%s)\n", res.AssignedEngine, res.LocalEngineName)
	fmt.Printf("    - Local Compute:      %v\n", res.LocalComputeAvailable)
	fmt.Printf("    - Decision Reason:    %s\n", res.Reason)
	if len(res.SelectedSkills) > 0 {
		fmt.Printf("    - Matched Skills:     %s\n", strings.Join(res.SelectedSkills, ", "))
	}
	fmt.Printf("    - Original Tokens:    %d\n", res.OriginalTokens)
	fmt.Printf("    - Compacted Tokens:   %d\n", res.CompactedTokens)
	fmt.Printf("    - Token Savings:      %.1f%%\n", res.TokenSavingsPct)
	fmt.Printf("    - Estimated Cost:     $%.6f (saved $%.6f)\n", res.EstimatedCostUSD, res.CostWithoutTriageUSD-res.EstimatedCostUSD)
	if res.LocalExecutionResult != "" {
		fmt.Printf("    - Local Execution:    %s\n", res.LocalExecutionResult)
	}
	if res.IsDeclarativeOrchestrator && res.DeclarativePlan != nil {
		fmt.Printf("    - Orchestrator Mode:  Declarative Cloud Orchestrator -> Local Worker\n")
		fmt.Printf("    - Orchestrator Model: %s\n", res.OrchestratorModel)
		fmt.Printf("    - Local Worker Model: %s (%s)\n", res.WorkerModel, res.WorkerEngine)
		fmt.Printf("    - Output Savings:     %.1f%% (%d plan tokens vs %d direct tokens)\n",
			res.DeclarativePlan.OutputTokenSavingsPct,
			res.DeclarativePlan.EstimatedOrchestratorTokens,
			res.DeclarativePlan.EstimatedDirectOutputTokens)
		fmt.Printf("    - Declarative Steps:  %d instructions, %d skill constraints\n",
			len(res.DeclarativePlan.Instructions), len(res.DeclarativePlan.Constraints))
		if res.LocalWorkerExecutionResult != "" {
			fmt.Printf("    - Local Worker Run:   %s\n", res.LocalWorkerExecutionResult)
		}
	}
	fmt.Printf("    - Triage Latency:     %v\n", res.ExecutionDuration)
}
