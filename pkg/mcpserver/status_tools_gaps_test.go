package mcpserver

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestTicketUpdateFallsBackToPlane(t *testing.T) {
	sp := newScriptedPlane(t,
		on("GET", "/states/", 200, `{"results":[{"id":"st-todo","name":"To Do","group":"unstarted"},{"id":"st-prog","name":"In Progress","group":"started"},{"id":"st-done","name":"Done","group":"completed"}]}`),
		on("GET", "/issues/", 200, `{"results":[{"id":"11111111-2222-3333-4444-555555555555","name":"[DEN-5] work","sequence_id":5,"state":"st-todo"}]}`),
		on("PATCH", "/issues/", 200, `{}`),
	)
	srv := toolsWith(t, sp, filepath.Join(t.TempDir(), "missing.md"))
	text, isErr := callTool(t, srv, "dendrite_ticket_update", `{"ticket_id":"DEN-5","status":"in_progress"}`)
	if isErr || !strings.Contains(text, "in Plane service") {
		t.Fatalf("text = %q (error %v)", text, isErr)
	}
}

func TestStatusToolsWithoutPlaneOrWithOddArguments(t *testing.T) {
	noEnvPlane(t)
	srv := toolsWith(t, nil, "")
	if text, isErr := callTool(t, srv, "dendrite_plane_issues", `{}`); !isErr || !strings.Contains(text, "plane client error") {
		t.Errorf("plane issues: %q (error %v)", text, isErr)
	}
	if text, isErr := callTool(t, srv, "dendrite_topology_advisor", `{"nodes":" , ,"}`); isErr {
		t.Errorf("a list of blank nodes falls back to the local one: %s", text)
	}
}

func TestGatewayUpdateReportsAConfigItCannotWrite(t *testing.T) {
	blocker := filepath.Join(t.TempDir(), "file")
	if err := os.WriteFile(blocker, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	srv := toolsWith(t, nil, "")
	args := `{"file":"` + filepath.Join(blocker, "sub", "litellm.yaml") + `","dry_run":false,"use_container_host":false}`
	if text, isErr := callTool(t, srv, "dendrite_gateway_update", args); !isErr || !strings.Contains(text, "gateway update failed") {
		t.Fatalf("text = %q (error %v)", text, isErr)
	}
}
