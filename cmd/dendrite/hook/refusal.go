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

// RefusalHandoff is the Stop hook: when the Claude session that claimed an REA ticket ends its turn with a
// refusal, the ticket goes to AGY as the [rea] directive says. Anything it cannot judge (no claim, another
// agent, no transcript, a ticket that is not REA work) leaves everything alone.
func RefusalHandoff(ctx context.Context, l *ticketlock.Lifecycle, board func() (ticketlock.Board, error), in ticketlock.HookInput) (string, error) {
	claim, claimed := l.Store.ClaimOfDir(in.Cwd)
	if !claimed || !strings.EqualFold(claim.Agent, "claude") {
		return "", nil
	}
	set, err := ownership.LoadRule(l.Dir)
	if err != nil || !strings.EqualFold(set.Rea.RefusalOwner, directive.RefusalOwnerAGY) {
		return "", nil
	}
	why, refused := ownership.DetectRefusal(in.TranscriptPath)
	if !refused {
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

// HandoffOnSessionEnd is the session-end hook: a refusal hands the ticket to AGY; otherwise a ticket still In
// Progress is blocked with an automatic handoff. A refusal that could not be handed over still gets the
// automatic handoff, and both problems are reported.
func HandoffOnSessionEnd(ctx context.Context, l *ticketlock.Lifecycle, board func() (ticketlock.Board, error), in ticketlock.HookInput) (string, error) {
	msg, refusalErr := RefusalHandoff(ctx, l, board, in)
	if msg != "" {
		return msg, nil
	}
	auto, autoErr := ticketlock.AutoHandoff(ctx, l, board, in)
	return auto, errors.Join(refusalErr, autoErr)
}
