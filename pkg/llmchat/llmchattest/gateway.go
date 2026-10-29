// Package llmchattest runs a real OpenAI-compatible gateway on a local port for tests, so the code under test
// talks to the production llmchat.Client over HTTP instead of to a stand-in.
package llmchattest

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"

	"Dendrite/pkg/llmchat"
)

// Request is what the gateway was asked.
type Request struct {
	System, User string
}

// Gateway answers chat completions with fixed answers, in order (the last one repeats), or with one fixed HTTP
// error.
type Gateway struct {
	// Client talks to this gateway.
	Client llmchat.Client

	mu       sync.Mutex
	requests []Request
}

// New starts a gateway that answers every request with answer.
func New(t *testing.T, answer string) *Gateway { return start(t, http.StatusOK, answer) }

// Sequence starts a gateway that answers the first request with answers[0], the second with answers[1], and so
// on; the last answer repeats.
func Sequence(t *testing.T, answers ...string) *Gateway { return start(t, http.StatusOK, answers...) }

// Failing starts a gateway that answers every request with the HTTP status.
func Failing(t *testing.T, status int) *Gateway { return start(t, status) }

func start(t *testing.T, status int, answers ...string) *Gateway {
	t.Helper()
	g := &Gateway{}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body struct {
			Messages []struct{ Role, Content string } `json:"messages"`
		}
		_ = json.NewDecoder(r.Body).Decode(&body)
		var req Request
		for _, m := range body.Messages {
			if m.Role == "system" {
				req.System = m.Content
			} else {
				req.User = m.Content
			}
		}
		g.mu.Lock()
		g.requests = append(g.requests, req)
		answer := ""
		if len(answers) > 0 {
			answer = answers[min(len(g.requests), len(answers))-1]
		}
		g.mu.Unlock()
		w.WriteHeader(status)
		_ = json.NewEncoder(w).Encode(map[string]any{"choices": []map[string]any{{"message": map[string]string{"content": answer}}}})
	}))
	t.Cleanup(srv.Close)
	g.Client = llmchat.Client{URL: srv.URL, Model: "test-model"}
	return g
}

// Requests is how many requests the gateway has answered.
func (g *Gateway) Requests() int {
	g.mu.Lock()
	defer g.mu.Unlock()
	return len(g.requests)
}

// Last is the most recent request, or the zero Request when there was none.
func (g *Gateway) Last() Request {
	g.mu.Lock()
	defer g.mu.Unlock()
	if len(g.requests) == 0 {
		return Request{}
	}
	return g.requests[len(g.requests)-1]
}
