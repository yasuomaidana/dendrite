package agent

import (
	"Dendrite/pkg/agentjobs"
	"Dendrite/pkg/memory"
	"Dendrite/pkg/projectid"
	"context"
	"fmt"
)

// runJob runs a job in the foreground (a seam for tests).
var runJob = agentjobs.RunJob

// runJobAndRemember runs the job and then journals how it ended, so the next session knows what the agent
// did. Journaling is best-effort: it never changes the job's result.
func runJobAndRemember(ctx context.Context, id string) error {
	err := runJob(ctx, id)
	if job, loadErr := agentjobs.NewLedger("").Load(id); loadErr == nil {
		rememberOutcome(job)
	}
	return err
}

// rememberOutcome journals a job's final status. Only a final status is worth remembering: a job that will
// be retried is not over.
func rememberOutcome(job *agentjobs.Job) {
	switch job.Status {
	case agentjobs.StatusSucceeded, agentjobs.StatusEarlyExit, agentjobs.StatusNeedsReview, agentjobs.StatusFailed:
	default:
		return
	}
	home, err := projectid.HomeDir()
	if err != nil {
		return
	}
	attempts := 0
	for _, n := range job.Attempts {
		attempts += n
	}
	_ = memory.Record(home, memory.DefaultJournal(), memory.Entry{
		Type:   "outcome",
		Ticket: job.Ticket,
		Source: "job " + job.ID,
		Title:  fmt.Sprintf("Job %s %s", job.ID, job.Status),
		Body:   fmt.Sprintf("Agent %s (model %q) ended job %s for ticket %q as %s after %d attempt(s).", job.Agent, job.Model, job.ID, job.Ticket, job.Status, attempts),
	})
}
