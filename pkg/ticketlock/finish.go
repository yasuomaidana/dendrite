package ticketlock

import (
	"Dendrite/pkg/assert"
	"Dendrite/pkg/branchpolicy"
	"Dendrite/pkg/directive"
	"Dendrite/pkg/llmchat"
	"Dendrite/pkg/quality/gate"
	"Dendrite/pkg/rag"
	"Dendrite/pkg/tracker"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"time"
)

// HumanReviewLabel is the default label that keeps `finish` from setting Done: a person verifies the ticket.
// The project's `[done] human_review_labels` directive replaces it.
const HumanReviewLabel = "needs-human-review"

// GateResult is the outcome of the deterministic quality gate, the first stage of `finish`.
type GateResult struct {
	Passed bool   `json:"passed"`
	Report string `json:"report"`
}

// ACVerdict is the acceptance review's judgement of one acceptance criterion: met (with evidence found in the
// change), not met, or unproven (no verifiable evidence either way).
type ACVerdict struct {
	AC       string `json:"ac"`
	Status   string `json:"status"`
	Evidence string `json:"evidence,omitempty"`
	Reason   string `json:"reason,omitempty"`
}

// ReviewInput is what the acceptance review sees.
type ReviewInput struct {
	Ticket      string
	Description string
	Criteria    []string
	Diff        string
}

// FinishChecks are the two stages `finish` runs and the policy it applies to the result.
type FinishChecks struct {
	// Gate runs the tests and the quality gates on the working copy in dir, for the work done for ticket.
	Gate func(ctx context.Context, dir, ticket string) (GateResult, error)
	// Review judges the diff against every acceptance criterion.
	Review func(ctx context.Context, in ReviewInput) ([]ACVerdict, error)
	// Diff is the whole patch of the work done for the ticket; Review picks the parts it needs.
	Diff func(dir, ticket string) string
	// Propose opens the pull request of a branch that merges into a trunk agents never push, and returns its URL.
	// Nil proposes nothing.
	Propose func(ctx context.Context, p Proposal) (string, error)
	// AutoDone lets a clean finish set Done; without it every clean finish waits in To Verify.
	AutoDone bool
	// HumanReviewLabels are the ticket labels that always need a person; empty means HumanReviewLabel.
	HumanReviewLabels []string
}

// FinishResult says where `finish` left the ticket and why.
type FinishResult struct {
	Ticket   string      `json:"ticket"`
	State    string      `json:"state"`
	Gate     GateResult  `json:"gate"`
	Verdicts []ACVerdict `json:"verdicts,omitempty"`
	Notes    []string    `json:"notes,omitempty"`
}

// ErrNotInProgress is returned when a ticket that is not In Progress is finished.
var ErrNotInProgress = errors.New("only a ticket that is In Progress can be finished")

// Finish ends the session's work on a ticket. Stage 1 is the quality gate, stage 2 the review of the branch's
// diff against each [AC-n]. A failed stage blocks the ticket with the three-part handoff. A clean finish sets
// Done when the policy allows it and the ticket carries no human-review label, and To Verify (with the report as
// a comment) otherwise, including when the ticket lists no criteria or the review could not run: an unchecked
// ticket is never Done. A branch that merges into main or master is proposed by pull request, and its ticket
// waits in To Verify for a person to merge it.
func (l *Lifecycle) Finish(ctx context.Context, ref string, c FinishChecks) (FinishResult, error) {
	st, issue, err := l.Board.Status(ctx, ref)
	if err != nil {
		return FinishResult{}, err
	}
	ticket := canonical(issue, ref)
	if st != tracker.StatusInProgress {
		return FinishResult{}, fmt.Errorf("%w: %s is %s", ErrNotInProgress, ticket, tracker.StatusDisplayName(st))
	}
	res := FinishResult{Ticket: ticket}

	res.Gate = runGate(ctx, c, l.Dir, ticket)
	if !res.Gate.Passed {
		return l.blockFinish(ctx, res, "The quality gate failed.\n"+res.Gate.Report, "Make the quality gate pass, then finish again.")
	}

	qc, err := l.QueryContext(ctx, ticket)
	if err != nil {
		return FinishResult{}, err
	}
	if !c.AutoDone {
		res.Notes = append(res.Notes, "automatic Done is off for this project")
	}
	reviewed := l.reviewCriteria(ctx, c, ticket, qc, &res) // both stages always run, so every reason reaches the ticket
	held := l.heldByLabel(ctx, c, ticket, &res)
	proposed := l.propose(ctx, c, qc, &res)
	human := !c.AutoDone || reviewed || held || proposed

	return l.settle(ctx, res, human)
}

