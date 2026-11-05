package rag

import (
	"bytes"
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"Dendrite/pkg/sidecar"
)

func TestAskUsesTheWorkingDirectoryAndStdoutByDefault(t *testing.T) {
	e, _ := standInAsk(t)
	t.Chdir(ragProject(t))
	res, err := e.Ask(context.Background(), AskRequest{Question: "where is LocateBinary"})
	if err != nil || res.SourceTier != "local-rag" {
		t.Fatalf("res = %+v, err = %v", res, err)
	}
}

func TestAskReportsWhatTheSidecarSaid(t *testing.T) {
	cases := []struct {
		name string
		env  map[string]string
		want string
		tier string
	}{
		{"an abstention without a tier", map[string]string{"STANDIN_ABSTAIN": "untiered"}, "Abstaining", "abstained"},
		{"index warnings", map[string]string{"STANDIN_WARN": "dense coverage is low"}, "[!] dense coverage is low", ""},
		{"an embedding failure", map[string]string{"STANDIN_EMBED_ERROR": "model missing"}, "Embeddings unavailable (model missing)", ""},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			for k, v := range c.env {
				t.Setenv(k, v)
			}
			e, _ := standInAsk(t)
			var out bytes.Buffer
			res, err := e.Ask(context.Background(), AskRequest{Question: "q", ProjectPath: ragProject(t), Output: &out})
			if err != nil {
				t.Fatal(err)
			}
			if !strings.Contains(out.String(), c.want) {
				t.Errorf("output lacks %q:\n%s", c.want, out.String())
			}
			if c.tier != "" && res.SourceTier != c.tier {
				t.Errorf("tier = %q, want %q", res.SourceTier, c.tier)
			}
		})
	}
}

func TestAskReportsAPaidCallThatFailsAfterApproval(t *testing.T) {
	t.Setenv("STANDIN_LOCAL", "fail")
	t.Setenv("STANDIN_PAID", "fail")
	e, _ := standInAsk(t)
	var out bytes.Buffer
	_, err := e.Ask(context.Background(), AskRequest{Question: "q", ProjectPath: ragProject(t), Input: strings.NewReader("y\n"), Output: &out})
	if err == nil || !strings.Contains(err.Error(), "ask:") {
		t.Fatalf("err = %v, want the paid call's failure", err)
	}
}

func TestGatewayEnvNeedsAProjectThatExists(t *testing.T) {
	if got := gatewayEnv(filepath.Join(t.TempDir(), "missing")); got != nil {
		t.Fatalf("gatewayEnv = %v, want nil", got)
	}
}

func TestRemoteAskFailureModes(t *testing.T) {
	ctx := context.Background()
	serve := func(h http.HandlerFunc) string {
		s := httptest.NewServer(h)
		t.Cleanup(s.Close)
		return s.URL
	}
	closed := httptest.NewServer(http.NotFoundHandler())
	closed.Close()

	cases := []struct {
		name string
		url  string
		want string
	}{
		{"an unusable URL", "http://%zz", "failed to construct central RAG request"},
		{"a server that is down", closed.URL, "failed to contact central RAG server"},
		{"an answer that is not JSON", serve(func(w http.ResponseWriter, _ *http.Request) { _, _ = w.Write([]byte("nope")) }), "failed to decode response"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			_, err := (&AskEngine{}).Ask(ctx, AskRequest{Question: "q", ServerURL: c.url})
			if err == nil || !strings.Contains(err.Error(), c.want) {
				t.Fatalf("err = %v, want %q", err, c.want)
			}
		})
	}
	t.Run("the default tier and client", func(t *testing.T) {
		url := serve(func(w http.ResponseWriter, _ *http.Request) { _, _ = w.Write([]byte(`{"answer":"hi"}`)) })
		res, err := (&AskEngine{}).Ask(ctx, AskRequest{Question: "q", ServerURL: url})
		if err != nil || res.SourceTier != "central-rag" {
			t.Fatalf("res = %+v, err = %v", res, err)
		}
	})
}

