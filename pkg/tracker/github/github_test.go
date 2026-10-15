package github

import (
	"context"
	"strings"
	"testing"

	"Dendrite/pkg/tracker"
)

type TestScriptExecutor struct {
	history []string
	calls   map[string]string
}

func newTestScriptExecutor() *TestScriptExecutor {
	return &TestScriptExecutor{
		history: make([]string, 0),
		calls:   make(map[string]string),
	}
}

func (m *TestScriptExecutor) Execute(ctx context.Context, name string, args ...string) (string, error) {
	cmd := name + " " + strings.Join(args, " ")
	m.history = append(m.history, cmd)
	if resp, ok := m.calls[cmd]; ok {
		return resp, nil
	}
	if strings.Contains(cmd, "issue create") {
		return "https://github.com/yasuomaidana/dendrite/issues/42", nil
	}
	return "", nil
}

func TestDefaultCommandExecutor_RealCommand(t *testing.T) {
	exec := &DefaultCommandExecutor{}
	out, err := exec.Execute(context.Background(), "echo", "hello", "world")
	if err != nil {
		t.Fatalf("real echo failed: %v", err)
	}
	if strings.TrimSpace(out) != "hello world" {
		t.Fatalf("expected 'hello world', got '%s'", out)
	}
}

func TestNewGitHubClient_DefaultRepo(t *testing.T) {
	c := NewGitHubClient("")
	if c.Repo != "yasuomaidana/dendrite" {
		t.Fatalf("expected default repo, got %s", c.Repo)
	}
}

func TestBuildIssueBody(t *testing.T) {
	ticket := &tracker.RoutingTicket{
		ID:          "DEN-702",
		Title:       "Ticket Domain Model",
		Description: "Detailed description of ticket domain model",
		Priority:    "P0_TOP_BLOCKER",
		Difficulty:  tracker.TierL2Low,
		DependsOn:   []string{"DEN-701", "DEN-UNKNOWN"},
		Blocks:      []string{"DEN-703", "DEN-UNKNOWN2"},
	}

	idMap := map[string]int{"DEN-701": 10, "DEN-703": 12}
	body := BuildIssueBody(ticket, idMap)

	if !strings.Contains(body, "Detailed description") {
		t.Errorf("expected body to contain description")
	}
	if !strings.Contains(body, "Depends on #10 (`DEN-701`)") {
		t.Errorf("expected body to link dependency #10: %s", body)
	}
	if !strings.Contains(body, "Depends on `DEN-UNKNOWN`") {
		t.Errorf("expected body to link unmapped dependency: %s", body)
	}
	if !strings.Contains(body, "Blocks #12 (`DEN-703`)") {
		t.Errorf("expected body to link blocked #12: %s", body)
	}
	if !strings.Contains(body, "Blocks `DEN-UNKNOWN2`") {
		t.Errorf("expected body to link unmapped blocked: %s", body)
	}
}

func TestGitHubClient_ProvisionLabels(t *testing.T) {
	mock := newTestScriptExecutor()
	client := NewGitHubClient("yasuomaidana/dendrite").WithExecutor(mock)

	err := client.ProvisionLabels(context.Background(), "vcs", "tracker")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(mock.history) < 3 {
		t.Fatalf("expected at least 3 label create commands, got %d", len(mock.history))
	}
}

func TestGitHubClient_CreateOrUpdateIssue_ExistingNumber(t *testing.T) {
	mock := newTestScriptExecutor()
	client := NewGitHubClient("yasuomaidana/dendrite").WithExecutor(mock)

	tkt := &tracker.RoutingTicket{
		ID:             "DEN-701",
		Title:          "Git Branch Guard",
		GitHubIssueNum: 42,
		Priority:       "P0_TOP_BLOCKER",
		LocalFeasible:  true,
		RequiresAudit:  true,
		Module:         "vcs",
	}

	num, err := client.CreateOrUpdateIssue(context.Background(), tkt, map[string]int{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if num != 42 {
		t.Fatalf("expected issue #42, got %d", num)
	}
}

func TestGitHubClient_CreateOrUpdateIssue_FoundViaSearch(t *testing.T) {
	mock := newTestScriptExecutor()
	searchCmd := "gh issue list --repo yasuomaidana/dendrite --search [DEN-702] --json number,title --limit 1"
	mock.calls[searchCmd] = `[{"number":99,"title":"[DEN-702] Existing"}]`

	client := NewGitHubClient("yasuomaidana/dendrite").WithExecutor(mock)

	tkt := &tracker.RoutingTicket{
		ID:            "DEN-702",
		Title:         "Ticket Model",
		Priority:      "urgent",
		LocalFeasible: true,
	}

	num, err := client.CreateOrUpdateIssue(context.Background(), tkt, map[string]int{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if num != 99 {
		t.Fatalf("expected issue #99 from search, got %d", num)
	}
}

func TestGitHubClient_SyncTickets(t *testing.T) {
	mock := newTestScriptExecutor()
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
	mock := newTestScriptExecutor()
	client := NewGitHubClient("yasuomaidana/dendrite").WithExecutor(mock)

	if err := client.CloseIssue(context.Background(), 42, "Done"); err != nil {
		t.Fatalf("close issue failed: %v", err)
	}
	if err := client.RemoveLabel(context.Background(), 42, "needs-vendor-audit"); err != nil {
		t.Fatalf("remove label failed: %v", err)
	}
}
