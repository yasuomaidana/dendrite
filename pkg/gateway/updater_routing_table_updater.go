package gateway

import (
	"context"
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
	modelList := cloudModelList(opts)

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
		modelList = append(modelList, localModelEntry(aliasName, modelID, engType, normEP, tokens))
		fallbackModelNames = append(fallbackModelNames, aliasName)
	}

	// 1. Roles are assigned only among installed chat models that fit this machine's memory.
	//    discovered is ordered smartest first; slower is acceptable, starving the machine is not.
	chat, embed := splitFitting(discovered)
	role := func(aliases []string, dm DiscoveredModel, tokens int) {
		for _, a := range aliases {
			addLocalEntry(a, dm.ModelName, dm.EngineType, dm.Endpoint, tokens)
		}
	}
	if len(chat) > 0 {
		// reason: the smartest model that fits (the old local-qwen3-27b slot).
		role([]string{"local-reason", "local-qwen3-27b"}, chat[0], 8192)
		// coder: the smartest coder-tuned model, else the smartest overall.
		role([]string{"local-coder", "local-qwen-coder"}, pickCoder(chat), 8192)
		// fast: the smallest model that is still capable (score >= 50), for short, frequent calls.
		role([]string{"local-fast", "local-qwen3-9b"}, pickFast(chat), 4096)
	}
	if len(embed) > 0 {
		role([]string{"local-embed"}, embed[0], 2048)
	}
	// Explicit per-model aliases, for the models that fit only.
	for _, dm := range chat {
		addLocalEntry(modelAlias(dm), dm.ModelName, dm.EngineType, dm.Endpoint, 4096)
	}

	routerSettings := buildRouterSettings(opts, fallbackModelNames)

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

// splitFitting separates the discovered models that fit this machine's memory into chat and embedding models,
// keeping their order (smartest first). A model that does not fit is dropped.
func splitFitting(discovered []DiscoveredModel) (chat, embed []DiscoveredModel) {
	for _, dm := range discovered {
		switch {
		case !dm.Fits:
		case dm.Embed:
			embed = append(embed, dm)
		default:
			chat = append(chat, dm)
		}
	}
	return chat, embed
}

// pickCoder is the smartest coder-tuned chat model, else the smartest overall. chat must not be empty.
func pickCoder(chat []DiscoveredModel) DiscoveredModel {
	for _, dm := range chat {
		if strings.Contains(strings.ToLower(dm.ModelName), "coder") {
			return dm
		}
	}
	return chat[0]
}

// pickFast is the smallest chat model that is still capable (score >= 50), else the smallest. chat is ordered
// smartest first and must not be empty.
func pickFast(chat []DiscoveredModel) DiscoveredModel {
	for i := len(chat) - 1; i >= 0; i-- {
		if chat[i].IntelligenceScore >= 50 {
			return chat[i]
		}
	}
	return chat[len(chat)-1]
}

// modelAlias is the explicit per-model alias of a chat model, e.g. local-qwen3-8b for qwen3:8b.
func modelAlias(dm DiscoveredModel) string {
	clean := strings.ToLower(dm.ModelName)
	return "local-" + strings.ReplaceAll(strings.ReplaceAll(clean, ":", "-"), "/", "-")
}

// cloudModelList is the commercial part of the model list: the primary, backup and fast cloud models.
func cloudModelList(opts UpdaterOptions) []ModelListEntry {
	return []ModelListEntry{
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
}

// localModelEntry routes alias to modelID on a self-hosted engine reachable at normEP.
func localModelEntry(aliasName, modelID, engType, normEP string, tokens int) ModelListEntry {
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

	return ModelListEntry{
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
}

// buildRouterSettings orders the local aliases by intelligence and builds the cascade of fallbacks between the cloud
// models and the local ones.
func buildRouterSettings(opts UpdaterOptions, fallbackModelNames []string) *RouterSettings {
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
		// The local aliases are added below, only for the ones that were emitted into model_list.
		ContextWindowFallbacks: []map[string]string{
			{opts.FastCloudModel: opts.PrimaryCloudModel},
		},
	}

	addContextWindowFallbacks(routerSettings, fallbackModelNames, opts.PrimaryCloudModel)
	return routerSettings
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
	// Each run owns its stop channel and its stop function: the goroutine of an earlier run can only ever stop its
	// own run, never one started after it.
	stop := make(chan struct{})
	u.stopCh = stop
	var stopOnce sync.Once
	stopFunc := func() {
		stopOnce.Do(func() {
			u.mu.Lock()
			if u.stopCh == stop {
				u.running = false
			}
			u.mu.Unlock()
			close(stop)
		})
	}
	u.stopFn = stopFunc
	interval := u.opts.Interval
	if interval <= 0 {
		interval = DefaultUpdateInterval
	}
	u.mu.Unlock()

	outCh := make(chan *UpdateReport, 10)

	go u.runDaemon(ctx, stop, stopFunc, interval, outCh)

	return outCh, stopFunc
}

// runDaemon is the body of one daemon run: an update at once, then one per tick, until ctx ends or stop closes.
// Reports go to outCh without blocking; outCh is closed and the run stopped when it returns.
func (u *RoutingTableUpdater) runDaemon(ctx context.Context, stop <-chan struct{}, stopRun func(), interval time.Duration, outCh chan<- *UpdateReport) {
	defer close(outCh)
	defer stopRun()

	publish := func(rep *UpdateReport) {
		if rep == nil {
			return
		}
		select {
		case outCh <- rep:
		default:
		}
	}
	if rep, err := u.UpdateNow(ctx); err == nil {
		publish(rep)
	}

	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-stop:
			return
		case <-ticker.C:
			rep, _ := u.UpdateNow(ctx)
			publish(rep)
		}
	}
}

// Stop terminates a running background daemon.
func (u *RoutingTableUpdater) Stop() {
	u.mu.Lock()
	stop := u.stopFn
	u.mu.Unlock()
	if stop != nil {
		stop()
	}
}
