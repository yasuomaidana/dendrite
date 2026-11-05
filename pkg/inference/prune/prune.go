// Package prune finds the installed Ollama models nothing uses and removes them, so a small machine
// does not keep gigabytes of models it never loads. Each removed model comes with the command that
// restores it.
package prune

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"regexp"
	"sort"
	"strings"
)

// Model is one installed model.
type Model struct {
	Name      string `json:"name"`
	SizeBytes int64  `json:"size"`
}

// Installed lists the models Ollama has at url.
func Installed(ctx context.Context, hc *http.Client, url string) ([]Model, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, strings.TrimRight(url, "/")+"/api/tags", nil)
	if err != nil {
		return nil, err
	}
	resp, err := hc.Do(req)
	if err != nil {
		return nil, fmt.Errorf("ollama not reachable at %s: %w", url, err)
	}
	defer resp.Body.Close()
	var out struct {
		Models []Model `json:"models"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return nil, fmt.Errorf("unreadable model list: %w", err)
	}
	sort.Slice(out.Models, func(i, j int) bool { return out.Models[i].SizeBytes > out.Models[j].SizeBytes })
	return out.Models, nil
}

// Remove deletes one model from Ollama.
func Remove(ctx context.Context, hc *http.Client, url, name string) error {
	body, _ := json.Marshal(map[string]string{"model": name})
	req, err := http.NewRequestWithContext(ctx, http.MethodDelete, strings.TrimRight(url, "/")+"/api/delete", bytes.NewReader(body))
	if err != nil {
		return err
	}
	resp, err := hc.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		msg, _ := io.ReadAll(io.LimitReader(resp.Body, 256))
		return fmt.Errorf("ollama %d: %s", resp.StatusCode, strings.TrimSpace(string(msg)))
	}
	return nil
}

var routeModel = regexp.MustCompile(`ollama(?:_chat)?/([A-Za-z0-9._:/-]+)`)

// ModelsInConfig lists the Ollama models a LiteLLM configuration routes to.
func ModelsInConfig(yaml string) []string {
	var out []string
	for _, m := range routeModel.FindAllStringSubmatch(yaml, -1) {
		out = append(out, m[1])
	}
	return out
}

// Embedding reports whether a model is an embedding model, which serves retrieval and is never a candidate.
func Embedding(name string) bool { return strings.Contains(strings.ToLower(name), "embed") }

// Unused is the installed models that are neither in keep, nor routed in the configuration, nor
// embedding models. keep and the configuration are compared by name with and without the ":latest" tag.
func Unused(installed []Model, keep []string) []Model {
	kept := map[string]bool{}
	for _, k := range keep {
		kept[base(k)] = true
	}
	var out []Model
	for _, m := range installed {
		if !kept[base(m.Name)] && !Embedding(m.Name) {
			out = append(out, m)
		}
	}
	return out
}

func base(name string) string {
	return strings.TrimSuffix(strings.ToLower(strings.TrimSpace(name)), ":latest")
}

// RestoreNote is the command that brings a removed model back.
func RestoreNote(name string) string { return "ollama pull " + name }

// GB formats bytes as gigabytes.
func GB(b int64) string { return fmt.Sprintf("%.1f GB", float64(b)/(1<<30)) }
