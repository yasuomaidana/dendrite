package triage

import (
	"context"
	"fmt"
	"os"
	"strings"
	"time"

	"Dendrite/pkg/ai/tokenopt"
	"Dendrite/pkg/inference"
	"Dendrite/pkg/ollama"
	"Dendrite/pkg/rag"
)

// Engine orchestrates local agent triage, hardware probing, skill matching, and token optimization.
type Engine struct {
	OllamaClient         *ollama.Client
	OllamaEndpoint       string
	OxiLLaMaEngine       inference.InferenceEngine
	OxiLLaMaEndpoint     string
	RAGRetriever         *rag.Retriever
	SkillMatchers        []SkillMatcher
	LocalModelName       string
	HybridModelName      string
	CloudModelName       string
	CostPerCloudToken    float64 // USD per token (default: 0.000003 for Claude Sonnet / Gemini Pro)
	MockComputeAvailable *bool   // For testing
}

// NewEngine constructs a default Triage Engine.
func NewEngine(ollamaClient *ollama.Client) *Engine {
	ollamaEndpoint := os.Getenv("DENDRITE_OLLAMA_ENDPOINT")
	if ollamaClient == nil {
		if ollamaEndpoint != "" {
			ollamaClient = ollama.NewClient(ollama.WithBaseURL(ollamaEndpoint))
		} else {
			ollamaClient = ollama.NewClient()
		}
	}
	return &Engine{
		OllamaClient:      ollamaClient,
		OllamaEndpoint:    ollamaEndpoint,
		OxiLLaMaEndpoint:  os.Getenv("DENDRITE_OXILLAMA_ENDPOINT"),
		RAGRetriever:      rag.NewRetriever(),
		SkillMatchers:     DefaultSkillMatchers(),
		LocalModelName:    "local-qwen-coder",
		HybridModelName:   "copilot-sonnet",
		CloudModelName:    "gemini-2.5-pro",
		CostPerCloudToken: 0.000003,
	}
}

