package ticketlock

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"
)

const twoCriteria = "<p>goal</p><ul><li>[AC-1] it works</li><li>[AC-2] it is tested</li></ul>"

// finishRig is a claimed session on a ticket with two acceptance criteria and checks that pass.
type finishRig struct {
	*session
	checks FinishChecks
}

func newFinishRig(t *testing.T, state, description string) *finishRig {
	t.Helper()
	s := newSession(t, map[string]string{"DEN-377": state})
	s.board.SetDescription("DEN-377", description)
	r := &finishRig{session: s}
	r.checks = FinishChecks{
		Gate: func(context.Context, string) (GateResult, error) {
			return GateResult{Passed: true, Report: "tests ok"}, nil
		},
		Review: func(_ context.Context, in ReviewInput) ([]ACVerdict, error) {
			var out []ACVerdict
			for _, c := range in.Criteria {
				out = append(out, ACVerdict{AC: acItem.FindString(c), Pass: true, Reason: "seen in the diff"})
			}
			return out, nil
		},
		Diff:     func(string, string) string { return "diff --git a/x b/x" },
		AutoDone: true,
	}
	if state != "Done" && state != "To Verify" {
		if _, err := s.Claim(context.Background(), "DEN-377", ClaimOptions{}); err != nil {
			t.Fatal(err)
		}
	}
	return r
}

func (r *finishRig) finish() (FinishResult, error) {
	return r.Finish(context.Background(), "DEN-377", r.checks)
}

func (r *finishRig) lastComment() string {
	c := texts(r.board, "DEN-377")
	return c[len(c)-1]
}

func TestFinishSetsDoneWhenEveryCheckPasses(t *testing.T) {
	r := newFinishRig(t, "To Do", twoCriteria)
	res, err := r.finish()
	if err != nil || res.State != "Done" || res.Ticket != "DEN-377" || len(res.Verdicts) != 2 {
		t.Fatalf("%+v, %v", res, err)
	}
	if got := r.board.State("DEN-377"); got != "Done" {
		t.Errorf("Plane state = %q", got)
	}
	if _, err := r.Store.ClaimFor("agent/den-377"); !errors.Is(err, ErrNoClaim) {
		t.Errorf("a finished ticket releases its claim: %v", err)
	}
	if c := r.lastComment(); !strings.HasPrefix(c, "[dendrite:milestone] Finished: the quality gate passed.") || !strings.Contains(c, "[AC-1] pass") {
		t.Errorf("report comment = %q", c)
	}
}

func TestFinishWaitsForAPersonWhenPolicyOrLabelsSayNot(t *testing.T) {
	cases := map[string]func(r *finishRig){
		"auto-Done is off": func(r *finishRig) { r.checks.AutoDone = false },
		"human review label": func(r *finishRig) {
			if _, _, err := r.Board.Svc.PatchLabels(context.Background(), r.Board.Client, "DEN-377", []string{HumanReviewLabel}, nil); err != nil {
				t.Fatal(err)
			}
		},
		"the review cannot run": func(r *finishRig) {
			r.checks.Review = func(context.Context, ReviewInput) ([]ACVerdict, error) { return nil, errors.New("gateway down") }
		},
	}
	for name, arrange := range cases {
		t.Run(name, func(t *testing.T) {
			r := newFinishRig(t, "To Do", twoCriteria)
			arrange(r)
			res, err := r.finish()
			if err != nil || res.State != "To Verify" || r.board.State("DEN-377") != "To Verify" {
				t.Fatalf("%+v, %v, Plane %q", res, err, r.board.State("DEN-377"))
			}
			if len(res.Notes) == 0 || !strings.Contains(r.lastComment(), "Note: ") {
				t.Errorf("the reason must be on the ticket: notes %v, last %q", res.Notes, r.lastComment())
			}
		})
	}
}

func TestFinishWithoutCriteriaHasNothingToReview(t *testing.T) {
	r := newFinishRig(t, "To Do", "<p>goal only</p>")
	r.checks.Review = func(context.Context, ReviewInput) ([]ACVerdict, error) {
		t.Fatal("no criteria, no review")
		return nil, nil
	}
	res, err := r.finish()
	if err != nil || res.State != "Done" || !strings.Contains(strings.Join(res.Notes, " "), "no [AC-n] criteria") {
		t.Fatalf("%+v, %v", res, err)
	}
}

