package inference

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

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