// settle posts the finish report and moves the ticket: To Verify when a person has to verify, else Done with the
// claim released.
func (l *Lifecycle) settle(ctx context.Context, res FinishResult, human bool) (FinishResult, error) {
	claim, _ := l.Store.ClaimFor(l.Branch(l.Dir))
	if err := l.post(ctx, res.Ticket, "milestone", finishReport(res), claim); err != nil {
		return FinishResult{}, err
	}
	if human {
		out, err := l.Transition(ctx, res.Ticket, "verify", Handoff{})
		res.State = out.State
		return res, err
	}
	if err := l.Board.set(ctx, res.Ticket, tracker.StatusDone); err != nil {
		return FinishResult{}, fmt.Errorf("set %s to Done: %w", res.Ticket, err)
	}
	res.State = tracker.StatusDisplayName(tracker.StatusDone)
	return res, l.Store.ReleaseClaim(l.Branch(l.Dir))
}

// runGate is stage 1 of a finish; a gate that cannot run fails with the reason as its report.
func runGate(ctx context.Context, c FinishChecks, dir, ticket string) GateResult {
	gate, err := c.Gate(ctx, dir, ticket)
	if err != nil {
		return GateResult{Report: "the quality gate could not run: " + err.Error()}
	}
	return gate
}

// reviewCriteria has the acceptance review judge the diff against each criterion of the ticket and records the
// verdicts and notes in res. It reports whether a person has to verify: the ticket lists no criteria, the review
// could not run, or it found no evidence for one.
func (l *Lifecycle) reviewCriteria(ctx context.Context, c FinishChecks, ticket string, qc Context, res *FinishResult) (human bool) {
	if len(qc.AcceptanceCriteria) == 0 {
		res.Notes = append(res.Notes, "the ticket lists no [AC-n] criteria, so there was nothing to review and a person has to verify")
		return true
	}
	verdicts, err := c.Review(ctx, ReviewInput{Ticket: ticket, Description: qc.Description, Criteria: qc.AcceptanceCriteria, Diff: c.Diff(l.Dir, ticket)})
	if err != nil {
		res.Notes = append(res.Notes, "the acceptance review could not run ("+err.Error()+"), so a person has to verify")
		return true
	}
	res.Verdicts = verdicts
	// The review can fail to prove a criterion, or doubt it, but only the quality gate blocks a ticket: a small
	// model cannot prove an absence, so its doubt goes to a person with its reason.
	if unproven := judgeCriteria(qc.AcceptanceCriteria, verdicts); len(unproven) > 0 {
		res.Notes = append(res.Notes, "no verifiable evidence in the change for:\n"+strings.Join(unproven, "\n"))
		return true
	}
	return false
}

// heldByLabel reports whether the ticket carries a human-review label, or its labels cannot be read, and notes why.
func (l *Lifecycle) heldByLabel(ctx context.Context, c FinishChecks, ticket string, res *FinishResult) bool {
	_, labels, err := l.Board.Svc.PatchLabels(ctx, l.Board.Client, ticket, nil, nil)
	if err != nil {
		res.Notes = append(res.Notes, "the ticket's labels could not be read ("+err.Error()+"), so a person has to verify")
		return true
	}
	held := firstLabel(labels, c.humanReviewLabels())
	if held != "" {
		res.Notes = append(res.Notes, "the ticket is labelled "+held)
	}
	return held != ""
}

// blockFinish blocks the ticket with a handoff built from what the branch holds and why finishing failed.
func (l *Lifecycle) blockFinish(ctx context.Context, res FinishResult, challenges, missing string) (FinishResult, error) {
	claim, _ := l.Store.ClaimFor(l.Branch(l.Dir))
	claim.Worktree = firstNonEmpty(claim.Worktree, l.Dir)
	claim.Branch = firstNonEmpty(claim.Branch, l.Branch(l.Dir))
	h := Handoff{Completed: l.completedWork(ctx, claim), Challenges: challenges, Missing: missing + "\n" + l.pendingWork(ctx, claim)}
	out, err := l.Transition(ctx, res.Ticket, "blocked", h)
	res.State = out.State
	return res, err
}