func TestFinishBlocksWithAHandoffWhenAStageFails(t *testing.T) {
	cases := map[string]struct {
		arrange func(r *finishRig)
		want    []string
	}{
		"the gate fails": {func(r *finishRig) {
			r.checks.Gate = func(context.Context, string) (GateResult, error) {
				return GateResult{Report: "FAIL pkg/x"}, nil
			}
		}, []string{"The quality gate failed.", "FAIL pkg/x"}},
		"the gate cannot run": {func(r *finishRig) {
			r.checks.Gate = func(context.Context, string) (GateResult, error) { return GateResult{}, errors.New("no toolchain") }
		}, []string{"the quality gate could not run: no toolchain"}},
		"a criterion is not met and one has no verdict": {func(r *finishRig) {
			r.checks.Review = func(context.Context, ReviewInput) ([]ACVerdict, error) {
				return []ACVerdict{{AC: "ac-1", Pass: false, Reason: "nothing calls it"}}, nil
			}
		}, []string{"The acceptance review failed", "[AC-1] it works: nothing calls it", "[AC-2] it is tested: the review gave no verdict"}},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			r := newFinishRig(t, "To Do", twoCriteria)
			tc.arrange(r)
			res, err := r.finish()
			if err != nil || res.State != "Blocked" || r.board.State("DEN-377") != "Blocked" {
				t.Fatalf("%+v, %v, Plane %q", res, err, r.board.State("DEN-377"))
			}
			handoff := ""
			for _, c := range texts(r.board, "DEN-377") {
				if strings.HasPrefix(c, "[dendrite:handoff]") {
					handoff = c
				}
			}
			for _, want := range append([]string{"Completed:", "Challenges:", "Missing:"}, tc.want...) {
				if !strings.Contains(handoff, want) {
					t.Errorf("handoff lacks %q: %q", want, handoff)
				}
			}
			if _, err := r.Store.ClaimFor("agent/den-377"); err != nil {
				t.Errorf("a blocked ticket keeps its claim for whoever resumes: %v", err)
			}
		})
	}
}

func TestFinishRefusals(t *testing.T) {
	r := newFinishRig(t, "To Do", twoCriteria)
	if _, err := r.Finish(context.Background(), "DEN-999", r.checks); err == nil {
		t.Error("an unknown ticket cannot be finished")
	}
	done := newFinishRig(t, "Done", twoCriteria)
	if _, err := done.finish(); !errors.Is(err, ErrNotInProgress) || !strings.Contains(err.Error(), "Done") {
		t.Errorf("a ticket that is not In Progress: %v", err)
	}
}

func TestFinishFailuresOfTheBoardAreReported(t *testing.T) {
	r := newFinishRig(t, "To Do", twoCriteria)
	r.board.Fail("labels", 0)
	res, err := r.finish()
	if err != nil || res.State != "To Verify" || !strings.Contains(strings.Join(res.Notes, " "), "labels could not be read") {
		t.Fatalf("unreadable labels mean a person verifies: %+v, %v", res, err)
	}
}

func TestCutDiffAndACID(t *testing.T) {
	if got := cutDiff("short"); got != "short" {
		t.Errorf("short diffs pass: %q", got)
	}
	if got := cutDiff(strings.Repeat("x", MaxReviewDiff+10)); !strings.HasSuffix(got, "bytes]") || len(got) > MaxReviewDiff+60 {
		t.Errorf("long diffs are cut: %d", len(got))
	}
	var big strings.Builder
	for i := 0; i < 6; i++ {
		big.WriteString("\ndiff --git a/f" + fmt.Sprint(i) + " b/f" + fmt.Sprint(i) + "\n+" + strings.Repeat("y", 30000))
	}
	cut := cutDiff(big.String())
	if len(cut) > MaxReviewDiff+60 {
		t.Errorf("the cut diff is %d bytes", len(cut))
	}
	for i := 0; i < 5; i++ {
		if !strings.Contains(cut, "diff --git a/f"+fmt.Sprint(i)+" ") {
			t.Errorf("file %d is missing from the cut diff: every file keeps its start", i)
		}
	}
	for _, in := range []string{"[AC-1]", "AC-1", " ac-1 "} {
		if acID(in) != "AC-1" {
			t.Errorf("acID(%q) = %q", in, acID(in))
		}
	}
	if !hasLabel([]string{"a", "Needs-Human-Review"}, HumanReviewLabel) || hasLabel(nil, "x") {
		t.Error("hasLabel")
	}
}

func TestFinishReportsBoardErrorsAtEachStep(t *testing.T) {
	cases := map[string]struct {
		method string
		after  int
		want   string
	}{
		"reading the ticket for its criteria": {"GET", 3, "list issues"},
		"posting the report":                  {"POST", 0, "add comment"},
		"setting Done":                        {"PATCH", 1, "to Done"},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			r := newFinishRig(t, "To Do", twoCriteria)
			r.board.Fail(tc.method, tc.after)
			if _, err := r.finish(); err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Errorf("err = %v, want one mentioning %q", err, tc.want)
			}
		})
	}
}

func TestFinishReportMarksAnExtraFailingVerdict(t *testing.T) {
	r := newFinishRig(t, "To Do", twoCriteria)
	inner := r.checks.Review
	r.checks.Review = func(ctx context.Context, in ReviewInput) ([]ACVerdict, error) {
		v, _ := inner(ctx, in)
		return append(v, ACVerdict{AC: "AC-9", Pass: false, Reason: "not a criterion of this ticket"}), nil
	}
	if res, err := r.finish(); err != nil || res.State != "Done" || !strings.Contains(r.lastComment(), "AC-9 FAIL") {
		t.Fatalf("%+v, %v, %q", res, err, r.lastComment())
	}
}
