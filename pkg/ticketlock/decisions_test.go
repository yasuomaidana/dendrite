package ticketlock

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// askEvent is a PostToolUse event for AskUserQuestion, with the response in the shape Claude Code records.
const askEvent = `{"session_id":"s-9","cwd":"%CWD%","tool_name":"AskUserQuestion",
 "tool_input":{"questions":[
   {"question":"Which model?","header":"Model","options":[{"label":"Keep"},{"label":"Merge"}]},
   {"question":"Done rights?","header":"Done","options":[{"label":"Human"},{"label":"Auto"}]}]},
 "tool_response":{"questions":[],"answers":{"Which model?":"Keep","Done rights?":"Auto"}}}`

func parse(t *testing.T, raw, cwd string) HookInput {
	t.Helper()
	in, ok := ParseHookInput(strings.NewReader(strings.ReplaceAll(raw, "%CWD%", cwd)))
	if !ok {
		t.Fatal("event did not parse")
	}
	return in
}

func TestDecisionsFromTheRealEventShape(t *testing.T) {
	got := DecisionsFromEvent(parse(t, askEvent, "/x"))
	want := []Decision{
		{Question: "Which model?", Answer: "Keep", Options: []string{"Keep", "Merge"}},
		{Question: "Done rights?", Answer: "Auto", Options: []string{"Human", "Auto"}},
	}
	if !equalDecisions(got, want) {
		t.Errorf("got %+v", got)
	}
}

func equalDecisions(a, b []Decision) bool {
	x, _ := json.Marshal(a)
	y, _ := json.Marshal(b)
	return string(x) == string(y)
}

func TestDecisionsFromOddResponses(t *testing.T) {
	question := `"tool_input":{"questions":[{"question":"Q?","options":[{"label":"a"}]}]}`
	cases := map[string]struct {
		event string
		want  []Decision
	}{
		"response is a JSON string": {`{"session_id":"s",` + question + `,"tool_response":"{\"answers\":{\"Q?\":\"a\"}}"}`,
			[]Decision{{Question: "Q?", Answer: "a", Options: []string{"a"}}}},
		"response without answers is kept as text": {`{"session_id":"s",` + question + `,"tool_response":"the user declined"}`,
			[]Decision{{Question: "Q?", Answer: "the user declined", Options: []string{"a"}}}},
		"no response at all": {`{"session_id":"s",` + question + `}`,
			[]Decision{{Question: "Q?", Answer: "(no answer recorded)", Options: []string{"a"}}}},
		"null response": {`{"session_id":"s",` + question + `,"tool_response":null}`,
			[]Decision{{Question: "Q?", Answer: "(no answer recorded)", Options: []string{"a"}}}},
		"answers for questions that were not listed, in order": {`{"session_id":"s","tool_response":{"answers":{"Zed?":"z","Abe?":"b"}}}`,
			[]Decision{{Question: "Abe?", Answer: "b"}, {Question: "Zed?", Answer: "z"}}},
		"nothing to record": {`{"session_id":"s"}`, nil},
	}
	for name, tc := range cases {
		if got := DecisionsFromEvent(parse(t, tc.event, "")); !equalDecisions(got, tc.want) {
			t.Errorf("%s: got %+v, want %+v", name, got, tc.want)
		}
	}
	long := strings.Repeat("x", maxRawAnswer+50)
	got := DecisionsFromEvent(parse(t, `{"session_id":"s",`+question+`,"tool_response":"`+long+`"}`, ""))
	if len(got) != 1 || len(got[0].Answer) != maxRawAnswer+3 || !strings.HasSuffix(got[0].Answer, "...") {
		t.Errorf("a raw answer is cut: %d bytes", len(got[0].Answer))
	}
}