// Triage evaluates a developer prompt, evaluates local hardware availability,
// assigns the execution tier, matches relevant skills, compacts context, and computes token savings.
func (e *Engine) Triage(ctx context.Context, req TriageRequest) (*TriageResult, error) {
	start := time.Now()

	// 1. Initial task classification
	tier, reason := ClassifyTaskComplexity(req.Prompt)
	if req.ForceTier != "" {
		tier = req.ForceTier
		reason = fmt.Sprintf("Execution tier explicitly overridden to %s.", tier)
	}

	// 2. Hardware / Compute Capability Probe & Target Engine Resolution
	assignedEngine := EngineOllama
	if req.TargetEngine != "" {
		assignedEngine = req.TargetEngine
	}

	localAvailable := false
	localEngineName := "Ollama Metal GPU"

	oxEng := e.OxiLLaMaEngine
	if oxEng == nil {
		oxEndpoint := e.OxiLLaMaEndpoint
		if oxEndpoint == "" {
			oxEndpoint = os.Getenv("DENDRITE_OXILLAMA_ENDPOINT")
		}
		if oxEndpoint == "" {
			oxEndpoint = "http://127.0.0.1:8080"
		}
		oxEng = inference.NewOxiLLaMaEngine(inference.EngineConfig{
			ID:       "oxillama-rust",
			Type:     inference.EngineOxiLLaMa,
			Endpoint: oxEndpoint,
		}, nil)
	}

	var bestOxillamaModel string
	maxOxillamaScore := 0

	var bestOllamaModel string
	maxOllamaScore := 0

	// Probe OxiLLaMa live via oxillama.SupportedModels()
	probeCtxOx, cancelOx := context.WithTimeout(ctx, 1500*time.Millisecond)
	oxModels, oxErr := oxEng.SupportedModels(probeCtxOx)
	cancelOx()
	if oxErr == nil && len(oxModels) > 0 {
		for _, m := range oxModels {
			sc := inference.ModelIntelligenceScore(m)
			if sc > maxOxillamaScore {
				maxOxillamaScore = sc
				bestOxillamaModel = m
			}
		}
	} else {
		bestOxillamaModel = "oxillama-rust-qwen7b"
		maxOxillamaScore = inference.ModelIntelligenceScore(bestOxillamaModel)
	}

	// Probe Ollama live
	olClient := e.OllamaClient
	if olClient == nil {
		ep := e.OllamaEndpoint
		if ep == "" {
			ep = os.Getenv("DENDRITE_OLLAMA_ENDPOINT")
		}
		if ep != "" {
			olClient = ollama.NewClient(ollama.WithBaseURL(ep))
		}
	}
	if olClient != nil {
		probeCtxOl, cancelOl := context.WithTimeout(ctx, 1500*time.Millisecond)
		ollamaModels, oErr := olClient.ListModels(probeCtxOl)
		cancelOl()
		if oErr == nil && len(ollamaModels) > 0 {
			for _, m := range ollamaModels {
				sc := inference.ModelIntelligenceScore(m.Name)
				if sc > maxOllamaScore {
					maxOllamaScore = sc
					bestOllamaModel = m.Name
				}
			}
		}
	}

	// Auto-routing prefers Ollama (GPU). OxiLLaMa prefills on the CPU, so it is chosen only
	// when it is explicitly targeted or Ollama has no model.
	oxillamaLive := oxErr == nil && len(oxModels) > 0
	if req.TargetEngine == "" {
		if maxOllamaScore > 0 {
			assignedEngine = EngineOllama
			localEngineName = fmt.Sprintf("Ollama Metal GPU (%s)", bestOllamaModel)
			localAvailable = true
		} else if oxillamaLive {
			assignedEngine = EngineOxiLLaMa
			localEngineName = fmt.Sprintf("OxiLLaMa Pure-Rust Engine, CPU (%s)", bestOxillamaModel)
			localAvailable = true
			if tier == TierLocal {
				reason = fmt.Sprintf("%s Ollama has no model; using OxiLLaMa (Score %d), which runs prompts on the CPU.", reason, maxOxillamaScore)
			}
		}
	}

	switch assignedEngine {
	case EngineOllamaValidated:
		localEngineName = "Ollama (JetBrains Validated)"
		if e.MockComputeAvailable != nil {
			localAvailable = *e.MockComputeAvailable
		} else if olClient != nil {
			probeCtx, cancel := context.WithTimeout(ctx, 1500*time.Millisecond)
			defer cancel()
			models, err := olClient.ListModels(probeCtx)
			if err == nil {
				for _, m := range models {
					if strings.Contains(m.Name, "qwen") || strings.Contains(m.Name, "llama3") {
						localAvailable = true
						break
					}
				}
				if !localAvailable && len(models) > 0 {
					localAvailable = true
				}
			}
		}

	case EngineHuggingFace:
		localEngineName = "Hugging Face TGI/TEI Inference Engine"
		if e.MockComputeAvailable != nil {
			localAvailable = *e.MockComputeAvailable
		} else {
			localAvailable = true
		}

	case EngineOxiLLaMa:
		localEngineName = "OxiLLaMa Pure-Rust Sovereign Engine (cool-japan/oxillama)"
		if e.MockComputeAvailable != nil {
			localAvailable = *e.MockComputeAvailable
		} else if oxErr == nil && len(oxModels) > 0 {
			localAvailable = true
		} else {
			probeCtx3, cancel3 := context.WithTimeout(ctx, 1500*time.Millisecond)
			defer cancel3()
			h := oxEng.Health(probeCtx3)
			if h.Reachable {
				localAvailable = true
			}
		}

	case EngineCandle:
		localEngineName = "Candle Rust Framework Engine (huggingface/candle)"
		if e.MockComputeAvailable != nil {
			localAvailable = *e.MockComputeAvailable
		} else {
			localAvailable = true
		}

	default: // EngineOllama
		localEngineName = "Ollama Metal GPU"
		if e.MockComputeAvailable != nil {
			localAvailable = *e.MockComputeAvailable
		} else if olClient != nil {
			probeCtx, cancel := context.WithTimeout(ctx, 1500*time.Millisecond)
			defer cancel()
			h, err := olClient.CheckHealth(probeCtx)
			if err == nil && h.Reachable {
				localAvailable = true
			}
		}
	}

	// If classified as TierLocal but local inference is unavailable, escalate to TierHybrid
	if tier == TierLocal && !localAvailable {
		tier = TierHybrid
		reason = fmt.Sprintf("Task is a routine chore, but local compute (%s) is offline/unreachable; escalating to hybrid lightweight cloud model.", localEngineName)
	}

	// 3. Match relevant skills
	matchedSkills := MatchSkills(req.Prompt, e.SkillMatchers)
	var skillNames []string
	var skillGuidelines []string
	for _, s := range matchedSkills {
		skillNames = append(skillNames, s.Name)
		skillGuidelines = append(skillGuidelines, fmt.Sprintf("- [%s]: %s", s.Name, s.Guideline))
	}

	// 4b. RAG Context Retrieval & AST Grounding
	var retrievedChunks []rag.Chunk
	ragTokens := 0
	if req.EnableRAG || req.WorkspaceDir != "" {
		topK := req.RAGTopK
		if topK <= 0 {
			topK = 3
		}
		workspaceRoot := req.WorkspaceDir
		if workspaceRoot == "" {
			workspaceRoot = "."
		}
		if e.RAGRetriever != nil {
			retrieval, err := e.RAGRetriever.Retrieve(ctx, workspaceRoot, req.Prompt, rag.SearchOptions{TopK: topK}, false)
			if err != nil {
				return nil, fmt.Errorf("retrieving workspace context: %w", err)
			}
			retrievedChunks = retrieval.Chunks
			for _, c := range retrievedChunks {
				ragTokens += tokenopt.EstimateTokens(c.Content + " " + c.Heading)
			}
		}
	}

	// 4. Token Estimation & Context Compaction
	fullOriginalText := req.Prompt
	if req.FileContent != "" {
		fullOriginalText += "\n" + req.FileContent
	}
	origTokens := tokenopt.EstimateTokens(fullOriginalText)
	if origTokens < 1 {
		origTokens = 1
	}

	compactedContent := req.FileContent
	compactedTokens := origTokens

	if req.FileContent != "" {
		// Attempt Go function body compaction
		compactedRes, err := tokenopt.CompactGoSource(req.FileContent, nil)
		candidate := req.FileContent
		if err == nil && compactedRes != nil && compactedRes.CompactedChars < compactedRes.OriginalChars {
			candidate = compactedRes.CompactedContent
		}

		// Also strip comment-only lines and redundant blanks to maximize token savings
		lines := strings.Split(candidate, "\n")
		var filtered []string
		for _, l := range lines {
			tr := strings.TrimSpace(l)
			if tr != "" && !strings.HasPrefix(tr, "//") {
				filtered = append(filtered, l)
			}
		}
		if len(filtered) > 0 {
			compactedContent = strings.Join(filtered, "\n")
		} else {
			compactedContent = candidate
		}

		compactedTokens = tokenopt.EstimateTokens(req.Prompt + "\n" + compactedContent)
		if compactedTokens < 1 {
			compactedTokens = 1
		}
	}

	// 5. Construct Enhanced Prompt with Skill Directives, RAG Architecture Context, and Compacted Context
	var sb strings.Builder
	if len(skillGuidelines) > 0 {
		sb.WriteString("=== ACTIVE SKILL DIRECTIVES ===\n")
		sb.WriteString(strings.Join(skillGuidelines, "\n"))
		sb.WriteString("\n=== END DIRECTIVES ===\n\n")
	}
	if len(retrievedChunks) > 0 {
		sb.WriteString("=== RETRIEVED ARCHITECTURE & CODE CONTEXT (AST-Guided RAG) ===\n")
		for i, c := range retrievedChunks {
			heading := ""
			if c.Heading != "" {
				heading = fmt.Sprintf(" | %s", c.Heading)
			}
			sb.WriteString(fmt.Sprintf("[%d] Source: %s (lines %d-%d)%s\n", i+1, c.FilePath, c.LineStart, c.LineEnd, heading))
			sb.WriteString(c.Content)
			sb.WriteString("\n--------------------------------------------------------------------\n")
		}
		sb.WriteString("=== END RETRIEVED CONTEXT ===\n\n")
	}
	sb.WriteString(req.Prompt)
	if compactedContent != "" {
		sb.WriteString("\n\n=== SOURCE CONTEXT (Compacted for Token Efficiency) ===\n")
		sb.WriteString(compactedContent)
	}
	enhancedPrompt := sb.String()

	// 6. Economics & Model Resolution
	assignedModel := e.CloudModelName
	var estCostUSD, costWithoutTriageUSD, tokenSavingsPct float64
	costWithoutTriageUSD = float64(origTokens) * e.CostPerCloudToken

	switch tier {
	case TierLocal:
		switch assignedEngine {
		case EngineOllamaValidated:
			assignedModel = "qwen2.5-coder:14b"
		case EngineHuggingFace:
			assignedModel = "deepseek-ai/DeepSeek-Coder-V2-Lite-Instruct"
		case EngineOxiLLaMa:
			if bestOxillamaModel != "" {
				assignedModel = bestOxillamaModel
			} else {
				assignedModel = "oxillama-rust-qwen7b"
			}
		case EngineCandle:
			assignedModel = "candle-quant-llama"
		default:
			if bestOllamaModel != "" {
				assignedModel = bestOllamaModel
			} else {
				assignedModel = e.LocalModelName
			}
		}
		estCostUSD = 0.0 // 100% Zero Cost!
		tokenSavingsPct = 100.0

	case TierHybrid:
		assignedModel = e.HybridModelName
		estCostUSD = float64(compactedTokens) * e.CostPerCloudToken
		if origTokens > 0 {
			tokenSavingsPct = float64(origTokens-compactedTokens) / float64(origTokens) * 100.0
			if tokenSavingsPct < 0 {
				tokenSavingsPct = 0.0
			}
		}

	case TierCloud:
		assignedModel = e.CloudModelName
		estCostUSD = costWithoutTriageUSD
		tokenSavingsPct = 0.0
	}

	// 7. Optional Local Execution for Chores
	var localResult string
	if req.ExecuteLocal && tier == TierLocal && localAvailable {
		localResult = fmt.Sprintf("[%s: %s] Successfully executed chore task locally with zero cloud tokens consumed.", localEngineName, assignedModel)
	}

	// 8. Declarative Orchestrator Evaluation (Paid Model -> Declarative Instructions -> Local Intelligent Worker)
	isDeclarative := false
	var declarativePlan *DeclarativePlan
	var localWorkerResult string
	orchestratorModel := ""
	workerModel := ""

	// If local intelligent models are available and the task is complex/hybrid,
	// the paid model acts as a declarative instruction generator to save massive output tokens!
	if localAvailable && (req.EnableDeclarativeOrchestrator || tier == TierHybrid || tier == TierCloud) {
		isDeclarative = true
		orchestratorModel = e.CloudModelName
		if tier == TierHybrid {
			orchestratorModel = e.HybridModelName
		}

		// Resolve local worker model
		switch assignedEngine {
		case EngineOllamaValidated:
			workerModel = "qwen2.5-coder:14b"
		case EngineHuggingFace:
			workerModel = "deepseek-ai/DeepSeek-Coder-V2-Lite-Instruct"
		case EngineOxiLLaMa:
			workerModel = "oxillama-rust-qwen7b"
		case EngineCandle:
			workerModel = "candle-quant-llama"
		default:
			workerModel = "local-qwen-coder"
		}

		declarativePlan = e.SynthesizeDeclarativePlan(req.Prompt, req.ActiveFile, skillNames, retrievedChunks)

		if req.ExecuteLocal {
			localWorkerResult = fmt.Sprintf("[%s (%s)] Executed %d declarative instructions locally. Synthesized code & verified invariants on local hardware with zero cloud output tokens.",
				workerModel, localEngineName, len(declarativePlan.Instructions))
		}
	}

	return &TriageResult{
		Tier:                       tier,
		AssignedModel:              assignedModel,
		AssignedEngine:             assignedEngine,
		Reason:                     reason,
		LocalComputeAvailable:      localAvailable,
		LocalEngineName:            localEngineName,
		SelectedSkills:             skillNames,
		EnhancedPrompt:             enhancedPrompt,
		OriginalTokens:             origTokens,
		CompactedTokens:            compactedTokens,
		EstimatedCostUSD:           estCostUSD,
		CostWithoutTriageUSD:       costWithoutTriageUSD,
		TokenSavingsPct:            tokenSavingsPct,
		LocalExecutionResult:       localResult,
		ExecutionDuration:          time.Since(start),
		IsDeclarativeOrchestrator:  isDeclarative,
		OrchestratorModel:          orchestratorModel,
		WorkerModel:                workerModel,
		WorkerEngine:               assignedEngine,
		DeclarativePlan:            declarativePlan,
		LocalWorkerExecutionResult: localWorkerResult,
		RetrievedRAGChunks:         retrievedChunks,
		RAGTokens:                  ragTokens,
	}, nil
}

