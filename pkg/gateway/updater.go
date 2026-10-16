package gateway

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"Dendrite/pkg/inference"
)

// Default paths and intervals for LiteLLM routing table updater.
const (
	DefaultConfigPath     = "stacks/litellm_config.yaml"
	DefaultUpdateInterval = 24 * time.Hour
)

// DiscoveredModel tracks a model discovered on a live inference engine.
type DiscoveredModel struct {
	ModelName         string `json:"model_name"`
	EngineID          string `json:"engine_id"`
	EngineType        string `json:"engine_type"`
	IntelligenceScore int    `json:"intelligence_score"`
	Endpoint          string `json:"endpoint"`
	ZeroCost          bool   `json:"zero_cost"`
}

// UpdaterOptions configures the LiteLLM routing table updater.
type UpdaterOptions struct {
	ConfigPath        string             // Target YAML path (default: stacks/litellm_config.yaml)
	Interval          time.Duration      // Scheduled update interval (default: 24h)
	UseContainerHost  bool               // Rewrite localhost/127.0.0.1 to host.docker.internal
	DryRun            bool               // Return generated config without writing to disk
	Manager           *inference.Manager // Source inference manager
	OllamaEndpoint    string             // Custom Ollama endpoint override
	OxillamaEndpoint  string             // Custom OxiLLaMa endpoint override
	PrimaryCloudModel string             // Primary commercial model (default: gemini-2.5-pro)
	BackupCloudModel  string             // Secondary commercial model (default: copilot-sonnet)
	FastCloudModel    string             // Fast commercial model (default: gemini-2.5-flash)
	Logger            func(format string, args ...interface{})
}

// DefaultUpdaterOptions initializes options from environment variables with sensible defaults.
func DefaultUpdaterOptions() UpdaterOptions {
	cfgPath := os.Getenv("DENDRITE_LITELLM_CONFIG_PATH")
	if strings.TrimSpace(cfgPath) == "" {
		cfgPath = DefaultConfigPath
	}

	interval := DefaultUpdateInterval
	if rawInterval := os.Getenv("DENDRITE_ROUTING_UPDATE_INTERVAL"); rawInterval != "" {
		if d, err := time.ParseDuration(rawInterval); err == nil && d > 0 {
			interval = d
		}
	}

	useContainerHost := false
	if rawHost := os.Getenv("DENDRITE_ROUTING_CONTAINER_HOSTS"); rawHost != "" {
		if b, err := strconv.ParseBool(rawHost); err == nil {
			useContainerHost = b
		}
	} else if strings.Contains(cfgPath, "stacks/") {
		// By default, if writing to stacks/litellm_config.yaml, it targets docker-compose
		useContainerHost = true
	}

	ollamaEP := os.Getenv("DENDRITE_OLLAMA_ENDPOINT")
	if ollamaEP == "" {
		ollamaEP = os.Getenv("OLLAMA_HOST")
	}

	oxillamaEP := os.Getenv("DENDRITE_OXILLAMA_ENDPOINT")
	if oxillamaEP == "" {
		oxillamaEP = os.Getenv("OXILLAMA_HOST")
	}

	return UpdaterOptions{
		ConfigPath:        cfgPath,
		Interval:          interval,
		UseContainerHost:  useContainerHost,
		DryRun:            false,
		OllamaEndpoint:    ollamaEP,
		OxillamaEndpoint:  oxillamaEP,
		PrimaryCloudModel: "gemini-2.5-pro",
		BackupCloudModel:  "copilot-sonnet",
		FastCloudModel:    "gemini-2.5-flash",
	}
}

// UpdateReport details the results of a routing table update run.
type UpdateReport struct {
	Timestamp                time.Time         `json:"timestamp"`
	ConfigPath               string            `json:"config_path"`
	EnginesProbed            int               `json:"engines_probed"`
	OnlineEngines            []string          `json:"online_engines"`
	OfflineEngines           []string          `json:"offline_engines"`
	DiscoveredModels         []DiscoveredModel `json:"discovered_models"`
	ActiveLocalPrimary       string            `json:"active_local_primary"`
	ActiveLocalPrimaryScore  int               `json:"active_local_primary_score"`
	ActiveLocalPrimaryEngine string            `json:"active_local_primary_engine"`
	TotalModelsConfigured    int               `json:"total_models_configured"`
	BytesWritten             int               `json:"bytes_written"`
	DryRun                   bool              `json:"dry_run"`
	Duration                 time.Duration     `json:"duration"`
	YAMLContent              string            `json:"yaml_content,omitempty"`
	Error                    string            `json:"error,omitempty"`
}

