package tracker

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestSyncEditsOnlyChangedLinesInPlace(t *testing.T) {
	path := filepath.Join(t.TempDir(), "DEV_STATUS.md")
	original := strings.Join([]string{
		"# Status",
		"- [ ] **DEN-809**: Local LLM Inference Concurrency Limiter [State: To Verify, Priority: High]",
		"- [ ] **DEN-1014**: feat(agentjobs): per-session isolated worktrees [State: Backlog, Priority: Medium]",
		"  - [ ] **DEN-2**: Legacy (`pkg/x`) *(IN PROGRESS)*",
		"- [ ] **DEN-3**: Legacy todo",
		"- [ ] **DEN-809**: Model Migration Engine [State: Backlog, Priority: High]",
		"",
	}, "\n")
	if err := os.WriteFile(path, []byte(original), 0o644); err != nil {
		t.Fatal(err)
	}
	m := NewMemoryTracker(path)
	if err := m.LoadFromMarkdown(path); err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	for id, st := range map[TicketID]Status{"DEN-1014": StatusInReview, "DEN-2": StatusDone, "DEN-3": StatusInProgress} {
		if err := m.UpdateStatus(ctx, id, st); err != nil {
			t.Fatal(err)
		}
	}
	if err := m.Sync(ctx); err != nil {
		t.Fatal(err)
	}
	data, _ := os.ReadFile(path)
	want := strings.Join([]string{
		"# Status",
		"- [ ] **DEN-809**: Local LLM Inference Concurrency Limiter [State: To Verify, Priority: High]",
		"- [ ] **DEN-1014**: feat(agentjobs): per-session isolated worktrees [State: To Verify, Priority: Medium]",
		"  - [x] **DEN-2**: Legacy (`pkg/x`)",
		"- [ ] **DEN-3**: Legacy todo *(IN PROGRESS)*",
		"- [ ] **DEN-809**: Model Migration Engine [State: Backlog, Priority: High]",
		"",
	}, "\n")
	if string(data) != want {
		t.Fatalf("DEV_STATUS.md after sync:\n%s\nwant:\n%s", data, want)
	}

	// A second Sync without changes leaves the file byte for byte.
	if err := m.Sync(ctx); err != nil {
		t.Fatal(err)
	}
	if again, _ := os.ReadFile(path); string(again) != want {
		t.Fatalf("idempotent sync changed the file:\n%s", again)
	}
}

func TestStatusDisplayName(t *testing.T) {
	cases := map[Status]string{
		StatusDone: "Done", StatusInReview: "To Verify", StatusInProgress: "In Progress",
		StatusTodo: "To Do", StatusBacklog: "Backlog", Status("other"): "Backlog",
	}
	for st, want := range cases {
		if got := StatusDisplayName(st); got != want {
			t.Errorf("StatusDisplayName(%s) = %s, want %s", st, got, want)
		}
	}
}
