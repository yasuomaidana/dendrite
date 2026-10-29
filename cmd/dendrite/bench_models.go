package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"Dendrite/pkg/inference/bench"
	"Dendrite/pkg/ollama"
	"Dendrite/pkg/projectid"
	"Dendrite/pkg/rag"
)

// benchSystemPrompt is the system prompt `dendrite ask` sends, so the benchmark measures the real workload.
const benchSystemPrompt = "You are Dendrite, an AI development and infrastructure assistant for this project. " +
	"Answer the user's question accurately, concisely, and helpfully based on the retrieved project context. " +
	"Include concrete CLI commands or instructions if requested."

// benchQuestions are answerable from this repository, each needing a different part of the code.
var benchQuestions = []struct{ Name, Question string }{
	{"sidecar-pool", "How does the sidecar Pool queue requests, and what happens when the queue is full?"},
	{"ticket-guard", "How does dendrite decide whether a commit may proceed for a ticket, and which ticket states are refused?"},
	{"plane-token", "Where does the Plane API token come from and what happens when it is not set?"},
	{"sync-rag", "How does dendrite sync refresh a synapse member's code index, and when is a member skipped?"},
	{"search-guard", "How does the pre-tool hook decide to block a raw recursive search, and how does an agent get past it?"},
	{"ticket-index", "How are Plane tickets made searchable by ask, and how often is the index refreshed?"},
}

func newSpikeBenchCmd() *cobra.Command {
	var (
		models                    string
		runs, numPredict, numCtx  int
		judge, project, ollamaURL string
		out                       string
		budget                    time.Duration
		maxSwap                   float64
	)
	cmd := &cobra.Command{
		Use:   "bench",
		Short: "Benchmark local chat models on this machine (speed, first word, memory pressure, answer quality)",
		Long: "Runs real, grounded dendrite-ask prompts through each model, one resident model at a time, and reports tokens/s,\n" +
			"time to first visible word, swap growth and (with --judge) answer quality, then applies the selection rule:\n" +
			"the best-scoring model that answers within --budget at the median without growing swap.",
		Example: "dendrite spike bench --models hermes3:8b,gpt-oss:20b --judge gemini-3.8-flash-medium --budget 60s",
		Args:    noArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			w := cmd.OutOrStdout()
			list, err := splitModels(models, ollamaURL)
			if err != nil {
				return err
			}
			prompts, err := buildBenchPrompts(cmd.Context(), project)
			if err != nil {
				return err
			}
			cfg := bench.Config{
				OllamaURL: ollamaURL, Models: list, Prompts: prompts, Runs: runs, NumPredict: numPredict, NumCtx: numCtx,
				Sample:   bench.SampleMachine,
				Progress: func(s string) { fmt.Fprintln(w, "    "+s) },
			}
			if judge != "" {
				cfg.Judge = bench.GatewayJudge{URL: gatewayBase(), Key: os.Getenv("LITELLM_MASTER_KEY"), Model: judge}
			}
			if m := bench.SampleMachine(); m.FreePct > 0 {
				fmt.Fprintf(w, "[+] Benchmarking %d model(s) over %d prompts x %d run(s). Start: swap %.0f MB used, %d%% memory free, throttled=%v\n",
					len(list), len(prompts), runs, m.SwapUsedMB, m.FreePct, m.Throttled)
			}
			results, err := bench.Execute(cmd.Context(), cfg)
			if err != nil {
				return err
			}
			rule := bench.Rule{Budget: budget, MaxSwapGrowthMB: maxSwap}
			sums := bench.Summarize(results)
			fmt.Fprintln(w)
			fmt.Fprint(w, bench.Table(sums, rule))
			if best, ok := bench.Select(sums, rule); ok {
				fmt.Fprintf(w, "\n[✓] Selected: %s (median %.1fs, %.1f tok/s, score %.1f)\n", best.Model, float64(best.MedianTotalMS)/1000, best.MedianTokPerS, best.MeanScore)
			} else {
				fmt.Fprintf(w, "\n[!] No model met the rule (median within %s, no swap growth over %.0f MB, no failures)\n", budget, maxSwap)
			}
			path, werr := saveBench(out, results, sums)
			if werr == nil {
				fmt.Fprintf(w, "    Raw runs (answers included): %s\n", path)
			}
			return werr
		},
	}
	f := cmd.Flags()
	f.StringVar(&models, "models", "", "Comma-separated Ollama models (default: every installed chat model)")
	f.IntVar(&runs, "runs", 2, "Runs per prompt and model")
	f.IntVar(&numPredict, "num-predict", 600, "Answer cap in tokens (what dendrite ask uses)")
	f.IntVar(&numCtx, "num-ctx", 8192, "Context window")
	f.StringVar(&judge, "judge", "", "Gateway model that scores each answer 1-5 (e.g. gemini-3.8-flash-medium); omit to skip quality")
	f.StringVar(&project, "project", ".", "Project whose code supplies the context packs")
	f.StringVar(&ollamaURL, "ollama", ollama.DefaultBaseURL, "Ollama base URL")
	f.StringVar(&out, "out", "", "Where to write the raw runs as JSON (default: $DENDRITE_HOME/bench/)")
	f.DurationVar(&budget, "budget", 60*time.Second, "Longest acceptable median answer time")
	f.Float64Var(&maxSwap, "max-swap-growth", 200, "Most swap (MB) a run may add before the model is judged to swap")
	return cmd
}

