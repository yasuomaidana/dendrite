package mcpserver

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"Dendrite/pkg/tracker/plane"
)

func TestPlaneTicketSetResolvesRefsAndPatchesState(t *testing.T) {
	var patched, body string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case r.Method == http.MethodGet && strings.HasSuffix(r.URL.Path, "/issues/"):
			_, _ = w.Write([]byte(`{"results":[
				{"id":"iss-fresh","name":"Fresh ticket","sequence_id":324,"state":"st-todo"},
				{"id":"iss-old","name":"[DEN-1014] legacy","sequence_id":300,"state":"st-todo"}]}`))
		case r.Method == http.MethodGet && strings.HasSuffix(r.URL.Path, "/states/"):
			_, _ = w.Write([]byte(`{"results":[{"id":"st-todo","name":"To Do","group":"unstarted"},{"id":"st-prog","name":"In Progress","group":"started"}]}`))
		case r.Method == http.MethodPatch:
			b, _ := io.ReadAll(r.Body)
			patched, body = r.URL.Path, string(b)
			_, _ = w.Write([]byte(`{}`))
		default:
			_, _ = w.Write([]byte(`{}`))
		}
	}))
	defer srv.Close()
	client, err := plane.NewClient(plane.Config{BaseURL: srv.URL, WorkspaceSlug: "ws", ProjectID: "proj", DirectToken: "k"}, srv.Client())
	if err != nil {
		t.Fatal(err)
	}
	s := NewServer(ServerConfig{})
	registerPlaneTicketSetTool(s, Dependencies{PlaneClient: client})
	h, ok := s.Handler("dendrite_plane_ticket_set")
	if !ok {
		t.Fatal("tool not registered")
	}

	if _, err := h(context.Background(), map[string]any{"ticket": "DEN-324", "status": "in progress"}); err != nil {
		t.Fatal(err)
	}
	if !strings.HasSuffix(patched, "/issues/iss-fresh/") || !strings.Contains(body, "st-prog") {
		t.Errorf("patched %q with %q", patched, body)
	}

	_, err = h(context.Background(), map[string]any{"ticket": "DEN-999", "status": "done"})
	if err == nil || !strings.Contains(err.Error(), "DEN-999") {
		t.Errorf("unknown ticket error = %v", err)
	}
	if _, err = h(context.Background(), map[string]any{"ticket": "DEN-324", "status": "bogus"}); err == nil {
		t.Error("bogus status accepted")
	}
}

func TestCoreProfileIncludesPlaneTicketSet(t *testing.T) {
	if !ProfileCore.InProfile("dendrite_plane_ticket_set") {
		t.Fatal("core must expose dendrite_plane_ticket_set")
	}
}
