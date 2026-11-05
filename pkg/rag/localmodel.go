package rag

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"os"
	"sort"
	"strings"
	"sync"
	"time"

	"Dendrite/pkg/inference"
)

// DefaultLocalModel is the gateway alias the sidecar answers with when none is chosen.
const DefaultLocalModel = "local-coder"

// LocalChoice is the local model an answer uses and the engine serving it.
type LocalChoice struct {
	Model  string               `json:"model"`
	Engine inference.EngineType `json:"engine"`
}

// gatewayModel is one entry of LiteLLM's /v1/model/info.
type gatewayModel struct {
	ModelName     string `json:"model_name"`
	LiteLLMParams struct {
		Model   string `json:"model"`
		APIBase string `json:"api_base"`
	} `json:"litellm_params"`
	ModelInfo struct {
		Mode              string   `json:"mode"`
		InputCostPerToken *float64 `json:"input_cost_per_token"`
	} `json:"model_info"`
}

// gatewayURL is the LiteLLM gateway the sidecar calls (LITELLM_PROXY_URL, else localhost:4000).
func gatewayURL() string {
	base := os.Getenv("LITELLM_PROXY_URL")
	if base == "" {
		base = "http://localhost:4000"
	}
	return strings.TrimSuffix(strings.TrimRight(base, "/"), "/v1")
}

// PreferLocalModel picks the gateway's local chat model on the most preferred engine that
// runs on a GPU and is reachable: GPU-capable engines first, OxiLLaMa (CPU prefill) never
// (inference.EnginePreference, inference.GPUAccelerated). Among equals it keeps the
// sidecar's default alias, then coder models. ok is false when the gateway cannot be asked
// or serves no such model; the sidecar's default applies then.
func PreferLocalModel(ctx context.Context, key string) (LocalChoice, bool) {
	cacheKey := gatewayURL() + "\x00" + key
	localChoiceMu.Lock()
	if c, ok := localChoices[cacheKey]; ok && time.Since(c.at) < localChoiceTTL {
		localChoiceMu.Unlock()
		return c.choice, c.ok
	}
	localChoiceMu.Unlock()

	choice, ok := preferLocalModel(ctx, key)
	localChoiceMu.Lock()
	localChoices[cacheKey] = cachedChoice{choice: choice, ok: ok, at: time.Now()}
	localChoiceMu.Unlock()
	return choice, ok
}

// localChoiceTTL is how long a gateway answer is reused. Each lookup asks the gateway for its model
// list and probes engines over TCP (up to 3 s); a process that asks several questions, such as an MCP
// server, should not repeat that for every one.
const localChoiceTTL = time.Minute

type cachedChoice struct {
	choice LocalChoice
	ok     bool
	at     time.Time
}

var (
	localChoiceMu sync.Mutex
	localChoices  = map[string]cachedChoice{}
)

func preferLocalModel(ctx context.Context, key string) (LocalChoice, bool) {
	ctx, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, gatewayURL()+"/v1/model/info", nil)
	if err != nil {
		return LocalChoice{}, false
	}
	if key != "" {
		req.Header.Set("Authorization", "Bearer "+key)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return LocalChoice{}, false
	}
	defer resp.Body.Close()
	var info struct {
		Data []gatewayModel `json:"data"`
	}
	if resp.StatusCode != http.StatusOK || json.NewDecoder(resp.Body).Decode(&info) != nil {
		return LocalChoice{}, false
	}
	return pickLocal(info.Data, reachable)
}

// pickLocal ranks the zero-cost chat models served by GPU-accelerated engines that up
// reports reachable.
func pickLocal(models []gatewayModel, up func(addr string) bool) (LocalChoice, bool) {
	type candidate struct {
		LocalChoice
		addr string
	}
	var cands []candidate
	for _, m := range models {
		cost := m.ModelInfo.InputCostPerToken
		if cost == nil || *cost != 0 || (m.ModelInfo.Mode != "" && m.ModelInfo.Mode != "chat") ||
			strings.Contains(strings.ToLower(m.ModelName+m.LiteLLMParams.Model), "embed") {
			continue
		}
		engine, ok := inference.EngineAt(m.LiteLLMParams.APIBase)
		if !ok && strings.HasPrefix(m.LiteLLMParams.Model, "ollama") {
			engine, ok = inference.EngineOllama, true
		}
		if !ok || !inference.GPUAccelerated(engine) {
			continue
		}
		cands = append(cands, candidate{LocalChoice{m.ModelName, engine}, inference.HostAddress(m.LiteLLMParams.APIBase)})
	}
	rank := func(c candidate) (int, int) {
		switch {
		case c.Model == DefaultLocalModel:
			return inference.EnginePreference(c.Engine), 0
		case strings.Contains(c.Model, "coder"):
			return inference.EnginePreference(c.Engine), 1
		}
		return inference.EnginePreference(c.Engine), 2
	}
	sort.SliceStable(cands, func(i, j int) bool {
		ei, ti := rank(cands[i])
		ej, tj := rank(cands[j])
		if ei != ej {
			return ei < ej
		}
		if ti != tj {
			return ti < tj
		}
		return cands[i].Model < cands[j].Model
	})
	checked := map[string]bool{}
	for _, c := range cands {
		ok, seen := checked[c.addr]
		if !seen {
			ok = c.addr == "" || up(c.addr)
			checked[c.addr] = ok
		}
		if ok {
			return c.LocalChoice, true
		}
	}
	return LocalChoice{}, false
}

// reachable reports whether a TCP connection to addr opens quickly.
func reachable(addr string) bool {
	conn, err := net.DialTimeout("tcp", addr, 300*time.Millisecond)
	if err != nil {
		return false
	}
	_ = conn.Close()
	return true
}

// String names the choice for the terminal.
func (c LocalChoice) String() string {
	return fmt.Sprintf("%s (%s, GPU)", c.Model, c.Engine)
}