// judgeCriteria lists the criteria the review did not prove: the ones it doubts (with its reason), the ones it
// found no verifiable evidence for, and the ones it gave no verdict. A verdict matches a criterion by its AC-n id.
func judgeCriteria(criteria []string, verdicts []ACVerdict) (unproven []string) {
	byID := map[string]ACVerdict{}
	for _, v := range verdicts {
		byID[acID(v.AC)] = v
	}
	for _, c := range criteria {
		v, ok := byID[acID(acItem.FindString(c))]
		switch {
		case !ok:
			unproven = append(unproven, "- "+c+": the review gave no verdict")
		case v.Status == StatusNotMet:
			unproven = append(unproven, "- "+c+": the review believes it is not met: "+firstNonEmpty(v.Reason, "no reason given"))
		case v.Status != StatusMet:
			unproven = append(unproven, "- "+c+": "+firstNonEmpty(v.Reason, "unproven"))
		}
	}
	return unproven
}

// acID normalises "[AC-1]", "AC-1" and "ac-1" to "AC-1".
func acID(s string) string {
	return strings.ToUpper(strings.Trim(strings.TrimSpace(s), "[]"))
}

// humanReviewLabels are the labels that hold a ticket for a person: the configured ones, else the default.
func (c FinishChecks) humanReviewLabels() []string {
	if len(c.HumanReviewLabels) == 0 {
		return []string{HumanReviewLabel}
	}
	return c.HumanReviewLabels
}

// firstLabel is the first of want that labels holds, in the spelling of want, or "" when it holds none.
func firstLabel(labels, want []string) string {
	for _, w := range want {
		if hasLabel(labels, w) {
			return w
		}
	}
	return ""
}

func hasLabel(labels []string, want string) bool {
	for _, l := range labels {
		if strings.EqualFold(l, want) {
			return true
		}
	}
	return false
}

// finishReport is the comment a clean finish leaves: the gate report, one line per criterion and the notes.
func finishReport(res FinishResult) string {
	lines := []string{"Finished: the quality gate passed.", strings.TrimSpace(res.Gate.Report)}
	for _, v := range res.Verdicts {
		line := fmt.Sprintf("- %s %s: %s", v.AC, v.Status, v.Reason)
		if v.Evidence != "" {
			line += " | evidence: " + v.Evidence
		}
		lines = append(lines, line)
	}
	for _, n := range res.Notes {
		lines = append(lines, "Note: "+n)
	}
	return strings.Join(lines, "\n")
}

// gateTimeout bounds the test run of the default gate.
const gateTimeout = 30 * time.Minute

// DefaultFinishChecks wires `finish` to the real world for the working copy in dir: the project's own test
// and coverage run as the gate, the local model behind the LiteLLM gateway as the acceptance reviewer, the
// branch's diff against the integration branch, and the project's `[done] auto_done` directive (on when unset).
func DefaultFinishChecks(dir string) FinishChecks {
	return FinishChecks{Gate: qualityGate, Review: proofReviewer(dir).Review, Diff: branchDiff, Propose: proposeByPullRequest, AutoDone: AutoDone(dir), HumanReviewLabels: HumanReviewLabels(dir)}
}

// qualityGate is stage 1 of a finish: the project's tests, then the quality gates of its directives judged on what
// the ticket changed, so a ticket is held to the directives for its own work and not failed for the debt of files
// it did not touch. The repository-wide gate is what CI runs.
func qualityGate(ctx context.Context, dir, ticket string) (GateResult, error) {
	tests, err := toolchainGate(ctx, dir)
	if err != nil || !tests.Passed {
		return tests, err
	}
	set, err := directive.Load(dir)
	if err != nil {
		return GateResult{Report: tests.Report + "\n\nThe quality gate could not read the directives: " + err.Error()}, nil
	}
	rep := gate.Run(ctx, gate.Options{Dir: dir, Set: set, Sidecar: rag.NewRetriever(), Scope: gate.ChangesOfTicket(dir, ticket)})
	// Passing test output is noise next to the gate's findings: keep the command and say it passed.
	command, _, _ := strings.Cut(tests.Report, "\n")
	return GateResult{Passed: rep.Passed, Report: command + " passed.\n\n" + rep.Markdown()}, nil
}

