package agentjobs

import (
	"strings"
	"testing"
)

func TestCheckpointCarriesTheClaimedTicket(t *testing.T) {
	cp, err := BuildCheckpoint(NewLedger(t.TempDir()), CheckpointOptions{Worktree: t.TempDir(), Ticket: "  DEN-375 "})
	if err != nil {
		t.Fatal(err)
	}
	if cp.Ticket != "DEN-375" {
		t.Errorf("Ticket = %q, want it trimmed", cp.Ticket)
	}
	if out := RenderCheckpoint(cp); !strings.Contains(out, "- **Ticket**: DEN-375\n") || !strings.Contains(out, `"ticket": "DEN-375"`) {
		t.Errorf("rendered checkpoint must name the ticket in text and JSON:\n%s", out)
	}
}

func TestCheckpointWithoutATicketRendersAsBefore(t *testing.T) {
	cp, err := BuildCheckpoint(NewLedger(t.TempDir()), CheckpointOptions{Worktree: t.TempDir()})
	if err != nil {
		t.Fatal(err)
	}
	if out := RenderCheckpoint(cp); strings.Contains(out, "**Ticket**") || strings.Contains(out, `"ticket":`) {
		t.Errorf("no ticket, no ticket line:\n%s", out)
	}
}
