// Package bench measures local chat models on the machine they will run on. Token speed on a Mac is
// set by memory bandwidth and by whether the model fits in RAM, so published numbers say little: this
// runs the same grounded prompts through each candidate, one resident model at a time, and records
// speed, time to first token, memory pressure and (optionally) answer quality.
package bench

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"sort"
	"strings"
	"time"
)

// Prompt is one grounded question: the system prompt, and the user message with its context pack.
type Prompt struct {
	Name   string `json:"name"`
	System string `json:"system"`
	User   string `json:"user"`
}

// Run is one generation.
type Run struct {
	Model  string `json:"model"`
	Prompt string `json:"prompt"`
	Index  int    `json:"run"`
	// TTFTMS is the time to the first token of any kind; TTFCMS to the first visible (non-thinking)
	// one. A reasoning model thinks silently before it answers, so only the second is what a user waits for.
	TTFTMS     int64   `json:"ttft_ms"`
	TTFCMS     int64   `json:"ttfc_ms"`
	TotalMS    int64   `json:"total_ms"`
	EvalTokens int     `json:"eval_tokens"`
	TokPerSec  float64 `json:"tok_per_sec"`
	Answer     string  `json:"answer,omitempty"`
	Score      int     `json:"score,omitempty"`
	ScoreNote  string  `json:"score_note,omitempty"`
	// SwapDeltaMB is how much swap grew during the run.
	SwapDeltaMB float64 `json:"swap_delta_mb"`
	Err         string  `json:"error,omitempty"`
}

// Machine is a snapshot of the host's memory and thermal state.
type Machine struct {
	SwapUsedMB float64 `json:"swap_used_mb"`
	FreePct    int     `json:"free_pct"`
	Throttled  bool    `json:"throttled"`
}

// Judge scores an answer from 1 (wrong or ungrounded) to 5 (correct and grounded in the context).
type Judge interface {
	Score(ctx context.Context, p Prompt, answer string) (int, string, error)
}

// Config describes a benchmark.
type Config struct {
	OllamaURL  string
	Models     []string
	Prompts    []Prompt
	Runs       int
	NumPredict int
	NumCtx     int
	Judge      Judge
	// Sample reads the host state; nil disables memory and thermal reporting.
	Sample func() Machine
	// Progress receives one line per finished run; nil is silent.
	Progress func(string)
	Client   *http.Client
}

// Execute runs every model over every prompt Runs times. Before a model starts, every other model is
// unloaded, so a candidate never shares memory with another and swap growth is its own.
func Execute(ctx context.Context, cfg Config) ([]Run, error) {
	if cfg.Runs <= 0 {
		cfg.Runs = 1
	}
	if cfg.NumPredict <= 0 {
		cfg.NumPredict = 600
	}
	if cfg.NumCtx <= 0 {
		cfg.NumCtx = 8192
	}
	if cfg.Client == nil {
		cfg.Client = &http.Client{Timeout: 15 * time.Minute}
	}
	cfg.OllamaURL = strings.TrimRight(cfg.OllamaURL, "/")
	if len(cfg.Models) == 0 || len(cfg.Prompts) == 0 {
		return nil, fmt.Errorf("bench needs at least one model and one prompt")
	}

	var runs []Run
	for _, model := range cfg.Models {
		if err := unloadOthers(ctx, cfg, model); err != nil {
			return runs, err
		}
		// A first, unrecorded generation loads the model so load time is not charged to the first prompt.
		_, _ = generate(ctx, cfg, model, Prompt{Name: "warm-up", User: "Reply with the word ready."}, 8)
		for _, p := range cfg.Prompts {
			for i := 1; i <= cfg.Runs; i++ {
				if ctx.Err() != nil {
					return runs, ctx.Err()
				}
				var before Machine
				if cfg.Sample != nil {
					before = cfg.Sample()
				}
				r, err := generate(ctx, cfg, model, p, cfg.NumPredict)
				r.Model, r.Prompt, r.Index = model, p.Name, i
				if err != nil {
					r.Err = err.Error()
				}
				if cfg.Sample != nil {
					r.SwapDeltaMB = cfg.Sample().SwapUsedMB - before.SwapUsedMB
				}
				if err == nil && cfg.Judge != nil {
					if s, note, jerr := cfg.Judge.Score(ctx, p, r.Answer); jerr == nil {
						r.Score, r.ScoreNote = s, note
					} else {
						r.ScoreNote = "judge failed: " + jerr.Error()
					}
				}
				runs = append(runs, r)
				if cfg.Progress != nil {
					cfg.Progress(fmt.Sprintf("%-22s %-16s run %d: %5.1f tok/s, first word %5.1fs, total %5.1fs, swap %+.0f MB %s",
						model, p.Name, i, r.TokPerSec, float64(r.TTFCMS)/1000, float64(r.TotalMS)/1000, r.SwapDeltaMB, r.Err))
				}
			}
		}
	}
	return runs, nil
}