func gatewayBase() string {
	if u := os.Getenv("LITELLM_PROXY_URL"); u != "" {
		return strings.TrimSuffix(strings.TrimRight(u, "/"), "/v1")
	}
	return "http://localhost:4000"
}

// splitModels returns the requested models, or every installed chat model (embedding models excluded).
func splitModels(csv, ollamaURL string) ([]string, error) {
	if strings.TrimSpace(csv) != "" {
		var out []string
		for _, m := range strings.Split(csv, ",") {
			if m = strings.TrimSpace(m); m != "" {
				out = append(out, m)
			}
		}
		return out, nil
	}
	resp, err := http.Get(strings.TrimRight(ollamaURL, "/") + "/api/tags")
	if err != nil {
		return nil, fmt.Errorf("ollama not reachable at %s: %w", ollamaURL, err)
	}
	defer resp.Body.Close()
	data, _ := io.ReadAll(resp.Body)
	return chatModels(data)
}

// chatModels extracts the chat models from an /api/tags body, smallest first.
func chatModels(tags []byte) ([]string, error) {
	var t struct {
		Models []struct {
			Name string `json:"name"`
			Size int64  `json:"size"`
		} `json:"models"`
	}
	if err := json.Unmarshal(tags, &t); err != nil {
		return nil, err
	}
	sort.Slice(t.Models, func(i, j int) bool { return t.Models[i].Size < t.Models[j].Size })
	var out []string
	for _, m := range t.Models {
		if !strings.Contains(strings.ToLower(m.Name), "embed") {
			out = append(out, m.Name)
		}
	}
	if len(out) == 0 {
		return nil, fmt.Errorf("no chat models installed in Ollama")
	}
	return out, nil
}

// buildBenchPrompts retrieves a context pack for each question exactly as `dendrite ask` would and
// assembles the user message the same way.
func buildBenchPrompts(ctx context.Context, project string) ([]bench.Prompt, error) {
	r := rag.NewRetriever()
	var prompts []bench.Prompt
	for _, q := range benchQuestions {
		res, err := r.Retrieve(ctx, project, q.Question, rag.SearchOptions{TopK: 6}, false)
		if err != nil {
			return nil, fmt.Errorf("retrieve context for %q: %w", q.Name, err)
		}
		prompts = append(prompts, bench.Prompt{Name: q.Name, System: benchSystemPrompt, User: benchUserMessage(q.Question, res.Chunks)})
	}
	return prompts, nil
}

func benchUserMessage(question string, chunks []rag.Chunk) string {
	var b strings.Builder
	b.WriteString("Retrieved Project Context:\n")
	for i, c := range chunks {
		fmt.Fprintf(&b, "[%d] Source: %s (lines %d-%d)\n%s\n---\n", i+1, c.FilePath, c.LineStart, c.LineEnd, strings.TrimSpace(c.Content))
	}
	fmt.Fprintf(&b, "\nQuestion: %s", question)
	return b.String()
}

func saveBench(out string, runs []bench.Run, sums []bench.Summary) (string, error) {
	if out == "" {
		home, err := projectid.HomeDir()
		if err != nil {
			return "", err
		}
		out = filepath.Join(home, "bench", "inference-"+time.Now().Format("20060102-150405")+".json")
	}
	if err := os.MkdirAll(filepath.Dir(out), 0o755); err != nil {
		return "", err
	}
	data, _ := json.MarshalIndent(map[string]any{"summaries": sums, "runs": runs}, "", "  ")
	return out, os.WriteFile(out, data, 0o644)
}