func TestEverySidecarCallReportsASidecarThatCannotStart(t *testing.T) {
	notRunnable := filepath.Join(t.TempDir(), "dendrite-rag")
	if err := os.WriteFile(notRunnable, []byte("not a program"), 0o644); err != nil {
		t.Fatal(err)
	}
	t.Setenv(sidecar.EnvVar(SidecarBinary), notRunnable)
	r := NewRetriever()
	dir := ragProject(t)
	ctx := context.Background()

	if _, err := r.Retrieve(ctx, dir, "q", SearchOptions{}, false); err == nil {
		t.Error("Retrieve needs a sidecar that starts")
	}
	if err := r.Quality(ctx, dir, "quality.loc", nil, nil); err == nil {
		t.Error("Quality needs a sidecar that starts")
	}
	if _, err := r.Rank(ctx, dir, "q", nil, 0); err == nil {
		t.Error("Rank needs a sidecar that starts")
	}
	if _, err := r.Communities(ctx, dir, CommunitiesOptions{}); err == nil {
		t.Error("graph queries need a sidecar that starts")
	}
}

func TestRetrievalCallsReportASidecarMethodThatFails(t *testing.T) {
	ctx := context.Background()
	dir := ragProject(t)
	for _, method := range []string{"index", "context", "chunks.get"} {
		t.Run(method, func(t *testing.T) {
			t.Setenv("STANDIN_FAIL", method)
			r, _ := standInRetriever(t)
			var err error
			switch method {
			case "context":
				_, err = r.ContextWith(ctx, dir, "q", 4, ContextOptions{}, false)
			case "chunks.get":
				_, err = r.Show(ctx, dir, []string{"a#1"})
			default:
				_, err = r.Retrieve(ctx, dir, "q", SearchOptions{}, false)
			}
			if err == nil || !strings.Contains(err.Error(), "boom") {
				t.Fatalf("err = %v", err)
			}
		})
	}
	t.Run("graph index", func(t *testing.T) {
		t.Setenv("STANDIN_FAIL", "index")
		r, _ := standInRetriever(t)
		if _, err := r.Communities(ctx, dir, CommunitiesOptions{}); err == nil || !strings.Contains(err.Error(), "boom") {
			t.Fatalf("err = %v", err)
		}
	})
}

func TestRetrieveDefaultsToTheWorkingDirectoryAndBudgetsTokens(t *testing.T) {
	t.Setenv("STANDIN_PROTOCOL", "1.20.0")
	r, log := standInRetriever(t)
	r.EmbedBudgetTokens = 500
	t.Chdir(ragProject(t))
	if _, err := r.Retrieve(context.Background(), "", "q", SearchOptions{}, false); err != nil {
		t.Fatal(err)
	}
	calls, _ := os.ReadFile(log)
	if !strings.Contains(string(calls), `"embed_budget_tokens":500`) {
		t.Errorf("the token budget did not reach the sidecar:\n%s", calls)
	}
}

func TestGraphListReportsAnAnswerOfTheWrongShape(t *testing.T) {
	_, err := graphList[Community](context.Background(), standInRetrieverOnly(t), ragProject(t), "graph.communities", "communities", map[string]interface{}{})
	if err != nil {
		t.Fatalf("a normal answer decodes, got %v", err)
	}
	// STANDIN returns objects under "communities"; asking for them as a list of numbers cannot decode.
	if _, err := graphList[int](context.Background(), standInRetrieverOnly(t), ragProject(t), "graph.communities", "communities", map[string]interface{}{}); err == nil {
		t.Fatal("a list of the wrong element type must be an error")
	}
	if got, err := graphList[Community](context.Background(), standInRetrieverOnly(t), ragProject(t), "graph.communities", "absent", map[string]interface{}{}); err != nil || got != nil {
		t.Fatalf("a missing field is an empty list, got %v, %v", got, err)
	}
}

func standInRetrieverOnly(t *testing.T) *Retriever {
	r, _ := standInRetriever(t)
	return r
}

