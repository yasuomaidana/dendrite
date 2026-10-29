package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strings"
	"time"

	"Dendrite/pkg/agentjobs"

	"github.com/spf13/cobra"
)

func newJobsWatchCmd() *cobra.Command {
	var ticket string
	var once bool
	interval := 10 * time.Second
	cmd := &cobra.Command{
		Use:   "watch [<job-id>]",
		Short: "Follow a job live: status, process, model requests, changed files and the stall countdown",
		Long: "Prints one line per interval until the job reaches a final state (Ctrl-C stops watching, not the job).\n" +
			"Model requests come from agy's own log, so a quiet line with a growing request count is a busy agent;\n" +
			"no requests, no changed files and a shrinking stall countdown is a stuck one.",
		Example: "dendrite bouton jobs watch --ticket DEN-123\ndendrite bouton jobs watch DEN-123-20261029T053329Z-ab7522 --once --json",
		Args:    maxArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			var id string
			if len(args) == 1 {
				id = args[0]
			}
			if (id == "") == (ticket == "") {
				return usageErrorf("specify exactly one of <job-id> and --ticket")
			}
			if interval <= 0 {
				return usageErrorf("--interval must be positive")
			}
			l, id, err := resolveJob(agentjobs.NewLedger(""), id, ticket)
			if err != nil {
				return err
			}
			return runJobsWatch(cmd.Context(), agentjobs.NewWatcher(l, nil), id, once, interval, cmd.OutOrStdout())
		},
	}
	addTicketFlag(cmd, &ticket, "")
	cmd.Flags().BoolVar(&once, "once", false, "Print one snapshot and exit")
	cmd.Flags().DurationVar(&interval, "interval", interval, "Time between snapshots")
	return cmd
}

// resolveJob finds the ledger and id of a job given either its id or a ticket (its newest job).
func resolveJob(l *agentjobs.Ledger, id, ticket string) (*agentjobs.Ledger, string, error) {
	if ticket == "" {
		if _, err := l.Load(id); err == nil {
			return l, id, nil
		}
		if t := agentjobs.TicketFromJobID(id); t != "" && t != id {
			if rl, _, err := agentjobs.ResolveTicketLedger(l, t); err == nil && rl != nil {
				if _, err := rl.Load(id); err == nil {
					return rl, id, nil
				}
			}
		}
		return nil, "", fmt.Errorf("no job %q found", id)
	}
	rl, matches, err := agentjobs.ResolveTicketLedger(l, ticket)
	if err != nil || len(matches) == 0 {
		return nil, "", fmt.Errorf("no matching jobs found for ticket %s", ticket)
	}
	return rl, agentjobs.Latest(matches).ID, nil
}

// runJobsWatch prints a snapshot of the job every interval until it reaches a final state or ctx ends.
func runJobsWatch(ctx context.Context, w *agentjobs.Watcher, id string, once bool, interval time.Duration, out io.Writer) error {
	for {
		s, err := w.Snapshot(ctx, id)
		if err != nil {
			return err
		}
		if globals.JSON {
			if err := writeJSON(out, s); err != nil {
				return err
			}
		} else {
			fmt.Fprintf(out, "%s %s\n", time.Now().Format("15:04:05"), s.Line())
		}
		if once || s.Final() {
			return nil
		}
		select {
		case <-ctx.Done():
			return nil
		case <-time.After(interval):
		}
	}
}

func newJobsListCmd() *cobra.Command {
	var ticket string
	cmd := &cobra.Command{
		Use:     "list",
		Aliases: []string{"ls"},
		Short:   "List jobs",
		Args:    noArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runJobsList(agentjobs.NewLedger(""), ticket, cmd.OutOrStdout())
		},
	}
	addTicketFlag(cmd, &ticket, "")
	return cmd
}

func runJobsList(l *agentjobs.Ledger, ticket string, out io.Writer) error {
	var jobs []*agentjobs.Job
	if ticket != "" {
		resolvedLedger, matches, err := agentjobs.ResolveTicketLedger(l, ticket)
		if err != nil || len(matches) == 0 {
			return fmt.Errorf("no matching jobs found for ticket %s", ticket)
		}
		_ = resolvedLedger
		jobs = matches
	} else {
		var err error
		jobs, err = l.List()
		if err != nil {
			return fmt.Errorf("list jobs: %w", err)
		}
	}
	if globals.JSON {
		if jobs == nil {
			jobs = []*agentjobs.Job{}
		}
		return writeJSON(out, jobs)
	}
	fmt.Fprintf(out, "[+] Agent Jobs (%d):\n", len(jobs))
	if len(jobs) == 0 {
		fmt.Fprintln(out, "    No jobs recorded.")
		return nil
	}
	fmt.Fprintf(out, "  %-38s %-10s %-8s %-16s %s\n", "JOB ID", "TICKET", "AGENT", "STATUS", "UPDATED")
	for _, j := range jobs {
		fmt.Fprintf(out, "  %-38s %-10s %-8s %-16s %s\n", j.ID, j.Ticket, j.Agent, j.Status, j.UpdatedAt.Format(time.RFC3339))
	}
	return nil
}