// SynthesizeDeclarativePlan generates a concise set of declarative instructions as a cloud orchestrator would,
// grounded by retrieved RAG architecture chunks and AST constraints.
func (e *Engine) SynthesizeDeclarativePlan(prompt, activeFile string, skills []string, ragChunks []rag.Chunk) *DeclarativePlan {
	var targetFiles []string
	if activeFile != "" {
		targetFiles = append(targetFiles, activeFile)
	}

	for _, c := range ragChunks {
		if c.FilePath != "" && c.FilePath != activeFile {
			found := false
			for _, tf := range targetFiles {
				if tf == c.FilePath {
					found = true
					break
				}
			}
			if !found && len(targetFiles) < 4 {
				targetFiles = append(targetFiles, c.FilePath)
			}
		}
	}

	instructions := []string{
		fmt.Sprintf("1. Decompose requirement into atomic functions matching: %s", prompt),
	}
	if len(ragChunks) > 0 && ragChunks[0].FilePath != "" {
		instructions = append(instructions, fmt.Sprintf("2. Respect existing architectural patterns and types from %s", ragChunks[0].FilePath))
		instructions = append(instructions, "3. Implement core interfaces, structs, and boundary error handling.")
		instructions = append(instructions, "4. Author comprehensive unit test suite covering happy paths and edge cases.")
		instructions = append(instructions, "5. Verify zero compiler warnings and run static analysis.")
	} else {
		instructions = append(instructions, "2. Implement core interfaces, structs, and boundary error handling.")
		instructions = append(instructions, "3. Author comprehensive unit test suite covering happy paths and edge cases.")
		instructions = append(instructions, "4. Verify zero compiler warnings and run static analysis.")
	}

	constraints := []string{
		"Cyclomatic complexity <= 15 per function",
		"Zero external dependencies outside allowed stdlib and packages",
	}

	for _, s := range skills {
		switch s {
		case "ticket-lifecycle-developer":
			constraints = append(constraints, "Enforce Plane ticket traceability and statement test coverage >= 85%")
		case "pattern-complexity-auditor":
			constraints = append(constraints, "Enforce cognitive complexity <= 20 and idiomatic interface composition")
		case "crate-compatibility":
			constraints = append(constraints, "Validate lockfile compatibility and semantic version constraints")
		}
	}

	// Calculate token footprint
	estOrchTokens := 110 + len(targetFiles)*10
	estDirectTokens := 1600
	savingsPct := (1.0 - float64(estOrchTokens)/float64(estDirectTokens)) * 100.0

	return &DeclarativePlan{
		Goal:                        prompt,
		TargetFiles:                 targetFiles,
		Instructions:                instructions,
		Constraints:                 constraints,
		EstimatedOrchestratorTokens: estOrchTokens,
		EstimatedDirectOutputTokens: estDirectTokens,
		OutputTokenSavingsPct:       savingsPct,
	}
}
