package bench

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"
)

// ollamaServer is a small real HTTP server speaking the two Ollama routes the benchmark uses: a
// streaming /api/chat that "thinks" first for some models, and /api/ps with unload requests.
type ollamaServer struct {
	*httptest.Server
	mu       sync.Mutex
	loaded   map[string]bool
	unloaded []string
}

func newOllamaServer(t *testing.T) *ollamaServer {
	t.Helper()
	o := &ollamaServer{loaded: map[string]bool{"other-model": true}}
	o.Server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/ps":
			o.mu.Lock()
			var ms []map[string]string
			for m := range o.loaded {
				ms = append(ms, map[string]string{"name": m})
			}
			o.mu.Unlock()
			_ = json.NewEncoder(w).Encode(map[string]any{"models": ms})
		case "/api/generate": // unload
			var req struct {
				Model     string `json:"model"`
				KeepAlive int    `json:"keep_alive"`
			}
			_ = json.NewDecoder(r.Body).Decode(&req)
			o.mu.Lock()
			if req.KeepAlive == 0 {
				delete(o.loaded, req.Model)
				o.unloaded = append(o.unloaded, req.Model)
			}
			o.mu.Unlock()
			_, _ = w.Write([]byte(`{}`))
		case "/api/chat":
			var req struct {
				Model string `json:"model"`
			}
			_ = json.NewDecoder(r.Body).Decode(&req)
			if strings.HasPrefix(req.Model, "broken") {
				http.Error(w, "model not found", http.StatusNotFound)
				return
			}
			o.mu.Lock()
			o.loaded[req.Model] = true
			o.mu.Unlock()
			enc := json.NewEncoder(w)
			chunk := func(content, thinking string) {
				_ = enc.Encode(map[string]any{"message": map[string]string{"content": content, "thinking": thinking}, "done": false})
				w.(http.Flusher).Flush()
			}
			if strings.HasPrefix(req.Model, "thinker") {
				chunk("", "hmm ")
				time.Sleep(60 * time.Millisecond)
			}
			chunk("Hello ", "")
			chunk("world", "")
			_ = enc.Encode(map[string]any{"message": map[string]string{}, "done": true, "eval_count": 100, "eval_duration": int64(2e9)})
		}
	}))
	t.Cleanup(o.Close)
	return o
}

func prompts() []Prompt {
	return []Prompt{{Name: "q1", System: "sys", User: "context and question"}}
}

