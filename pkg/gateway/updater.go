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
	"Dendrite/pkg/memfit"
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
	SizeBytes         int64  `json:"size_bytes,omitempty"`
	// Fits is false when the model would not run inside this machine's memory budget; such models
	// never get a role alias or a place in the fallback cascade.
	Fits      bool   `json:"fits"`
	FitReason string `json:"fit_reason,omitempty"`
	// Embed marks embedding models, which serve the local-embed role and never a chat role.
	Embed bool `json:"embed,omitempty"`
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
	PrimaryCloudModel string             // Primary commercial model (default: gemini-4-argon-medium)
	BackupCloudModel  string             // Secondary commercial model (default: copilot-sonnet)
	FastCloudModel    string             // Fast commercial model (default: gemini-3.8-flash-medium)
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
		PrimaryCloudModel: "gemini-4-argon-medium",
		BackupCloudModel:  "copilot-sonnet",
		FastCloudModel:    "gemini-3.8-flash-medium",
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
	TooLargeModels           []string          `json:"too_large_models,omitempty"`
	MemoryBudgetBytes        uint64            `json:"memory_budget_bytes"`
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

	var ollamaEP, oxiEP string
	for _, e := range engines {
		switch e.Type() {
		case inference.EngineOllama:
			ollamaEP = e.Config().Endpoint
		case inference.EngineOxiLLaMa:
			oxiEP = e.Config().Endpoint
		}
	}
	fit := memfit.Assess(ctx, ollamaEP, oxiEP)
	report.MemoryBudgetBytes = fit.BudgetBytes

	for r := range resCh {
		if r.reachable {
			report.OnlineEngines = append(report.OnlineEngines, r.id)
			for _, m := range r.models {
				cleanModel := strings.TrimSpace(m)
				if cleanModel == "" {
					continue
				}
				score := inference.ModelIntelligenceScore(cleanModel)
				dm := DiscoveredModel{
					ModelName:         cleanModel,
					EngineID:          r.id,
					EngineType:        string(r.engType),
					IntelligenceScore: score,
					Endpoint:          r.endpoint,
					ZeroCost:          true,
					Fits:              true,
					Embed:             strings.Contains(strings.ToLower(cleanModel), "embed"),
				}
				if m, ok := fit.Find(string(r.engType), cleanModel); ok {
					dm.SizeBytes, dm.Fits, dm.FitReason = m.SizeBytes, m.Fits, m.Reason
				}
				if !dm.Fits {
					report.TooLargeModels = append(report.TooLargeModels, cleanModel)
				}
				report.DiscoveredModels = append(report.DiscoveredModels, dm)
			}
		} else {
			report.OfflineEngines = append(report.OfflineEngines, r.id)
		}
	}

	// Sort online and offline engines deterministically
	sort.Strings(report.OnlineEngines)
	sort.Strings(report.OfflineEngines)

	// Smartest first: slower is acceptable, starving the machine is not (models that do not fit are
	// already excluded above). A GPU engine wins a tie, since it answers faster.
	sort.SliceStable(report.DiscoveredModels, func(i, j int) bool {
		a, b := report.DiscoveredModels[i], report.DiscoveredModels[j]
		if a.Fits != b.Fits { // models that fit come first
			return a.Fits
		}
		if a.Embed != b.Embed { // chat models before embedders
			return !a.Embed
		}
		if a.IntelligenceScore != b.IntelligenceScore {
			return a.IntelligenceScore > b.IntelligenceScore
		}
		ga, gb := inference.GPUAccelerated(inference.EngineType(a.EngineType)), inference.GPUAccelerated(inference.EngineType(b.EngineType))
		if ga != gb {
			return ga
		}
		return a.ModelName < b.ModelName
	})

	if len(report.DiscoveredModels) > 0 && report.DiscoveredModels[0].Fits && !report.DiscoveredModels[0].Embed {
		report.ActiveLocalPrimary = report.DiscoveredModels[0].ModelName
		report.ActiveLocalPrimaryScore = report.DiscoveredModels[0].IntelligenceScore
		report.ActiveLocalPrimaryEngine = report.DiscoveredModels[0].EngineID
	}
	sort.Strings(report.TooLargeModels)

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

		// Write directly to preserve the file inode for Docker container bind mounts on macOS/Linux
		if wErr := os.WriteFile(opts.ConfigPath, yamlBytes, 0644); wErr != nil {
			report.Error = wErr.Error()
			report.Duration = time.Since(start)
			return report, fmt.Errorf("failed to write config %s: %w", opts.ConfigPath, wErr)
		}

		report.BytesWritten = len(yamlBytes)
	}

	report.Duration = time.Since(start)
	return report, nil
}
