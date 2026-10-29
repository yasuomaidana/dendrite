package mcpserver

import (
	"context"
	"fmt"
	"os"
	"strings"

	"Dendrite/pkg/planesvc"
	"Dendrite/pkg/ticketlock"
	"Dendrite/pkg/tracker/plane"
)

// lifecycleFor builds the lifecycle of the session: the Plane project of the repository, and the working copy
// named by the optional worktree argument (the server's current directory when absent), which is the copy a
// claim unlocks for editing.
func lifecycleFor(deps Dependencies, args map[string]interface{}) (*ticketlock.Lifecycle, error) {
	client := deps.PlaneClient
	if client == nil {
		var err error
		if client, err = plane.NewClientFromEnv(); err != nil {
			return nil, fmt.Errorf("plane client error: %w", err)
		}
	}
	dir, _ := args["worktree"].(string)
	if strings.TrimSpace(dir) == "" {
		dir, _ = os.Getwd()
	}
	return ticketlock.NewLifecycle(ticketlock.Board{Svc: planesvc.Default(), Client: client}, dir), nil
}

func stringArg(args map[string]interface{}, key string) string {
	s, _ := args[key].(string)
	return strings.TrimSpace(s)
}

func requireArgs(args map[string]interface{}, keys ...string) error {
	for _, k := range keys {
		if stringArg(args, k) == "" {
			return fmt.Errorf("%s is required", strings.Join(keys, ", "))
		}
	}
	return nil
}

var (
	ticketProp   = map[string]interface{}{"type": "string", "description": "Ticket reference: DEN-377, its Plane number or the issue UUID"}
	worktreeProp = map[string]interface{}{"type": "string", "description": "Working copy the claim unlocks (default: the server's current directory)"}
)

func objectSchema(required []string, props map[string]interface{}) map[string]interface{} {
	return map[string]interface{}{"type": "object", "properties": props, "required": required}
}