// RoutingTableUpdater manages dynamic discovery, intelligence-based prioritization,
// and automated synchronization of the LiteLLM routing configuration.
type RoutingTableUpdater struct {
	mu      sync.RWMutex
	opts    UpdaterOptions
	mgr     *inference.Manager
	client  *http.Client
	stopCh  chan struct{}
	running bool
}

// NewRoutingTableUpdater constructs an updater instance.
func NewRoutingTableUpdater(opts UpdaterOptions) *RoutingTableUpdater {
	defaults := DefaultUpdaterOptions()
	if opts.ConfigPath == "" {
		opts.ConfigPath = defaults.ConfigPath
	}
	if opts.Interval <= 0 {
		opts.Interval = defaults.Interval
	}
	if opts.PrimaryCloudModel == "" {
		opts.PrimaryCloudModel = defaults.PrimaryCloudModel
	}
	if opts.BackupCloudModel == "" {
		opts.BackupCloudModel = defaults.BackupCloudModel
	}
	if opts.FastCloudModel == "" {
		opts.FastCloudModel = defaults.FastCloudModel
	}

	mgr := opts.Manager
	if mgr == nil {
		mgr = inference.DefaultManager()
	}

	return &RoutingTableUpdater{
		opts:   opts,
		mgr:    mgr,
		client: &http.Client{Timeout: 5 * time.Second},
		stopCh: make(chan struct{}),
	}
}

// Options returns a copy of active options.
func (u *RoutingTableUpdater) Options() UpdaterOptions {
	u.mu.RLock()
	defer u.mu.RUnlock()
	return u.opts
}

// SetOptions dynamically reconfigures the updater.
func (u *RoutingTableUpdater) SetOptions(opts UpdaterOptions) {
	u.mu.Lock()
	defer u.mu.Unlock()
	u.opts = opts
}

