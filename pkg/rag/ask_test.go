package rag

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
)

// standInAsk returns an AskEngine on the stand-in sidecar and the file its calls go to.
func standInAsk(t *testing.T) (*AskEngine, string) {
	t.Helper()
	r, log := standInRetriever(t)
	return &AskEngine{Retriever: r}, log // no PickLocal: the stand-in answers with its default
}

func askCalls(t *testing.T, log string) []string {
	t.Helper()
	data, _ := os.ReadFile(log)
	var asks []string
	for _, line := range strings.Split(string(data), "\n") {
		if strings.HasPrefix(line, "ask ") {
			asks = append(asks, line)
		}
	}
	return asks
}

func TestAsk_EmptyQuestion(t *testing.T) {
	if _, err := NewAskEngine().Ask(context.Background(), AskRequest{Question: "  "}); !errors.Is(err, ErrEmptyQuestion) {
		t.Fatalf("err = %v", err)
	}
}

func TestAsk_LocalAnswerFromTheSidecar(t *testing.T) {
	e, log := standInAsk(t)
	var out bytes.Buffer
	res, err := e.Ask(context.Background(), AskRequest{Question: "where is LocateBinary", ProjectPath: ragProject(t), Output: &out})
	if err != nil {
		t.Fatal(err)
	}
	if res.SourceTier != "local-rag" || res.Model != "local-coder" || !strings.Contains(res.Answer, "LocateBinary") || res.Escalated {
		t.Fatalf("result = %+v", res)
	}
	if len(res.RetrievedChunks) == 0 || !strings.HasSuffix(res.RetrievedChunks[0].FilePath, "demo.go") {
		t.Fatalf("chunks = %+v", res.RetrievedChunks)
	}
	if tm := res.Timings; tm == nil || tm.Local != 120 || tm.Retrieval != 3 || tm.Embed != 40 {
		t.Fatalf("timings = %+v", res.Timings)
	}
	asks := askCalls(t, log)
	if len(asks) != 1 || !strings.Contains(asks[0], `"corpus":"all"`) || !strings.Contains(asks[0], `"mode":"local"`) ||
		!strings.Contains(asks[0], `"allow_paid":false`) || !strings.Contains(asks[0], `"paid_model":"gemini-2.5-pro"`) {
		t.Fatalf("ask calls = %v", asks)
	}
	calls, _ := os.ReadFile(log)
	if !strings.Contains(string(calls), `"focus":"where is LocateBinary"`) {
		t.Errorf("index call does not focus the question:\n%s", calls)
	}
	if !strings.Contains(out.String(), "covers 50% of the project") {
		t.Errorf("no embedding coverage warning:\n%s", out.String())
	}
}

func TestAsk_ModesAndCorpusReachTheSidecar(t *testing.T) {
	for _, tc := range []struct {
		name string
		req  AskRequest
		want []string
		tier string
	}{
		{"hybrid", AskRequest{HybridMode: true}, []string{`"mode":"hybrid"`}, "hybrid"},
		{"forced paid", AskRequest{ForcePaid: true}, []string{`"mode":"paid"`}, "paid-cloud"},
		{"paid", AskRequest{ForcePaid: true, PaidModel: "gemini-2.5-flash"}, []string{`"mode":"paid"`, `"paid_model":"gemini-2.5-flash"`}, "paid-cloud"},
		{"code only", AskRequest{Corpus: "code"}, []string{`"corpus":"code"`}, "local-rag"},
		{"with docs", AskRequest{Corpus: "all", TopK: 3}, []string{`"corpus":"all"`, `"top_k":3`}, "local-rag"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			e, log := standInAsk(t)
			tc.req.Question, tc.req.ProjectPath, tc.req.Output = "LocateBinary", ragProject(t), &bytes.Buffer{}
			res, err := e.Ask(context.Background(), tc.req)
			if err != nil {
				t.Fatal(err)
			}
			if res.SourceTier != tc.tier {
				t.Errorf("tier = %s, want %s", res.SourceTier, tc.tier)
			}
			asks := askCalls(t, log)
			for _, w := range tc.want {
				if len(asks) != 1 || !strings.Contains(asks[0], w) {
					t.Errorf("ask calls %v lack %s", asks, w)
				}
			}
		})
	}
}