// registerTicketLifecycleTools registers the tools of the ticket lifecycle: claim, comment, plan, transition
// and context. They share one implementation with the `dendrite chronos` commands (pkg/ticketlock).
func registerTicketLifecycleTools(s *Server, deps Dependencies) {
	register := func(name, desc string, schema map[string]interface{}, run func(ctx context.Context, l *ticketlock.Lifecycle, args map[string]interface{}) (interface{}, error)) {
		s.RegisterTool(Tool{Name: name, Description: desc, InputSchema: schema}, func(ctx context.Context, args map[string]interface{}) (*CallToolResult, error) {
			if err := requireArgs(args, "ticket"); err != nil {
				return nil, err
			}
			l, err := lifecycleFor(deps, args)
			if err != nil {
				return nil, err
			}
			out, err := run(ctx, l, args)
			if err != nil {
				return nil, err
			}
			return jsonResult(out)
		})
	}

	register("dendrite_ticket_claim",
		"Claim a ticket for this session before changing any file: moves it to In Progress, assigns it, labels it agent:/tier:, records the claim that unlocks edits in this working copy and returns the ticket's plan, if any. Refused when the ticket is finished or held by another live session.",
		objectSchema([]string{"ticket"}, map[string]interface{}{
			"ticket": ticketProp, "worktree": worktreeProp,
			"agent":      map[string]interface{}{"type": "string", "description": "claude, agy, copilot, ... (default: detected)"},
			"model_tier": map[string]interface{}{"type": "string", "description": "Model tier of this session (default: DENDRITE_MODEL_TIER)"},
			"steal":      map[string]interface{}{"type": "boolean", "description": "Take the ticket over from another live session"},
		}),
		func(ctx context.Context, l *ticketlock.Lifecycle, a map[string]interface{}) (interface{}, error) {
			steal, _ := a["steal"].(bool)
			return l.Claim(ctx, stringArg(a, "ticket"), ticketlock.ClaimOptions{Agent: stringArg(a, "agent"), Tier: stringArg(a, "model_tier"), Steal: steal})
		})

	register("dendrite_ticket_comment",
		"Post a structured progress, milestone, note or handoff comment on a ticket (marker, text, and a footer naming session, agent, tier, branch and commit).",
		objectSchema([]string{"ticket", "kind", "body"}, map[string]interface{}{
			"ticket": ticketProp, "worktree": worktreeProp,
			"kind": map[string]interface{}{"type": "string", "description": strings.Join(ticketlock.CommentKinds, ", ")},
			"body": map[string]interface{}{"type": "string", "description": "The comment text"},
		}),
		func(ctx context.Context, l *ticketlock.Lifecycle, a map[string]interface{}) (interface{}, error) {
			if err := l.Comment(ctx, stringArg(a, "ticket"), stringArg(a, "kind"), stringArg(a, "body")); err != nil {
				return nil, err
			}
			return map[string]interface{}{"ok": true}, nil
		})

	register("dendrite_ticket_upload_plan",
		"Store the plan of a ticket (markdown with `- [ ]` / `- [x]` steps) in Plane, as one comment edited in place, and in the local plan store. Call it when the plan changes so a resumed session continues from the next unchecked step.",
		objectSchema([]string{"ticket", "markdown"}, map[string]interface{}{
			"ticket": ticketProp, "worktree": worktreeProp,
			"markdown": map[string]interface{}{"type": "string", "description": "The plan, with `- [ ]` and `- [x]` steps"},
		}),
		func(ctx context.Context, l *ticketlock.Lifecycle, a map[string]interface{}) (interface{}, error) {
			md, _ := a["markdown"].(string)
			return l.UploadPlan(ctx, stringArg(a, "ticket"), md)
		})

	register("dendrite_ticket_get_plan",
		"Read the plan of a ticket: from Plane, or from the local plan store when Plane is down. Returns the markdown, its version and the next unchecked step.",
		objectSchema([]string{"ticket"}, map[string]interface{}{"ticket": ticketProp, "worktree": worktreeProp}),
		func(ctx context.Context, l *ticketlock.Lifecycle, a map[string]interface{}) (interface{}, error) {
			return l.GetPlan(ctx, stringArg(a, "ticket"))
		})

	register("dendrite_ticket_transition",
		"End or pause work on a ticket: verify (finished, awaiting a human), blocked (needs a handoff with completed, challenges and missing), cancelled or backlog. done is set only by `dendrite chronos finish`.",
		objectSchema([]string{"ticket", "outcome"}, map[string]interface{}{
			"ticket": ticketProp, "worktree": worktreeProp,
			"outcome": map[string]interface{}{"type": "string", "description": strings.Join(ticketlock.Outcomes, ", ")},
			"handoff": map[string]interface{}{
				"type":        "object",
				"description": "Required for blocked: what is done, what got in the way, what is missing against the acceptance criteria",
				"properties": map[string]interface{}{
					"completed":  map[string]interface{}{"type": "string"},
					"challenges": map[string]interface{}{"type": "string"},
					"missing":    map[string]interface{}{"type": "string"},
				},
			},
		}),
		func(ctx context.Context, l *ticketlock.Lifecycle, a map[string]interface{}) (interface{}, error) {
			var h ticketlock.Handoff
			if raw, ok := a["handoff"].(map[string]interface{}); ok {
				h.Completed, _ = raw["completed"].(string)
				h.Challenges, _ = raw["challenges"].(string)
				h.Missing, _ = raw["missing"].(string)
			}
			return l.Transition(ctx, stringArg(a, "ticket"), stringArg(a, "outcome"), h)
		})

	register("dendrite_query_context",
		"Everything needed to start on a ticket: description, acceptance criteria, plan, state and the files it names. Call it right after claiming.",
		objectSchema([]string{"ticket"}, map[string]interface{}{"ticket": ticketProp, "worktree": worktreeProp}),
		func(ctx context.Context, l *ticketlock.Lifecycle, a map[string]interface{}) (interface{}, error) {
			return l.QueryContext(ctx, stringArg(a, "ticket"))
		})
}
