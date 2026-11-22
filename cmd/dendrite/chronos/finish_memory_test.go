package chronos

import (
	"Dendrite/pkg/memory"
	"Dendrite/pkg/ticketlock"
	"testing"
)

func TestRememberFinishJournalsTheOutcomeOfATicket(t *testing.T) {
	t.Setenv("DENDRITE_HOME", t.TempDir())
	t.Setenv("DENDRITE_STATE_DIR", t.TempDir())
	rememberFinish(ticketlock.FinishResult{Ticket: "DEN-9", State: "To Verify", Notes: []string{"no [AC-n] criteria"}})

	got, _, _ := memory.DefaultJournal().Pending()
	if len(got) != 1 || got[0].Type != "outcome" || got[0].Ticket != "DEN-9" || got[0].Title != "DEN-9 finished: To Verify" || got[0].Body == "" {
		t.Fatalf("journal = %+v", got)
	}
}

func TestRememberFinishIsSkippedWithoutAHome(t *testing.T) {
	t.Setenv("DENDRITE_HOME", "")
	t.Setenv("HOME", "")
	t.Setenv("DENDRITE_STATE_DIR", t.TempDir())
	rememberFinish(ticketlock.FinishResult{Ticket: "DEN-9", State: "Done"})
	if got, _, _ := memory.DefaultJournal().Pending(); len(got) != 0 {
		t.Fatalf("no home, no journal: %+v", got)
	}
}
