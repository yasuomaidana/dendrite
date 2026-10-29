// Package memfit decides which local models this machine can run without starving itself. A model
// "fits" when its weights plus a KV-cache allowance stay inside a memory budget that is shared with
// whatever the other engines already hold in memory. The package has no Dendrite dependencies so
// every caller (gateway, inference engines, the Ollama client, the CLI) can use it.
package memfit

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"time"
)

const (
	// DefaultBudgetPercent is the share of RAM local models may hold. The rest is for the OS,
	// Docker, the IDE and the build tools; a 13 GB model on a 24 GB Mac with those running pushed
	// swap up 4 GB in seconds.
	DefaultBudgetPercent = 50
	// overhead covers the KV cache and runtime buffers on top of the weights.
	overhead = 1.15

	EnvBudgetGB = "DENDRITE_MODEL_MEM_BUDGET_GB"
	EnvGuard    = "DENDRITE_MODEL_GUARD" // "off" disables the call-time guard

	DefaultOllamaEndpoint   = "http://127.0.0.1:11434"
	DefaultOxiLLaMaEndpoint = "http://127.0.0.1:8080"
)

// ErrTooLarge means a model would not fit in the memory budget.
var ErrTooLarge = errors.New("model does not fit this machine's memory budget")

// TotalRAMBytes is the physical memory, 0 when it cannot be read.
func TotalRAMBytes() uint64 {
	switch runtime.GOOS {
	case "darwin":
		out, err := exec.Command("sysctl", "-n", "hw.memsize").Output()
		if err != nil {
			return 0
		}
		n, _ := strconv.ParseUint(strings.TrimSpace(string(out)), 10, 64)
		return n
	case "linux":
		data, err := os.ReadFile("/proc/meminfo")
		if err != nil {
			return 0
		}
		for _, line := range strings.Split(string(data), "\n") {
			if f := strings.Fields(line); len(f) >= 2 && f[0] == "MemTotal:" {
				kb, _ := strconv.ParseUint(f[1], 10, 64)
				return kb * 1024
			}
		}
	}
	return 0
}

// BudgetBytes is what local models may hold in total: DENDRITE_MODEL_MEM_BUDGET_GB when set,
// otherwise DefaultBudgetPercent of RAM. An unreadable RAM size yields a conservative 4 GB.
func BudgetBytes() uint64 {
	if raw := strings.TrimSpace(os.Getenv(EnvBudgetGB)); raw != "" {
		if gb, err := strconv.ParseFloat(raw, 64); err == nil && gb > 0 {
			return uint64(gb * (1 << 30))
		}
	}
	if total := TotalRAMBytes(); total > 0 {
		return total / 100 * DefaultBudgetPercent
	}
	return 4 << 30
}

// Fits reports whether a model of sizeBytes fits next to residentOthers bytes that other loaded
// models already hold. An unknown size (0) is allowed: there is nothing to judge it by.
func Fits(sizeBytes int64, budget, residentOthers uint64) (bool, string) {
	if sizeBytes <= 0 {
		return true, "size unknown"
	}
	need := uint64(float64(sizeBytes) * overhead)
	avail := uint64(0)
	if budget > residentOthers {
		avail = budget - residentOthers
	}
	if need <= avail {
		return true, ""
	}
	return false, fmt.Sprintf("needs %s, %s available of a %s budget (%s held by other loaded models)",
		GB(need), GB(avail), GB(budget), GB(residentOthers))
}

// GB formats bytes as gigabytes.
func GB(b uint64) string { return fmt.Sprintf("%.1f GB", float64(b)/(1<<30)) }

// Model is one installed model and whether it can run here.
type Model struct {
	Name      string `json:"name"`
	Engine    string `json:"engine"` // "ollama" or "oxillama"
	SizeBytes int64  `json:"size_bytes"`
	Resident  bool   `json:"resident"` // currently loaded in memory
	Fits      bool   `json:"fits"`
	Reason    string `json:"reason,omitempty"`
}

// Report is the memory picture of this machine's local engines.
type Report struct {
	TotalRAMBytes   uint64  `json:"total_ram_bytes"`
	BudgetBytes     uint64  `json:"budget_bytes"`
	ResidentBytes   uint64  `json:"resident_bytes"`
	Models          []Model `json:"models"`
	OllamaReachable bool    `json:"ollama_reachable"`
	OxiReachable    bool    `json:"oxillama_reachable"`
}

// Assess lists the models installed on the local Ollama and OxiLLaMa and marks which fit. An
// engine that is not running contributes nothing. Models are sorted largest first within an engine.
func Assess(ctx context.Context, ollamaEndpoint, oxiEndpoint string) Report {
	hc := &http.Client{Timeout: 3 * time.Second}
	if ollamaEndpoint == "" {
		ollamaEndpoint = DefaultOllamaEndpoint
	}
	if oxiEndpoint == "" {
		oxiEndpoint = DefaultOxiLLaMaEndpoint
	}
	r := Report{TotalRAMBytes: TotalRAMBytes(), BudgetBytes: BudgetBytes()}

	tags, tagsErr := ollamaTags(ctx, hc, ollamaEndpoint)
	loaded, _ := ollamaLoaded(ctx, hc, ollamaEndpoint)
	r.OllamaReachable = tagsErr == nil
	oxiName, oxiSize, oxiErr := oxiServed(ctx, hc, oxiEndpoint)
	r.OxiReachable = oxiErr == nil

	var ollamaResident uint64
	for _, sz := range loaded {
		ollamaResident += uint64(sz)
	}
	var oxiResident uint64
	if r.OxiReachable && oxiSize > 0 {
		oxiResident = uint64(oxiSize)
	}
	r.ResidentBytes = ollamaResident + oxiResident

	for name, sz := range tags {
		_, isLoaded := loaded[name]
		// Ollama evicts its own loaded models to make room, so only the other engine's weights
		// count against an Ollama model.
		fits, why := Fits(sz, r.BudgetBytes, oxiResident)
		r.Models = append(r.Models, Model{Name: name, Engine: "ollama", SizeBytes: sz, Resident: isLoaded, Fits: fits, Reason: why})
	}
	if r.OxiReachable {
		// OxiLLaMa already holds its model; it fits when Ollama's loaded models leave room for it.
		fits, why := Fits(oxiSize, r.BudgetBytes, ollamaResident)
		r.Models = append(r.Models, Model{Name: oxiName, Engine: "oxillama", SizeBytes: oxiSize, Resident: true, Fits: fits, Reason: why})
	}
	sort.SliceStable(r.Models, func(i, j int) bool {
		if r.Models[i].Engine != r.Models[j].Engine {
			return r.Models[i].Engine < r.Models[j].Engine
		}
		return r.Models[i].SizeBytes > r.Models[j].SizeBytes
	})
	return r
}

