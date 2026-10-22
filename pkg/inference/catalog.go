package inference

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"Dendrite/pkg/statedir"
)

// ModelCapability defines capabilities, context windows, benchmark metrics,
// and complexity bounds for an inference engine model instance.
type ModelCapability struct {
	ID              string     `json:"id"`
	Engine          EngineType `json:"engine"`
	ReasoningTier   int        `json:"reasoning_tier"`
	CodingBenchmark float64    `json:"coding_benchmark"`
	ContextWindow   int        `json:"context_window"`
	TokensPerSec    float64    `json:"tokens_per_sec"`
	MinComplexity   int        `json:"min_complexity"`
	MaxComplexity   int        `json:"max_complexity"`
}

// Catalog represents a thread-safe registry of model capabilities.
type Catalog struct {
	mu     sync.RWMutex
	Path   string            `json:"-"`
	Models []ModelCapability `json:"models"`
}

// DefaultCatalogPath resolves the default capability catalog location (.dendrite/models.json)
// via pkg/statedir.Resolve.
func DefaultCatalogPath() string {
	return statedir.Resolve(".dendrite/models.json")
}

// DefaultCatalog returns a baseline Catalog seeded with standard models and capabilities
// derived from ModelIntelligenceScore as a cold-start prior.
func DefaultCatalog() *Catalog {
	models := []ModelCapability{
		DefaultCapability("qwen2.5-coder:14b", EngineOxiLLaMa),
		DefaultCapability("qwen2.5-coder:14b", EngineOllama),
		DefaultCapability("qwen2.5-coder-7b", EngineOxiLLaMa),
		DefaultCapability("qwen3.8:27b", EngineOxiLLaMa),
		DefaultCapability("qwen3.8:27b", EngineOllama),
		DefaultCapability("qwen3.5:9b", EngineOxiLLaMa),
		DefaultCapability("qwen3.5:9b", EngineOllama),
		DefaultCapability("mistral-7b-instruct", EngineOxiLLaMa),
		DefaultCapability("deepseek-coder:6.7b", EngineOllama),
		DefaultCapability("llama3.2:3b", EngineOllama),
		DefaultCapability("Qwen/Qwen2.5-Coder-32B-Instruct", EngineVLLM),
		DefaultCapability("deepseek-ai/DeepSeek-Coder-V2-Lite-Instruct", EngineHuggingFaceTGI),
		DefaultCapability("candle-quant-llama", EngineCandle),
		DefaultCapability("mistral-7b-instruct-q4", EngineLlamaCpp),
	}
	return &Catalog{
		Path:   DefaultCatalogPath(),
		Models: models,
	}
}

// DefaultCapability constructs a baseline ModelCapability for a model and engine,
// seeded from ModelIntelligenceScore as a cold-start prior.
func DefaultCapability(id string, engine EngineType) ModelCapability {
	clean := strings.ToLower(strings.TrimSpace(id))
	score := ModelIntelligenceScore(clean)
	tier := reasoningTierForScore(clean, score)

	bench := float64(score) / 100.0
	if strings.Contains(clean, "qwen2.5-coder:14b") {
		bench = 0.62
	}

	minC, maxC := complexityBoundsForTier(tier)

	return ModelCapability{
		ID:              id,
		Engine:          engine,
		ReasoningTier:   tier,
		CodingBenchmark: bench,
		ContextWindow:   contextWindowForModel(clean),
		TokensPerSec:    0,
		MinComplexity:   minC,
		MaxComplexity:   maxC,
	}
}

func reasoningTierForScore(clean string, score int) int {
	if strings.Contains(clean, "14b") {
		return 3
	}
	tier := score / 20
	if tier < 1 {
		return 1
	}
	if tier > 5 {
		return 5
	}
	return tier
}

func complexityBoundsForTier(tier int) (int, int) {
	switch {
	case tier <= 2:
		return 1, 3
	case tier == 3:
		return 1, 6
	case tier == 4:
		return 2, 8
	default:
		return 3, 10
	}
}

func contextWindowForModel(clean string) int {
	if strings.Contains(clean, "128k") || strings.Contains(clean, "131072") {
		return 131072
	}
	if strings.Contains(clean, "llama3.2") || strings.Contains(clean, "3b") || strings.Contains(clean, "1b") {
		return 8192
	}
	return 32768
}

// List returns a defensive copy of all registered model capabilities.
func (c *Catalog) List() []ModelCapability {
	c.mu.RLock()
	defer c.mu.RUnlock()

	out := make([]ModelCapability, len(c.Models))
	copy(out, c.Models)
	return out
}

