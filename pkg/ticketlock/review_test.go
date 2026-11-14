package ticketlock

import (
	"Dendrite/pkg/llmchat"
	"Dendrite/pkg/llmchat/llmchattest"
	"Dendrite/pkg/rag"
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// reviewPatch is a change with one hunk about coverage rules and one about the confirmation prompt, plus enough
// unrelated hunks that ranking has something to leave out.
func reviewPatch() string {
	var b strings.Builder
	b.WriteString("diff --git a/pkg/directive/parse.go b/pkg/directive/parse.go\n--- a/pkg/directive/parse.go\n+++ b/pkg/directive/parse.go\n")
	b.WriteString("@@ -0,0 +1,2 @@\n+// Parse reads rules such as 95% code coverage\n+func Parse(text string) []Change { return coverageChange(text) }\n")
	b.WriteString("diff --git a/cmd/dendrite/directive.go b/cmd/dendrite/directive.go\n--- a/cmd/dendrite/directive.go\n+++ b/cmd/dendrite/directive.go\n")
	b.WriteString("@@ -0,0 +1,2 @@\n+\tfmt.Fprint(out, \"[?] Apply? [y/N] \")\n+\tanswer, _ := reader.ReadString('\\n')\n")
	for i := 0; i < 8; i++ {
		fmt.Fprintf(&b, "diff --git a/pkg/other/o%d.go b/pkg/other/o%d.go\n--- a/pkg/other/o%d.go\n+++ b/pkg/other/o%d.go\n@@ -0,0 +1 @@\n+var unrelated%d = %d\n", i, i, i, i, i, i)
	}
	return b.String()
}

func lexicalOnly(_ context.Context, query string, docs []rag.RankDocument, topK int) ([]string, error) {
	return lexicalRank(query, docs, topK), nil
}

const shows = `{"shows":true,"why":"it does"}`

func answer(verdict, evidence, reason string) string {
	return fmt.Sprintf(`{"verdict":%q,"evidence":%q,"reason":%q}`, verdict, evidence, reason)
}

func input(criteria ...string) ReviewInput {
	return ReviewInput{Ticket: "DEN-380", Description: "dendrite directive command", Criteria: criteria, Diff: reviewPatch()}
}

func TestProofReviewerAcceptsMetOnlyWithEvidenceFromTheChange(t *testing.T) {
	gw := llmchattest.Sequence(t,
		answer("met", "+func Parse(text string) []Change { return coverageChange(text) }", "Parse handles coverage"), shows,
		answer("met", `fmt.Fprint(out, "[?] Apply? [y/N] ")`, "the prompt asks"), shows,
		answer("not_met", "", "nothing writes the file"),
	)
	got, err := ProofReviewer{Chat: gw.Client, Rank: lexicalOnly}.Review(context.Background(),
		input("[AC-1] coverage rules are parsed", "[AC-2] the change is confirmed with a prompt Apply", "[AC-3] the file is written"))
	if err != nil || len(got) != 3 {
		t.Fatalf("%+v, %v", got, err)
	}
	if got[0].Status != StatusMet || got[0].AC != "AC-1" || got[0].Evidence == "" || got[1].Status != StatusMet || got[2].Status != StatusNotMet || got[2].Reason != "nothing writes the file" {
		t.Errorf("verdicts = %+v", got)
	}
	if gw.Requests() != 5 {
		t.Errorf("a question per criterion and a confirmation for each met, got %d", gw.Requests())
	}
}

func TestProofReviewerShowsEachCriterionItsMostRelevantHunks(t *testing.T) {
	gw := llmchattest.New(t, answer("unclear", "", "cannot tell"))
	if _, err := (ProofReviewer{Chat: gw.Client, Rank: lexicalOnly}).Review(context.Background(), input("[AC-2] the change is confirmed with a prompt Apply")); err != nil {
		t.Fatal(err)
	}
	last := gw.Last()
	for _, want := range []string{"Criterion: [AC-2] the change is confirmed with a prompt Apply", "=== cmd/dendrite/directive.go ===", `Apply? [y/N]`, "Summary of the change:", "pkg/directive/parse.go: +2 -0", "dendrite directive command"} {
		if !strings.Contains(last.User, want) {
			t.Errorf("prompt lacks %q:\n%s", want, last.User)
		}
	}
	if strings.Count(last.User, "=== ") != topHunks {
		t.Errorf("the model sees the %d best hunks, not all 10:\n%s", topHunks, last.User)
	}
	if !strings.Contains(last.System, "word for word") {
		t.Error("the model is told the evidence must be a quote")
	}
}

func TestProofReviewerDoesNotTrustAMetThatItCannotCheck(t *testing.T) {
	for name, evidence := range map[string]string{
		"invented line": "the function validates the schema",
		"empty":         "",
		"too short":     "}",
	} {
		gw := llmchattest.New(t, answer("met", evidence, "looks right"))
		got, err := ProofReviewer{Chat: gw.Client, Rank: lexicalOnly}.Review(context.Background(), input("[AC-1] coverage rules are parsed"))
		if err != nil || got[0].Status != StatusUnproven || !strings.Contains(got[0].Reason, "not in the change it was shown") {
			t.Errorf("%s: %+v, %v", name, got, err)
		}
	}
}

func TestProofReviewerReadsOtherAnswersAsUnproven(t *testing.T) {
	for name, tc := range map[string]struct{ answer, want string }{
		"unclear":        {answer("unclear", "", "the excerpts do not show it"), "the excerpts do not show it"},
		"unclear bare":   {answer("unclear", "", ""), "do not show it either way"},
		"unknown status": {answer("maybe", "", ""), "do not show it either way"},
	} {
		gw := llmchattest.New(t, tc.answer)
		got, err := ProofReviewer{Chat: gw.Client, Rank: lexicalOnly}.Review(context.Background(), input("[AC-1] coverage rules are parsed"))
		if err != nil || got[0].Status != StatusUnproven || !strings.Contains(got[0].Reason, tc.want) {
			t.Errorf("%s: %+v, %v", name, got, err)
		}
	}
}

func TestProofReviewerAsksOnceMoreAndThenGivesUp(t *testing.T) {
	good := answer("met", "+func Parse(text string) []Change { return coverageChange(text) }", "ok")
	gw := llmchattest.Sequence(t, "Let me think about [AC-1] first.", good, shows)
	got, err := ProofReviewer{Chat: gw.Client, Rank: lexicalOnly}.Review(context.Background(), input("[AC-1] coverage rules are parsed"))
	if err != nil || got[0].Status != StatusMet || gw.Requests() != 3 {
		t.Fatalf("%+v, %v, %d requests", got, err, gw.Requests())
	}

	twice := llmchattest.Sequence(t, "no idea", `{"verdict": 7}`)
	got, err = ProofReviewer{Chat: twice.Client, Rank: lexicalOnly}.Review(context.Background(), input("[AC-1] coverage rules are parsed"))
	if err != nil || got[0].Status != StatusUnproven || !strings.Contains(got[0].Reason, "could not be read") || twice.Requests() != 2 {
		t.Errorf("an unreadable review of one criterion is unproven, not a failure: %+v, %v", got, err)
	}
}

func TestProofReviewerReportsWhatKeepsItFromAsking(t *testing.T) {
	ctx := context.Background()
	down := llmchattest.Failing(t, 503)
	if _, err := (ProofReviewer{Chat: down.Client, Rank: lexicalOnly}).Review(ctx, input("[AC-1] a")); err == nil || !strings.Contains(err.Error(), "HTTP 503") {
		t.Errorf("a gateway that is down: %v", err)
	}
	retryDown := llmchattest.Sequence(t, "prose")
	badRank := func(context.Context, string, []rag.RankDocument, int) ([]string, error) {
		return nil, fmt.Errorf("rank exploded")
	}
	if _, err := (ProofReviewer{Chat: retryDown.Client, Rank: badRank}).Review(ctx, input("[AC-1] a")); err == nil || !strings.Contains(err.Error(), "rank exploded") {
		t.Errorf("a ranker that fails: %v", err)
	}
	none := llmchattest.New(t, "never asked")
	in := input("[AC-1] a", "[AC-2] b")
	in.Diff = ""
	got, err := ProofReviewer{Chat: none.Client, Rank: lexicalOnly}.Review(ctx, in)
	if err != nil || none.Requests() != 0 || got[0].Status != StatusUnproven || got[1].AC != "AC-2" || !strings.Contains(got[0].Reason, "no changes to review") {
		t.Errorf("with nothing to review the model is not asked: %+v, %v, %d requests", got, err, none.Requests())
	}
	if got, err := (ProofReviewer{Chat: none.Client, Rank: lexicalOnly}).Review(ctx, input()); err != nil || len(got) != 0 {
		t.Errorf("no criteria, no verdicts: %+v, %v", got, err)
	}
}

func TestProofReviewerIgnoresIdsTheRankerInvented(t *testing.T) {
	gw := llmchattest.New(t, answer("unclear", "", ""))
	wild := func(context.Context, string, []rag.RankDocument, int) ([]string, error) {
		return []string{"99", "-1", "x", "1"}, nil
	}
	if _, err := (ProofReviewer{Chat: gw.Client, Rank: wild}).Review(context.Background(), input("[AC-1] a")); err != nil {
		t.Fatal(err)
	}
	if got := strings.Count(gw.Last().User, "=== "); got != 1 {
		t.Errorf("only the valid id gives an excerpt, got %d", got)
	}
}

func TestEvidenceIgnoresDiffMarkersAndSpacing(t *testing.T) {
	excerpts := "=== a.go ===\n@@ -0,0 +1,2 @@\n+\tfmt.Fprint(out,   \"[?] Apply? [y/N] \")\n-old := 1\n"
	for evidence, want := range map[string]bool{
		`fmt.Fprint(out, "[?] Apply? [y/N] ")`:      true,
		"+\tfmt.Fprint(out, \"[?] Apply? [y/N] \")": true,
		"old := 1 line":    false,
		"old := 1":         false, // under the minimum length
		"fmt.Println(out)": false,
	} {
		if got := evidenceFound(evidence, excerpts); got != want {
			t.Errorf("evidenceFound(%q) = %v, want %v", evidence, got, want)
		}
	}
}

func TestClientJSONModeIsAskedForByTheReviewer(t *testing.T) {
	if !proofReviewer(t.TempDir()).Chat.(llmchat.Client).JSON {
		t.Error("the reviewer asks the gateway for a JSON object")
	}
}

func TestProofReviewerReportsAGatewayThatDiesDuringTheRetry(t *testing.T) {
	var calls atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		if calls.Add(1) == 1 {
			_, _ = w.Write([]byte(`{"choices":[{"message":{"content":"prose, no object"}}]}`))
			return
		}
		w.WriteHeader(http.StatusServiceUnavailable)
	}))
	defer srv.Close()
	_, err := ProofReviewer{Chat: llmchat.Client{URL: srv.URL}, Rank: lexicalOnly}.Review(context.Background(), input("[AC-1] a"))
	if err == nil || !strings.Contains(err.Error(), "HTTP 503") || calls.Load() != 2 {
		t.Errorf("%v after %d calls", err, calls.Load())
	}
}

