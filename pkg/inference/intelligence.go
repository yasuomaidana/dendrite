package inference

import (
	"context"
	"fmt"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"sync"
)

var (
	paramRegex = regexp.MustCompile(`(?i)(\d+(?:\.\d+)?)\s*b\b`)
)

// ModelIntelligenceScore computes a deterministic heuristic intelligence score (1..100)
// based on parameter scale, reasoning capability, coding specialization, and model generation.
func ModelIntelligenceScore(modelName string) int {
	clean := strings.ToLower(strings.TrimSpace(modelName))
	if clean == "" {
		return 10
	}

	// Remove common prefixes
	clean = strings.TrimPrefix(clean, "ollama/")
	clean = strings.TrimPrefix(clean, "oxillama/")
	clean = strings.TrimPrefix(clean, "openai/")
	clean = strings.TrimPrefix(clean, "local-")

	var paramB float64 = 0.0

	// Check explicit parameter tags like 27b, 14b, 7b, 3.8:27b, etc.
	matches := paramRegex.FindAllStringSubmatch(clean, -1)
	if len(matches) > 0 {
		lastMatch := matches[len(matches)-1]
		if len(lastMatch) > 1 {
			if v, err := strconv.ParseFloat(lastMatch[1], 64); err == nil {
				paramB = v
			}
		}
	}

	// Specific fallback checks for known identifiers without explicit 'b' regex match
	if paramB == 0.0 {
		switch {
		case strings.Contains(clean, "27b"):
			paramB = 27.0
		case strings.Contains(clean, "32b"):
			paramB = 32.0
		case strings.Contains(clean, "14b"):
			paramB = 14.0
		case strings.Contains(clean, "9b"):
			paramB = 9.0
		case strings.Contains(clean, "8b"):
			paramB = 8.0
		case strings.Contains(clean, "7b"):
			paramB = 7.0
		case strings.Contains(clean, "6.7b"):
			paramB = 6.7
		case strings.Contains(clean, "3b") || strings.Contains(clean, "3.2"):
			paramB = 3.0
		case strings.Contains(clean, "1.5b"):
			paramB = 1.5
		}
	}

	// Base score by parameter tier
	baseScore := 50
	switch {
	case paramB >= 30.0:
		baseScore = 92
	case paramB >= 25.0: // e.g. 27B
		baseScore = 88
	case paramB >= 13.0: // e.g. 14B
		baseScore = 78
	case paramB >= 8.0: // e.g. 9B, 8B
		baseScore = 65
	case paramB >= 6.5: // e.g. 7B, 6.7B
		baseScore = 60
	case paramB >= 3.0: // e.g. 3B
		baseScore = 38
	case paramB > 0.0 && paramB < 3.0: // e.g. 1.5B, 1B
		baseScore = 25
	default:
		// Parameter unknown: estimate from family names
		if strings.Contains(clean, "coder") || strings.Contains(clean, "instruct") {
			baseScore = 60
		} else {
			baseScore = 45
		}
	}

	// Architecture & Specialization Modifiers
	modifier := 0
	if strings.Contains(clean, "qwen3") || strings.Contains(clean, "qwen-3") {
		modifier += 8 // Latest generation reasoning
	} else if strings.Contains(clean, "qwen2.5") || strings.Contains(clean, "deepseek") {
		modifier += 5
	}

	if strings.Contains(clean, "coder") {
		modifier += 5 // Highly tuned for developer / coding tasks
	}

	if strings.Contains(clean, "mistral") || strings.Contains(clean, "hermes") {
		modifier += 2
	}

	score := baseScore + modifier
	if score > 100 {
		score = 100
	}
	if score < 1 {
		score = 1
	}

	return score
}

// EngineMaxIntelligence evaluates the maximum model intelligence score available on an engine.
func EngineMaxIntelligence(ctx context.Context, eng InferenceEngine) (int, string) {
	if eng == nil {
		return 0, ""
	}

	// 1. Try SupportedModels
	models, err := eng.SupportedModels(ctx)
	if err != nil || len(models) == 0 {
		models = eng.Config().Models
	}

	maxScore := 0
	bestModel := ""
	for _, m := range models {
		score := ModelIntelligenceScore(m)
		if score > maxScore {
			maxScore = score
			bestModel = m
		}
	}

	return maxScore, bestModel
}

