package ownership

import (
	"context"
	"errors"
	"reflect"
	"strings"
	"testing"

	"Dendrite/pkg/agentjobs"
	"Dendrite/pkg/directive"
)

var errBoom = errors.New("boom")

func TestIsREAMatchesLabelsAndTitlePrefixes(t *testing.T) {
	rule := directive.Defaults().Rea
	for name, tc := range map[string]struct {
		title  string
		labels []string
		want   bool
	}{
		"label":             {"Anything", []string{"Rea"}, true},
		"prefix":            {"[REA] PE analysis", nil, true},
		"prefix after id":   {"[DEN-1061] [REA] ownership", nil, true},
		"bracket not an id": {"[wip] [REA] x", nil, false},
		"lower case prefix": {"[rea] x", nil, true},
		"other work":        {"[DEN-1] Fix the parser", []string{"bug"}, false},
		"no brackets":       {"Plain title", nil, false},
	} {
		if got := IsREA(tc.title, tc.labels, rule); got != tc.want {
			t.Errorf("%s: IsREA = %t, want %t", name, got, tc.want)
		}
	}
	empty := directive.Rea{TitlePrefixes: []string{""}}
	if IsREA("[REA] x", nil, empty) {
		t.Error("an empty prefix must match nothing")
	}
}

func TestHasOwnerReadsTheOwnerLabel(t *testing.T) {
	if !HasOwner([]string{"x", "Owner:AGY"}) || HasOwner([]string{"agent:claude"}) || HasOwner(nil) {
		t.Error("only owner:agy marks an AGY-owned ticket")
	}
}

// run is the state one Transfer test observes.
type run struct {
	comments []string
	labels   []string
	released []string
	specs    []agentjobs.SubmitSpec
}

// ports builds Ports around issue that record into r; fail names the one effect that errors.
func (r *run) ports(issue Issue, fail string) Ports {
	pick := func(name string) error {
		if fail == name {
			return errBoom
		}
		return nil
	}
	return Ports{
		Issue: func(context.Context, string) (Issue, error) { return issue, pick("issue") },
		Comment: func(_ context.Context, _, kind, body string) error {
			r.comments = append(r.comments, kind+": "+body)
			return pick("comment")
		},
		AddLabel: func(_ context.Context, _, label string) error {
			r.labels = append(r.labels, label)
			return pick("label")
		},
		ReleaseClaims: func(ticket string) error { r.released = append(r.released, ticket); return pick("release") },
		Submit: func(spec agentjobs.SubmitSpec) (string, error) {
			r.specs = append(r.specs, spec)
			return "job-1", pick("submit")
		},
	}
}

func reaIssue() Issue {
	return Issue{Ticket: "DEN-1062", Title: "[DEN-1062] [REA] retro analyze", Description: "Parse PE64.", Estimate: 8}
}

func TestTransferHandsARefusedTicketToAGY(t *testing.T) {
	var r run
	res, err := Transfer(context.Background(), r.ports(reaIssue(), ""), directive.Defaults().Rea, "DEN-1062",
		Refusal{Reason: "stop_reason", Excerpt: "No."})
	if err != nil {
		t.Fatal(err)
	}
	if want := (Result{Ticket: "DEN-1062", JobID: "job-1"}); res != want {
		t.Errorf("result %+v, want %+v", res, want)
	}
	r.checkRecorded(t)
	r.checkJob(t)
}

// checkRecorded checks the refusal was recorded, the ticket labelled and the Claude claim released.
func (r *run) checkRecorded(t *testing.T) {
	t.Helper()
	if len(r.comments) != 1 || !strings.HasPrefix(r.comments[0], "decision: ") || !strings.Contains(r.comments[0], "No.") {
		t.Errorf("the refusal must be recorded as a decision: %v", r.comments)
	}
	if !reflect.DeepEqual(r.labels, []string{OwnerLabel}) || !reflect.DeepEqual(r.released, []string{"DEN-1062"}) {
		t.Errorf("labels %v, released %v", r.labels, r.released)
	}
}