func TestProofReviewerAsksAgainWhyBeforeBelievingAMet(t *testing.T) {
	quote := "+func Parse(text string) []Change { return coverageChange(text) }"
	met := answer("met", quote, "Parse handles coverage")
	for name, tc := range map[string]struct{ confirm, want string }{
		"the line is unrelated": {`{"shows":false,"why":"it only mentions auto_done"}`, "not confirmed as showing it: it only mentions auto_done"},
		"false without a why":   {`{"shows":false}`, "not confirmed as showing it: the answer to the follow-up question could not be read"},
		"prose instead":         {"I think so", "could not be read"},
		"wrong shape":           {`{"shows":"yes"}`, "could not be read"},
	} {
		gw := llmchattest.Sequence(t, met, tc.confirm)
		got, err := ProofReviewer{Chat: gw.Client, Rank: lexicalOnly}.Review(context.Background(), input("[AC-1] coverage rules are parsed"))
		if err != nil || got[0].Status != StatusUnproven || !strings.Contains(got[0].Reason, tc.want) || got[0].Evidence != quote {
			t.Errorf("%s: %+v, %v", name, got, err)
		}
		last := gw.Last()
		if !strings.Contains(last.System, "ONE claim") || !strings.Contains(last.User, "Criterion: [AC-1] coverage rules are parsed") || !strings.Contains(last.User, quote) || !strings.Contains(last.User, "=== pkg/directive/parse.go ===") {
			t.Errorf("%s: the follow-up question carries the criterion, the quoted line and its hunk:\n%s", name, last.User)
		}
	}
}