type chatChunk struct {
	Message struct {
		Content  string `json:"content"`
		Thinking string `json:"thinking"`
	} `json:"message"`
	Done         bool  `json:"done"`
	EvalCount    int   `json:"eval_count"`
	EvalDuration int64 `json:"eval_duration"`
}

// postJSON posts payload to an Ollama route.
func postJSON(ctx context.Context, cfg Config, path string, payload any) (*http.Response, error) {
	body, _ := json.Marshal(payload)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, cfg.OllamaURL+path, bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	return cfg.Client.Do(req)
}

// generate streams one chat completion from Ollama and times it.
func generate(ctx context.Context, cfg Config, model string, p Prompt, numPredict int) (Run, error) {
	var msgs []map[string]string
	if p.System != "" {
		msgs = append(msgs, map[string]string{"role": "system", "content": p.System})
	}
	msgs = append(msgs, map[string]string{"role": "user", "content": p.User})
	start := time.Now()
	resp, err := postJSON(ctx, cfg, "/api/chat", map[string]any{
		"model": model, "messages": msgs, "stream": true, "keep_alive": "10m",
		"options": map[string]any{"num_predict": numPredict, "num_ctx": cfg.NumCtx, "temperature": 0},
	})
	if err != nil {
		return Run{}, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		msg, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
		return Run{}, fmt.Errorf("ollama %d: %s", resp.StatusCode, strings.TrimSpace(string(msg)))
	}

	var r Run
	var answer strings.Builder
	var sawToken, sawContent bool
	sc := bufio.NewScanner(resp.Body)
	sc.Buffer(make([]byte, 0, 64*1024), 4<<20)
	for sc.Scan() {
		var c chatChunk
		if json.Unmarshal(sc.Bytes(), &c) != nil {
			continue
		}
		now := time.Since(start).Milliseconds()
		if !sawToken && (c.Message.Content != "" || c.Message.Thinking != "") {
			sawToken, r.TTFTMS = true, now
		}
		if !sawContent && c.Message.Content != "" {
			sawContent, r.TTFCMS = true, now
		}
		answer.WriteString(c.Message.Content)
		if c.Done {
			r.EvalTokens = c.EvalCount
			if c.EvalDuration > 0 {
				r.TokPerSec = float64(c.EvalCount) / (float64(c.EvalDuration) / 1e9)
			}
		}
	}
	r.TotalMS = time.Since(start).Milliseconds()
	r.Answer = strings.TrimSpace(answer.String())
	if err := sc.Err(); err != nil {
		return r, err
	}
	return r, nil
}