func TestHistoryDefaultsToTenCommits(t *testing.T) {
	r, log := standInRetriever(t)
	if _, err := r.History(context.Background(), ragProject(t), "q", 0); err != nil {
		t.Fatal(err)
	}
	calls, _ := os.ReadFile(log)
	if !strings.Contains(string(calls), `"top_k":10`) {
		t.Errorf("history did not default to 10:\n%s", calls)
	}
}

func TestProjectRootReportsAnUnresolvablePath(t *testing.T) {
	old := absPath
	absPath = func(string) (string, error) { return "", os.ErrNotExist }
	t.Cleanup(func() { absPath = old })
	if _, _, err := projectRoot("anywhere"); err == nil {
		t.Fatal("expected the path error")
	}
}

func TestGatewayEnvIsEmptyWithoutAKey(t *testing.T) {
	t.Setenv("DENDRITE_HOME", t.TempDir())
	t.Setenv("LITELLM_MASTER_KEY", "")
	if got := gatewayEnv(t.TempDir()); got != nil {
		t.Fatalf("gatewayEnv = %v, want nil", got)
	}
}

func TestSidecarThatCannotExecuteIsReportedByEveryCall(t *testing.T) {
	broken := filepath.Join(t.TempDir(), "dendrite-rag")
	if err := os.WriteFile(broken, []byte("not a program"), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv(sidecar.EnvVar(SidecarBinary), broken)
	if _, err := NewRetriever().dial(context.Background()); err == nil || !strings.Contains(err.Error(), broken) {
		t.Fatalf("err = %v, want the binary named", err)
	}
}

func TestGraphQueriesDefaultToTheWorkingDirectoryAndANilRetriever(t *testing.T) {
	t.Chdir(ragProject(t))
	var r *Retriever
	if _, err := r.Communities(context.Background(), "", CommunitiesOptions{}); err != nil {
		t.Fatal(err)
	}
}

func TestGatewayLookupFailureModes(t *testing.T) {
	t.Setenv("LITELLM_PROXY_URL", "")
	if got := gatewayURL(); got != "http://localhost:4000" {
		t.Errorf("default gateway = %q", got)
	}
	ctx := context.Background()

	t.Run("an unusable gateway URL", func(t *testing.T) {
		t.Setenv("LITELLM_PROXY_URL", "http://%zz")
		if _, ok := preferLocalModel(ctx, ""); ok {
			t.Fatal("an unusable URL has no choice")
		}
	})
	for name, h := range map[string]http.HandlerFunc{
		"a gateway that answers an error": func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusInternalServerError) },
		"a gateway that answers garbage":  func(w http.ResponseWriter, _ *http.Request) { _, _ = w.Write([]byte("nope")) },
	} {
		t.Run(name, func(t *testing.T) {
			s := httptest.NewServer(h)
			defer s.Close()
			t.Setenv("LITELLM_PROXY_URL", s.URL)
			if _, ok := preferLocalModel(ctx, ""); ok {
				t.Fatal("an unreadable answer has no choice")
			}
		})
	}
}

func TestPickLocalRanksOllamaCoderAndNamesLast(t *testing.T) {
	clearEndpoints(t)
	zero := 0.0
	model := func(name, backend string) gatewayModel {
		var m gatewayModel
		m.ModelName = name
		m.LiteLLMParams.Model = backend
		m.ModelInfo.Mode = "chat"
		m.ModelInfo.InputCostPerToken = &zero
		return m
	}
	up := func(string) bool { return true }
	// "ollama/..." models without an api_base are Ollama by name; a coder model beats a plain one,
	// and equal candidates fall back to alphabetical order.
	got, ok := pickLocal([]gatewayModel{model("zeta", "ollama/a"), model("alpha-coder", "ollama/b"), model("beta", "ollama/c"), model("alpha", "ollama/d")}, up)
	if !ok || got.Model != "alpha-coder" {
		t.Fatalf("pick = %+v %v, want the coder model", got, ok)
	}
	got, ok = pickLocal([]gatewayModel{model("zeta", "ollama/a"), model("alpha", "ollama/d")}, up)
	if !ok || got.Model != "alpha" {
		t.Fatalf("pick = %+v %v, want alphabetical order", got, ok)
	}
}
