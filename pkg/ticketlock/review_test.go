package ticketlock

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func gateway(t *testing.T, status int, content string, capture *map[string]any) GatewayReviewer {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if capture != nil {
			var body map[string]any
			_ = json.NewDecoder(r.Body).Decode(&body)
			body["auth"] = r.Header.Get("Authorization")
			*capture = body
		}
		w.WriteHeader(status)
		_ = json.NewEncoder(w).Encode(map[string]any{"choices": []map[string]any{{"message": map[string]string{"content": content}}}})
	}))
	t.Cleanup(srv.Close)
	return GatewayReviewer{URL: srv.URL, Key: "sk-test", Model: "local-coder"}
}

func TestGatewayReviewerAsksTheModelAndReadsTheVerdicts(t *testing.T) {
	var seen map[string]any
	g := gateway(t, 200, "Here you go:\n```json\n[{\"ac\":\"AC-1\",\"pass\":true,\"reason\":\"seen\"},{\"ac\":\"AC-2\",\"pass\":false,\"reason\":\"no test\"}]\n```", &seen)
	got, err := g.Review(context.Background(), ReviewInput{Ticket: "DEN-1", Description: "goal", Criteria: []string{"[AC-1] a", "[AC-2] b"}, Diff: "diff --git"})
	if err != nil || len(got) != 2 || !got[0].Pass || got[1].Pass || got[1].Reason != "no test" {
		t.Fatalf("%+v, %v", got, err)
	}
	prompt, _ := json.Marshal(seen["messages"])
	if seen["auth"] != "Bearer sk-test" || seen["model"] != "local-coder" || !strings.Contains(string(prompt), "[AC-2] b") || !strings.Contains(string(prompt), "diff --git") {
		t.Errorf("request = %v", seen)
	}
}

func TestGatewayReviewerFailures(t *testing.T) {
	ctx := context.Background()
	in := ReviewInput{Ticket: "DEN-1"}
	for name, tc := range map[string]struct {
		g    GatewayReviewer
		want string
	}{
		"http error":        {gateway(t, 503, "", nil), "HTTP 503"},
		"no json array":     {gateway(t, 200, "looks fine to me", nil), "JSON array"},
		"invalid json":      {gateway(t, 200, "[{oops]", nil), "not valid JSON"},
		"unreachable":       {GatewayReviewer{URL: "http://127.0.0.1:1"}, "connect"},
		"bad url":           {GatewayReviewer{URL: "http://[::1"}, "missing"},
		"empty completions": {GatewayReviewer{URL: emptyChoices(t)}, "no answer"},
	} {
		if _, err := tc.g.Review(ctx, in); err == nil || !strings.Contains(err.Error(), tc.want) {
			t.Errorf("%s: %v, want %q", name, err, tc.want)
		}
	}
}

func emptyChoices(t *testing.T) string {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { _, _ = w.Write([]byte(`{"choices":[]}`)) }))
	t.Cleanup(srv.Close)
	return srv.URL
}

func TestGatewayReviewerWithoutKeyOrClientSendsNoAuthorization(t *testing.T) {
	var seen map[string]any
	g := gateway(t, 200, "[]", &seen)
	g.Key, g.HTTP = "", http.DefaultClient
	if v, err := g.Review(context.Background(), ReviewInput{}); err != nil || len(v) != 0 || seen["auth"] != "" {
		t.Errorf("%v, %v, auth %v", v, err, seen["auth"])
	}
}
