package llmchat

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"Dendrite/pkg/rag"
)

func gateway(t *testing.T, status int, content string, seen *map[string]any) Client {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if seen != nil {
			var body map[string]any
			_ = json.NewDecoder(r.Body).Decode(&body)
			body["auth"] = r.Header.Get("Authorization")
			*seen = body
		}
		w.WriteHeader(status)
		_ = json.NewEncoder(w).Encode(map[string]any{"choices": []map[string]any{{"message": map[string]string{"content": content}}}})
	}))
	t.Cleanup(srv.Close)
	return Client{URL: srv.URL, Key: "sk-test", Model: "local-coder"}
}

func TestCompleteSendsBothPromptsAndReturnsTheAnswer(t *testing.T) {
	var seen map[string]any
	c := gateway(t, 200, "the answer", &seen)
	got, err := c.Complete(context.Background(), "be brief", "what is 2+2")
	if err != nil || got != "the answer" {
		t.Fatalf("%q, %v", got, err)
	}
	msgs, _ := json.Marshal(seen["messages"])
	if seen["auth"] != "Bearer sk-test" || seen["model"] != "local-coder" || seen["temperature"] != float64(0) ||
		!strings.Contains(string(msgs), "be brief") || !strings.Contains(string(msgs), "what is 2+2") {
		t.Errorf("request = %v", seen)
	}
}

func TestCompleteWithoutKeyOrClientSendsNoAuthorization(t *testing.T) {
	var seen map[string]any
	c := gateway(t, 200, "ok", &seen)
	c.Key, c.HTTP = "", http.DefaultClient
	if _, err := c.Complete(context.Background(), "s", "u"); err != nil || seen["auth"] != "" {
		t.Errorf("%v, auth %v", err, seen["auth"])
	}
}

func TestCompleteFailures(t *testing.T) {
	empty := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { _, _ = w.Write([]byte(`{"choices":[]}`)) }))
	t.Cleanup(empty.Close)
	for name, tc := range map[string]struct {
		c    Client
		want string
	}{
		"http error":        {gateway(t, 503, "", nil), "HTTP 503"},
		"unreachable":       {Client{URL: "http://127.0.0.1:1"}, "connect"},
		"bad url":           {Client{URL: "http://[::1"}, "missing"},
		"empty completions": {Client{URL: empty.URL}, "no answer"},
	} {
		if _, err := tc.c.Complete(context.Background(), "s", "u"); err == nil || !strings.Contains(err.Error(), tc.want) {
			t.Errorf("%s: %v, want %q", name, err, tc.want)
		}
	}
}

func TestGatewayReadsTheEnvironment(t *testing.T) {
	prev := preferLocalModel
	defer func() { preferLocalModel = prev }()
	preferLocalModel = func(_ context.Context, key string) (rag.LocalChoice, bool) {
		return rag.LocalChoice{Model: "local-smart-" + key}, true
	}
	t.Setenv("LITELLM_PROXY_URL", "http://gw:4000/v1/")
	t.Setenv("LITELLM_MASTER_KEY", "k1")
	c := Gateway(t.TempDir())
	if c.URL != "http://gw:4000/v1/chat/completions" || c.Key != "k1" || c.Model != "local-smart-k1" {
		t.Errorf("client = %+v", c)
	}

	t.Setenv("LITELLM_PROXY_URL", "  ")
	preferLocalModel = func(context.Context, string) (rag.LocalChoice, bool) { return rag.LocalChoice{}, false }
	c = Gateway(t.TempDir())
	if c.URL != "http://localhost:4000/v1/chat/completions" || c.Model != rag.DefaultLocalModel {
		t.Errorf("defaults = %+v", c)
	}
}

func TestJSONCutsTheValueOutOfAChattyAnswer(t *testing.T) {
	for name, tc := range map[string]struct {
		in         string
		open, shut byte
		want       string
	}{
		"bare array":      {`[1,2]`, '[', ']', `[1,2]`},
		"code fence":      {"Sure:\n```json\n[{\"a\":1}]\n```\nDone.", '[', ']', `[{"a":1}]`},
		"think block":     {"<think>maybe [wrong] here</think>\n[3]", '[', ']', `[3]`},
		"object":          {`Result: {"k":"v"} ok`, '{', '}', `{"k":"v"}`},
		"multiline think": {"<think>\nstep [1]\nstep 2\n</think>[4]", '[', ']', `[4]`},
	} {
		got, err := JSON(tc.in, tc.open, tc.shut)
		if err != nil || got != tc.want {
			t.Errorf("%s: %q, %v, want %q", name, got, err, tc.want)
		}
	}
	_, err := JSON("I could not decide, sorry. "+strings.Repeat("x", 300), '[', ']')
	if err == nil || !strings.Contains(err.Error(), "JSON array") || !strings.Contains(err.Error(), "I could not decide") || len(err.Error()) > 300 {
		t.Errorf("a failed read quotes the start of the answer: %v", err)
	}
	if _, err := JSON("nothing", '{', '}'); err == nil || !strings.Contains(err.Error(), "JSON object") {
		t.Errorf("object error: %v", err)
	}
}