func TestProofReviewerReportsAGatewayThatDiesBeforeTheConfirmation(t *testing.T) {
	var calls atomic.Int32
	met := answer("met", "+func Parse(text string) []Change { return coverageChange(text) }", "ok")
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		if calls.Add(1) > 1 {
			w.WriteHeader(http.StatusServiceUnavailable)
			return
		}
		_, _ = fmt.Fprintf(w, `{"choices":[{"message":{"content":%q}}]}`, met)
	}))
	defer srv.Close()
	if _, err := (ProofReviewer{Chat: llmchat.Client{URL: srv.URL}, Rank: lexicalOnly}).Review(context.Background(), input("[AC-1] coverage rules are parsed")); err == nil || !strings.Contains(err.Error(), "HTTP 503") {
		t.Errorf("%v", err)
	}
}

func TestProofReviewerOnlyOffersTestHunksForCriteriaAboutTests(t *testing.T) {
	patch := "diff --git a/pkg/feature.go b/pkg/feature.go\n--- a/pkg/feature.go\n+++ b/pkg/feature.go\n@@ -0,0 +1 @@\n+func Feature() {}\n" +
		"diff --git a/pkg/feature_test.go b/pkg/feature_test.go\n--- a/pkg/feature_test.go\n+++ b/pkg/feature_test.go\n@@ -0,0 +1 @@\n+func TestFeature(t *testing.T) {}\n"
	for criterion, wantTests := range map[string]bool{
		"[AC-1] the feature works":                false,
		"[AC-2] the feature is covered by tests":  true,
		"[AC-3] rstest cases for every language":  true,
		"[AC-4] a fixtures directory exists":      true,
		"[AC-5] the contest ranking is displayed": false,
	} {
		gw := llmchattest.New(t, answer("unclear", "", ""))
		in := ReviewInput{Ticket: "DEN-1", Criteria: []string{criterion}, Diff: patch}
		if _, err := (ProofReviewer{Chat: gw.Client, Rank: lexicalOnly}).Review(context.Background(), in); err != nil {
			t.Fatal(err)
		}
		if got := strings.Contains(gw.Last().User, "=== pkg/feature_test.go ==="); got != wantTests {
			t.Errorf("%s: test hunk offered = %v, want %v", criterion, got, wantTests)
		}
		if !strings.Contains(gw.Last().User, "=== pkg/feature.go ===") {
			t.Errorf("%s: production code is always offered", criterion)
		}
	}
	onlyTests := patch[strings.Index(patch, "diff --git a/pkg/feature_test.go"):]
	gw := llmchattest.New(t, answer("unclear", "", ""))
	if _, err := (ProofReviewer{Chat: gw.Client, Rank: lexicalOnly}).Review(context.Background(), ReviewInput{Ticket: "DEN-1", Criteria: []string{"[AC-1] the feature works"}, Diff: onlyTests}); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(gw.Last().User, "=== pkg/feature_test.go ===") {
		t.Error("a change made only of tests still shows them")
	}
}

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
	if err := s.Plans.Save(unclaimedKey(repoScope(s.Dir), "s-9"), newPlan("- [x] design\n- [ ] build", 1)); err != nil {
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
	if _, ok := s.Plans.TakeUnclaimed(repoScope(s.Dir), time.Time{}); ok {
		t.Error("the adopted plan must not stay unclaimed")
	}
}

