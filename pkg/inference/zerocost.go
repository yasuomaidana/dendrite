package inference

import (
	"fmt"
	"os"
	"strings"
	"sync"
	"time"

	"Dendrite/pkg/statedir"
)

// Environment variable keys for zero-cost model configuration.
const (
	EnvZeroCostModels   = "DENDRITE_ZERO_COST_MODELS"
	EnvZeroCostPrefixes = "DENDRITE_ZERO_COST_PREFIXES"
)

// Default storage file for dynamic zero-cost model persistence.
const DefaultZeroCostConfigFile = ".dendrite/zero_cost_models.json"

// Built-in default zero-cost models and prefixes.
var (
	DefaultZeroCostModels = []string{
		"local-qwen-coder",
		"local-qwen3-27b",
		"local-qwen3-9b",
		"oxillama-rust",
		"vllm-cluster",
		"hf-tgi-remote",
		"llamacpp-local",
	}

	DefaultZeroCostPrefixes = []string{
		"ollama/",
		"oxillama/",
		"vllm/",
		"llamacpp/",
		"huggingface-tei/",
	}
)

// ModelSource indicates where a zero-cost rule originated.
type ModelSource string

const (
	SourceDefault ModelSource = "default"
	SourceEnv     ModelSource = "env"
	SourceCustom  ModelSource = "custom"
)

// ZeroCostEntry describes an individual model or prefix rule.
type ZeroCostEntry struct {
	Pattern  string      `json:"pattern"`
	IsPrefix bool        `json:"is_prefix"`
	Source   ModelSource `json:"source"`
	Active   bool        `json:"active"`
}

// ZeroCostDetails provides full visibility into active and overridden zero-cost rules.
type ZeroCostDetails struct {
	Models   []ZeroCostEntry `json:"models"`
	Prefixes []ZeroCostEntry `json:"prefixes"`
	Total    int             `json:"total_active"`
}

// PersistentZeroCostConfig defines JSON file schema for disk persistence.
type PersistentZeroCostConfig struct {
	AddedModels     []string  `json:"added_models"`
	AddedPrefixes   []string  `json:"added_prefixes"`
	RemovedModels   []string  `json:"removed_models"`
	RemovedPrefixes []string  `json:"removed_prefixes"`
	UpdatedAt       time.Time `json:"updated_at"`
}

// ZeroCostRegistry is a thread-safe registry controlling zero-cost self-hosted models and prefixes.
type ZeroCostRegistry struct {
	mu              sync.RWMutex
	defaultModels   map[string]bool
	defaultPrefixes map[string]bool
	envModels       map[string]bool
	envPrefixes     map[string]bool
	customModels    map[string]bool
	customPrefixes  map[string]bool
	removedModels   map[string]bool
	removedPrefixes map[string]bool
	configPath      string
}

var (
	globalZeroCostRegistry *ZeroCostRegistry
	onceZeroCostRegistry   sync.Once
)

// GetZeroCostRegistry returns the global singleton registry.
func GetZeroCostRegistry() *ZeroCostRegistry {
	onceZeroCostRegistry.Do(func() {
		globalZeroCostRegistry = NewZeroCostRegistry(statedir.Resolve(DefaultZeroCostConfigFile))
		globalZeroCostRegistry.AutoLoad()
	})
	return globalZeroCostRegistry
}

// NewZeroCostRegistry creates a new ZeroCostRegistry instance.
func NewZeroCostRegistry(configPath string) *ZeroCostRegistry {
	r := &ZeroCostRegistry{
		defaultModels:   make(map[string]bool),
		defaultPrefixes: make(map[string]bool),
		envModels:       make(map[string]bool),
		envPrefixes:     make(map[string]bool),
		customModels:    make(map[string]bool),
		customPrefixes:  make(map[string]bool),
		removedModels:   make(map[string]bool),
		removedPrefixes: make(map[string]bool),
		configPath:      configPath,
	}

	for _, m := range DefaultZeroCostModels {
		r.defaultModels[strings.ToLower(m)] = true
	}
	for _, p := range DefaultZeroCostPrefixes {
		r.defaultPrefixes[strings.ToLower(p)] = true
	}

	return r
}

// AutoLoad loads the environment (the CLI has already applied the project's .env and the
// global $DENDRITE_HOME/.env, see pkg/dotenv) and the persistent JSON config.
func (r *ZeroCostRegistry) AutoLoad() {
	r.LoadFromEnv()

	if r.configPath != "" {
		if _, err := os.Stat(r.configPath); err == nil {
			_ = r.LoadFromFile(r.configPath)
		}
	}
}

// IsZeroCost checks if a given model identifier matches an active zero-cost model or prefix.
func (r *ZeroCostRegistry) IsZeroCost(model string) bool {
	m := strings.ToLower(strings.TrimSpace(model))
	if m == "" {
		return false
	}

	r.mu.RLock()
	defer r.mu.RUnlock()

	// Check custom exact models
	if r.customModels[m] {
		return true
	}

	// Check env exact models (unless removed)
	if r.envModels[m] && !r.removedModels[m] {
		return true
	}

	// Check default exact models (unless removed)
	if r.defaultModels[m] && !r.removedModels[m] {
		return true
	}

	// Check custom prefixes
	for p := range r.customPrefixes {
		if strings.HasPrefix(m, p) {
			return true
		}
	}

	// Check env prefixes (unless removed)
	for p := range r.envPrefixes {
		if !r.removedPrefixes[p] && strings.HasPrefix(m, p) {
			return true
		}
	}

	// Check default prefixes (unless removed)
	for p := range r.defaultPrefixes {
		if !r.removedPrefixes[p] && strings.HasPrefix(m, p) {
			return true
		}
	}

	return false
}

