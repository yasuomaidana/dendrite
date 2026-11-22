package hook

import (
	"Dendrite/pkg/directive"
	"Dendrite/pkg/rea/ownership"
	"Dendrite/pkg/ticketlock"
	"context"
	"errors"
	"fmt"
	"strings"
)

// refusalPorts builds the effects of a transfer (a seam for tests).
var refusalPorts = ownership.BoardPorts

// RefusalHandoff is the Stop hook: when the Claude session that claimed an REA ticket ends its turn with an API
// refusal, the ticket goes to AGY as the [rea] directive says. A refusal that is only the wording of a reply
// is not acted on here, because the session may go on (the user can answer); it waits for the session to end.
// Anything it cannot judge (no claim, another agent, no transcript, a ticket that is not REA work) leaves
// everything alone.
func RefusalHandoff(ctx context.Context, l *ticketlock.Lifecycle, board func() (ticketlock.Board, error), in ticketlock.HookInput) (string, error) {
	return refusalHandoff(ctx, l, board, in, false)
}

// refusalHandoff hands the ticket over on an exact refusal, and on a suspected one when final says the session
// is over and the refusal was its last word.
func refusalHandoff(ctx context.Context, l *ticketlock.Lifecycle, board func() (ticketlock.Board, error), in ticketlock.HookInput, final bool) (string, error) {
	claim, set, why, act := judgeRefusal(l, in, final)
	if !act {
		return "", nil
	}
	b, err := board()
	if err != nil {
		return "", fmt.Errorf("refusal of %s not handed to AGY: %w", claim.Ticket, err)
	}
	l.Board = b
	res, err := ownership.Transfer(ctx, refusalPorts(l), set.Rea, claim.Ticket, why)
	switch {
	case errors.Is(err, ownership.ErrNotREA), err == nil && res.AlreadyOwned:
		return "", nil
	case err != nil:
		return "", err
	}
	return fmt.Sprintf("%s refused by Claude: handed to AGY (job %s)", res.Ticket, res.JobID), nil
}

// judgeRefusal decides whether the event is a refusal to act on: a Claude session with a claim, a project that
// hands refusals to AGY, and a refusal that is exact (or suspected, when the session is over). act is false for
// anything it cannot judge.
func judgeRefusal(l *ticketlock.Lifecycle, in ticketlock.HookInput, final bool) (claim ticketlock.Claim, set directive.Set, why ownership.Refusal, act bool) {
	claim, claimed := l.Store.ClaimOfDir(in.Cwd)
	if !claimed || !strings.EqualFold(claim.Agent, "claude") {
		return
	}
	set, err := ownership.LoadRule(l.Dir)
	if err != nil || !strings.EqualFold(set.Rea.RefusalOwner, directive.RefusalOwnerAGY) {
		return
	}
	why, refused := ownership.DetectRefusal(in.TranscriptPath)
	act = refused && (why.Confidence == ownership.ConfidenceExact || final)
	return
}

// HandoffOnSessionEnd is the session-end hook: a refusal (exact, or the wording of the session's last reply) hands the ticket to AGY; otherwise a ticket still In
// Progress is blocked with an automatic handoff. A refusal that could not be handed over still gets the
// automatic handoff, and both problems are reported.
func HandoffOnSessionEnd(ctx context.Context, l *ticketlock.Lifecycle, board func() (ticketlock.Board, error), in ticketlock.HookInput) (string, error) {
	msg, refusalErr := refusalHandoff(ctx, l, board, in, true)
	if msg != "" {
		return msg, nil
	}
	auto, autoErr := ticketlock.AutoHandoff(ctx, l, board, in)
	return auto, errors.Join(refusalErr, autoErr)
}
