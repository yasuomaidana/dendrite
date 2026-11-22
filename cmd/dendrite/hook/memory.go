package hook

import (
	"Dendrite/pkg/memory"
	"Dendrite/pkg/projectid"
	"Dendrite/pkg/ticketlock"
	"context"
	"errors"
	"fmt"
	"strings"
)

// hookAct is what a ticket hook does with its event; runAfterHook runs one.
type hookAct = func(context.Context, *ticketlock.Lifecycle, func() (ticketlock.Board, error), ticketlock.HookInput) (string, error)

// memoryHome is the Dendrite home that holds the memory mode (a seam for tests).
var memoryHome = projectid.HomeDir

const (
	maxTitle = 100
	maxBody  = 2000
)

// journaling journals what the hook's event carries, tagged with the ticket the working copy claimed, and
// then runs act. A memory that cannot be journaled never changes what the hook does.
func journaling(act hookAct, entries func(ticketlock.HookInput) []memory.Entry) hookAct {
	return func(ctx context.Context, l *ticketlock.Lifecycle, board func() (ticketlock.Board, error), in ticketlock.HookInput) (string, error) {
		if home, err := memoryHome(); err == nil {
			claim, _ := l.Store.ClaimOfDir(in.Cwd)
			for _, e := range entries(in) {
				e.Ticket, e.Source = claim.Ticket, "hook:"+in.ToolName
				_ = memory.Record(home, memory.DefaultJournal(), e)
			}
		}
		return act(ctx, l, board, in)
	}
}

// flushing runs act and then flushes the journal into Engram, so a session that ends leaves its memories
// in the store the next session reads. It says something only when it flushed or failed.
func flushing(act hookAct) hookAct {
	return func(ctx context.Context, l *ticketlock.Lifecycle, board func() (ticketlock.Board, error), in ticketlock.HookInput) (string, error) {
		msg, err := act(ctx, l, board, in)
		home, herr := memoryHome()
		if herr != nil {
			return msg, err
		}
		n, ferr := memory.FlushPending(ctx, home, memory.DefaultJournal())
		if n > 0 {
			msg = strings.TrimSpace(msg + fmt.Sprintf("\n%d memory item(s) flushed to Engram", n))
		}
		return msg, errors.Join(err, ferr)
	}
}

// decisionEntries are the answers a user gave to an AskUserQuestion call.
func decisionEntries(in ticketlock.HookInput) []memory.Entry {
	var out []memory.Entry
	for _, d := range ticketlock.DecisionsFromEvent(in) {
		out = append(out, memory.Entry{
			Type:  "decision",
			Title: "Decision: " + memory.Clip(d.Question, maxTitle),
			Body:  memory.Clip("Q: "+d.Question+"\nA: "+d.Answer, maxBody),
		})
	}
	return out
}

// planEntries is the plan an agent got approved.
func planEntries(in ticketlock.HookInput) []memory.Entry {
	plan := strings.TrimSpace(in.ToolInput.Plan)
	if plan == "" {
		return nil
	}
	first, _, _ := strings.Cut(strings.TrimLeft(plan, "# \n"), "\n")
	return []memory.Entry{{Type: "plan", Title: "Plan: " + memory.Clip(first, maxTitle), Body: memory.Clip(plan, maxBody)}}
}
