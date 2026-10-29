package gateway

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"sync"
	"time"

	"Dendrite/pkg/inference"
)

// buildDynamicConfig generates a complete LiteLLMConfig reflecting live model precedence.
func (u *RoutingTableUpdater) buildDynamicConfig(
	opts UpdaterOptions,
	engines []inference.InferenceEngine,
	discovered []DiscoveredModel,
) *LiteLLMConfig {
	// Base commercial models
	modelList := []ModelListEntry{
		{
			ModelName: opts.PrimaryCloudModel,
			LitellmParams: ModelParams{
				Model:     "gemini/" + opts.PrimaryCloudModel,
				APIKey:    "os.environ/GEMINI_API_KEY",
				MaxTokens: 8192,
			},
			ModelInfo: &ModelInfo{
				ID:                 opts.PrimaryCloudModel,
				Mode:               "chat",
				InputCostPerToken:  0.00000125,
				OutputCostPerToken: 0.000005,
				MaxInputTokens:     1048576,
				MaxOutputTokens:    8192,
				BaseModel:          "gemini/" + opts.PrimaryCloudModel,
			},
		},
		{
			ModelName: opts.BackupCloudModel,
			LitellmParams: ModelParams{
				Model:      "github_copilot/" + CopilotModel(),
				APIKey:     "os.environ/COPILOT_API_KEY",
				DropParams: true,
				ExtraHeaders: map[string]string{
					"Copilot-Integration-Id": "vscode-chat",
					"Editor-Version":         "vscode/1.95.0",
				},
				MaxTokens: 8192,
			},
			ModelInfo: &ModelInfo{
				ID:                 opts.BackupCloudModel,
				Mode:               "chat",
				InputCostPerToken:  0.000003,
				OutputCostPerToken: 0.000015,
				MaxInputTokens:     200000,
				MaxOutputTokens:    8192,
				BaseModel:          CopilotModel(),
			},
		},
		{
			ModelName: opts.FastCloudModel,
			LitellmParams: ModelParams{
				Model:     "gemini/" + opts.FastCloudModel,
				APIKey:    "os.environ/GEMINI_API_KEY",
				MaxTokens: 8192,
			},
			ModelInfo: &ModelInfo{
				ID:                 opts.FastCloudModel,
				Mode:               "chat",
				InputCostPerToken:  0.000000075,
				OutputCostPerToken: 0.0000003,
				MaxInputTokens:     1048576,
				MaxOutputTokens:    8192,
				BaseModel:          "gemini/" + opts.FastCloudModel,
			},
		},
	}

	normalizeEndpoint := func(ep string) string {
		ep = strings.TrimRight(ep, "/")
		if opts.UseContainerHost {
			ep = strings.ReplaceAll(ep, "127.0.0.1", "host.docker.internal")
			ep = strings.ReplaceAll(ep, "localhost", "host.docker.internal")
		}
		return ep
	}

	var fallbackModelNames []string
	registeredAliases := make(map[string]bool)

	// Helper to add a local model entry
	addLocalEntry := func(aliasName string, modelID string, engType string, endpoint string, tokens int) {
		if registeredAliases[aliasName] {
			return
		}
		registeredAliases[aliasName] = true

		normEP := normalizeEndpoint(endpoint)
		var upstreamModel, apiBase, apiKey, provider string

		switch engType {
		case string(inference.EngineOxiLLaMa):
			upstreamModel = "openai/" + modelID
			apiBase = normEP + "/v1"
			apiKey = "EMPTY"
			provider = "openai"

		case string(inference.EngineOllama):
			upstreamModel = "ollama/" + modelID
			apiBase = normEP
			provider = "ollama"

		case string(inference.EngineVLLM):
			upstreamModel = "openai/" + modelID
			apiBase = normEP + "/v1"
			apiKey = "EMPTY"
			provider = "openai"

		case string(inference.EngineLlamaCpp):
			upstreamModel = "openai/" + modelID
			apiBase = normEP + "/v1"
			apiKey = "EMPTY"
			provider = "openai"

		default:
			upstreamModel = "openai/" + modelID
			apiBase = normEP + "/v1"
			apiKey = "EMPTY"
		}

		if tokens <= 0 {
			tokens = 4096
		}

		entry := ModelListEntry{
			ModelName: aliasName,
			LitellmParams: ModelParams{
				Model:             upstreamModel,
				APIBase:           apiBase,
				APIKey:            apiKey,
				CustomLLMProvider: provider,
				MaxTokens:         tokens,
			},
			ModelInfo: &ModelInfo{
				ID:                 aliasName,
				Mode:               "chat",
				InputCostPerToken:  0.0,
				OutputCostPerToken: 0.0,
				MaxInputTokens:     32768,
				MaxOutputTokens:    tokens,
				BaseModel:          modelID,
				SkipBudgetChecks:   true,
			},
		}

		modelList = append(modelList, entry)
		fallbackModelNames = append(fallbackModelNames, aliasName)
	}

	// 1. Roles are assigned only among installed chat models that fit this machine's memory.
	//    discovered is ordered smartest first; slower is acceptable, starving the machine is not.
	var chat, embed []DiscoveredModel
	for _, dm := range discovered {
		switch {
		case !dm.Fits:
		case dm.Embed:
			embed = append(embed, dm)
		default:
			chat = append(chat, dm)
		}
	}
	role := func(aliases []string, dm DiscoveredModel, tokens int) {
		for _, a := range aliases {
			addLocalEntry(a, dm.ModelName, dm.EngineType, dm.Endpoint, tokens)
		}
	}
	if len(chat) > 0 {
		// reason: the smartest model that fits (the old local-qwen3-27b slot).
		role([]string{"local-reason", "local-qwen3-27b"}, chat[0], 8192)
		// coder: the smartest coder-tuned model, else the smartest overall.
		coder := chat[0]
		for _, dm := range chat {
			if strings.Contains(strings.ToLower(dm.ModelName), "coder") {
				coder = dm
				break
			}
		}
		role([]string{"local-coder", "local-qwen-coder"}, coder, 8192)
		// fast: the smallest model that is still capable (score >= 50), for short, frequent calls.
		fast := chat[len(chat)-1]
		for i := len(chat) - 1; i >= 0; i-- {
			if chat[i].IntelligenceScore >= 50 {
				fast = chat[i]
				break
			}
		}
		role([]string{"local-fast", "local-qwen3-9b"}, fast, 4096)
	}
	if len(embed) > 0 {
		role([]string{"local-embed"}, embed[0], 2048)
	}
	// Explicit per-model aliases, for the models that fit only.
	for _, dm := range chat {
		clean := strings.ToLower(dm.ModelName)
		addLocalEntry(fmt.Sprintf("local-%s", strings.ReplaceAll(strings.ReplaceAll(clean, ":", "-"), "/", "-")),
			dm.ModelName, dm.EngineType, dm.Endpoint, 4096)
	}

	// Dynamic cascade fallbacks
	// Reorder fallbackModelNames by ModelIntelligenceScore
	sort.SliceStable(fallbackModelNames, func(i, j int) bool {
		return inference.ModelIntelligenceScore(fallbackModelNames[i]) > inference.ModelIntelligenceScore(fallbackModelNames[j])
	})

	proFallbacks := append([]string{opts.BackupCloudModel, opts.FastCloudModel}, fallbackModelNames...)
	sonnetFallbacks := append([]string{opts.FastCloudModel}, fallbackModelNames...)
	flashFallbacks := append([]string(nil), fallbackModelNames...)

	routerSettings := &RouterSettings{
		RoutingStrategy:     "usage-based-routing",
		Timeout:             120,
		EnablePreCallChecks: true,
		// LiteLLM requires one model per fallback entry.
		Fallbacks: []map[string][]string{
			{opts.PrimaryCloudModel: proFallbacks},
			{opts.BackupCloudModel: sonnetFallbacks},
			{opts.FastCloudModel: flashFallbacks},
		},
		ContextWindowFallbacks: []map[string]string{
			{"local-qwen-coder": opts.PrimaryCloudModel},
			{"local-qwen3-27b": opts.PrimaryCloudModel},
			{"local-qwen3-9b": opts.PrimaryCloudModel},
			{opts.FastCloudModel: opts.PrimaryCloudModel},
		},
	}

	addContextWindowFallbacks(routerSettings, fallbackModelNames, opts.PrimaryCloudModel)

	generalSettings := &GeneralSettings{
		MasterKey:   "os.environ/LITELLM_MASTER_KEY",
		DatabaseURL: "os.environ/DATABASE_URL",
		RedisURL:    "os.environ/REDIS_URL",
	}

	litellmSettings := &LitellmSettings{
		DropParams:     true,
		BudgetDuration: "7d",
	}

	return &LiteLLMConfig{
		ModelList:       modelList,
		RouterSettings:  routerSettings,
		GeneralSettings: generalSettings,
		LitellmSettings: litellmSettings,
	}
}