// Get finds a capability entry by model ID; when several engines serve the model, the
// preferred engine (EnginePreference: GPU-capable first, OxiLLaMa last) wins.
func (c *Catalog) Get(id string) (ModelCapability, bool) {
	c.mu.RLock()
	defer c.mu.RUnlock()

	clean := strings.ToLower(strings.TrimSpace(id))
	var best ModelCapability
	found := false

	for _, m := range c.Models {
		mClean := strings.ToLower(m.ID)
		engineClean := strings.ToLower(string(m.Engine))
		scoped := engineClean + "/" + mClean

		if mClean == clean || scoped == clean {
			if !found || EnginePreference(m.Engine) < EnginePreference(best.Engine) {
				best = m
				found = true
			}
		}
	}

	if found {
		return best, true
	}
	return ModelCapability{}, false
}

// GetByEngine retrieves a capability entry for an explicit model ID and engine type.
func (c *Catalog) GetByEngine(id string, engine EngineType) (ModelCapability, bool) {
	c.mu.RLock()
	defer c.mu.RUnlock()

	clean := strings.ToLower(strings.TrimSpace(id))
	for _, m := range c.Models {
		if strings.ToLower(m.ID) == clean && m.Engine == engine {
			return m, true
		}
	}
	return ModelCapability{}, false
}

// Set adds or replaces a ModelCapability in the catalog.
func (c *Catalog) Set(cap ModelCapability) {
	c.mu.Lock()
	defer c.mu.Unlock()

	for i, m := range c.Models {
		if strings.EqualFold(m.ID, cap.ID) && m.Engine == cap.Engine {
			c.Models[i] = cap
			return
		}
	}
	c.Models = append(c.Models, cap)
}

// UpdateThroughput records a measured tokens_per_sec throughput rate for a model ID.
func (c *Catalog) UpdateThroughput(id string, tps float64) bool {
	c.mu.Lock()
	defer c.mu.Unlock()

	clean := strings.TrimSpace(id)
	var prefixEngine string
	subModel := clean

	if idx := strings.Index(clean, "/"); idx != -1 {
		prefixEngine = clean[:idx]
		subModel = clean[idx+1:]
	}

	updated := false
	for i := range c.Models {
		m := &c.Models[i]
		if prefixEngine != "" {
			if strings.EqualFold(string(m.Engine), prefixEngine) &&
				(strings.EqualFold(m.ID, subModel) || strings.EqualFold(m.ID, clean)) {
				m.TokensPerSec = tps
				updated = true
			}
		} else {
			if strings.EqualFold(m.ID, clean) {
				m.TokensPerSec = tps
				updated = true
			}
		}
	}

	if !updated {
		eng := EngineOllama
		if prefixEngine != "" {
			eng = EngineType(prefixEngine)
		}
		cap := DefaultCapability(subModel, eng)
		cap.TokensPerSec = tps
		c.Models = append(c.Models, cap)
	}

	return true
}

// Save writes the catalog to disk. If customPath is omitted, c.Path or DefaultCatalogPath is used.
func (c *Catalog) Save(customPath ...string) error {
	return SaveCatalog(c, customPath...)
}

// MarshalJSON implements json.Marshaler.
func (c *Catalog) MarshalJSON() ([]byte, error) {
	c.mu.RLock()
	defer c.mu.RUnlock()

	wrapper := struct {
		Models []ModelCapability `json:"models"`
	}{
		Models: c.Models,
	}
	return json.MarshalIndent(wrapper, "", "  ")
}

// UnmarshalJSON implements json.Unmarshaler supporting both object and array formats.
func (c *Catalog) UnmarshalJSON(data []byte) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	trimmed := bytes.TrimSpace(data)
	if len(trimmed) == 0 {
		c.Models = nil
		return nil
	}

	if trimmed[0] == '[' {
		var slice []ModelCapability
		if err := json.Unmarshal(trimmed, &slice); err != nil {
			return err
		}
		c.Models = slice
		return nil
	}

	var wrapper struct {
		Models []ModelCapability `json:"models"`
		Model  []ModelCapability `json:"model"`
	}
	if err := json.Unmarshal(trimmed, &wrapper); err != nil {
		return err
	}
	if len(wrapper.Models) > 0 {
		c.Models = wrapper.Models
	} else {
		c.Models = wrapper.Model
	}
	return nil
}

// FormatTOML formats the catalog as a TOML document with [[model]] tables.
func (c *Catalog) FormatTOML() string {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return formatTOML(c.Models)
}