// toolchainGate runs the tests of the detected toolchain. It is the floor of the quality gate: the directive
// gates (production lines, duplication, coverage floor) join it as they land.
func toolchainGate(ctx context.Context, dir string) (GateResult, error) {
	adapter, err := assert.DetectToolchain(dir)
	if err != nil {
		return GateResult{}, err
	}
	argv := adapter.TestCommand("", nil)
	for _, a := range argv { // the profile's directory (.dendrite/cov) does not exist in a fresh checkout
		if path, ok := strings.CutPrefix(a, "-coverprofile="); ok {
			_ = os.MkdirAll(filepath.Dir(filepath.Join(dir, path)), 0o755)
		}
	}
	ctx, cancel := context.WithTimeout(ctx, gateTimeout)
	defer cancel()
	cmd := exec.CommandContext(ctx, argv[0], argv[1:]...)
	cmd.Dir = dir
	cmd.Env = gate.CleanEnv(os.Environ())
	out, runErr := cmd.CombinedOutput()
	return GateResult{Passed: runErr == nil, Report: gateReport(argv, string(out), runErr == nil)}, nil
}

// failureLine matches what test runners print for a failure, whatever the language.
var failureLine = regexp.MustCompile(`(?i)^(FAIL\b|--- FAIL|panic:|error[:\[]|FAILED)|DATA RACE|\bFAILED\b`)

// maxFailureLines bounds the failures a report lists.
const maxFailureLines = 30

// gateReport describes a gate run. A passing run is the command and the last lines. A failing run lists the
// failing lines first, because the last lines of a long run are usually the packages that passed.
func gateReport(argv []string, output string, passed bool) string {
	head := "`" + strings.Join(argv, " ") + "`\n"
	if passed {
		return head + tail(output, 40)
	}
	var failures []string
	for _, line := range strings.Split(output, "\n") {
		if failureLine.MatchString(strings.TrimSpace(line)) && len(failures) < maxFailureLines {
			failures = append(failures, line)
		}
	}
	if len(failures) == 0 {
		return head + tail(output, 40)
	}
	return head + "Failures:\n" + strings.Join(failures, "\n") + "\n\nEnd of the output:\n" + tail(output, 15)
}

func tail(s string, lines int) string {
	parts := strings.Split(strings.TrimRight(s, "\n"), "\n")
	if len(parts) > lines {
		parts = parts[len(parts)-lines:]
	}
	return strings.Join(parts, "\n")
}

// generatedPaths are left out of the patch the review reads: files Dendrite or a package manager writes, which say
// nothing about the work.
var generatedPaths = []string{
	":(exclude)*.lock", ":(exclude)go.sum", ":(exclude)package-lock.json", ":(exclude)pnpm-lock.yaml", ":(exclude)bun.lockb",
	":(exclude)docs/cli/**", ":(exclude)docs/mcp/**", ":(exclude).agents/**", ":(exclude).claude/**", ":(exclude).gemini/**",
}

// branchDiff is the work done for a ticket: the branch's changes against the integration branch while they are not merged, and
// the ticket's own commits (their subject starts with [TICKET]) once they are, so finishing after the merge
// still shows the work.
func branchDiff(dir, ticket string) string {
	for _, base := range branchpolicy.For(dir).Bases() {
		args := append([]string{"diff", base + "...HEAD", "--", "."}, generatedPaths...)
		if out, err := gitBytes(dir, args...); err == nil && len(strings.TrimSpace(string(out))) > 0 {
			return string(out)
		}
	}
	var b strings.Builder
	shas, _ := gate.TicketCommits(dir, ticket)
	for _, sha := range shas {
		args := append([]string{"show", "--format=", "--patch", sha, "--", "."}, generatedPaths...)
		if out, err := gitBytes(dir, args...); err == nil {
			b.Write(out)
		}
	}
	return b.String()
}

// proofReviewer is the reviewer for the project at dir: the gateway's model, asked for a JSON object, and the
// project's sidecar to find the hunks that matter to each criterion.
func proofReviewer(dir string) ProofReviewer {
	chat := llmchat.Gateway(dir)
	chat.JSON = true
	return ProofReviewer{Chat: chat, Rank: sidecarRanker(dir)}
}

// AutoDone says whether a clean finish may set Done: the `[done] auto_done` directive of the project, on when
// the project has no directives or they cannot be read (a broken file must not silently hand Done to agents,
// so an unreadable file turns it off).
func AutoDone(dir string) bool {
	set, err := directive.Load(dir)
	return err == nil && set.Done.AutoDone
}

// HumanReviewLabels are the labels of the project's `[done] human_review_labels` directive, or nil (meaning the
// default HumanReviewLabel) when the project has none or its directives cannot be read.
func HumanReviewLabels(dir string) []string {
	set, err := directive.Load(dir)
	if err != nil {
		return nil
	}
	return set.Done.HumanReviewLabels
}