// unloadOthers evicts every loaded model except keep.
func unloadOthers(ctx context.Context, cfg Config, keep string) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, cfg.OllamaURL+"/api/ps", nil)
	if err != nil {
		return err
	}
	resp, err := cfg.Client.Do(req)
	if err != nil {
		return fmt.Errorf("ollama not reachable at %s: %w", cfg.OllamaURL, err)
	}
	defer resp.Body.Close()
	var ps struct {
		Models []struct {
			Name string `json:"name"`
		} `json:"models"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&ps); err != nil {
		return err
	}
	for _, m := range ps.Models {
		if m.Name == keep {
			continue
		}
		if resp, err := postJSON(ctx, cfg, "/api/generate", map[string]any{"model": m.Name, "keep_alive": 0}); err == nil {
			_ = resp.Body.Close()
		}
	}
	return nil
}

// Summary condenses one model's runs.
type Summary struct {
	Model          string  `json:"model"`
	Runs           int     `json:"runs"`
	Failures       int     `json:"failures"`
	MedianTotalMS  int64   `json:"median_total_ms"`
	MedianTTFCMS   int64   `json:"median_first_word_ms"`
	MedianTokPerS  float64 `json:"median_tok_per_sec"`
	MeanScore      float64 `json:"mean_score"`
	Scored         int     `json:"scored"`
	MaxSwapGrowth  float64 `json:"max_swap_growth_mb"`
	MedianEvalToks int     `json:"median_eval_tokens"`
}

// Summarize groups runs by model, in first-seen order.
func Summarize(runs []Run) []Summary {
	var order []string
	by := map[string][]Run{}
	for _, r := range runs {
		if _, ok := by[r.Model]; !ok {
			order = append(order, r.Model)
		}
		by[r.Model] = append(by[r.Model], r)
	}
	var out []Summary
	for _, m := range order {
		s := Summary{Model: m, Runs: len(by[m])}
		var total, ttfc, evals []int64
		var speeds []float64
		var sum float64
		for _, r := range by[m] {
			if r.Err != "" {
				s.Failures++
				continue
			}
			total, ttfc, evals = append(total, r.TotalMS), append(ttfc, r.TTFCMS), append(evals, int64(r.EvalTokens))
			speeds = append(speeds, r.TokPerSec)
			if r.SwapDeltaMB > s.MaxSwapGrowth {
				s.MaxSwapGrowth = r.SwapDeltaMB
			}
			if r.Score > 0 {
				sum += float64(r.Score)
				s.Scored++
			}
		}
		s.MedianTotalMS, s.MedianTTFCMS = medianInt(total), medianInt(ttfc)
		s.MedianEvalToks = int(medianInt(evals))
		s.MedianTokPerS = medianFloat(speeds)
		if s.Scored > 0 {
			s.MeanScore = sum / float64(s.Scored)
		}
		out = append(out, s)
	}
	return out
}

func medianInt(v []int64) int64 {
	if len(v) == 0 {
		return 0
	}
	s := append([]int64(nil), v...)
	sort.Slice(s, func(i, j int) bool { return s[i] < s[j] })
	return s[len(s)/2]
}

func medianFloat(v []float64) float64 {
	if len(v) == 0 {
		return 0
	}
	s := append([]float64(nil), v...)
	sort.Float64s(s)
	return s[len(s)/2]
}

// Rule is the selection constraint: a model qualifies when it never fails, answers within Budget at the
// median, and does not push the machine into swap.
type Rule struct {
	Budget          time.Duration
	MaxSwapGrowthMB float64
}

// Select returns the qualifying model with the best mean score (fastest median wins ties and
// unscored runs), and whether any model qualified.
func Select(sums []Summary, rule Rule) (Summary, bool) {
	var best Summary
	found := false
	for _, s := range sums {
		if s.Failures > 0 || s.MedianTotalMS <= 0 || time.Duration(s.MedianTotalMS)*time.Millisecond > rule.Budget || s.MaxSwapGrowth > rule.MaxSwapGrowthMB {
			continue
		}
		if !found || s.MeanScore > best.MeanScore || (s.MeanScore == best.MeanScore && s.MedianTotalMS < best.MedianTotalMS) {
			best, found = s, true
		}
	}
	return best, found
}

// Table renders summaries for a terminal.
func Table(sums []Summary, rule Rule) string {
	var b strings.Builder
	fmt.Fprintf(&b, "%-24s %5s %9s %11s %9s %7s %10s  %s\n", "MODEL", "RUNS", "TOK/S", "FIRST WORD", "TOTAL", "SCORE", "SWAP GROWTH", "VERDICT")
	for _, s := range sums {
		verdict := "ok"
		switch {
		case s.Failures > 0:
			verdict = fmt.Sprintf("%d failed", s.Failures)
		case time.Duration(s.MedianTotalMS)*time.Millisecond > rule.Budget:
			verdict = "over budget"
		case s.MaxSwapGrowth > rule.MaxSwapGrowthMB:
			verdict = "swaps"
		}
		score := "-"
		if s.Scored > 0 {
			score = fmt.Sprintf("%.1f", s.MeanScore)
		}
		fmt.Fprintf(&b, "%-24s %5d %9.1f %10.1fs %8.1fs %7s %9.0f MB  %s\n", s.Model, s.Runs, s.MedianTokPerS,
			float64(s.MedianTTFCMS)/1000, float64(s.MedianTotalMS)/1000, score, s.MaxSwapGrowth, verdict)
	}
	return b.String()
}
