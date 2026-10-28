package devtool

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"Dendrite/pkg/tracker"
)

// boardServer is a small real HTTP server speaking the Plane routes the guard uses. Tickets are
// DEN-<n> with the state named in tickets; PATCH requests are recorded.
type boardServer struct {
	*httptest.Server
	mu      sync.Mutex
	patched map[string]string // issue id -> state id
}

func newBoardServer(t *testing.T, tickets map[string]string) *boardServer {
	t.Helper()
	b := &boardServer{patched: map[string]string{}}
	stateIDs := map[string]string{"Backlog": "s-backlog", "To Do": "s-todo", "In Progress": "s-prog", "Blocked": "s-block", "To Verify": "s-verify", "Done": "s-done", "Cancelled": "s-cancel"}
	b.Server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		p := r.URL.Path
		switch {
		case r.Method == http.MethodPatch:
			body, _ := io.ReadAll(r.Body)
			b.mu.Lock()
			parts := strings.Split(strings.Trim(p, "/"), "/")
			b.patched[parts[len(parts)-1]] = string(body)
			b.mu.Unlock()
			_, _ = w.Write([]byte(`{}`))
		case strings.HasSuffix(p, "/states/"):
			_, _ = w.Write([]byte(`{"results":[
				{"id":"s-backlog","name":"Backlog","group":"backlog"},{"id":"s-todo","name":"To Do","group":"unstarted"},
				{"id":"s-prog","name":"In Progress","group":"started"},{"id":"s-block","name":"Blocked","group":"started"},
				{"id":"s-verify","name":"To Verify","group":"started"},{"id":"s-done","name":"Done","group":"completed"},
				{"id":"s-cancel","name":"Cancelled","group":"cancelled"}]}`))
		case strings.HasSuffix(p, "/issues/"):
			var rows []string
			n := 1
			for name, state := range tickets {
				rows = append(rows, fmt.Sprintf(`{"id":"iss-%s","sequence_id":%d,"state":%q,"name":"[%s] work"}`, name, n, stateIDs[state], name))
				n++
			}
			_, _ = w.Write([]byte(`{"results":[` + strings.Join(rows, ",") + `],"next_page_results":false}`))
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(b.Close)
	t.Setenv("PLANE_BASE_URL", b.URL)
	t.Setenv("PLANE_API_KEY", "test-token")
	t.Setenv("PLANE_WORKSPACE_SLUG", "ws")
	t.Setenv("PLANE_PROJECT_ID", "proj")
	return b
}

func (b *boardServer) movedTo(issue string) string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.patched[issue]
}

func TestGuardLifecycleByState(t *testing.T) {
	b := newBoardServer(t, map[string]string{
		"DEN-1": "In Progress", "DEN-2": "To Do", "DEN-3": "Backlog", "DEN-4": "Blocked",
		"DEN-5": "To Verify", "DEN-6": "Done", "DEN-7": "Cancelled",
	})
	ctx := context.Background()
	cases := []struct {
		ticket  string
		mode    GuardMode
		wantErr string // substring; empty means success
		moved   bool
	}{
		{"DEN-1", GuardCommit, "", false},
		{"DEN-2", GuardCommit, "", true},
		{"DEN-3", GuardCommit, "", true},
		{"DEN-4", GuardCommit, "resume it first", false},
		{"DEN-4", GuardStart, "", true},
		{"DEN-5", GuardCommit, "awaiting verification", false},
		{"DEN-6", GuardCommit, "it is finished", false},
		{"DEN-7", GuardStart, "it is finished", false},
		{"DEN-9", GuardCommit, "does not exist in Plane", false},
		{"", GuardCommit, "no ticket", false},
	}
	for _, c := range cases {
		res, err := EnsureTicketInProgress(ctx, "", c.ticket, c.mode, false)
		switch {
		case c.wantErr != "" && (err == nil || !strings.Contains(err.Error(), c.wantErr)):
			t.Errorf("%q mode %d: err = %v, want it to mention %q", c.ticket, c.mode, err, c.wantErr)
		case c.wantErr == "" && err != nil:
			t.Errorf("%q mode %d: unexpected error %v", c.ticket, c.mode, err)
		}
		if res.Moved != c.moved {
			t.Errorf("%q mode %d: moved = %v, want %v", c.ticket, c.mode, res.Moved, c.moved)
		}
		if c.moved && res.After != tracker.StatusInProgress {
			t.Errorf("%q: after = %s", c.ticket, res.After)
		}
	}
	for _, id := range []string{"iss-DEN-2", "iss-DEN-3", "iss-DEN-4"} {
		if !strings.Contains(b.movedTo(id), "s-prog") {
			t.Errorf("%s was not moved to In Progress: %q", id, b.movedTo(id))
		}
	}
	for _, id := range []string{"iss-DEN-1", "iss-DEN-5", "iss-DEN-6", "iss-DEN-7"} {
		if b.movedTo(id) != "" {
			t.Errorf("%s must not be changed: %q", id, b.movedTo(id))
		}
	}
}

func TestGuardDryRunReportsWithoutChangingAnything(t *testing.T) {
	b := newBoardServer(t, map[string]string{"DEN-2": "To Do"})
	res, err := EnsureTicketInProgress(context.Background(), "", "DEN-2", GuardCommit, true)
	if err != nil || res.Moved || !strings.Contains(res.Note, "would start DEN-2") {
		t.Errorf("res = %+v, err = %v", res, err)
	}
	if b.movedTo("iss-DEN-2") != "" {
		t.Error("a dry run changed the ticket")
	}
}

func TestGuardNeverBlocksWhenPlaneCannotBeAsked(t *testing.T) {
	// Not configured: no token.
	t.Setenv("PLANE_API_KEY", "")
	res, err := EnsureTicketInProgress(context.Background(), "", "DEN-2", GuardCommit, false)
	if err != nil || !strings.Contains(res.Skipped, "not configured") {
		t.Errorf("unconfigured: res = %+v, err = %v", res, err)
	}
	// Configured but nothing listens.
	t.Setenv("PLANE_API_KEY", "k")
	t.Setenv("PLANE_BASE_URL", "http://127.0.0.1:1")
	t.Setenv("PLANE_PROJECT_ID", "proj")
	res, err = EnsureTicketInProgress(context.Background(), "", "DEN-2", GuardCommit, false)
	if err != nil || !strings.Contains(res.Skipped, "did not answer") {
		t.Errorf("unreachable: res = %+v, err = %v", res, err)
	}
}

func TestGuardCanBeSwitchedOff(t *testing.T) {
	if !GuardEnabled() {
		t.Fatal("the guard must be on by default")
	}
	for _, v := range []string{"off", "0", "false", "OFF"} {
		t.Setenv(TicketGuardEnv, v)
		if GuardEnabled() {
			t.Errorf("%s=%s did not turn the guard off", TicketGuardEnv, v)
		}
	}
}