func TestRenderDecisions(t *testing.T) {
	got := renderDecisions([]Decision{{Question: "Q1?", Answer: "A1", Options: []string{"A1", "B1"}, Note: "because"}, {Question: "Q2?", Answer: "A2"}}, "Preface:")
	want := "Preface:\n\nQ: Q1?\nA: A1\nOptions: A1 | B1\nNote: because\n\nQ: Q2?\nA: A2"
	if got != want {
		t.Errorf("got\n%s\nwant\n%s", got, want)
	}
	if got := renderDecisions([]Decision{{Question: "Q?", Answer: "A"}}, ""); got != "Q: Q?\nA: A" {
		t.Errorf("no preface: %q", got)
	}
}

func TestPendingDecisionsAccumulateExpireAndSkipBrokenFiles(t *testing.T) {
	store := &Store{Dir: t.TempDir()}
	if ds, files := store.Pending(time.Time{}); len(ds) != 0 || len(files) != 0 {
		t.Fatal("an empty store has nothing pending")
	}
	if err := store.AddPending("s-1", t0, []Decision{{Question: "one", Answer: "1"}}); err != nil {
		t.Fatal(err)
	}
	if err := store.AddPending("s-1", t0.Add(time.Hour), []Decision{{Question: "two", Answer: "2"}}); err != nil {
		t.Fatal(err)
	}
	if err := store.AddPending("s-2", t0.Add(-48*time.Hour), []Decision{{Question: "old", Answer: "x"}}); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(store.Dir, "decisions", "broken.json"), []byte("{"), 0o644); err != nil {
		t.Fatal(err)
	}
	ds, files := store.Pending(t0.Add(-24 * time.Hour))
	if len(ds) != 2 || ds[0].Question != "one" || ds[1].Question != "two" || len(files) != 1 {
		t.Errorf("pending = %+v from %v: one session file, the old and the broken ones skipped", ds, files)
	}
	blocked := &Store{Dir: filepath.Join(t.TempDir(), "file")}
	if err := os.WriteFile(blocked.Dir, nil, 0o644); err != nil {
		t.Fatal(err)
	}
	if err := blocked.AddPending("s", t0, []Decision{{Question: "q"}}); err == nil {
		t.Error("an unwritable store is an error")
	}
}

func TestSaveDecisionsOnTheClaimedTicketOrLocally(t *testing.T) {
	ctx := context.Background()
	s := newHookRig(t, "To Do")
	in := parse(t, askEvent, s.Dir)

	msg, err := SaveDecisions(ctx, s.Lifecycle, s.boardFunc(), in)
	if err != nil || !strings.Contains(msg, "2 decision(s) kept locally") {
		t.Fatalf("unclaimed: %q, %v", msg, err)
	}
	if pending, _ := s.Store.Pending(time.Time{}); len(pending) != 2 {
		t.Fatalf("pending = %+v", pending)
	}

	res, err := s.Claim(ctx, "DEN-377", ClaimOptions{})
	if err != nil || res.Ticket != "DEN-377" {
		t.Fatal(err)
	}
	got := texts(s.board, "DEN-377")
	last := got[len(got)-1]
	if !strings.HasPrefix(last, "[dendrite:decision] Decisions made before this ticket was claimed:") || !strings.Contains(last, "Q: Which model? A: Keep") {
		t.Fatalf("the claim must post what was decided before it: %q", last)
	}
	if pending, files := s.Store.Pending(time.Time{}); len(pending) != 0 || len(files) != 0 {
		t.Errorf("posted decisions are removed: %+v", pending)
	}

	msg, err = SaveDecisions(ctx, s.Lifecycle, s.boardFunc(), in)
	if err != nil || msg != "2 decision(s) recorded on DEN-377" {
		t.Fatalf("claimed: %q, %v", msg, err)
	}
	if got := texts(s.board, "DEN-377"); !strings.HasPrefix(got[len(got)-1], "[dendrite:decision] Q: Which model?") {
		t.Errorf("comment = %q", got[len(got)-1])
	}

	if msg, err := SaveDecisions(ctx, s.Lifecycle, s.boardFunc(), HookInput{SessionID: "s"}); msg != "" || err != nil {
		t.Errorf("an event with no decisions records nothing: %q, %v", msg, err)
	}
	failing := func() (Board, error) { return Board{}, errors.New("Plane is not configured") }
	if _, err := SaveDecisions(ctx, s.Lifecycle, failing, in); err == nil || !strings.Contains(err.Error(), "decisions not recorded on DEN-377") {
		t.Errorf("no board: %v", err)
	}
	s.board.Fail("POST", 0)
	if _, err := SaveDecisions(ctx, s.Lifecycle, s.boardFunc(), in); err == nil {
		t.Error("a failed post is an error")
	}
}

