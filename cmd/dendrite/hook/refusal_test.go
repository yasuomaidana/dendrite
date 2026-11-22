package hook

import (
	"Dendrite/cmd/dendrite/internal/planecli"
	"Dendrite/pkg/agentjobs"
	"Dendrite/pkg/directive"
	"Dendrite/pkg/rea/ownership"
	"Dendrite/pkg/ticketlock"
	"bytes"
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const (
	refusalTranscript = `{"type":"user","message":{"role":"user","content":"analyse the binary"}}` + "\n" +
		`{"type":"assistant","message":{"role":"assistant","stop_reason":"refusal","content":[{"type":"text","text":"I can't continue with this."}]}}` + "\n"
	// wordingTranscript ends in a short reply that only declines in words: a suspected refusal.
	wordingTranscript = `{"type":"user","message":{"role":"user","content":"analyse the binary"}}` + "\n" +
		`{"type":"assistant","message":{"role":"assistant","stop_reason":"end_turn","content":[{"type":"text","text":"I can't help with that."}]}}` + "\n"
	workingTranscript = `{"type":"assistant","message":{"role":"assistant","stop_reason":"end_turn","content":[{"type":"text","text":"Parsed the import table; next the exports."}]}}` + "\n"
)

// reaRig is a hook rig whose ticket is REA work, with AGY job submission recorded instead of started.
type reaRig struct {
	*hookRig
	jobs []agentjobs.SubmitSpec
	// submitErr makes the job submission fail.
	submitErr error
}

func newReaRig(t *testing.T, title string) *reaRig {
	t.Helper()
	r := &reaRig{hookRig: newHookRig(t, "To Do")}
	r.board.SetTitle("DEN-377", title)
	prev := refusalPorts
	refusalPorts = func(l *ticketlock.Lifecycle) ownership.Ports {
		p := ownership.BoardPorts(l)
		p.Submit = func(s agentjobs.SubmitSpec) (string, error) {
			r.jobs = append(r.jobs, s)
			return "job-3", r.submitErr
		}
		return p
	}
	t.Cleanup(func() { refusalPorts = prev })
	return r
}

// stopEvent is a Stop event whose transcript holds content.
func (r *reaRig) stopEvent(content string) *strings.Reader {
	path := filepath.Join(r.t.TempDir(), "transcript.jsonl")
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		r.t.Fatal(err)
	}
	return strings.NewReader(`{"session_id":"s-1","cwd":"` + r.repo + `","transcript_path":"` + path + `"}`)
}

// quietStop runs the Stop hook on a transcript and fails the test unless it said and did nothing.
func (r *reaRig) quietStop(t *testing.T, transcript string) {
	t.Helper()
	var out bytes.Buffer
	if err := runStop(context.Background(), r.stopEvent(transcript), &out); err != nil || out.Len() != 0 || len(r.jobs) != 0 {
		t.Errorf("the hook must leave this alone: %v %q jobs=%d", err, out.String(), len(r.jobs))
	}
}

func TestStopLeavesASessionThatIsNotRefusingAlone(t *testing.T) {
	r := newReaRig(t, "[DEN-377] [REA] retro analyze")
	r.quietStop(t, refusalTranscript) // no claim, nothing to judge
	r.claim()
	r.quietStop(t, workingTranscript)
}

func TestStopHandsARefusedREATicketToAGY(t *testing.T) {
	r := newReaRig(t, "[DEN-377] [REA] retro analyze")
	r.claim()
	var out bytes.Buffer
	if err := runStop(context.Background(), r.stopEvent(refusalTranscript), &out); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), "DEN-377 refused by Claude: handed to AGY (job job-3)") || len(r.jobs) != 1 || r.jobs[0].Agent != "agy" {
		t.Fatalf("output %q, jobs %+v", out.String(), r.jobs)
	}
	if labels := strings.Join(r.board.LabelNames("DEN-377"), ","); !strings.Contains(labels, ownership.OwnerLabel) {
		t.Errorf("labels = %s", labels)
	}
	if _, ok := ticketlock.NewStore().ClaimOfDir(r.repo); ok {
		t.Error("the Claude claim must be released")
	}
	r.jobs = nil
	r.quietStop(t, refusalTranscript) // a second Stop changes nothing
}

func TestStopLeavesWhatItCannotJudgeAlone(t *testing.T) {
	t.Run("not an REA ticket", func(t *testing.T) {
		r := newReaRig(t, "[DEN-377] Fix the parser")
		r.claim()
		r.quietStop(t, refusalTranscript)
	})
	t.Run("another agent holds the claim", func(t *testing.T) {
		r := newReaRig(t, "[DEN-377] [REA] x")
		if _, err := planecli.LifecycleIn(mustClient(t), r.repo).Claim(context.Background(), "DEN-377", ticketlock.ClaimOptions{Agent: "agy"}); err != nil {
			t.Fatal(err)
		}
		r.quietStop(t, refusalTranscript)
	})
	for name, toml := range map[string]string{
		"the directive keeps it with Claude": "schema = 1\n[rea]\nrefusal_owner = \"none\"\n",
		"unreadable directives":              "not = [valid",
	} {
		t.Run(name, func(t *testing.T) {
			r := newReaRig(t, "[DEN-377] [REA] x")
			r.claim()
			r.writeDirectives(t, toml)
			r.quietStop(t, refusalTranscript)
		})
	}
}

