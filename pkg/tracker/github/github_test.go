package github

import (
	"context"
	"strings"
	"testing"

	"Dendrite/pkg/tracker"
)

type mockExecutor struct {
	history []string
	calls   map[string]string
}

func newMockExecutor() *mockExecutor {
	return &mockExecutor{
		history: make([]string, 0),
		calls:   make(map[string]string),
	}
}

func (m *mockExecutor) Execute(ctx context.Context, name string, args ...string) (string, error) {
	cmd := name + " " + strings.Join(args, " ")
	m.history = append(m.history, cmd)
	if resp, ok := m.calls[cmd]; ok {
		return resp, nil
	}
	// Default mock behavior for issue creation
	if strings.Contains(cmd, "issue create") {
		return "https://github.com/yasuomaidana/dendrite/issues/42", nil
	}
	return "", nil
}

func TestBuildIssueBody(t *testing.T) {
	ticket := &tracker.RoutingTicket{
		ID:          "DEN-702",
		Title:       "Ticket Domain Model",
		Description: "Detailed description of ticket domain model",
		Priority:    "P0_TOP_BLOCKER",
		Difficulty:  tracker.TierL2Low,
		DependsOn:   []string{"DEN-701"},
		Blocks:      []string{"DEN-703"},
	}

	idMap := map[string]int{"DEN-701": 10, "DEN-703": 12}
	body := BuildIssueBody(ticket, idMap)

	if !strings.Contains(body, "Detailed description") {
		t.Errorf("expected body to contain description")
	}
	if !strings.Contains(body, "Depends on #10 (`DEN-701`)") {
		t.Errorf("expected body to link dependency #10: %s", body)
	}
	if !strings.Contains(body, "Blocks #12 (`DEN-703`)") {
		t.Errorf("expected body to link blocked #12: %s", body)
	}
}

func TestGitHubClient_ProvisionLabels(t *testing.T) {
	mock := newMockExecutor()
	client := NewGitHubClient("yasuomaidana/dendrite").WithExecutor(mock)

	err := client.ProvisionLabels(context.Background(), "vcs", "tracker")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(mock.history) < 3 {
		t.Fatalf("expected at least 3 label create commands, got %d", len(mock.history))
	}
}

func TestGitHubClient_CreateOrUpdateIssue(t *testing.T) {
	mock := newMockExecutor()
	client := NewGitHubClient("yasuomaidana/dendrite").WithExecutor(mock)

	tkt := &tracker.RoutingTicket{
		ID:            "DEN-701",
		Title:         "Git Branch Guard",
		Priority:      "P0_TOP_BLOCKER",
		LocalFeasible: true,
		RequiresAudit: true,
		Module:        "vcs",
	}

	num, err := client.CreateOrUpdateIssue(context.Background(), tkt, map[string]int{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if num != 42 {
		t.Fatalf("expected issue #42, got %d", num)
	}
}

func TestGitHubClient_SyncTickets(t *testing.T) {
	mock := newMockExecutor()
	client := NewGitHubClient("yasuomaidana/dendrite").WithExecutor(mock)

	tickets := []*tracker.RoutingTicket{
		{ID: "DEN-701", Title: "Guard", Module: "vcs"},
		{ID: "DEN-702", Title: "Model", Module: "tracker", DependsOn: []string{"DEN-701"}},
	}

	synced, err := client.SyncTickets(context.Background(), tickets)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(synced) != 2 {
		t.Fatalf("expected 2 tickets synced, got %d", len(synced))
	}
}

func TestGitHubClient_CloseIssueAndRemoveLabel(t *testing.T) {
	mock := newMockExecutor()
	client := NewGitHubClient("yasuomaidana/dendrite").WithExecutor(mock)

	if err := client.CloseIssue(context.Background(), 42, "Done"); err != nil {
		t.Fatalf("close issue failed: %v", err)
	}
	if err := client.RemoveLabel(context.Background(), 42, "needs-vendor-audit"); err != nil {
		t.Fatalf("remove label failed: %v", err)
	}
}