func TestClaimKeepsTheTicketsOwnPlanOverAnUnclaimedOne(t *testing.T) {
	s := newSession(t, map[string]string{"DEN-377": "To Do"})
	_ = s.Plans.Save("DEN-377", newPlan("- [ ] own", 2))
	_ = s.Plans.Save(unclaimedKey(repoScope(s.Dir), "s-9"), newPlan("- [ ] stray", 1))
	res, err := s.Claim(context.Background(), "DEN-377", ClaimOptions{})
	if err != nil || res.Plan == nil || res.Plan.NextStep != "own" {
		t.Fatalf("plan = %+v, %v", res.Plan, err)
	}
	if _, ok := s.Plans.TakeUnclaimed(repoScope(s.Dir), time.Time{}); !ok {
		t.Error("the stray plan belongs to another ticket and must stay")
	}
}

// testScope is the repository scope of the plans in unclaimedStore.
const testScope = "abc123"

// unclaimedStore holds a ticket's plan and unclaimed plans of different ages (and a broken one); now is the clock.
func unclaimedStore(t *testing.T) (store PlanStore, now time.Time) {
	t.Helper()
	store = PlanStore{Dir: t.TempDir()}
	_ = store.Save("DEN-1", newPlan("- [ ] ticket plan", 1))
	_ = store.Save(unclaimedKey(testScope, "old"), newPlan("- [ ] old", 1))
	_ = store.Save(unclaimedKey(testScope, "new"), newPlan("- [ ] new", 1))
	_ = store.Save(unclaimedKey(testScope, "older"), newPlan("- [ ] older", 1))
	index := filepath.Join(store.Dir, "index")
	now = time.Now()
	for name, age := range map[string]time.Duration{"unclaimed-" + testScope + "-old.json": 48 * time.Hour, "unclaimed-" + testScope + "-older.json": 72 * time.Hour, "unclaimed-" + testScope + "-new.json": time.Hour} {
		if err := os.Chtimes(filepath.Join(index, name), now.Add(-age), now.Add(-age)); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.WriteFile(filepath.Join(index, "unclaimed-"+testScope+"-broken.json"), []byte("{"), 0o644); err != nil {
		t.Fatal(err)
	}
	return store, now
}

func TestTakeUnclaimedFromAnEmptyStore(t *testing.T) {
	store := PlanStore{Dir: t.TempDir()}
	if _, ok := store.TakeUnclaimed(testScope, time.Time{}); ok {
		t.Fatal("an empty store has nothing to take")
	}
}

func TestTakeUnclaimedTakesTheNewestRecentPlanOnce(t *testing.T) {
	store, now := unclaimedStore(t)
	p, ok := store.TakeUnclaimed(testScope, now.Add(-6*time.Hour))
	if !ok || p.NextStep != "new" {
		t.Fatalf("took %+v, %v: want the newest recent plan", p, ok)
	}
	if _, ok := store.TakeUnclaimed(testScope, now.Add(-6*time.Hour)); ok {
		t.Error("the plan is gone once taken, and a ticket's plan or a broken file is never taken")
	}
}

func TestTakeUnclaimedWithALongerWindowTakesAnOlderPlan(t *testing.T) {
	store, now := unclaimedStore(t)
	store.TakeUnclaimed(testScope, now.Add(-6*time.Hour))
	if p, ok := store.TakeUnclaimed(testScope, now.Add(-60*time.Hour)); !ok || p.NextStep != "old" {
		t.Errorf("with a longer window the older plan is taken: %+v, %v", p, ok)
	}
	if _, err := store.Load("DEN-1"); err != nil {
		t.Errorf("the ticket plan must be untouched: %v", err)
	}
}

func TestCommentClaimedWithoutAClaimPostsNothing(t *testing.T) {
	ctx := context.Background()
	s := newSession(t, map[string]string{"DEN-377": "To Do"})
	if ticket, err := s.CommentClaimed(ctx, "progress", "nothing claimed"); ticket != "" || err != nil {
		t.Fatalf("no claim posts nothing: %q, %v", ticket, err)
	}
	if got := s.board.Texts("DEN-377"); len(got) != 0 {
		t.Fatalf("comments = %q", got)
	}
}

func TestCommentClaimedPostsToTheClaimedTicket(t *testing.T) {
	ctx := context.Background()
	s := newSession(t, map[string]string{"DEN-377": "To Do"})
	if _, err := s.Claim(ctx, "DEN-377", ClaimOptions{}); err != nil {
		t.Fatal(err)
	}
	ticket, err := s.CommentClaimed(ctx, "progress", "did a thing (abc1234)")
	if err != nil || ticket != "DEN-377" {
		t.Fatalf("posted to %q, %v", ticket, err)
	}
	got := s.board.Texts("DEN-377")
	if len(got) != 2 || !strings.HasPrefix(got[1], "[dendrite:progress] did a thing (abc1234)") {
		t.Errorf("comments = %q", got)
	}
	if _, err := s.CommentClaimed(ctx, "bogus", "x"); err == nil || !strings.Contains(err.Error(), "unknown comment kind") {
		t.Errorf("a bad kind must be refused: %v", err)
	}
}

func TestTakeUnclaimedOnlyTakesPlansOfTheSameRepository(t *testing.T) {
	store := PlanStore{Dir: t.TempDir()}
	_ = store.Save(unclaimedKey("repo-a", "s-1"), newPlan("- [ ] a", 1))
	_ = store.Save(unclaimedKey("repo-b", "s-1"), newPlan("- [ ] b", 1))
	if p, ok := store.TakeUnclaimed("repo-b", time.Time{}); !ok || p.NextStep != "b" {
		t.Fatalf("took %+v, %v: want the plan of repo-b", p, ok)
	}
	if _, ok := store.TakeUnclaimed("repo-b", time.Time{}); ok {
		t.Error("repo-b has no other plan")
	}
	if p, ok := store.TakeUnclaimed("repo-a", time.Time{}); !ok || p.NextStep != "a" {
		t.Errorf("the plan of repo-a must have stayed: %+v, %v", p, ok)
	}
}

func TestTakeUnclaimedLeavesAPlanAnotherClaimTookFirst(t *testing.T) {
	store := PlanStore{Dir: t.TempDir()}
	_ = store.Save(unclaimedKey("repo", "s-1"), newPlan("- [ ] only", 1))
	renameIndex = func(string, string) error { return os.ErrNotExist }
	t.Cleanup(func() { renameIndex = os.Rename })
	if _, ok := store.TakeUnclaimed("repo", time.Time{}); ok {
		t.Error("a plan someone else took is not ours")
	}
}

func TestTakeUnclaimedGivesOnePlanToOneOfManyConcurrentClaims(t *testing.T) {
	store := PlanStore{Dir: t.TempDir()}
	_ = store.Save(unclaimedKey("repo", "s-1"), newPlan("- [ ] only", 1))
	const claimers = 16
	var taken atomic.Int32
	var wg sync.WaitGroup
	for range claimers {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if _, ok := store.TakeUnclaimed("repo", time.Time{}); ok {
				taken.Add(1)
			}
		}()
	}
	wg.Wait()
	if got := taken.Load(); got != 1 {
		t.Errorf("%d claims took the one plan, want exactly 1", got)
	}
}