// StartDailyDaemon starts a background ticker executing routing table updates
// at the specified interval (default: 24h). It returns a channel emitting update
// reports and a cancel function.
func (u *RoutingTableUpdater) StartDailyDaemon(ctx context.Context) (<-chan *UpdateReport, func()) {
	u.mu.Lock()
	if u.running {
		u.mu.Unlock()
		ch := make(chan *UpdateReport)
		close(ch)
		return ch, func() {}
	}
	u.running = true
	u.stopCh = make(chan struct{})
	interval := u.opts.Interval
	if interval <= 0 {
		interval = DefaultUpdateInterval
	}
	u.mu.Unlock()

	outCh := make(chan *UpdateReport, 10)
	stopOnce := sync.Once{}

	stopFunc := func() {
		stopOnce.Do(func() {
			u.mu.Lock()
			if u.running {
				close(u.stopCh)
				u.running = false
			}
			u.mu.Unlock()
		})
	}

	go func() {
		defer close(outCh)
		defer stopFunc()

		// Run immediate first update on start
		if rep, err := u.UpdateNow(ctx); err == nil {
			select {
			case outCh <- rep:
			default:
			}
		}

		ticker := time.NewTicker(interval)
		defer ticker.Stop()

		for {
			select {
			case <-ctx.Done():
				return
			case <-u.stopCh:
				return
			case <-ticker.C:
				rep, _ := u.UpdateNow(ctx)
				if rep != nil {
					select {
					case outCh <- rep:
					default:
					}
				}
			}
		}
	}()

	return outCh, stopFunc
}

// Stop terminates a running background daemon.
func (u *RoutingTableUpdater) Stop() {
	u.mu.Lock()
	defer u.mu.Unlock()
	if u.running {
		close(u.stopCh)
		u.running = false
	}
}
