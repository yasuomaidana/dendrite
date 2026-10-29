package ticketlock

import (
	"context"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"
)

func TestReClaimClearsTheNeedsInputLabel(t *testing.T) {
	ctx := context.Background()
	s := newSession(t, map[string]string{"DEN-377": "In Progress"})
	if _, err := s.Claim(ctx, "DEN-377", ClaimOptions{}); err != nil {
		t.Fatal(err)
	}
	if _, _, err := s.Board.Svc.PatchLabels(ctx, s.Board.Client, "DEN-377", []string{NeedsInputLabel}, nil); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Claim(ctx, "DEN-377", ClaimOptions{}); err != nil {
		t.Fatal(err)
	}
	if got := s.board.LabelNames("DEN-377"); !reflect.DeepEqual(got, []string{"agent:claude", "tier:7"}) {
		t.Errorf("labels = %v: needs-input must go when the ticket is claimed again", got)
	}
}

func TestClaimAdoptsAPlanMadeBeforeTheClaim(t *testing.T) {
	ctx := context.Background()
	s := newSession(t, map[string]string{"DEN-377": "To Do"})
	if err := s.Plans.Save("unclaimed/s-9", newPlan("- [x] design\n- [ ] build", 1)); err != nil {
		t.Fatal(err)
	}
	res, err := s.Claim(ctx, "DEN-377", ClaimOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if res.Plan == nil || res.Plan.NextStep != "build" || res.Plan.Source != "plane" {
		t.Fatalf("plan = %+v, want the unclaimed plan stored on the ticket", res.Plan)
	}
	if got, err := s.GetPlan(ctx, "DEN-377"); err != nil || got.PendingSteps != 1 {
		t.Errorf("plan on the ticket = %+v, %v", got, err)
	}
	if _, ok := s.Plans.TakeUnclaimed(time.Time{}); ok {
		t.Error("the adopted plan must not stay unclaimed")
	}
}

func TestClaimKeepsTheTicketsOwnPlanOverAnUnclaimedOne(t *testing.T) {
	s := newSession(t, map[string]string{"DEN-377": "To Do"})
	_ = s.Plans.Save("DEN-377", newPlan("- [ ] own", 2))
	_ = s.Plans.Save("unclaimed/s-9", newPlan("- [ ] stray", 1))
	res, err := s.Claim(context.Background(), "DEN-377", ClaimOptions{})
	if err != nil || res.Plan == nil || res.Plan.NextStep != "own" {
		t.Fatalf("plan = %+v, %v", res.Plan, err)
	}
	if _, ok := s.Plans.TakeUnclaimed(time.Time{}); !ok {
		t.Error("the stray plan belongs to another ticket and must stay")
	}
}

func TestTakeUnclaimed(t *testing.T) {
	store := PlanStore{Dir: t.TempDir()}
	if _, ok := store.TakeUnclaimed(time.Time{}); ok {
		t.Fatal("an empty store has nothing to take")
	}
	_ = store.Save("DEN-1", newPlan("- [ ] ticket plan", 1))
	_ = store.Save("unclaimed/old", newPlan("- [ ] old", 1))
	_ = store.Save("unclaimed/new", newPlan("- [ ] new", 1))
	_ = store.Save("unclaimed/older", newPlan("- [ ] older", 1))
	index := filepath.Join(store.Dir, "index")
	now := time.Now()
	for name, age := range map[string]time.Duration{"unclaimed-old.json": 48 * time.Hour, "unclaimed-older.json": 72 * time.Hour, "unclaimed-new.json": time.Hour} {
		if err := os.Chtimes(filepath.Join(index, name), now.Add(-age), now.Add(-age)); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.WriteFile(filepath.Join(index, "unclaimed-broken.json"), []byte("{"), 0o644); err != nil {
		t.Fatal(err)
	}

	p, ok := store.TakeUnclaimed(now.Add(-6 * time.Hour))
	if !ok || p.NextStep != "new" {
		t.Fatalf("took %+v, %v: want the newest recent plan", p, ok)
	}
	if _, ok := store.TakeUnclaimed(now.Add(-6 * time.Hour)); ok {
		t.Error("the plan is gone once taken, and a ticket's plan or a broken file is never taken")
	}
	if p, ok := store.TakeUnclaimed(now.Add(-60 * time.Hour)); !ok || p.NextStep != "old" {
		t.Errorf("with a longer window the older plan is taken: %+v, %v", p, ok)
	}
	if _, err := store.Load("DEN-1"); err != nil {
		t.Errorf("the ticket plan must be untouched: %v", err)
	}
}

func TestCommentClaimed(t *testing.T) {
	ctx := context.Background()
	s := newSession(t, map[string]string{"DEN-377": "To Do"})
	if ticket, err := s.CommentClaimed(ctx, "progress", "nothing claimed"); ticket != "" || err != nil {
		t.Fatalf("no claim posts nothing: %q, %v", ticket, err)
	}
	if got := texts(s.board, "DEN-377"); len(got) != 0 {
		t.Fatalf("comments = %q", got)
	}
	if _, err := s.Claim(ctx, "DEN-377", ClaimOptions{}); err != nil {
		t.Fatal(err)
	}
	ticket, err := s.CommentClaimed(ctx, "progress", "did a thing (abc1234)")
	if err != nil || ticket != "DEN-377" {
		t.Fatalf("posted to %q, %v", ticket, err)
	}
	got := texts(s.board, "DEN-377")
	if len(got) != 2 || !strings.HasPrefix(got[1], "[dendrite:progress] did a thing (abc1234)") {
		t.Errorf("comments = %q", got)
	}
	if _, err := s.CommentClaimed(ctx, "bogus", "x"); err == nil || !strings.Contains(err.Error(), "unknown comment kind") {
		t.Errorf("a bad kind must be refused: %v", err)
	}
}