func TestAsk_LocalFailureAsksBeforePaying(t *testing.T) {
	for _, tc := range []struct {
		name, input string
		auto        bool
		asks        int
		declined    bool
	}{
		{"declined", "n\n", false, 1, true},
		{"no answer", "", false, 1, true},
		{"accepted", "y\n", false, 2, false},
		{"approved with -y", "", true, 1, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			e, log := standInAsk(t)
			t.Setenv("STANDIN_LOCAL", "fail")
			var out bytes.Buffer
			res, err := e.Ask(context.Background(), AskRequest{
				Question: "LocateBinary", ProjectPath: ragProject(t), AutoApprovePaid: tc.auto,
				Input: strings.NewReader(tc.input), Output: &out,
			})
			asks := askCalls(t, log)
			if len(asks) != tc.asks {
				t.Fatalf("ask calls = %v", asks)
			}
			if tc.declined {
				if !errors.Is(err, ErrPaidEscalationDeclined) || !strings.Contains(out.String(), "local model down") {
					t.Fatalf("err = %v, out = %s", err, out.String())
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if res.SourceTier != "paid-cloud" || !res.Escalated || !res.EscalationApproved {
				t.Fatalf("result = %+v", res)
			}
			if !strings.Contains(asks[len(asks)-1], `"allow_paid":true`) {
				t.Errorf("paid use not allowed in %s", asks[len(asks)-1])
			}
			if len(asks) == 2 && !strings.Contains(asks[1], `"mode":"paid"`) {
				t.Errorf("after consent the local model must not run again: %s", asks[1])
			}
		})
	}
}

func TestAsk_SingleFileProjectRestrictsRetrievalToIt(t *testing.T) {
	e, log := standInAsk(t)
	file := ragProject(t) + "/pkg/demo/demo.go"
	res, err := e.Ask(context.Background(), AskRequest{Question: "LocateBinary", ProjectPath: file, Output: &bytes.Buffer{}})
	if err != nil {
		t.Fatal(err)
	}
	if asks := askCalls(t, log); len(asks) != 1 || !strings.Contains(asks[0], `"paths":["demo.go"]`) {
		t.Fatalf("ask calls = %v", asks)
	}
	if len(res.RetrievedChunks) == 0 || res.RetrievedChunks[0].FilePath != file {
		t.Fatalf("chunks = %+v", res.RetrievedChunks)
	}
}

func TestAsk_OldSidecarIsRefusedWithAnInstallHint(t *testing.T) {
	e, _ := standInAsk(t)
	t.Setenv("STANDIN_PROTOCOL", "1.4.0")
	_, err := e.Ask(context.Background(), AskRequest{Question: "LocateBinary", ProjectPath: ragProject(t), Output: &bytes.Buffer{}})
	if !errors.Is(err, ErrSidecarTooOld) || !strings.Contains(err.Error(), "cargo install") {
		t.Fatalf("err = %v", err)
	}
}

func TestRetrieverShowReturnsTheTextBehindOutlines(t *testing.T) {
	r, _ := standInRetriever(t)
	dir := ragProject(t)
	pack, err := r.ContextWith(context.Background(), dir, "LocateBinary", 1000, ContextOptions{Corpus: "code", Detail: "map"}, false)
	if err != nil {
		t.Fatal(err)
	}
	if len(pack.Chunks) == 0 || !pack.Chunks[0].Outline || strings.Contains(pack.Chunks[0].Content, "\n") {
		t.Fatalf("map pack = %+v", pack.Chunks)
	}
	full, err := r.Show(context.Background(), dir, []string{pack.Chunks[0].ID})
	if err != nil {
		t.Fatal(err)
	}
	if len(full) != 1 || full[0].Outline || !strings.Contains(full[0].Content, "func LocateBinary") {
		t.Fatalf("shown = %+v", full)
	}
}

func TestAsk_CentralRAG_Delegation(t *testing.T) {
	// Mock Central RAG daemon
	centralServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/rag/ask" {
			http.NotFound(w, r)
			return
		}
		var req AskRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, "bad json", http.StatusBadRequest)
			return
		}

		res := AskResult{
			Question:   req.Question,
			Answer:     "Central RAG Response: Shared environment coordination active.",
			SourceTier: "central-rag",
			Model:      "shared-qwen-72b",
			Engine:     "central-daemon",
			RetrievedChunks: []Chunk{
				{FilePath: "shared/docs/architecture.md", Content: "Central RAG coordinator"},
			},
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(res)
	}))
	defer centralServer.Close()

	engine := NewAskEngine()
	res, err := engine.Ask(context.Background(), AskRequest{
		Question:  "Where does the RAG service live in shared environments?",
		ServerURL: centralServer.URL,
	})
	if err != nil {
		t.Fatalf("central RAG query failed: %v", err)
	}

	if res.SourceTier != "central-rag" {
		t.Errorf("expected SourceTier 'central-rag', got: %s", res.SourceTier)
	}
	if !strings.Contains(res.Answer, "Central RAG Response") {
		t.Errorf("unexpected answer: %s", res.Answer)
	}
}

func TestAsk_CentralRAG_ErrorResponse(t *testing.T) {
	centralServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "central index unavailable", http.StatusInternalServerError)
	}))
	defer centralServer.Close()

	engine := NewAskEngine()
	_, err := engine.Ask(context.Background(), AskRequest{
		Question:  "Query central server",
		ServerURL: centralServer.URL,
	})
	if err == nil || !strings.Contains(err.Error(), "HTTP 500") {
		t.Fatalf("expected HTTP 500 error from central RAG, got: %v", err)
	}
}