// checkJob checks the AGY job: isolated, for the ticket, sized by its estimate and briefed with its text.
func (r *run) checkJob(t *testing.T) {
	t.Helper()
	spec := r.specs[0]
	if spec.Agent != "agy" || !spec.Isolate || spec.Ticket != "DEN-1062" || spec.Complexity != 8 {
		t.Errorf("job spec %+v", spec)
	}
	for _, want := range []string{"Parse PE64.", "No."} {
		if !strings.Contains(spec.Prompt, want) {
			t.Errorf("the job prompt lacks %q:\n%s", want, spec.Prompt)
		}
	}
}

func TestTransferUsesTheDirectiveModelAndClampsTheComplexity(t *testing.T) {
	rule := directive.Defaults().Rea
	rule.RefusalModel = "gemini-3.8-flash-high"
	for est, want := range map[int]int{0: 1, 3: 3, 40: 10} {
		issue := reaIssue()
		issue.Estimate, issue.Description = est, ""
		var r run
		if _, err := Transfer(context.Background(), r.ports(issue, ""), rule, "x", Refusal{}); err != nil {
			t.Fatal(err)
		}
		if got := r.specs[0]; got.Model != rule.RefusalModel || got.Complexity != want || strings.Contains(got.Prompt, "## Ticket") {
			t.Errorf("estimate %d: %+v", est, got)
		}
	}
}

func TestTransferIsIdempotentAndHonoursTheRule(t *testing.T) {
	rule := directive.Defaults().Rea
	owned := reaIssue()
	owned.Labels = []string{OwnerLabel}
	var r run
	res, err := Transfer(context.Background(), r.ports(owned, ""), rule, "x", Refusal{})
	if err != nil || !res.AlreadyOwned || len(r.specs)+len(r.comments)+len(r.labels) != 0 {
		t.Errorf("a ticket AGY owns: %+v, %v, %+v", res, err, r)
	}

	off := rule
	off.RefusalOwner = directive.RefusalOwnerNone
	if _, err := Transfer(context.Background(), r.ports(reaIssue(), ""), off, "x", Refusal{}); !errors.Is(err, ErrDisabled) {
		t.Errorf("rule off: %v", err)
	}
	other := Issue{Ticket: "DEN-5", Title: "[DEN-5] Fix the parser"}
	if _, err := Transfer(context.Background(), r.ports(other, ""), rule, "x", Refusal{}); !errors.Is(err, ErrNotREA) {
		t.Errorf("not REA: %v", err)
	}
}

func TestTransferReportsWhichEffectFailed(t *testing.T) {
	for _, tc := range []struct{ fail, want string }{
		{"issue", "read x"}, {"comment", "boom"}, {"submit", "start the AGY job"},
		{"label", "job-1 started but DEN-1062 is not labelled"}, {"release", "job-1 started but the Claude claim"},
	} {
		var r run
		res, err := Transfer(context.Background(), r.ports(reaIssue(), tc.fail), directive.Defaults().Rea, "x", Refusal{})
		if err == nil || !errors.Is(err, errBoom) || !strings.Contains(err.Error(), tc.want) {
			t.Errorf("%s: %v, want %q", tc.fail, err, tc.want)
		}
		if (tc.fail == "label" || tc.fail == "release") && res.JobID != "job-1" {
			t.Errorf("%s: the started job must be reported: %+v", tc.fail, res)
		}
	}
}

func TestDecisionNoteWorksWithoutASignal(t *testing.T) {
	if n := decisionNote(Refusal{}); strings.Contains(n, "Signal") || strings.Contains(n, "What Claude said") {
		t.Errorf("note %q", n)
	}
}

func TestDecisionNoteSaysHowSureTheRefusalWas(t *testing.T) {
	for _, tc := range []struct {
		why  Refusal
		want string
	}{
		{Refusal{Reason: "stop_reason", Confidence: ConfidenceExact}, "(exact)"},
		{Refusal{Reason: "declined", Confidence: ConfidenceSuspected}, "(suspected)"},
		{Refusal{Reason: "explicit"}, "(explicit)"},
	} {
		if n := decisionNote(tc.why); !strings.Contains(n, tc.want) {
			t.Errorf("%+v: %q lacks %q", tc.why, n, tc.want)
		}
	}
}