func formatTOML(models []ModelCapability) string {
	var sb strings.Builder
	for _, m := range models {
		sb.WriteString("[[model]]\n")
		sb.WriteString(fmt.Sprintf("id = %q\n", m.ID))
		sb.WriteString(fmt.Sprintf("engine = %q\n", m.Engine))
		sb.WriteString(fmt.Sprintf("reasoning_tier = %d\n", m.ReasoningTier))
		sb.WriteString(fmt.Sprintf("coding_benchmark = %.4g\n", m.CodingBenchmark))
		sb.WriteString(fmt.Sprintf("context_window = %d\n", m.ContextWindow))
		sb.WriteString(fmt.Sprintf("tokens_per_sec = %.2f\n", m.TokensPerSec))
		sb.WriteString(fmt.Sprintf("min_complexity = %d\n", m.MinComplexity))
		sb.WriteString(fmt.Sprintf("max_complexity = %d\n\n", m.MaxComplexity))
	}
	return sb.String()
}

// LoadCatalog loads capability catalog from disk.
// If customPath is provided and non-empty, it loads directly from that path.
// Otherwise, it checks for .dendrite/models.toml and .dendrite/models.json resolved
// via statedir.Resolve. If neither file exists on disk, it returns DefaultCatalog()
// with no error, using DefaultCatalog as a cold-start prior.
func LoadCatalog(customPath ...string) (*Catalog, error) {
	if len(customPath) > 0 && customPath[0] != "" {
		targetPath := customPath[0]
		data, err := os.ReadFile(targetPath)
		if err != nil {
			return nil, err
		}
		cat, err := parseCatalogData(data, targetPath)
		if err != nil {
			return nil, err
		}
		cat.Path = targetPath
		return cat, nil
	}

	tomlPath := statedir.Resolve(".dendrite/models.toml")
	if data, err := os.ReadFile(tomlPath); err == nil {
		cat, err := parseCatalogData(data, tomlPath)
		if err != nil {
			return nil, err
		}
		cat.Path = tomlPath
		return cat, nil
	}

	jsonPath := DefaultCatalogPath()
	if data, err := os.ReadFile(jsonPath); err == nil {
		cat, err := parseCatalogData(data, jsonPath)
		if err != nil {
			return nil, err
		}
		cat.Path = jsonPath
		return cat, nil
	}

	def := DefaultCatalog()
	def.Path = jsonPath
	return def, nil
}

// SaveCatalog writes a Catalog to disk. If customPath is omitted, cat.Path or DefaultCatalogPath is used.
func SaveCatalog(cat *Catalog, customPath ...string) error {
	if cat == nil {
		return errors.New("cannot save nil catalog")
	}

	target := ""
	if len(customPath) > 0 && customPath[0] != "" {
		target = customPath[0]
	} else if cat.Path != "" {
		target = cat.Path
	} else {
		target = DefaultCatalogPath()
	}

	cat.mu.Lock()
	cat.Path = target
	cat.mu.Unlock()

	dir := filepath.Dir(target)
	if dir != "" && dir != "." {
		if err := os.MkdirAll(dir, 0755); err != nil {
			return fmt.Errorf("failed to create directory for catalog %s: %w", target, err)
		}
	}

	if strings.HasSuffix(target, ".toml") {
		content := cat.FormatTOML()
		return os.WriteFile(target, []byte(content), 0644)
	}

	data, err := cat.MarshalJSON()
	if err != nil {
		return fmt.Errorf("failed to marshal catalog to JSON: %w", err)
	}
	return os.WriteFile(target, data, 0644)
}

// UpdateThroughput loads the catalog, updates tokens_per_sec for the model ID, and saves.
func UpdateThroughput(id string, tps float64, customPath ...string) error {
	cat, err := LoadCatalog(customPath...)
	if err != nil {
		return err
	}
	cat.UpdateThroughput(id, tps)
	return SaveCatalog(cat, customPath...)
}

func parseCatalogData(data []byte, filename string) (*Catalog, error) {
	trimmed := bytes.TrimSpace(data)
	if len(trimmed) == 0 {
		return &Catalog{Models: nil}, nil
	}

	if strings.HasSuffix(filename, ".toml") || bytes.Contains(trimmed, []byte("[[model")) {
		return ParseTOMLCatalog(trimmed)
	}

	var cat Catalog
	if err := json.Unmarshal(trimmed, &cat); err == nil && len(cat.Models) > 0 {
		return &cat, nil
	}

	if tomlCat, err := ParseTOMLCatalog(trimmed); err == nil && len(tomlCat.Models) > 0 {
		return tomlCat, nil
	}

	var jsonErr Catalog
	err := json.Unmarshal(trimmed, &jsonErr)
	if err != nil {
		return nil, fmt.Errorf("failed to parse catalog at %s: %w", filename, err)
	}
	return &jsonErr, nil
}