func TestClaimDoesNotAdoptAPlanOfAnotherRepository(t *testing.T) {
	s := newSession(t, map[string]string{"DEN-377": "To Do"})
	if err := s.Plans.Save(unclaimedKey(repoScope(t.TempDir()), "s-9"), newPlan("- [ ] foreign", 1)); err != nil {
		t.Fatal(err)
	}
	res, err := s.Claim(context.Background(), "DEN-377", ClaimOptions{})
	if err != nil || res.Plan != nil {
		t.Fatalf("plan = %+v, %v: a plan made in another repository must not land on this ticket", res.Plan, err)
	}
	if _, ok := s.Plans.TakeUnclaimed(repoScope(s.Dir), time.Time{}); ok {
		t.Error("and this repository has no plan of its own to take")
	}
}

func TestRepoScopeIsSharedByAWorktreeAndItsRepository(t *testing.T) {
	repo := t.TempDir()
	for _, args := range [][]string{{"init", "-q", "-b", "develop"}, {"-c", "user.name=t", "-c", "user.email=t@e.x", "commit", "-q", "--allow-empty", "-m", "base"}, {"worktree", "add", "-q", "-b", "agent/x", filepath.Join(repo, "wt")}} {
		if out, err := gitCommand(repo, args...).CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, out)
		}
	}
	if repoScope(repo) != repoScope(filepath.Join(repo, "wt")) {
		t.Error("a worktree belongs to the repository it was made from")
	}
	if repoScope(repo) == repoScope(t.TempDir()) {
		t.Error("two repositories have different scopes")
	}
}
