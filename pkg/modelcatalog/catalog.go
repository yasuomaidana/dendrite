// Package modelcatalog knows which models exist and which one plays each role (local, cheap, strong,
// copilot), so callers ask for a role instead of hardcoding a model name. The catalog is rebuilt from
// the LiteLLM gateway, Ollama, `agy models` and the Gemini and Copilot listing APIs, kept in
// $DENDRITE_HOME/model-catalog.json and refreshed at least weekly (on a stale read, by the daemons'
// ticks, and by a weekly LaunchAgent).
package modelcatalog

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"sort"
	"strings"
	"time"
)

// TTL is how long a catalog is considered fresh.
const TTL = 7 * 24 * time.Hour

// FileName is the catalog's name inside the Dendrite home.
const FileName = "model-catalog.json"

// Roles the catalog computes.
const (
	RoleLocal   = "local"   // zero-cost chat model on a GPU engine
	RoleCheap   = "cheap"   // paid model with the lowest input price
	RoleStrong  = "strong"  // dearest paid non-Copilot model with a long context
	RoleCopilot = "copilot" // strongest model served through GitHub Copilot
)

const providerCopilot = "github_copilot"

// Model is one model some source knows about.
type Model struct {
	// Name is how callers address it: the gateway alias when it is routed, else the provider's id.
	Name string `json:"name"`
	// Provider is gemini, github_copilot, ollama, agy, ...
	Provider string `json:"provider"`
	// Upstream is the provider's own id for the model.
	Upstream   string  `json:"upstream"`
	InputCost  float64 `json:"input_cost,omitempty"`  // per token
	OutputCost float64 `json:"output_cost,omitempty"` // per token
	MaxInput   int     `json:"max_input,omitempty"`
	Mode       string  `json:"mode,omitempty"`
	Local      bool    `json:"local,omitempty"`
	// Routed models are served by the gateway today.
	Routed bool `json:"routed,omitempty"`
	// Listed means the provider's own listing confirms the model exists.
	Listed bool   `json:"listed,omitempty"`
	Source string `json:"source,omitempty"`
}

// Catalog is the stored model list with the roles computed from it.
type Catalog struct {
	FetchedAt time.Time         `json:"fetched_at"`
	Models    []Model           `json:"models"`
	Roles     map[string]string `json:"roles"`
	// Notes records sources that failed or were skipped.
	Notes []string `json:"notes,omitempty"`
}

// Stale reports whether the catalog is missing or older than ttl.
func (c Catalog) Stale(now time.Time, ttl time.Duration) bool {
	return c.FetchedAt.IsZero() || now.Sub(c.FetchedAt) > ttl
}

// Load reads the catalog in dir; a missing file is an empty catalog.
func Load(dir string) (Catalog, error) {
	data, err := os.ReadFile(filepath.Join(dir, FileName))
	if os.IsNotExist(err) {
		return Catalog{}, nil
	}
	if err != nil {
		return Catalog{}, err
	}
	var c Catalog
	if err := json.Unmarshal(data, &c); err != nil {
		return Catalog{}, fmt.Errorf("modelcatalog: %s is corrupt: %w", FileName, err)
	}
	return c, nil
}

// Save writes the catalog to dir atomically.
func (c Catalog) Save(dir string) error {
	data, _ := json.MarshalIndent(c, "", "  ") // plain structs always marshal
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	path := filepath.Join(dir, FileName)
	tmp := fmt.Sprintf("%s.%d.tmp", path, os.Getpid())
	if err := os.WriteFile(tmp, data, 0o644); err != nil {
		return err
	}
	if err := os.Rename(tmp, path); err != nil {
		_ = os.Remove(tmp)
		return err
	}
	return nil
}

// isPaidChat reports a routed chat model that costs money.
func isPaidChat(m Model) bool {
	return m.Routed && (m.Mode == "" || m.Mode == "chat") && m.InputCost > 0 &&
		!strings.Contains(strings.ToLower(m.Name+m.Upstream), "embed")
}

// ComputeRoles picks the model for each role among the routed models. local is the gateway's
// local choice (empty when none is known).
func ComputeRoles(models []Model, local string) map[string]string {
	roles := map[string]string{}
	if local != "" {
		roles[RoleLocal] = local
	}
	var paid, nonCopilot, copilot []Model
	for _, m := range models {
		if !isPaidChat(m) {
			continue
		}
		paid = append(paid, m)
		if m.Provider == providerCopilot {
			copilot = append(copilot, m)
		} else {
			nonCopilot = append(nonCopilot, m)
		}
	}
	if m, ok := pick(paid, func(a, b Model) bool { return a.InputCost < b.InputCost }); ok {
		roles[RoleCheap] = m.Name
	}
	long := filter(nonCopilot, func(m Model) bool { return m.MaxInput >= 200000 })
	if len(long) == 0 {
		long = nonCopilot
	}
	if m, ok := pick(long, func(a, b Model) bool { return a.InputCost > b.InputCost }); ok {
		roles[RoleStrong] = m.Name
	}
	if m, ok := pick(copilot, func(a, b Model) bool { return a.InputCost > b.InputCost }); ok {
		roles[RoleCopilot] = m.Name
	}
	return roles
}

func filter(models []Model, keep func(Model) bool) []Model {
	var out []Model
	for _, m := range models {
		if keep(m) {
			out = append(out, m)
		}
	}
	return out
}

// pick returns the best model by better, ties broken by name so the answer is stable.
func pick(models []Model, better func(a, b Model) bool) (Model, bool) {
	if len(models) == 0 {
		return Model{}, false
	}
	sorted := append([]Model(nil), models...)
	sort.SliceStable(sorted, func(i, j int) bool {
		if better(sorted[i], sorted[j]) != better(sorted[j], sorted[i]) {
			return better(sorted[i], sorted[j])
		}
		return sorted[i].Name < sorted[j].Name
	})
	return sorted[0], true
}

// Merge combines the lists of several sources. A model a provider lists that the gateway already
// routes only marks the routed entry as listed; the rest are kept as they are.
func Merge(lists ...[]Model) []Model {
	all := slices.Concat(lists...)
	routed := map[string]int{}
	for i, m := range all {
		if m.Routed {
			routed[identity(m)] = i
		}
	}
	for _, m := range all {
		if i, ok := routed[identity(m)]; ok && !m.Routed {
			all[i].Listed = true
			all[i].MaxInput = max(all[i].MaxInput, m.MaxInput)
		}
	}
	var out []Model
	for _, m := range all {
		if _, dup := routed[identity(m)]; !dup || m.Routed {
			out = append(out, m)
		}
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out
}

// identity is what two sources share when they mean the same model.
func identity(m Model) string { return m.Provider + "\x00" + m.Upstream }
