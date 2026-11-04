package planetools

import (
	"Dendrite/pkg/planesvc"
	"Dendrite/pkg/ticketlock"
	"Dendrite/pkg/tracker/plane"
	"context"
	"fmt"
	"os"
	"strings"

	. "Dendrite/pkg/mcpserver"
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
	ticketProp   = TypedProp("string", "Ticket reference: DEN-377, its Plane number or the issue UUID")
	worktreeProp = TypedProp("string", "Working copy the claim unlocks (default: the server's current directory)")
)

// RegisterTicketLifecycleTools registers the tools of the ticket lifecycle: claim, comment, plan, transition
// and context. They share one implementation with the `dendrite chronos` commands (pkg/ticketlock).
func RegisterTicketLifecycleTools(s *Server, deps Dependencies) {
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
			return JsonResult(out)
		})
	}

	register("dendrite_ticket_claim",
		"Claim a ticket for this session before changing any file: moves it to In Progress, assigns it, labels it agent:/tier:, records the claim that unlocks edits in this working copy and returns the ticket's plan, if any. Refused when the ticket is finished or held by another live session.",
		ObjectSchema([]string{"ticket"}, map[string]interface{}{
			"ticket": ticketProp, "worktree": worktreeProp,
			"agent":      TypedProp("string", "claude, agy, copilot, ... (default: detected)"),
			"model_tier": TypedProp("string", "Model tier of this session (default: DENDRITE_MODEL_TIER)"),
			"steal":      TypedProp("boolean", "Take the ticket over from another live session"),
		}),
		ticketClaimRun)

	register("dendrite_ticket_comment",
		"Post a structured progress, milestone, note or handoff comment on a ticket (marker, text, and a footer naming session, agent, tier, branch and commit).",
		ObjectSchema([]string{"ticket", "kind", "body"}, map[string]interface{}{
			"ticket": ticketProp, "worktree": worktreeProp,
			"kind": map[string]interface{}{"type": "string", "description": strings.Join(ticketlock.CommentKinds, ", ")},
			"body": TypedProp("string", "The comment text"),
		}),
		ticketCommentRun)

	register("dendrite_ticket_decision",
		"Record a question you asked the user and the answer you got, or a decision you took, on a ticket. Do it for every decision that shapes the work; Claude Code's AskUserQuestion is recorded by a hook, other agents call this.",
		ObjectSchema([]string{"ticket", "question", "answer"}, map[string]interface{}{
			"ticket": ticketProp, "worktree": worktreeProp,
			"question": TypedProp("string", "The question that was asked"),
			"answer":   TypedProp("string", "The answer or decision"),
			"options":  map[string]interface{}{"type": "array", "items": map[string]interface{}{"type": "string"}, "description": "Options that were on the table"},
			"note":     TypedProp("string", "Why, or what follows from it"),
		}),
		ticketDecisionRun)

	register("dendrite_ticket_comments",
		"Read the comments of a ticket: claims, progress, decisions, handoffs and plans. Filter by kind, or take only the last N. Use it to read a handoff or the decisions already taken before you start.",
		ObjectSchema([]string{"ticket"}, map[string]interface{}{
			"ticket": ticketProp, "worktree": worktreeProp,
			"kind": TypedProp("string", "claim, progress, milestone, note, handoff, decision or plan"),
			"last": TypedProp("number", "Only the last N comments"),
		}),
		ticketCommentsRun)

	register("dendrite_ticket_upload_plan",
		"Store the plan of a ticket (markdown with `- [ ]` / `- [x]` steps) in Plane, as one comment edited in place, and in the local plan store. Call it when the plan changes so a resumed session continues from the next unchecked step.",
		ObjectSchema([]string{"ticket", "markdown"}, map[string]interface{}{
			"ticket": ticketProp, "worktree": worktreeProp,
			"markdown": TypedProp("string", "The plan, with `- [ ]` and `- [x]` steps"),
		}),
		ticketUploadPlanRun)

	register("dendrite_ticket_get_plan",
		"Read the plan of a ticket: from Plane, or from the local plan store when Plane is down. Returns the markdown, its version and the next unchecked step.",
		ObjectSchema([]string{"ticket"}, map[string]interface{}{"ticket": ticketProp, "worktree": worktreeProp}),
		ticketGetPlanRun)

	register("dendrite_ticket_transition",
		"End or pause work on a ticket: verify (finished, awaiting a human), blocked (needs a handoff with completed, challenges and missing), cancelled or backlog. done is set only by `dendrite chronos finish`.",
		ObjectSchema([]string{"ticket", "outcome"}, map[string]interface{}{
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
		ticketTransitionRun)

	register("dendrite_ticket_finish",
		"Finish a ticket: run the quality gate and check the branch's diff against each [AC-n]. Sets Done when the project allows it and the ticket has no needs-human-review label, To Verify otherwise, and Blocked with a handoff when a check fails.",
		ObjectSchema([]string{"ticket"}, map[string]interface{}{"ticket": ticketProp, "worktree": worktreeProp}),
		ticketFinishRun)

	register("dendrite_query_context",
		"Everything needed to start on a ticket: description, acceptance criteria, plan, state and the files it names. Call it right after claiming.",
		ObjectSchema([]string{"ticket"}, map[string]interface{}{"ticket": ticketProp, "worktree": worktreeProp}),
		queryContextRun)
}

// ticketClaimRun runs the dendrite_ticket_claim tool.
func ticketClaimRun(ctx context.Context, l *ticketlock.Lifecycle, a map[string]interface{}) (interface{}, error) {
	steal, _ := a["steal"].(bool)
	return l.Claim(ctx, stringArg(a, "ticket"), ticketlock.ClaimOptions{Agent: stringArg(a, "agent"), Tier: stringArg(a, "model_tier"), Steal: steal})
}

// ticketCommentRun runs the dendrite_ticket_comment tool.
func ticketCommentRun(ctx context.Context, l *ticketlock.Lifecycle, a map[string]interface{}) (interface{}, error) {
	if err := l.Comment(ctx, stringArg(a, "ticket"), stringArg(a, "kind"), stringArg(a, "body")); err != nil {
		return nil, err
	}
	return map[string]interface{}{"ok": true}, nil
}

// ticketDecisionRun runs the dendrite_ticket_decision tool.
func ticketDecisionRun(ctx context.Context, l *ticketlock.Lifecycle, a map[string]interface{}) (interface{}, error) {
	d := ticketlock.Decision{Question: stringArg(a, "question"), Answer: stringArg(a, "answer"), Note: stringArg(a, "note")}
	if raw, ok := a["options"].([]interface{}); ok {
		for _, o := range raw {
			if text, ok := o.(string); ok {
				d.Options = append(d.Options, text)
			}
		}
	}
	if err := l.RecordDecisions(ctx, stringArg(a, "ticket"), []ticketlock.Decision{d}, ""); err != nil {
		return nil, err
	}
	return map[string]interface{}{"ok": true}, nil
}

// ticketCommentsRun runs the dendrite_ticket_comments tool.
func ticketCommentsRun(ctx context.Context, l *ticketlock.Lifecycle, a map[string]interface{}) (interface{}, error) {
	last, _ := a["last"].(float64)
	rows, err := l.Comments(ctx, stringArg(a, "ticket"), stringArg(a, "kind"), int(last))
	return map[string]interface{}{"comments": rows}, err
}

// ticketUploadPlanRun runs the dendrite_ticket_upload_plan tool.
func ticketUploadPlanRun(ctx context.Context, l *ticketlock.Lifecycle, a map[string]interface{}) (interface{}, error) {
	md, _ := a["markdown"].(string)
	return l.UploadPlan(ctx, stringArg(a, "ticket"), md)
}

// ticketGetPlanRun runs the dendrite_ticket_get_plan tool.
func ticketGetPlanRun(ctx context.Context, l *ticketlock.Lifecycle, a map[string]interface{}) (interface{}, error) {
	return l.GetPlan(ctx, stringArg(a, "ticket"))
}

// ticketTransitionRun runs the dendrite_ticket_transition tool.
func ticketTransitionRun(ctx context.Context, l *ticketlock.Lifecycle, a map[string]interface{}) (interface{}, error) {
	var h ticketlock.Handoff
	if raw, ok := a["handoff"].(map[string]interface{}); ok {
		h.Completed, _ = raw["completed"].(string)
		h.Challenges, _ = raw["challenges"].(string)
		h.Missing, _ = raw["missing"].(string)
	}
	return l.Transition(ctx, stringArg(a, "ticket"), stringArg(a, "outcome"), h)
}

// ticketFinishRun runs the dendrite_ticket_finish tool.
func ticketFinishRun(ctx context.Context, l *ticketlock.Lifecycle, a map[string]interface{}) (interface{}, error) {
	return l.Finish(ctx, stringArg(a, "ticket"), ticketlock.DefaultFinishChecks(l.Dir))
}

// queryContextRun runs the dendrite_query_context tool.
func queryContextRun(ctx context.Context, l *ticketlock.Lifecycle, a map[string]interface{}) (interface{}, error) {
	return l.QueryContext(ctx, stringArg(a, "ticket"))
}
