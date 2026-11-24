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
// is not acted on here, because the session may go on (the user can answer): it marks the claim as suspected,
// the next reply that does not decline clears the mark, and the session-end hook acts on a mark that stands.
// Anything it cannot judge (no claim, another agent, no transcript, a ticket that is not REA work) leaves
// everything alone.
func RefusalHandoff(ctx context.Context, l *ticketlock.Lifecycle, board func() (ticketlock.Board, error), in ticketlock.HookInput) (string, error) {
	claim, set, ok := refusalContext(l, in)
	if !ok || in.TranscriptPath == "" {
		return "", nil
	}
	why, refused := ownership.DetectRefusal(in.TranscriptPath)
	switch {
	case refused && why.Confidence == ownership.ConfidenceExact:
		return handOver(ctx, l, board, claim, set, why)
	case refused:
		return "", setSuspected(l, claim, &ticketlock.SuspectedRefusal{Excerpt: why.Excerpt, At: l.Now()})
	default:
		return "", setSuspected(l, claim, nil)
	}
}

// setSuspected stores the mark on the claim, or clears it when mark is nil; an unchanged claim is not rewritten.
func setSuspected(l *ticketlock.Lifecycle, claim ticketlock.Claim, mark *ticketlock.SuspectedRefusal) error {
	if mark == nil && claim.RefusalSuspected == nil {
		return nil
	}
	if mark != nil && claim.RefusalSuspected != nil && claim.RefusalSuspected.Excerpt == mark.Excerpt {
		return nil
	}
	claim.RefusalSuspected = mark
	return l.Store.WriteClaim(claim)
}

// refusalContext is a Claude session with a claim, in a project whose [rea] directive hands refusals to AGY.
// ok is false for anything else.
func refusalContext(l *ticketlock.Lifecycle, in ticketlock.HookInput) (claim ticketlock.Claim, set directive.Set, ok bool) {
	claim, claimed := l.Store.ClaimOfDir(in.Cwd)
	if !claimed || !strings.EqualFold(claim.Agent, "claude") {
		return
	}
	set, err := ownership.LoadRule(l.Dir)
	ok = err == nil && strings.EqualFold(set.Rea.RefusalOwner, directive.RefusalOwnerAGY)
	return
}

// handOver gives the claimed ticket to AGY. A ticket that is not REA work, or that AGY already owns, is left alone.
func handOver(ctx context.Context, l *ticketlock.Lifecycle, board func() (ticketlock.Board, error), claim ticketlock.Claim, set directive.Set, why ownership.Refusal) (string, error) {
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

// sessionEndRefusal hands the ticket to AGY when the session ends on an exact refusal, or while the claim still
// carries the suspected mark of its last reply.
func sessionEndRefusal(ctx context.Context, l *ticketlock.Lifecycle, board func() (ticketlock.Board, error), in ticketlock.HookInput) (string, error) {
	claim, set, ok := refusalContext(l, in)
	if !ok {
		return "", nil
	}
	why, refused := ownership.DetectRefusal(in.TranscriptPath)
	switch {
	case refused && why.Confidence == ownership.ConfidenceExact:
	case claim.RefusalSuspected != nil:
		why = ownership.Refusal{Reason: "declined", Excerpt: claim.RefusalSuspected.Excerpt, Confidence: ownership.ConfidenceSuspected}
	default:
		return "", nil
	}
	return handOver(ctx, l, board, claim, set, why)
}

// HandoffOnSessionEnd is the session-end hook: a refusal (exact, or the suspected mark of the session's last reply) hands the ticket to AGY; otherwise a ticket still In
// Progress is blocked with an automatic handoff. A refusal that could not be handed over still gets the
// automatic handoff, and both problems are reported.
func HandoffOnSessionEnd(ctx context.Context, l *ticketlock.Lifecycle, board func() (ticketlock.Board, error), in ticketlock.HookInput) (string, error) {
	msg, refusalErr := sessionEndRefusal(ctx, l, board, in)
	if msg != "" {
		return msg, nil
	}
	auto, autoErr := ticketlock.AutoHandoff(ctx, l, board, in)
	return auto, errors.Join(refusalErr, autoErr)
}