// ReorderEnginesByIntelligence orders engines dynamically: live engines before offline ones,
// GPU-accelerated engines before OxiLLaMa (CPU prefill, see GPUAccelerated), then the
// highest ModelIntelligenceScore, then EnginePreference.
func ReorderEnginesByIntelligence(ctx context.Context, engines []InferenceEngine) []InferenceEngine {
	type scoredEngine struct {
		eng       InferenceEngine
		maxScore  int
		bestModel string
		isLive    bool
	}

	scored := make([]scoredEngine, len(engines))
	var wg sync.WaitGroup

	for i, e := range engines {
		wg.Add(1)
		go func(idx int, eng InferenceEngine) {
			defer wg.Done()
			h := eng.Health(ctx)
			isLive := h.Reachable
			score, model := EngineMaxIntelligence(ctx, eng)
			scored[idx] = scoredEngine{
				eng:       eng,
				maxScore:  score,
				bestModel: model,
				isLive:    isLive,
			}
		}(i, e)
	}
	wg.Wait()

	sort.SliceStable(scored, func(i, j int) bool {
		if scored[i].isLive != scored[j].isLive {
			return scored[i].isLive
		}
		gi, gj := GPUAccelerated(scored[i].eng.Type()), GPUAccelerated(scored[j].eng.Type())
		if gi != gj {
			return gi
		}
		if scored[i].maxScore != scored[j].maxScore {
			return scored[i].maxScore > scored[j].maxScore
		}
		return EnginePreference(scored[i].eng.Type()) < EnginePreference(scored[j].eng.Type())
	})

	ordered := make([]InferenceEngine, len(scored))
	for i, se := range scored {
		ordered[i] = se.eng
	}
	return ordered
}

// SmartCascadeConfig governs failure counting and paid escalation rules.
type SmartCascadeConfig struct {
	MaxSmartFailures  int    // Number of failures on the smartest model before leveraging paid (default: 2, max: 3)
	PaidFallbackModel string // Paid cloud model for final escalation (e.g. "gemini-2.5-pro", "copilot-sonnet")
	SmartScoreFloor   int    // Threshold to classify a model as "smart" (0 = auto-calculated from available max)
}

// DefaultSmartCascadeConfig returns standard cascade policies.
func DefaultSmartCascadeConfig() SmartCascadeConfig {
	return SmartCascadeConfig{
		MaxSmartFailures:  2,
		PaidFallbackModel: "gemini-2.5-pro",
		SmartScoreFloor:   0,
	}
}

// SmartCascadeReport documents the cascade execution progression and failure counting.
type SmartCascadeReport struct {
	Attempts             int      `json:"attempts"`
	TrialFailuresIgnored int      `json:"trial_failures_ignored"` // Smaller model failures not counted against limit
	SmartFailuresCounted int      `json:"smart_failures_counted"` // Failures on top-tier smart models
	MaxSmartThreshold    int      `json:"max_smart_threshold"`
	LeveragedToPaid      bool     `json:"leveraged_to_paid"`
	SelectedEngine       string   `json:"selected_engine"`
	SelectedModel        string   `json:"selected_model"`
	ModelIntelligence    int      `json:"model_intelligence"`
	ProgressionLog       []string `json:"progression_log"`
}

