package agentjobs

import (
	"testing"
	"time"
)

func TestLatestPicksTheNewestJobAndBreaksTiesByID(t *testing.T) {
	t0 := time.Date(2026, 10, 29, 12, 0, 0, 0, time.UTC)
	older, newer, tieA, tieB := &Job{ID: "a", CreatedAt: t0}, &Job{ID: "b", CreatedAt: t0.Add(time.Hour)}, &Job{ID: "c", CreatedAt: t0.Add(time.Hour)}, &Job{ID: "d", CreatedAt: t0.Add(time.Hour)}
	if got := Latest([]*Job{older, newer}); got != newer {
		t.Errorf("newest = %v", got.ID)
	}
	if got := Latest([]*Job{tieA, newer, tieB}); got != tieB {
		t.Errorf("a tie goes to the larger ID, got %v", got.ID)
	}
	if Latest(nil) != nil {
		t.Error("no jobs, no latest")
	}
}