func TestExecuteTimesStreamsUnloadsOthersAndSeparatesThinking(t *testing.T) {
	o := newOllamaServer(t)
	var progress []string
	swap := 100.0
	runs, err := Execute(context.Background(), Config{
		OllamaURL: o.URL, Models: []string{"plain", "thinker-x"}, Prompts: prompts(), Runs: 2,
		Sample:   func() Machine { swap += 5; return Machine{SwapUsedMB: swap} },
		Progress: func(s string) { progress = append(progress, s) },
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(runs) != 4 || len(progress) != 4 {
		t.Fatalf("runs = %d, progress lines = %d, want 4 and 4", len(runs), len(progress))
	}
	for _, r := range runs {
		if r.Err != "" || r.Answer != "Hello world" || r.TokPerSec != 50 || r.EvalTokens != 100 {
			t.Errorf("run = %+v", r)
		}
		if r.SwapDeltaMB != 5 {
			t.Errorf("swap delta = %v, want the 5 MB between the two samples", r.SwapDeltaMB)
		}
	}
	plain, thinker := runs[0], runs[2]
	if plain.TTFTMS != plain.TTFCMS {
		t.Errorf("plain model: first token %d vs first word %d, want equal", plain.TTFTMS, plain.TTFCMS)
	}
	if thinker.TTFCMS < thinker.TTFTMS+50 {
		t.Errorf("thinking model: first token %dms, first word %dms; the silent thinking must separate them", thinker.TTFTMS, thinker.TTFCMS)
	}
	if len(o.unloaded) == 0 || o.unloaded[0] != "other-model" {
		t.Errorf("unloaded = %v, the model resident before the benchmark must be evicted first", o.unloaded)
	}
	if o.loaded["other-model"] {
		t.Error("another model stayed resident")
	}
}

func TestExecuteRecordsFailuresAndCancels(t *testing.T) {
	o := newOllamaServer(t)
	runs, err := Execute(context.Background(), Config{OllamaURL: o.URL, Models: []string{"broken-model"}, Prompts: prompts(), Runs: 1})
	if err != nil || len(runs) != 1 || !strings.Contains(runs[0].Err, "404") {
		t.Fatalf("runs = %+v, err = %v, want one failed run naming the 404", runs, err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := Execute(ctx, Config{OllamaURL: o.URL, Models: []string{"plain"}, Prompts: prompts()}); err == nil {
		t.Error("a cancelled context did not stop the benchmark")
	}
	if _, err := Execute(context.Background(), Config{OllamaURL: o.URL}); err == nil {
		t.Error("an empty configuration was accepted")
	}
}

type fixedJudge struct{ score int }

func (j fixedJudge) Score(context.Context, Prompt, string) (int, string, error) {
	return j.score, "because", nil
}

func TestSummarizeAndSelectApplyTheBudgetAndSwapRule(t *testing.T) {
	mk := func(model string, totalMS int64, tps float64, score int, swap float64, err string) Run {
		return Run{Model: model, TotalMS: totalMS, TTFCMS: 1500, TokPerSec: tps, EvalTokens: 400, Score: score, SwapDeltaMB: swap, Err: err}
	}
	runs := []Run{
		mk("small", 12000, 30, 3, 0, ""), mk("small", 14000, 32, 3, 0, ""),
		mk("moe", 30000, 25, 5, 10, ""), mk("moe", 34000, 24, 4, 20, ""),
		mk("huge", 400000, 1, 5, 9000, ""), mk("huge", 380000, 1, 5, 8000, ""),
		mk("flaky", 10000, 40, 5, 0, ""), mk("flaky", 0, 0, 0, 0, "ollama 500"),
		mk("swappy", 20000, 10, 5, 5000, ""), mk("swappy", 22000, 9, 5, 6000, ""),
	}
	sums := Summarize(runs)
	if len(sums) != 5 || sums[0].Model != "small" || sums[1].MeanScore != 4.5 || sums[3].Failures != 1 {
		t.Fatalf("summaries = %+v", sums)
	}
	best, ok := Select(sums, Rule{Budget: 60 * time.Second, MaxSwapGrowthMB: 100})
	if !ok || best.Model != "moe" {
		t.Errorf("60s budget: selected %+v, want moe (best score within budget, no swap, no failures; swappy scores 5 but swaps)", best)
	}
	best, ok = Select(sums, Rule{Budget: 20 * time.Second, MaxSwapGrowthMB: 100})
	if !ok || best.Model != "small" {
		t.Errorf("20s budget: selected %+v, want small", best)
	}
	if _, ok := Select(sums, Rule{Budget: 5 * time.Second, MaxSwapGrowthMB: 100}); ok {
		t.Error("a model qualified under an impossible budget")
	}
	table := Table(sums, Rule{Budget: 60 * time.Second, MaxSwapGrowthMB: 100})
	for _, want := range []string{"small", "moe", "over budget", "swaps", "1 failed"} {
		if !strings.Contains(table, want) {
			t.Errorf("table lacks %q:\n%s", want, table)
		}
	}
}

func TestExecuteScoresAnswersWithTheJudge(t *testing.T) {
	o := newOllamaServer(t)
	runs, err := Execute(context.Background(), Config{OllamaURL: o.URL, Models: []string{"plain"}, Prompts: prompts(), Judge: fixedJudge{4}})
	if err != nil || runs[0].Score != 4 || runs[0].ScoreNote != "because" {
		t.Errorf("runs = %+v, err = %v", runs, err)
	}
}

func TestGatewayJudgeParsesScoresAndSurfacesErrors(t *testing.T) {
	var gotAuth, gotModel string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotAuth = r.Header.Get("Authorization")
		var req struct {
			Model string `json:"model"`
		}
		_ = json.NewDecoder(r.Body).Decode(&req)
		gotModel = req.Model
		switch req.Model {
		case "judge-bad":
			http.Error(w, "unauthorized", http.StatusUnauthorized)
		case "judge-prose":
			fmt.Fprint(w, `{"choices":[{"message":{"content":"no idea"}}]}`)
		default:
			fmt.Fprint(w, `{"choices":[{"message":{"content":"Sure: {\"score\": 4, \"reason\": \"cites the right file\"}"}}]}`)
		}
	}))
	defer srv.Close()
	p := Prompt{User: "ctx and q"}

	score, note, err := GatewayJudge{URL: srv.URL, Key: "k", Model: "judge-ok"}.Score(context.Background(), p, "an answer")
	if err != nil || score != 4 || note != "cites the right file" || gotAuth != "Bearer k" || gotModel != "judge-ok" {
		t.Errorf("score=%d note=%q err=%v auth=%q model=%q", score, note, err, gotAuth, gotModel)
	}
	if s, _, _ := (GatewayJudge{URL: srv.URL, Model: "judge-ok"}).Score(context.Background(), p, "  "); s != 1 {
		t.Errorf("an empty answer scored %d, want 1 without calling the judge", s)
	}
	if _, _, err := (GatewayJudge{URL: srv.URL, Model: "judge-bad"}).Score(context.Background(), p, "x"); err == nil || !strings.Contains(err.Error(), "401") {
		t.Errorf("gateway error = %v", err)
	}
	if _, _, err := (GatewayJudge{URL: srv.URL, Model: "judge-prose"}).Score(context.Background(), p, "x"); err == nil {
		t.Error("a reply without a score was accepted")
	}
}
