package main

import (
	"bytes"
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func fakeOllama(t *testing.T) (*httptest.Server, *[]string) {
	t.Helper()
	var deleted []string
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/tags":
			_, _ = w.Write([]byte(`{"models":[{"name":"hermes3:8b","size":4900000000},{"name":"big-unused:70b","size":40000000000},{"name":"nomic-embed-text","size":270000000}]}`))
		case "/api/delete":
			buf := make([]byte, 128)
			n, _ := r.Body.Read(buf)
			deleted = append(deleted, string(buf[:n]))
		}
	}))
	t.Cleanup(s.Close)
	return s, &deleted
}

func TestSpikePruneListsRemovesAndRefusesWithoutAConfirmation(t *testing.T) {
	t.Setenv("DENDRITE_HOME", t.TempDir()) // no stored tiers, no gateway config
	t.Setenv("LITELLM_PROXY_URL", "")
	srv, deleted := fakeOllama(t)
	prevYes, prevTerm := globals.Yes, isTerminal
	t.Cleanup(func() { globals.Yes, isTerminal = prevYes, prevTerm })

	globals.Yes, isTerminal = false, func(io.Reader) bool { return false }
	var out bytes.Buffer
	if err := runSpikePrune(context.Background(), srv.URL, nil, strings.NewReader(""), &out); err == nil || len(*deleted) != 0 {
		t.Errorf("no terminal and no -y must refuse: %v %v", err, *deleted)
	}
	if !strings.Contains(out.String(), "big-unused:70b") || !strings.Contains(out.String(), "ollama pull big-unused:70b") || strings.Contains(out.String(), "hermes3") || strings.Contains(out.String(), "embed") {
		t.Errorf("listing:\n%s", out.String())
	}

	isTerminal = func(io.Reader) bool { return true }
	out.Reset()
	if err := runSpikePrune(context.Background(), srv.URL, nil, strings.NewReader("n\n"), &out); err != nil || len(*deleted) != 0 || !strings.Contains(out.String(), "Left as it was") {
		t.Errorf("declined: %v %v\n%s", err, *deleted, out.String())
	}
	out.Reset()
	if err := runSpikePrune(context.Background(), srv.URL, nil, strings.NewReader("yes\n"), &out); err != nil || len(*deleted) != 1 || !strings.Contains(out.String(), "Reclaimed 37.3 GB") {
		t.Errorf("confirmed: %v %v\n%s", err, *deleted, out.String())
	}

	globals.Yes = true
	out.Reset()
	if err := runSpikePrune(context.Background(), srv.URL, []string{"big-unused:70b"}, strings.NewReader(""), &out); err != nil || !strings.Contains(out.String(), "nothing to remove") {
		t.Errorf("--keep: %v\n%s", err, out.String())
	}
	if err := runSpikePrune(context.Background(), "http://127.0.0.1:1", nil, strings.NewReader(""), &out); err == nil {
		t.Error("an unreachable Ollama must fail")
	}
}