func TestGatewayKeyComesFromTheProjectEnvWhenUnset(t *testing.T) {
	dir := ragProject(t)
	if err := os.WriteFile(dir+"/.env", []byte("OTHER=1\nLITELLM_MASTER_KEY=\"sk-test\"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("LITELLM_MASTER_KEY", "")
	if got := gatewayEnv(dir); len(got) != 1 || got[0] != "LITELLM_MASTER_KEY=sk-test" {
		t.Fatalf("gatewayEnv = %v", got)
	}
	t.Setenv("LITELLM_MASTER_KEY", "from-shell")
	if got := gatewayEnv(dir); got != nil {
		t.Fatalf("an exported key must win, got %v", got)
	}
}

func TestAsk_ChosenLocalModelReachesTheSidecar(t *testing.T) {
	e, log := standInAsk(t)
	e.PickLocal = func(context.Context, string) (LocalChoice, bool) {
		return LocalChoice{Model: "local-hermes3-8b", Engine: "ollama"}, true
	}
	var out bytes.Buffer
	res, err := e.Ask(context.Background(), AskRequest{Question: "LocateBinary", ProjectPath: ragProject(t), Output: &out})
	if err != nil {
		t.Fatal(err)
	}
	if asks := askCalls(t, log); len(asks) != 1 || !strings.Contains(asks[0], `"local_model":"local-hermes3-8b"`) {
		t.Fatalf("ask calls = %v", asks)
	}
	if res.Engine != "ollama" || !strings.Contains(out.String(), "Local model: local-hermes3-8b (ollama, GPU)") {
		t.Fatalf("engine = %s, out = %s", res.Engine, out.String())
	}
	// The project's [ask] local_model, an explicit model or the paid tier skip the choice.
	t.Setenv("STANDIN_ASK_LOCAL_MODEL", "project-model")
	called := false
	e.PickLocal = func(context.Context, string) (LocalChoice, bool) { called = true; return LocalChoice{}, false }
	if _, err := e.Ask(context.Background(), AskRequest{Question: "x", ProjectPath: ragProject(t), Output: &out}); err != nil || called {
		t.Fatalf("err = %v, PickLocal called = %v", err, called)
	}
	t.Setenv("STANDIN_ASK_LOCAL_MODEL", "")
	e.PickLocal = func(context.Context, string) (LocalChoice, bool) {
		t.Fatal("PickLocal called")
		return LocalChoice{}, false
	}
	if _, err := e.Ask(context.Background(), AskRequest{Question: "x", ProjectPath: ragProject(t), LocalModel: "mine", Output: &out}); err != nil {
		t.Fatal(err)
	}
	if _, err := e.Ask(context.Background(), AskRequest{Question: "x", ProjectPath: ragProject(t), ForcePaid: true, Output: &out}); err != nil {
		t.Fatal(err)
	}
}

func TestAsk_TierMessagesMatchTheOldPipeline(t *testing.T) {
	e, _ := standInAsk(t)
	var out bytes.Buffer
	if _, err := e.Ask(context.Background(), AskRequest{Question: "LocateBinary", ProjectPath: ragProject(t), HybridMode: true, Output: &out}); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), "[+] Hybrid mode: local draft refined by the paid model (gemini-2.5-pro).") {
		t.Errorf("hybrid: %s", out.String())
	}
	out.Reset()
	if _, err := e.Ask(context.Background(), AskRequest{Question: "LocateBinary", ProjectPath: ragProject(t), ForcePaid: true, Output: &out}); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), "[+] Force-paid flag enabled") {
		t.Errorf("paid: %s", out.String())
	}
}

func TestAsk_ForcedPaidCountsAsApproved(t *testing.T) {
	e, _ := standInAsk(t)
	res, err := e.Ask(context.Background(), AskRequest{Question: "LocateBinary", ProjectPath: ragProject(t), ForcePaid: true, Output: &bytes.Buffer{}})
	if err != nil {
		t.Fatal(err)
	}
	if res.SourceTier != "paid-cloud" || !res.Escalated || !res.EscalationApproved {
		t.Fatalf("result = %+v", res)
	}
}

func TestAsk_HybridKeepsTheDraftWhenRefinementFails(t *testing.T) {
	e, _ := standInAsk(t)
	t.Setenv("STANDIN_PAID", "fail")
	var out bytes.Buffer
	res, err := e.Ask(context.Background(), AskRequest{Question: "LocateBinary", ProjectPath: ragProject(t), HybridMode: true, Output: &out})
	if err != nil {
		t.Fatal(err)
	}
	if res.SourceTier != "local-rag" || !strings.HasPrefix(res.Answer, "local answer") || res.Escalated {
		t.Fatalf("expected the local draft, got %+v", res)
	}
	if !strings.Contains(out.String(), "Paid refinement failed (paid refinement failed: paid model gemini: quota exceeded)") {
		t.Errorf("out = %s", out.String())
	}
}

func TestAsk_PaidFailureIsAnError(t *testing.T) {
	e, _ := standInAsk(t)
	t.Setenv("STANDIN_LOCAL", "fail")
	t.Setenv("STANDIN_PAID", "fail")
	_, err := e.Ask(context.Background(), AskRequest{Question: "LocateBinary", ProjectPath: ragProject(t), AutoApprovePaid: true, Output: &bytes.Buffer{}})
	if err == nil || !strings.Contains(err.Error(), "quota exceeded") {
		t.Fatalf("expected the paid model's error, got %v", err)
	}
}
