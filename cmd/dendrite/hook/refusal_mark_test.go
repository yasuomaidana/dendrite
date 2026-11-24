package hook

import (
	"bytes"
	"context"
	"strings"
	"testing"

	"Dendrite/pkg/ticketlock"
)

// suspectedMark reads the suspected-refusal mark of the rig's claim.
func (r *reaRig) suspectedMark(t *testing.T) *ticketlock.SuspectedRefusal {
	t.Helper()
	claim, ok := ticketlock.NewStore().ClaimOfDir(r.repo)
	if !ok {
		t.Fatal("the claim is gone")
	}
	return claim.RefusalSuspected
}

// sessionEnd runs the session-end hook on a transcript and returns what it said.
func (r *reaRig) sessionEnd(t *testing.T, transcript string) string {
	t.Helper()
	var out bytes.Buffer
	if err := runSessionEnd(context.Background(), r.stopEvent(transcript), &out); err != nil {
		t.Fatal(err)
	}
	return out.String()
}

func TestAnExactRefusalIsHandedOverAtOnceWithoutAMark(t *testing.T) { // AC-1
	r := newReaRig(t, "[DEN-377] [REA] x")
	r.claim()
	var out bytes.Buffer
	if err := runStop(context.Background(), r.stopEvent(refusalTranscript), &out); err != nil || len(r.jobs) != 1 {
		t.Fatalf("%v jobs=%d %q", err, len(r.jobs), out.String())
	}
}

func TestAWordingRefusalOnlyMarksTheClaim(t *testing.T) { // AC-2
	r := newReaRig(t, "[DEN-377] [REA] x")
	r.claim()
	r.quietStop(t, wordingTranscript)
	mark := r.suspectedMark(t)
	if mark == nil || !strings.Contains(mark.Excerpt, "can't help") || mark.At.IsZero() {
		t.Fatalf("the claim must carry the mark: %+v", mark)
	}
	r.quietStop(t, wordingTranscript) // a second Stop with the same reply changes nothing
}

func TestALaterReplyThatDoesNotDeclineClearsTheMark(t *testing.T) { // AC-3
	r := newReaRig(t, "[DEN-377] [REA] x")
	r.claim()
	r.quietStop(t, wordingTranscript)
	r.quietStop(t, workingTranscript)
	if m := r.suspectedMark(t); m != nil {
		t.Fatalf("the mark must be cleared: %+v", m)
	}
	if out := r.sessionEnd(t, workingTranscript); strings.Contains(out, "handed to AGY") || len(r.jobs) != 0 {
		t.Errorf("nothing to hand over once the mark is cleared: %q jobs=%d", out, len(r.jobs))
	}
}

func TestSessionEndHandsOverWhileTheMarkStands(t *testing.T) { // AC-4
	r := newReaRig(t, "[DEN-377] [REA] x")
	r.claim()
	r.quietStop(t, wordingTranscript)
	// The transcript the session ends with no longer shows the decline: the stored mark alone decides.
	if out := r.sessionEnd(t, workingTranscript); !strings.Contains(out, "handed to AGY") || len(r.jobs) != 1 {
		t.Fatalf("a standing mark: %q jobs=%d", out, len(r.jobs))
	}
	if !strings.Contains(strings.Join(r.board.Texts("DEN-377"), "\n"), "suspected") {
		t.Errorf("the ticket must record how sure the refusal was: %q", r.board.Texts("DEN-377"))
	}
}

func TestStopWithoutATranscriptKeepsTheMark(t *testing.T) {
	r := newReaRig(t, "[DEN-377] [REA] x")
	r.claim()
	r.quietStop(t, wordingTranscript)
	var out bytes.Buffer
	event := strings.NewReader(`{"session_id":"s-1","cwd":"` + r.repo + `"}`)
	if err := runStop(context.Background(), event, &out); err != nil || r.suspectedMark(t) == nil {
		t.Errorf("a Stop that cannot judge leaves the mark alone: %v", err)
	}
}