func TestSaveDecisionsWithoutAClaimNeedsAWritableStore(t *testing.T) {
	s := newHookRig(t, "To Do")
	s.Store = &Store{Dir: filepath.Join(t.TempDir(), "file")}
	if err := os.WriteFile(s.Store.Dir, nil, 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := SaveDecisions(context.Background(), s.Lifecycle, s.boardFunc(), parse(t, askEvent, s.Dir)); err == nil {
		t.Error("decisions that cannot be kept are an error")
	}
}

func TestClaimKeepsPendingDecisionsWhenThePostFailsAndIgnoresStaleOnes(t *testing.T) {
	ctx := context.Background()
	s := newSession(t, map[string]string{"DEN-377": "To Do"})
	_ = s.Store.AddPending("fresh", t0, []Decision{{Question: "q", Answer: "a"}})
	_ = s.Store.AddPending("stale", t0.Add(-PendingDecisionTTL-time.Hour), []Decision{{Question: "old", Answer: "x"}})
	s.board.Fail("comments/", 2) // the claim comment and the plan lookup go through, the decision comment does not
	if _, err := s.Claim(ctx, "DEN-377", ClaimOptions{}); err != nil {
		t.Fatalf("a failed decision post must not fail the claim: %v", err)
	}
	if ds, _ := s.Store.Pending(t0.Add(-PendingDecisionTTL - 2*time.Hour)); len(ds) != 2 {
		t.Errorf("both files stay when nothing was posted: %+v", ds)
	}
}

func TestCommentsReadsAndFilters(t *testing.T) {
	ctx := context.Background()
	s := newSession(t, map[string]string{"DEN-377": "To Do"})
	if _, err := s.Claim(ctx, "DEN-377", ClaimOptions{}); err != nil {
		t.Fatal(err)
	}
	_ = s.Comment(ctx, "DEN-377", "note", "first & <only>")
	_ = s.Comment(ctx, "DEN-377", "decision", "Q: a\nA: b")
	s.board.AddComment("DEN-377", "<p>plain human comment</p>")

	all, err := s.Comments(ctx, "DEN-377", "", 0)
	if err != nil || len(all) != 4 {
		t.Fatalf("%d comments, %v", len(all), err)
	}
	if all[0].Kind != "claim" || all[1].Text == "" || !strings.HasPrefix(all[1].Text, "[dendrite:note] first & <only>") || all[3].Kind != "" {
		t.Errorf("comments = %+v", all)
	}
	if dec, _ := s.Comments(ctx, "DEN-377", "decision", 0); len(dec) != 1 || !strings.Contains(dec[0].Text, "Q: a\nA: b") {
		t.Errorf("decision filter = %+v", dec)
	}
	if last, _ := s.Comments(ctx, "DEN-377", "", 2); len(last) != 2 || last[1].Text != "plain human comment" {
		t.Errorf("last 2 = %+v", last)
	}
	s.board.Fail("GET", 0)
	if _, err := s.Comments(ctx, "DEN-377", "", 0); err == nil || !strings.Contains(err.Error(), "read the comments of DEN-377") {
		t.Errorf("a failing board: %v", err)
	}
}