// ExecuteSmartCascade dispatches completion requests honoring:
// 1. GPU-accelerated engines run before OxiLLaMa; among them, smarter local models have precedence over smaller ones.
// 2. Failures on smaller / less-smart models do NOT count toward failure limit.
// 3. Failures are ONLY counted when executing on the smartest available local model(s).
// 4. Escalation to paid models occurs ONLY if the smartest models fail 2 or 3 times.
func ExecuteSmartCascade(
	ctx context.Context,
	engines []InferenceEngine,
	req CompletionRequest,
	paidCaller func(ctx context.Context, req CompletionRequest, paidModel string) (*CompletionResponse, error),
	cfg SmartCascadeConfig,
) (*CompletionResponse, *SmartCascadeReport, error) {
	if cfg.MaxSmartFailures <= 0 {
		cfg.MaxSmartFailures = 2
	}
	if cfg.MaxSmartFailures > 3 {
		cfg.MaxSmartFailures = 3
	}
	if cfg.PaidFallbackModel == "" {
		cfg.PaidFallbackModel = "gemini-2.5-pro"
	}

	report := &SmartCascadeReport{
		MaxSmartThreshold: cfg.MaxSmartFailures,
		ProgressionLog:    make([]string, 0),
	}

	// 1. Discover all candidate local models across registered healthy engines
	type candidate struct {
		engine InferenceEngine
		model  string
		score  int
	}

	var candidates []candidate
	maxAvailableScore := 0

	for _, eng := range engines {
		if !eng.IsSelfHosted() {
			continue
		}
		models, err := eng.SupportedModels(ctx)
		if err != nil || len(models) == 0 {
			models = eng.Config().Models
		}
		for _, m := range models {
			score := ModelIntelligenceScore(m)
			if score > maxAvailableScore {
				maxAvailableScore = score
			}
			candidates = append(candidates, candidate{
				engine: eng,
				model:  m,
				score:  score,
			})
		}
	}

	// GPU-accelerated engines first (OxiLLaMa prefills on the CPU), then smartest first,
	// then the preferred engine.
	sort.SliceStable(candidates, func(i, j int) bool {
		gi, gj := GPUAccelerated(candidates[i].engine.Type()), GPUAccelerated(candidates[j].engine.Type())
		if gi != gj {
			return gi
		}
		if candidates[i].score != candidates[j].score {
			return candidates[i].score > candidates[j].score
		}
		return EnginePreference(candidates[i].engine.Type()) < EnginePreference(candidates[j].engine.Type())
	})

	// Threshold for what counts as "smartest available"
	smartThreshold := cfg.SmartScoreFloor
	if smartThreshold <= 0 {
		// Models within 15 points of the maximum available score are considered top-tier smart
		smartThreshold = maxAvailableScore - 15
		if smartThreshold < 50 {
			smartThreshold = maxAvailableScore
		}
	}

	var lastErr error

	// 2. Execute across candidates
	for _, cand := range candidates {
		report.Attempts++
		isSmart := cand.score >= smartThreshold

		reqCopy := req
		reqCopy.Model = cand.model

		resp, err := cand.engine.Generate(ctx, reqCopy)
		if err == nil && resp != nil {
			report.SelectedEngine = cand.engine.ID()
			report.SelectedModel = cand.model
			report.ModelIntelligence = cand.score
			report.ProgressionLog = append(report.ProgressionLog,
				fmt.Sprintf("SUCCESS: Engine '%s' using model '%s' (Score %d).", cand.engine.ID(), cand.model, cand.score))
			return resp, report, nil
		}

		lastErr = err
		if !isSmart {
			// Smaller / less smart model failure: DO NOT COUNT against threshold
			report.TrialFailuresIgnored++
			report.ProgressionLog = append(report.ProgressionLog,
				fmt.Sprintf("TRIAL FAILURE IGNORED: Smaller model '%s' on '%s' (Score %d < threshold %d) failed (%v). Not counted against failure threshold.",
					cand.model, cand.engine.ID(), cand.score, smartThreshold, err))
		} else {
			// Smartest model failure: Count against threshold
			report.SmartFailuresCounted++
			report.ProgressionLog = append(report.ProgressionLog,
				fmt.Sprintf("SMART FAILURE COUNTED: Top-tier smart model '%s' on '%s' (Score %d) failed (%v) [Failure %d/%d].",
					cand.model, cand.engine.ID(), cand.score, err, report.SmartFailuresCounted, cfg.MaxSmartFailures))

			if report.SmartFailuresCounted >= cfg.MaxSmartFailures {
				report.ProgressionLog = append(report.ProgressionLog,
					fmt.Sprintf("ESCALATION TRIGGERED: Smartest models failed %d times (threshold reached). Leveraging paid model '%s'.",
						report.SmartFailuresCounted, cfg.PaidFallbackModel))
				break
			}
		}
	}

	// 3. If smartest models failed >= MaxSmartFailures (or no local models available), leverage to paid
	if paidCaller != nil {
		report.LeveragedToPaid = true
		report.SelectedEngine = "paid-cloud-orchestrator"
		report.SelectedModel = cfg.PaidFallbackModel
		report.ModelIntelligence = 100

		paidResp, paidErr := paidCaller(ctx, req, cfg.PaidFallbackModel)
		if paidErr == nil && paidResp != nil {
			report.ProgressionLog = append(report.ProgressionLog,
				fmt.Sprintf("PAID LEVERAGE SUCCESS: Invoked paid cloud model '%s' successfully.", cfg.PaidFallbackModel))
			return paidResp, report, nil
		}
		return nil, report, fmt.Errorf("local smart cascade exhausted and paid fallback failed: %w", paidErr)
	}

	return nil, report, fmt.Errorf("%w: last error: %v (smart failures: %d, ignored trials: %d)",
		ErrNoAvailableEngine, lastErr, report.SmartFailuresCounted, report.TrialFailuresIgnored)
}