// writeDirectives gives the working copy a directives file.
func (r *reaRig) writeDirectives(t *testing.T, toml string) {
	t.Helper()
	file := filepath.Join(r.repo, filepath.FromSlash(directive.Path))
	if err := os.MkdirAll(filepath.Dir(file), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(file, []byte(toml), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestStopReportsAProblemWithoutFailingTheAgent(t *testing.T) {
	ctx := context.Background()
	r := newReaRig(t, "[DEN-377] [REA] x")
	r.claim()
	l := ticketlock.NewLifecycle(ticketlock.Board{}, r.repo)
	noBoard := func() (ticketlock.Board, error) { return ticketlock.Board{}, errors.New("no Plane") }
	in, _ := ticketlock.ParseHookInput(r.stopEvent(refusalTranscript))
	if _, err := RefusalHandoff(ctx, l, noBoard, in); err == nil || !strings.Contains(err.Error(), "not handed to AGY") {
		t.Errorf("a board that cannot be opened: %v", err)
	}

	r.submitErr = errors.New("agy is down")
	var out bytes.Buffer
	if err := runStop(ctx, r.stopEvent(refusalTranscript), &out); err != nil || !strings.Contains(out.String(), "[!]") || !strings.Contains(out.String(), "agy is down") {
		t.Errorf("a failed handoff is reported, never raised: %v %q", err, out.String())
	}
}

func TestSessionEndHandsARefusedTicketToAGYAndFallsBackToTheAutomaticHandoff(t *testing.T) {
	ctx := context.Background()
	r := newReaRig(t, "[DEN-377] [REA] x")
	r.claim()
	var out bytes.Buffer
	if err := runSessionEnd(ctx, r.stopEvent(refusalTranscript), &out); err != nil || !strings.Contains(out.String(), "handed to AGY") {
		t.Fatalf("a refusal at session end: %v %q", err, out.String())
	}
	if st := r.board.State("DEN-377"); st != "In Progress" {
		t.Errorf("AGY owns the ticket, it stays In Progress: %s", st)
	}

	r2 := newReaRig(t, "[DEN-377] [REA] x")
	r2.claim()
	r2.submitErr = errors.New("agy is down")
	out.Reset()
	if err := runSessionEnd(ctx, r2.stopEvent(refusalTranscript), &out); err != nil {
		t.Fatal(err)
	}
	if r2.board.State("DEN-377") != "Blocked" || !strings.Contains(out.String(), "agy is down") || !strings.Contains(out.String(), "[!]") {
		t.Errorf("a refusal that cannot be handed over still gets the automatic handoff: state %s\n%s", r2.board.State("DEN-377"), out.String())
	}
}

func TestStopRunsThroughTheCLI(t *testing.T) {
	r := newReaRig(t, "[DEN-377] [REA] x")
	r.claim()
	event, err := os.CreateTemp(t.TempDir(), "event")
	if err != nil {
		t.Fatal(err)
	}
	body := new(strings.Builder)
	if _, err := r.stopEvent(refusalTranscript).WriteTo(body); err != nil {
		t.Fatal(err)
	}
	if _, err := event.WriteString(body.String()); err != nil {
		t.Fatal(err)
	}
	if _, err := event.Seek(0, 0); err != nil {
		t.Fatal(err)
	}
	prev := os.Stdin
	os.Stdin = event
	t.Cleanup(func() { os.Stdin = prev; _ = event.Close() })

	var buf bytes.Buffer
	if err := runCmd(context.Background(), &buf, "hook", "stop"); err != nil || !strings.Contains(buf.String(), "handed to AGY") {
		t.Errorf("`dendrite hook stop`: %v\n%s", err, buf.String())
	}
	buf.Reset()
	if err := runCmd(context.Background(), &buf, "hook", "--help"); err != nil || !strings.Contains(buf.String(), "stop") {
		t.Errorf("`dendrite hook` does not list stop: %v\n%s", err, buf.String())
	}
}

func TestAWordingOnlyRefusalWaitsForTheSessionToEnd(t *testing.T) {
	ctx := context.Background()
	r := newReaRig(t, "[DEN-377] [REA] x")
	r.claim()
	r.quietStop(t, wordingTranscript) // the user may still answer: Stop does nothing

	var out bytes.Buffer
	if err := runSessionEnd(ctx, r.stopEvent(wordingTranscript), &out); err != nil || !strings.Contains(out.String(), "handed to AGY") || len(r.jobs) != 1 {
		t.Fatalf("a session that ends on the refusal: %v %q jobs=%d", err, out.String(), len(r.jobs))
	}
	if !strings.Contains(strings.Join(r.board.Texts("DEN-377"), "\n"), "suspected") {
		t.Errorf("the ticket must record how sure the refusal was: %q", r.board.Texts("DEN-377"))
	}
}

func TestAWordingOnlyRefusalAnsweredByTheUserIsNotHandedOver(t *testing.T) {
	r := newReaRig(t, "[DEN-377] [REA] x")
	r.claim()
	followUp := wordingTranscript +
		`{"type":"user","message":{"role":"user","content":"it is my own binary, please continue"}}` + "\n" +
		`{"type":"assistant","message":{"role":"assistant","stop_reason":"end_turn","content":[{"type":"text","text":"Parsed the import table; next the exports."}]}}` + "\n"
	var out bytes.Buffer
	if err := runSessionEnd(context.Background(), r.stopEvent(followUp), &out); err != nil || len(r.jobs) != 0 {
		t.Errorf("a session that went on after the decline keeps its ticket: %v %q jobs=%d", err, out.String(), len(r.jobs))
	}
}
