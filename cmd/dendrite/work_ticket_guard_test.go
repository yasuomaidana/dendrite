package main

import (
	"bytes"
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func planeBoard(t *testing.T) {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case strings.HasSuffix(r.URL.Path, "/states/"):
			_, _ = w.Write([]byte(`{"results":[{"id":"s-prog","name":"In Progress","group":"started"},{"id":"s-verify","name":"To Verify","group":"started"}]}`))
		case strings.HasSuffix(r.URL.Path, "/issues/"):
			_, _ = w.Write([]byte(`{"results":[{"id":"i5","sequence_id":5,"state":"s-verify","name":"[DEN-5] awaiting sign-off"}],"next_page_results":false}`))
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(srv.Close)
	t.Setenv("PLANE_BASE_URL", srv.URL)
	t.Setenv("PLANE_API_KEY", "test-token")
	t.Setenv("PLANE_WORKSPACE_SLUG", "ws")
	t.Setenv("PLANE_PROJECT_ID", "proj")
}

func TestWorkNewRefusesATicketThatIsNotStartable(t *testing.T) {
	planeBoard(t)
	t.Chdir(t.TempDir()) // not a git repository: creating a copy would fail, so a refusal must come first
	var out bytes.Buffer
	err := runWorkNew(context.Background(), "DEN-5", "", "", false, &out)
	if err == nil || !strings.Contains(err.Error(), "awaiting verification") {
		t.Fatalf("err = %v, want a refusal explaining the To Verify state", err)
	}
	err = runWorkNew(context.Background(), "DEN-77", "", "", false, &out)
	if err == nil || !strings.Contains(err.Error(), "does not exist in Plane") {
		t.Fatalf("unknown ticket err = %v", err)
	}
}

func TestWorkNewSkipsTheGuardForOtherNamesAndWhenTurnedOff(t *testing.T) {
	planeBoard(t)
	t.Chdir(t.TempDir())
	var out bytes.Buffer
	// Not ticket-shaped: no Plane check, so the failure is the missing git repository, not the guard.
	if err := runWorkNew(context.Background(), "scratch-idea", "", "", false, &out); err == nil || strings.Contains(err.Error(), "Plane") {
		t.Errorf("err = %v, want a git error", err)
	}
	// Ticket-shaped but bypassed: same.
	if err := runWorkNew(context.Background(), "DEN-5", "", "", true, &out); err == nil || strings.Contains(err.Error(), "awaiting verification") {
		t.Errorf("--no-ticket-guard still checked the ticket: %v", err)
	}
	t.Setenv("DENDRITE_TICKET_GUARD", "off")
	if err := runWorkNew(context.Background(), "DEN-5", "", "", false, &out); err == nil || strings.Contains(err.Error(), "awaiting verification") {
		t.Errorf("DENDRITE_TICKET_GUARD=off still checked the ticket: %v", err)
	}
}