func newJobsShowCmd() *cobra.Command {
	var ticket string
	cmd := &cobra.Command{
		Use:   "show [<job-id>]",
		Short: "Show a job's specification and event history",
		Args:  maxArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			var id string
			if len(args) == 1 {
				id = args[0]
			}
			if id == "" && ticket == "" {
				return usageErrorf("specify a <job-id> or --ticket")
			}
			if id != "" && ticket != "" {
				return usageErrorf("<job-id> and --ticket are mutually exclusive")
			}
			return runJobsShowEntry(agentjobs.NewLedger(""), id, ticket, cmd.OutOrStdout())
		},
	}
	addTicketFlag(cmd, &ticket, "")
	return cmd
}

func runJobsShowEntry(l *agentjobs.Ledger, id, ticket string, out io.Writer) error {
	if ticket != "" {
		resolvedLedger, matches, err := agentjobs.ResolveTicketLedger(l, ticket)
		if err != nil || len(matches) == 0 {
			return fmt.Errorf("no matching jobs found for ticket %s", ticket)
		}
		return runJobsShow(resolvedLedger, agentjobs.Latest(matches).ID, out)
	}

	err := runJobsShow(l, id, out)
	if err != nil {
		ticketFromID := agentjobs.TicketFromJobID(id)
		if ticketFromID != "" && ticketFromID != id {
			if resolvedLedger, _, rErr := agentjobs.ResolveTicketLedger(l, ticketFromID); rErr == nil && resolvedLedger != nil {
				if rErr2 := runJobsShow(resolvedLedger, id, out); rErr2 == nil {
					return nil
				}
			}
		}
	}
	return err
}

func runJobsShow(l *agentjobs.Ledger, id string, out io.Writer) error {
	job, err := l.Load(id)
	if err != nil {
		return fmt.Errorf("load job %q: %w", id, err)
	}
	events, _ := l.Events(id)
	if globals.JSON {
		return writeJSON(out, map[string]any{"job": job, "events": events})
	}
	fmt.Fprintf(out, "[+] Job Specification: %s\n", job.ID)
	fmt.Fprintf(out, "    Ticket:          %s\n", job.Ticket)
	fmt.Fprintf(out, "    Status:          %s\n", job.Status)
	fmt.Fprintf(out, "    Agent:           %s\n", job.Agent)
	fmt.Fprintf(out, "    Model:           %s\n", job.Model)
	fmt.Fprintf(out, "    Complexity:      %d\n", job.Complexity)
	fmt.Fprintf(out, "    Worktree:        %s\n", job.Worktree)
	fmt.Fprintf(out, "    Scope:           %s\n", strings.Join(job.Scope, ", "))
	fmt.Fprintf(out, "    Prompt File:     %s\n", job.PromptFile)
	if job.ConversationID != "" {
		fmt.Fprintf(out, "    Conversation ID: %s\n", job.ConversationID)
	}
	if job.PGID > 0 {
		fmt.Fprintf(out, "    Process Group:   %d\n", job.PGID)
	}
	fmt.Fprintf(out, "    Created At:      %s\n", job.CreatedAt.Format(time.RFC3339))
	fmt.Fprintf(out, "    Updated At:      %s\n", job.UpdatedAt.Format(time.RFC3339))
	if len(events) > 0 {
		fmt.Fprintf(out, "\n[+] Events History (%d):\n", len(events))
		for _, ev := range events {
			data, _ := json.Marshal(ev.Data)
			fmt.Fprintf(out, "    %d. [%s] %s %s\n", ev.Seq, ev.Time.Format(time.RFC3339), ev.Type, string(data))
		}
	}
	return nil
}

func runJobsTail(l *agentjobs.Ledger, id string, n int, out io.Writer) error {
	if n <= 0 {
		return usageErrorf("--lines must be positive")
	}
	data, err := os.ReadFile(l.AgentLogPath(id))
	if err != nil {
		if os.IsNotExist(err) {
			ticketFromID := agentjobs.TicketFromJobID(id)
			if ticketFromID != "" && ticketFromID != id {
				if resolvedLedger, _, rErr := agentjobs.ResolveTicketLedger(l, ticketFromID); rErr == nil && resolvedLedger != nil {
					if rData, rErr2 := os.ReadFile(resolvedLedger.AgentLogPath(id)); rErr2 == nil {
						return printTailLines(rData, n, out)
					}
				}
			}
			fmt.Fprintf(out, "[-] No agent.log found for job %s\n", id)
			return nil
		}
		return fmt.Errorf("read agent.log: %w", err)
	}
	return printTailLines(data, n, out)
}

func printTailLines(data []byte, n int, out io.Writer) error {
	lines := strings.Split(strings.TrimRight(string(data), "\r\n"), "\n")
	if len(lines) > n {
		lines = lines[len(lines)-n:]
	}
	for _, line := range lines {
		fmt.Fprintln(out, line)
	}
	return nil
}