// Find returns the report entry for an engine and model name.
func (r Report) Find(engine, name string) (Model, bool) {
	for _, m := range r.Models {
		if m.Engine == engine && (m.Name == name || m.Name == name+":latest") {
			return m, true
		}
	}
	return Model{}, false
}

// BestFitting names the largest installed Ollama model that fits, "" when none does. A larger
// model is the smarter one, and slower is acceptable.
func (r Report) BestFitting(excluding string) string {
	for _, m := range r.Models {
		if m.Engine == "ollama" && m.Fits && m.Name != excluding && !strings.Contains(m.Name, "embed") {
			return m.Name
		}
	}
	return ""
}

// GuardOllama refuses to start loading a model that would starve the machine. A model that is
// already loaded, one of unknown size, an unreachable Ollama and DENDRITE_MODEL_GUARD=off all pass.
func GuardOllama(ctx context.Context, endpoint, model string) error {
	if strings.EqualFold(strings.TrimSpace(os.Getenv(EnvGuard)), "off") || model == "" {
		return nil
	}
	r := Assess(ctx, endpoint, "")
	m, ok := r.Find("ollama", model)
	if !ok || m.Resident || m.Fits {
		return nil
	}
	msg := m.Reason
	if alt := r.BestFitting(model); alt != "" {
		msg += fmt.Sprintf("; use %s instead (or raise %s)", alt, EnvBudgetGB)
	}
	return fmt.Errorf("%w: %s: %s", ErrTooLarge, model, msg)
}

func getJSON(ctx context.Context, hc *http.Client, url string, out any) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return err
	}
	resp, err := hc.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("%s: HTTP %d", url, resp.StatusCode)
	}
	return json.NewDecoder(resp.Body).Decode(out)
}

func ollamaTags(ctx context.Context, hc *http.Client, endpoint string) (map[string]int64, error) {
	var res struct {
		Models []struct {
			Name string `json:"name"`
			Size int64  `json:"size"`
		} `json:"models"`
	}
	if err := getJSON(ctx, hc, strings.TrimRight(endpoint, "/")+"/api/tags", &res); err != nil {
		return nil, err
	}
	out := make(map[string]int64, len(res.Models))
	for _, m := range res.Models {
		out[m.Name] = m.Size
	}
	return out, nil
}

// ollamaLoaded maps the models Ollama currently holds in memory to the bytes they occupy.
func ollamaLoaded(ctx context.Context, hc *http.Client, endpoint string) (map[string]int64, error) {
	var res struct {
		Models []struct {
			Name string `json:"name"`
			Size int64  `json:"size"`
		} `json:"models"`
	}
	if err := getJSON(ctx, hc, strings.TrimRight(endpoint, "/")+"/api/ps", &res); err != nil {
		return nil, err
	}
	out := make(map[string]int64, len(res.Models))
	for _, m := range res.Models {
		out[m.Name] = m.Size
	}
	return out, nil
}

// oxiServed returns the model OxiLLaMa serves and the size of its GGUF file under
// $HOME/.oxillama/models (0 when the file cannot be matched).
func oxiServed(ctx context.Context, hc *http.Client, endpoint string) (string, int64, error) {
	var res struct {
		Data []struct {
			ID string `json:"id"`
		} `json:"data"`
	}
	if err := getJSON(ctx, hc, strings.TrimRight(endpoint, "/")+"/v1/models", &res); err != nil {
		return "", 0, err
	}
	if len(res.Data) == 0 {
		return "", 0, errors.New("oxillama serves no model")
	}
	id := res.Data[0].ID
	return id, ggufSize(id), nil
}

// ggufSize finds the GGUF whose directory name matches the model id (qwen3-8b-q4_k_m matches
// qwen3_8b_q4km); with a single model installed that one is used.
func ggufSize(id string) int64 {
	home, err := os.UserHomeDir()
	if err != nil {
		return 0
	}
	files, _ := filepath.Glob(filepath.Join(home, ".oxillama", "models", "*", "*.gguf"))
	norm := func(s string) string {
		return strings.NewReplacer("-", "", "_", "", ".", "", ":", "").Replace(strings.ToLower(s))
	}
	var only int64
	for _, f := range files {
		info, err := os.Stat(f)
		if err != nil {
			continue
		}
		only = info.Size()
		if norm(filepath.Base(filepath.Dir(f))) == norm(id) {
			return info.Size()
		}
	}
	if len(files) == 1 {
		return only
	}
	return 0
}
