package planetools

import (
	"Dendrite/pkg/rea/ownership"
	"Dendrite/pkg/ticketlock"
	"context"

	. "Dendrite/pkg/mcpserver"
)

// RegisterReaTools registers the REA tools: dendrite_rea_handoff hands an REA ticket a session refuses to AGY,
// as the project's [rea] directive says. ports builds the effects of the handoff for a lifecycle.
func RegisterReaTools(s *Server, deps Dependencies, ports func(*ticketlock.Lifecycle) ownership.Ports) {
	s.RegisterTool(Tool{
		Name: "dendrite_rea_handoff",
		Description: "Hand a reverse-engineering (REA) ticket you refuse to do to AGY instead of leaving it half done: records the refusal on the ticket, " +
			"starts an AGY job in an isolated working copy, labels the ticket owner:agy and releases this session's claim. " +
			"Repeating it for a ticket AGY already owns changes nothing; a ticket that is not REA work is refused.",
		InputSchema: ObjectSchema([]string{"ticket"}, map[string]interface{}{
			"ticket": ticketProp, "worktree": worktreeProp,
			"reason": TypedProp("string", "Why you refuse, in a sentence; kept on the ticket"),
		}),
	}, func(ctx context.Context, args map[string]interface{}) (*CallToolResult, error) {
		if err := requireArgs(args, "ticket"); err != nil {
			return nil, err
		}
		l, err := lifecycleFor(deps, args)
		if err != nil {
			return nil, err
		}
		set, err := ownership.LoadRule(l.Dir)
		if err != nil {
			return nil, err
		}
		res, err := ownership.Transfer(ctx, ports(l), set.Rea, stringArg(args, "ticket"),
			ownership.Refusal{Reason: "explicit", Excerpt: stringArg(args, "reason")})
		if err != nil {
			return nil, err
		}
		return JsonResult(res)
	})
}