// Test evaluates a model string and returns match status along with the reason.
func (r *ZeroCostRegistry) Test(model string) (bool, string) {
	m := strings.ToLower(strings.TrimSpace(model))
	if m == "" {
		return false, "empty model name"
	}

	r.mu.RLock()
	defer r.mu.RUnlock()

	if r.customModels[m] {
		return true, fmt.Sprintf("exact match '%s' (custom addition)", m)
	}
	if r.envModels[m] && !r.removedModels[m] {
		return true, fmt.Sprintf("exact match '%s' (configured via .env)", m)
	}
	if r.defaultModels[m] && !r.removedModels[m] {
		return true, fmt.Sprintf("exact match '%s' (built-in default)", m)
	}

	for p := range r.customPrefixes {
		if strings.HasPrefix(m, p) {
			return true, fmt.Sprintf("prefix match '%s' (custom addition)", p)
		}
	}
	for p := range r.envPrefixes {
		if !r.removedPrefixes[p] && strings.HasPrefix(m, p) {
			return true, fmt.Sprintf("prefix match '%s' (configured via .env)", p)
		}
	}
	for p := range r.defaultPrefixes {
		if !r.removedPrefixes[p] && strings.HasPrefix(m, p) {
			return true, fmt.Sprintf("prefix match '%s' (built-in default)", p)
		}
	}

	if r.removedModels[m] {
		return false, fmt.Sprintf("model '%s' is explicitly removed / disabled", m)
	}

	return false, fmt.Sprintf("model '%s' does not match any active zero-cost model or prefix", m)
}

func cleanModel(model string) string {
	return strings.ToLower(strings.TrimSpace(model))
}

func cleanPrefix(prefix string) string {
	p := strings.ToLower(strings.TrimSpace(prefix))
	if p == "" {
		return ""
	}
	if !strings.HasSuffix(p, "/") {
		p += "/"
	}
	return p
}

func (r *ZeroCostRegistry) updateEntry(item string, removeMap, insertMap map[string]bool) bool {
	if item == "" {
		return false
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	delete(removeMap, item)
	insertMap[item] = true
	return true
}

func (r *ZeroCostRegistry) parseEnvConfig(models, prefixes string) {
	if models != "" {
		for _, part := range strings.Split(models, ",") {
			item := strings.ToLower(strings.TrimSpace(part))
			if item == "" {
				continue
			}
			if strings.HasSuffix(item, "/") {
				r.envPrefixes[item] = true
			} else {
				r.envModels[item] = true
			}
		}
	}

	if prefixes != "" {
		for _, part := range strings.Split(prefixes, ",") {
			item := strings.ToLower(strings.TrimSpace(part))
			if item == "" {
				continue
			}
			if !strings.HasSuffix(item, "/") {
				item += "/"
			}
			r.envPrefixes[item] = true
		}
	}
}

// AddModel registers an exact model name as zero-cost.
func (r *ZeroCostRegistry) AddModel(model string) bool {
	return r.updateEntry(cleanModel(model), r.removedModels, r.customModels)
}

// AddPrefix registers a prefix pattern as zero-cost.
func (r *ZeroCostRegistry) AddPrefix(prefix string) bool {
	return r.updateEntry(cleanPrefix(prefix), r.removedPrefixes, r.customPrefixes)
}

// Add registers an item as either a prefix (if ends with /) or exact model.
func (r *ZeroCostRegistry) Add(item string) (bool, bool) {
	s := strings.TrimSpace(item)
	if s == "" {
		return false, false
	}
	if strings.HasSuffix(s, "/") {
		return r.AddPrefix(s), true
	}
	return r.AddModel(s), false
}

// RemoveModel unregisters an exact model name from zero-cost eligibility.
func (r *ZeroCostRegistry) RemoveModel(model string) bool {
	return r.updateEntry(cleanModel(model), r.customModels, r.removedModels)
}

// RemovePrefix unregisters a prefix pattern.
func (r *ZeroCostRegistry) RemovePrefix(prefix string) bool {
	return r.updateEntry(cleanPrefix(prefix), r.customPrefixes, r.removedPrefixes)
}

// Remove unregisters an item as prefix (if ends with /) or exact model.
func (r *ZeroCostRegistry) Remove(item string) (bool, bool) {
	s := strings.TrimSpace(item)
	if s == "" {
		return false, false
	}
	if strings.HasSuffix(s, "/") {
		return r.RemovePrefix(s), true
	}
	// Try model first
	m := strings.ToLower(s)
	r.mu.Lock()
	_, isPrefix := r.customPrefixes[m+"/"]
	if !isPrefix {
		_, isPrefix = r.defaultPrefixes[m+"/"]
	}
	if !isPrefix {
		_, isPrefix = r.envPrefixes[m+"/"]
	}
	r.mu.Unlock()

	if isPrefix {
		return r.RemovePrefix(m + "/"), true
	}
	return r.RemoveModel(s), false
}

// Reset clears all custom additions and removals.
func (r *ZeroCostRegistry) Reset() {
	r.mu.Lock()
	defer r.mu.Unlock()

	r.customModels = make(map[string]bool)
	r.customPrefixes = make(map[string]bool)
	r.removedModels = make(map[string]bool)
	r.removedPrefixes = make(map[string]bool)
}

// LoadFromEnv loads zero-cost models and prefixes from os.Environ().
func (r *ZeroCostRegistry) LoadFromEnv() {
	rawModels := os.Getenv(EnvZeroCostModels)
	rawPrefixes := os.Getenv(EnvZeroCostPrefixes)

	r.mu.Lock()
	defer r.mu.Unlock()

	r.parseEnvConfig(rawModels, rawPrefixes)
}
