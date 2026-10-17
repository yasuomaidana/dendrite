package inference

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
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

// AutoLoad automatically attempts to discover and load .env files and persistent JSON config.
func (r *ZeroCostRegistry) AutoLoad() {
	r.LoadFromEnv()

	// Try standard .env locations if environment variables are not set
	envPaths := []string{".env", "stacks/.env", "../.env"}
	for _, p := range envPaths {
		if _, err := os.Stat(p); err == nil {
			_ = r.LoadFromEnvFile(p)
			break
		}
	}

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

// AddModel registers an exact model name as zero-cost.
func (r *ZeroCostRegistry) AddModel(model string) bool {
	m := strings.ToLower(strings.TrimSpace(model))
	if m == "" {
		return false
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	delete(r.removedModels, m)
	r.customModels[m] = true
	return true
}

// AddPrefix registers a prefix pattern as zero-cost.
func (r *ZeroCostRegistry) AddPrefix(prefix string) bool {
	p := strings.ToLower(strings.TrimSpace(prefix))
	if p == "" {
		return false
	}
	if !strings.HasSuffix(p, "/") {
		p += "/"
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	delete(r.removedPrefixes, p)
	r.customPrefixes[p] = true
	return true
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
	m := strings.ToLower(strings.TrimSpace(model))
	if m == "" {
		return false
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	delete(r.customModels, m)
	r.removedModels[m] = true
	return true
}

// RemovePrefix unregisters a prefix pattern.
func (r *ZeroCostRegistry) RemovePrefix(prefix string) bool {
	p := strings.ToLower(strings.TrimSpace(prefix))
	if p == "" {
		return false
	}
	if !strings.HasSuffix(p, "/") {
		p += "/"
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	delete(r.customPrefixes, p)
	r.removedPrefixes[p] = true
	return true
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

	if rawModels != "" {
		for _, part := range strings.Split(rawModels, ",") {
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

	if rawPrefixes != "" {
		for _, part := range strings.Split(rawPrefixes, ",") {
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

// LoadFromEnvFile parses a .env file and extracts DENDRITE_ZERO_COST_MODELS and DENDRITE_ZERO_COST_PREFIXES.
func (r *ZeroCostRegistry) LoadFromEnvFile(path string) error {
	f, err := os.Open(path)
	if err != nil {
		return err
	}
	defer f.Close()

	scanner := bufio.NewScanner(f)
	var envModels, envPrefixes string

	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		parts := strings.SplitN(line, "=", 2)
		if len(parts) != 2 {
			continue
		}
		key := strings.TrimSpace(parts[0])
		val := strings.Trim(strings.TrimSpace(parts[1]), "\"'")

		if key == EnvZeroCostModels {
			envModels = val
		} else if key == EnvZeroCostPrefixes {
			envPrefixes = val
		}
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	if envModels != "" {
		for _, part := range strings.Split(envModels, ",") {
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

	if envPrefixes != "" {
		for _, part := range strings.Split(envPrefixes, ",") {
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

	return scanner.Err()
}

// SaveToEnvFile updates or appends DENDRITE_ZERO_COST_MODELS and DENDRITE_ZERO_COST_PREFIXES in the target .env file.
func (r *ZeroCostRegistry) SaveToEnvFile(envPath string) error {
	details := r.GetDetails()

	var activeModels, activePrefixes []string
	for _, m := range details.Models {
		if m.Active {
			activeModels = append(activeModels, m.Pattern)
		}
	}
	for _, p := range details.Prefixes {
		if p.Active {
			activePrefixes = append(activePrefixes, p.Pattern)
		}
	}

	modelsVal := strings.Join(activeModels, ",")
	prefixesVal := strings.Join(activePrefixes, ",")

	content, err := os.ReadFile(envPath)
	if err != nil && !os.IsNotExist(err) {
		return err
	}

	lines := strings.Split(string(content), "\n")
	foundModels := false
	foundPrefixes := false

	for i, line := range lines {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, EnvZeroCostModels+"=") {
			lines[i] = fmt.Sprintf("%s=%s", EnvZeroCostModels, modelsVal)
			foundModels = true
		} else if strings.HasPrefix(trimmed, EnvZeroCostPrefixes+"=") {
			lines[i] = fmt.Sprintf("%s=%s", EnvZeroCostPrefixes, prefixesVal)
			foundPrefixes = true
		}
	}

	if !foundModels {
		lines = append(lines, fmt.Sprintf("%s=%s", EnvZeroCostModels, modelsVal))
	}
	if !foundPrefixes {
		lines = append(lines, fmt.Sprintf("%s=%s", EnvZeroCostPrefixes, prefixesVal))
	}

	output := strings.Join(lines, "\n")
	return os.WriteFile(envPath, []byte(output), 0644)
}

// SaveToFile serializes custom additions and removals into JSON format.
func (r *ZeroCostRegistry) SaveToFile(path string) error {
	r.mu.RLock()
	var addedModels []string
	for m := range r.customModels {
		addedModels = append(addedModels, m)
	}
	var addedPrefixes []string
	for p := range r.customPrefixes {
		addedPrefixes = append(addedPrefixes, p)
	}
	var removedModels []string
	for m := range r.removedModels {
		removedModels = append(removedModels, m)
	}
	var removedPrefixes []string
	for p := range r.removedPrefixes {
		removedPrefixes = append(removedPrefixes, p)
	}
	r.mu.RUnlock()

	sort.Strings(addedModels)
	sort.Strings(addedPrefixes)
	sort.Strings(removedModels)
	sort.Strings(removedPrefixes)

	cfg := PersistentZeroCostConfig{
		AddedModels:     addedModels,
		AddedPrefixes:   addedPrefixes,
		RemovedModels:   removedModels,
		RemovedPrefixes: removedPrefixes,
		UpdatedAt:       time.Now().UTC(),
	}

	data, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return err
	}

	dir := filepath.Dir(path)
	if dir != "" && dir != "." {
		if err := os.MkdirAll(dir, 0755); err != nil {
			return err
		}
	}

	return os.WriteFile(path, data, 0644)
}

// LoadFromFile restores custom additions and removals from a JSON configuration file.
func (r *ZeroCostRegistry) LoadFromFile(path string) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}

	var cfg PersistentZeroCostConfig
	if err := json.Unmarshal(data, &cfg); err != nil {
		return err
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	for _, m := range cfg.AddedModels {
		r.customModels[strings.ToLower(m)] = true
	}
	for _, p := range cfg.AddedPrefixes {
		norm := strings.ToLower(p)
		if !strings.HasSuffix(norm, "/") {
			norm += "/"
		}
		r.customPrefixes[norm] = true
	}
	for _, m := range cfg.RemovedModels {
		r.removedModels[strings.ToLower(m)] = true
	}
	for _, p := range cfg.RemovedPrefixes {
		norm := strings.ToLower(p)
		if !strings.HasSuffix(norm, "/") {
			norm += "/"
		}
		r.removedPrefixes[norm] = true
	}

	return nil
}

// GetDetails returns comprehensive rule state for CLI and Web UI display.
func (r *ZeroCostRegistry) GetDetails() ZeroCostDetails {
	r.mu.RLock()
	defer r.mu.RUnlock()

	modelsMap := make(map[string]ZeroCostEntry)
	for m := range r.defaultModels {
		modelsMap[m] = ZeroCostEntry{
			Pattern:  m,
			IsPrefix: false,
			Source:   SourceDefault,
			Active:   !r.removedModels[m],
		}
	}
	for m := range r.envModels {
		modelsMap[m] = ZeroCostEntry{
			Pattern:  m,
			IsPrefix: false,
			Source:   SourceEnv,
			Active:   !r.removedModels[m],
		}
	}
	for m := range r.customModels {
		modelsMap[m] = ZeroCostEntry{
			Pattern:  m,
			IsPrefix: false,
			Source:   SourceCustom,
			Active:   true,
		}
	}

	prefixesMap := make(map[string]ZeroCostEntry)
	for p := range r.defaultPrefixes {
		prefixesMap[p] = ZeroCostEntry{
			Pattern:  p,
			IsPrefix: true,
			Source:   SourceDefault,
			Active:   !r.removedPrefixes[p],
		}
	}
	for p := range r.envPrefixes {
		prefixesMap[p] = ZeroCostEntry{
			Pattern:  p,
			IsPrefix: true,
			Source:   SourceEnv,
			Active:   !r.removedPrefixes[p],
		}
	}
	for p := range r.customPrefixes {
		prefixesMap[p] = ZeroCostEntry{
			Pattern:  p,
			IsPrefix: true,
			Source:   SourceCustom,
			Active:   true,
		}
	}

	var models []ZeroCostEntry
	activeCount := 0
	for _, entry := range modelsMap {
		models = append(models, entry)
		if entry.Active {
			activeCount++
		}
	}
	sort.Slice(models, func(i, j int) bool {
		return models[i].Pattern < models[j].Pattern
	})

	var prefixes []ZeroCostEntry
	for _, entry := range prefixesMap {
		prefixes = append(prefixes, entry)
		if entry.Active {
			activeCount++
		}
	}
	sort.Slice(prefixes, func(i, j int) bool {
		return prefixes[i].Pattern < prefixes[j].Pattern
	})

	return ZeroCostDetails{
		Models:   models,
		Prefixes: prefixes,
		Total:    activeCount,
	}
}

// IsZeroCostSelfHostedModel checks if a model is exempt from financial token budgeting.
func IsZeroCostSelfHostedModel(model string) bool {
	return GetZeroCostRegistry().IsZeroCost(model)
}
