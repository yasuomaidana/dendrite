package planetools_test

import (
	"Dendrite/pkg/agentjobs"
	"Dendrite/pkg/mcpserver/planetools"
	"Dendrite/pkg/rea/ownership"
	"Dendrite/pkg/ticketlock"
	"Dendrite/pkg/tracker/plane"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	. "Dendrite/pkg/mcpserver"
)

// reaServer is a server with the REA tool whose ports see issue and record the AGY job.
func reaServer(t *testing.T, issue ownership.Issue) (*Server, *[]agentjobs.SubmitSpec) {
	t.Helper()
	client, err := plane.NewClient(plane.Config{BaseURL: "http://127.0.0.1:1", WorkspaceSlug: "w", ProjectID: "p", DirectToken: "k"}, nil)
	if err != nil {
		t.Fatal(err)
	}
	var jobs []agentjobs.SubmitSpec
	s := NewServer(ServerConfig{})
	planetools.RegisterReaTools(s, Dependencies{PlaneClient: client}, func(*ticketlock.Lifecycle) ownership.Ports {
		return ownership.Ports{
			Issue:         func(context.Context, string) (ownership.Issue, error) { return issue, nil },
			Comment:       func(context.Context, string, string, string) error { return nil },
			AddLabel:      func(context.Context, string, string) error { return nil },
			ReleaseClaims: func(string) error { return nil },
			Submit:        func(sp agentjobs.SubmitSpec) (string, error) { jobs = append(jobs, sp); return "job-5", nil },
		}
	})
	return s, &jobs
}

func TestReaHandoffToolHandsARefusedTicketToAGY(t *testing.T) {
	s, jobs := reaServer(t, ownership.Issue{Ticket: "DEN-1062", Title: "[DEN-1062] [REA] x"})
	h, ok := s.Handler("dendrite_rea_handoff")
	if !ok {
		t.Fatal("dendrite_rea_handoff is not registered")
	}
	res, err := h(context.Background(), map[string]any{"ticket": "DEN-1062", "reason": "will not analyse it", "worktree": t.TempDir()})
	if err != nil || !strings.Contains(res.Content[0].Text, `"job_id": "job-5"`) {
		t.Fatalf("%v %+v", err, res)
	}
	if len(*jobs) != 1 || !strings.Contains((*jobs)[0].Prompt, "will not analyse it") {
		t.Errorf("jobs %+v", *jobs)
	}
}

func TestReaHandoffToolReportsWhatItCannotDo(t *testing.T) {
	s, _ := reaServer(t, ownership.Issue{Ticket: "DEN-5", Title: "[DEN-5] Fix the parser"})
	h, _ := s.Handler("dendrite_rea_handoff")
	for name, args := range map[string]map[string]any{
		"no ticket":   {"worktree": t.TempDir()},
		"not REA":     {"ticket": "DEN-5", "worktree": t.TempDir()},
		"bad project": {"ticket": "DEN-5", "worktree": writeBadDirectives(t)},
	} {
		if _, err := h(context.Background(), args); err == nil {
			t.Errorf("%s: want an error", name)
		}
	}

	t.Setenv("PLANE_API_KEY", "")
	t.Setenv("PLANE_BASE_URL", "")
	bare := NewServer(ServerConfig{})
	planetools.RegisterReaTools(bare, Dependencies{}, ownership.BoardPorts)
	hb, _ := bare.Handler("dendrite_rea_handoff")
	if _, err := hb(context.Background(), map[string]any{"ticket": "DEN-1"}); err == nil || !strings.Contains(err.Error(), "plane client error") {
		t.Errorf("without a Plane client: %v", err)
	}
}

// writeBadDirectives makes a project directory whose directives file cannot be read.
func writeBadDirectives(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	file := filepath.Join(dir, ".github", "dendrite", "directives.toml")
	if err := os.MkdirAll(filepath.Dir(file), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(file, []byte("not = [valid"), 0o644); err != nil {
		t.Fatal(err)
	}
	return dir
}