// UpdateNow executes an immediate probe across registered engines, computes dynamic
// model intelligence rankings, constructs an updated LiteLLM config, and applies it to disk.
func (u *RoutingTableUpdater) UpdateNow(ctx context.Context) (*UpdateReport, error) {
	start := time.Now()

	u.mu.RLock()
	opts := u.opts
	mgr := u.mgr
	u.mu.RUnlock()

	report := &UpdateReport{
		Timestamp:        start,
		ConfigPath:       opts.ConfigPath,
		OnlineEngines:    make([]string, 0),
		OfflineEngines:   make([]string, 0),
		DiscoveredModels: make([]DiscoveredModel, 0),
		DryRun:           opts.DryRun,
	}

	engines := mgr.ListEngines()
	report.EnginesProbed = len(engines)

	// 1. Probe all engines concurrently to discover live healthy models
	type engineResult struct {
		id        string
		engType   inference.EngineType
		reachable bool
		endpoint  string
		models    []string
		err       error
	}

	resCh := make(chan engineResult, len(engines))
	var wg sync.WaitGroup

	for _, eng := range engines {
		wg.Add(1)
		go func(e inference.InferenceEngine) {
			defer wg.Done()
			pollCtx, cancel := context.WithTimeout(ctx, 3*time.Second)
			defer cancel()

			h := e.Health(pollCtx)
			var models []string
			var mErr error
			if h.Reachable {
				models, mErr = e.SupportedModels(pollCtx)
				if mErr != nil || len(models) == 0 {
					models = e.Config().Models
				}
			} else {
				models = e.Config().Models
			}

			resCh <- engineResult{
				id:        e.ID(),
				engType:   e.Type(),
				reachable: h.Reachable,
				endpoint:  e.Config().Endpoint,
				models:    models,
				err:       mErr,
			}
		}(eng)
	}

	wg.Wait()
	close(resCh)

	for r := range resCh {
		if r.reachable {
			report.OnlineEngines = append(report.OnlineEngines, r.id)
			for _, m := range r.models {
				cleanModel := strings.TrimSpace(m)
				if cleanModel == "" {
					continue
				}
				score := inference.ModelIntelligenceScore(cleanModel)
				report.DiscoveredModels = append(report.DiscoveredModels, DiscoveredModel{
					ModelName:         cleanModel,
					EngineID:          r.id,
					EngineType:        string(r.engType),
					IntelligenceScore: score,
					Endpoint:          r.endpoint,
					ZeroCost:          true,
				})
			}
		} else {
			report.OfflineEngines = append(report.OfflineEngines, r.id)
		}
	}

	// Sort online and offline engines deterministically
	sort.Strings(report.OnlineEngines)
	sort.Strings(report.OfflineEngines)

	// Sort discovered models descending by intelligence score: smartest first!
	sort.SliceStable(report.DiscoveredModels, func(i, j int) bool {
		if report.DiscoveredModels[i].IntelligenceScore != report.DiscoveredModels[j].IntelligenceScore {
			return report.DiscoveredModels[i].IntelligenceScore > report.DiscoveredModels[j].IntelligenceScore
		}
		return report.DiscoveredModels[i].ModelName < report.DiscoveredModels[j].ModelName
	})

	if len(report.DiscoveredModels) > 0 {
		report.ActiveLocalPrimary = report.DiscoveredModels[0].ModelName
		report.ActiveLocalPrimaryScore = report.DiscoveredModels[0].IntelligenceScore
		report.ActiveLocalPrimaryEngine = report.DiscoveredModels[0].EngineID
	}

	// 2. Synthesize dynamic LiteLLM configuration honoring discovered intelligence
	cfg := u.buildDynamicConfig(opts, engines, report.DiscoveredModels)
	report.TotalModelsConfigured = len(cfg.ModelList)

	// 3. Render YAML
	yamlBytes, err := RenderYAML(cfg)
	if err != nil {
		report.Error = err.Error()
		report.Duration = time.Since(start)
		return report, fmt.Errorf("failed to render updated LiteLLM config: %w", err)
	}
	report.YAMLContent = string(yamlBytes)

	// 4. Write to disk if not dry-run
	if !opts.DryRun {
		dir := filepath.Dir(opts.ConfigPath)
		if dir != "" && dir != "." {
			if mkErr := os.MkdirAll(dir, 0755); mkErr != nil {
				report.Error = mkErr.Error()
				report.Duration = time.Since(start)
				return report, fmt.Errorf("failed to create directory %s: %w", dir, mkErr)
			}
		}

		tmpFile := opts.ConfigPath + ".tmp"
		if wErr := os.WriteFile(tmpFile, yamlBytes, 0644); wErr != nil {
			report.Error = wErr.Error()
			report.Duration = time.Since(start)
			return report, fmt.Errorf("failed to write temporary config %s: %w", tmpFile, wErr)
		}

		if renErr := os.Rename(tmpFile, opts.ConfigPath); renErr != nil {
			_ = os.Remove(tmpFile)
			report.Error = renErr.Error()
			report.Duration = time.Since(start)
			return report, fmt.Errorf("failed to replace config %s: %w", opts.ConfigPath, renErr)
		}

		report.BytesWritten = len(yamlBytes)
	}

	report.Duration = time.Since(start)
	return report, nil
}

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
				Model:      "github_copilot/claude-sonnet-4.5",
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
				BaseModel:          "claude-sonnet-4.5",
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

	// 1. Process discovered models in intelligence order
	var assigned27b, assigned9b, assignedCoder bool
	for _, dm := range discovered {
		clean := strings.ToLower(dm.ModelName)

		// Map standard dynamic aliases to the best available models
		if !assigned27b && (strings.Contains(clean, "27b") || strings.Contains(clean, "32b") || dm.IntelligenceScore >= 85) {
			addLocalEntry("local-qwen3-27b", dm.ModelName, dm.EngineType, dm.Endpoint, 8192)
			assigned27b = true
		} else if !assigned9b && (strings.Contains(clean, "9b") || strings.Contains(clean, "8b") || (dm.IntelligenceScore >= 65 && dm.IntelligenceScore < 85)) {
			addLocalEntry("local-qwen3-9b", dm.ModelName, dm.EngineType, dm.Endpoint, 4096)
			assigned9b = true
		}

		if !assignedCoder && (strings.Contains(clean, "coder") || strings.Contains(clean, "instruct")) {
			addLocalEntry("local-qwen-coder", dm.ModelName, dm.EngineType, dm.Endpoint, 8192)
			assignedCoder = true
		}

		// Also register explicit model name
		explicitAlias := fmt.Sprintf("local-%s", strings.ReplaceAll(strings.ReplaceAll(clean, ":", "-"), "/", "-"))
		addLocalEntry(explicitAlias, dm.ModelName, dm.EngineType, dm.Endpoint, 4096)
	}

	// 2. Ensure standard fallback aliases exist even if engines are currently offline
	defaultOllamaEP := "http://127.0.0.1:11434"
	if opts.OllamaEndpoint != "" {
		defaultOllamaEP = opts.OllamaEndpoint
	}
	defaultOxillamaEP := "http://127.0.0.1:8080"
	if opts.OxillamaEndpoint != "" {
		defaultOxillamaEP = opts.OxillamaEndpoint
	}

	if !registeredAliases["local-qwen3-27b"] {
		addLocalEntry("local-qwen3-27b", "qwen3.8:27b", string(inference.EngineOxiLLaMa), defaultOxillamaEP, 8192)
	}
	if !registeredAliases["local-qwen3-9b"] {
		addLocalEntry("local-qwen3-9b", "qwen3.5:9b", string(inference.EngineOxiLLaMa), defaultOxillamaEP, 4096)
	}
	if !registeredAliases["local-qwen-coder"] {
		addLocalEntry("local-qwen-coder", "qwen2.5-coder-7b", string(inference.EngineOxiLLaMa), defaultOxillamaEP, 4096)
	}

	// Also register backward-compatible engine aliases
	addLocalEntry("oxillama-rust", "qwen3.8:27b", string(inference.EngineOxiLLaMa), defaultOxillamaEP, 8192)
	addLocalEntry("local-coder", "hermes3:8b", string(inference.EngineOllama), defaultOllamaEP, 4096)
	addLocalEntry("llamacpp-local", "mistral-7b-instruct-q4", string(inference.EngineLlamaCpp), "http://localhost:8081", 4096)
	addLocalEntry("vllm-cluster", "Qwen/Qwen2.5-Coder-32B-Instruct", string(inference.EngineVLLM), "http://vllm-node:8000", 4096)
	addLocalEntry("hf-deepseek", "deepseek-ai/DeepSeek-Coder-V2-Lite-Instruct", string(inference.EngineHuggingFaceTGI), "http://tgi-node:8080", 4096)

	// Dynamic cascade fallbacks
	// Reorder fallbackModelNames by ModelIntelligenceScore
	sort.SliceStable(fallbackModelNames, func(i, j int) bool {
		return inference.ModelIntelligenceScore(fallbackModelNames[i]) > inference.ModelIntelligenceScore(fallbackModelNames[j])
	})

	proFallbacks := append([]string{opts.BackupCloudModel, opts.FastCloudModel}, fallbackModelNames...)
	sonnetFallbacks := append([]string{opts.FastCloudModel}, fallbackModelNames...)
	flashFallbacks := append([]string(nil), fallbackModelNames...)

	routerSettings := &RouterSettings{
		RoutingStrategy: "usage-based-routing",
		Timeout:         120,
		Fallbacks: []map[string][]string{
			{
				opts.PrimaryCloudModel: proFallbacks,
				opts.BackupCloudModel:  sonnetFallbacks,
				opts.FastCloudModel:    flashFallbacks,
			},
		},
		ContextWindowFallbacks: []map[string]string{
			{"local-qwen-coder": opts.PrimaryCloudModel},
			{"local-qwen3-27b": opts.PrimaryCloudModel},
			{"local-qwen3-9b": opts.PrimaryCloudModel},
			{opts.FastCloudModel: opts.PrimaryCloudModel},
		},
	}

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
