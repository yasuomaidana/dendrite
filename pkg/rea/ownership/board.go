package ownership

import (
	"context"
	"strings"

	"Dendrite/pkg/agentjobs"
	"Dendrite/pkg/ticketlock"
)

// BoardPorts are the effects of a transfer on the Plane board and claim store of the lifecycle l, with the real
// AGY job dispatcher. Releasing claims leaves the ones AGY holds.
func BoardPorts(l *ticketlock.Lifecycle) Ports {
	return Ports{
		Issue: func(ctx context.Context, ref string) (Issue, error) {
			_, issue, err := l.Board.Status(ctx, ref)
			if err != nil {
				return Issue{}, err
			}
			ticket := ticketlock.CanonicalTicket(issue, ref)
			_, names, err := l.Board.Svc.PatchLabels(ctx, l.Board.Client, ticket, nil, nil)
			if err != nil {
				return Issue{}, err
			}
			return Issue{Ticket: ticket, Title: issue.Name, Labels: names, Description: issue.DescriptionStripped,
				Estimate: points(issue.EstimatePoint)}, nil
		},
		Comment: l.Comment,
		AddLabel: func(ctx context.Context, ticket, label string) error {
			_, _, err := l.Board.Svc.PatchLabels(ctx, l.Board.Client, ticket, []string{label}, nil)
			return err
		},
		ReleaseClaims: func(ticket string) error {
			for _, c := range l.Store.ClaimsOf(ticket) {
				if strings.EqualFold(c.Agent, "agy") {
					continue
				}
				if err := l.Store.ReleaseClaim(c.Branch); err != nil {
					return err
				}
			}
			return nil
		},
		Submit: agentjobs.Submit,
	}
}

// points is the estimate of a ticket, 0 when it has none.
func points(p *int) int {
	if p == nil {
		return 0
	}
	return *p
}
