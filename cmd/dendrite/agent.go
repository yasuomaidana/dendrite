package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"
	"time"

	"Dendrite/pkg/agentjobs"

	"github.com/spf13/cobra"
)

// newBoutonJobsCmd builds `dendrite bouton jobs`: the delegated agent job ledger.
// pkg/agentjobs re-executes `<binary> agent jobs run <id>` and `agent jobs daemon`, so those
// paths must keep working through the agent alias.
func newBoutonJobsCmd() *cobra.Command {
	cmd := newGroupCmd("jobs", "Submit, watch and manage delegated agent jobs", "")
	cmd.Example = "dendrite bouton jobs submit --ticket DEN-123 --agent agy --scope pkg/foo --prompt-file prompt.md --isolate\n" +
		"dendrite bouton jobs list --ticket DEN-123\n" +
		"dendrite bouton jobs show --ticket DEN-123\n" +
		"dendrite bouton jobs wait --ticket DEN-123 --timeout 2h"

	idCmd := func(use, short string, run func(cmd *cobra.Command, id string) error) *cobra.Command {
		return &cobra.Command{
			Use:   use + " <job-id>",
			Short: short,
			Args:  exactArgs(1),
			RunE:  func(cmd *cobra.Command, args []string) error { return run(cmd, args[0]) },
		}
	}

	tail := idCmd("tail", "Print the last lines of a job's agent.log", func(cmd *cobra.Command, id string) error {
		lines, _ := cmd.Flags().GetInt("lines")
		return runJobsTail(agentjobs.NewLedger(""), id, lines, cmd.OutOrStdout())
	})
	tail.Flags().IntP("lines", "n", 50, "Number of lines")

	run := idCmd("run", "Run a job in the foreground (used by the dispatcher)", func(cmd *cobra.Command, id string) error {
		return agentjobs.RunJob(cmd.Context(), id)
	})
	run.Hidden = true

	cmd.AddCommand(
		newJobsSubmitCmd(),
		newJobsListCmd(),
		newJobsShowCmd(),
		tail,
		newJobsWaitCmd(),
		idCmd("retry", "Re-dispatch a job", func(cmd *cobra.Command, id string) error {
			if err := agentjobs.DefaultDispatcher.Retry(id); err != nil {
				return err
			}
			fmt.Fprintf(cmd.OutOrStdout(), "[✓] Job %s re-dispatched successfully\n", id)
			return nil
		}),
		idCmd("cancel", "Cancel a job and stop its process group", func(cmd *cobra.Command, id string) error {
			if err := agentjobs.DefaultDispatcher.Cancel(id); err != nil {
				return err
			}
			fmt.Fprintf(cmd.OutOrStdout(), "[✓] Job %s cancelled successfully\n", id)
			return nil
		}),
		newJobsDaemonCmd(),
		&cobra.Command{
			Use:   "models",
			Short: "Refresh the cached `agy models` listing and show the AGY models Dendrite schedules",
			Args:  noArgs,
			RunE: func(cmd *cobra.Command, args []string) error {
				return runJobsModels(cmd.Context(), cmd.OutOrStdout(), cmd.ErrOrStderr())
			},
		},
		run,
	)
	return cmd
}

// runJobsModels lists the AGY models on offer, the allowed set and the pick per complexity.
func runJobsModels(ctx context.Context, out, errOut io.Writer) error {
	live, err := agentjobs.DefaultDispatcher.RefreshAGYModels(ctx)
	if err != nil {
		fmt.Fprintf(errOut, "[!] %v\n", err)
	} else {
		fmt.Fprintf(out, "[+] AGY offers %d models: %s\n", len(live), strings.Join(live, ", "))
	}
	fmt.Fprintf(out, "[+] Dendrite schedules (DENDRITE_AGY_MODELS): %s\n", strings.Join(agentjobs.AllowedAGYModels(), ", "))
	for _, c := range []int{2, 5, 8} {
		m, reason := agentjobs.DefaultDispatcher.PickAGYModel(c)
		fmt.Fprintf(out, "    complexity %d -> %s (%s)\n", c, m, reason)
	}
	return nil
}

func newJobsSubmitCmd() *cobra.Command {
	spec := agentjobs.SubmitSpec{Agent: "agy", Complexity: 1}
	cmd := &cobra.Command{
		Use:   "submit",
		Short: "Submit a job to an agent (runs detached)",
		Args:  noArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			if !spec.Isolate && spec.Worktree == "" {
				if out, sErr := exec.CommandContext(cmd.Context(), "git", "status", "--porcelain").Output(); sErr == nil && len(bytes.TrimSpace(out)) > 0 {
					fmt.Fprintln(cmd.ErrOrStderr(), "[!] Running in a checkout with uncommitted changes. The agent will see them; use --isolate to run in a dendrite-work/ copy.")
				}
			}
			id, err := agentjobs.Submit(spec)
			if err != nil {
				return fmt.Errorf("submit job: %w", err)
			}
			if globals.JSON {
				return writeJSON(cmd.OutOrStdout(), map[string]string{"id": id})
			}
			fmt.Fprintf(cmd.OutOrStdout(), "[✓] Job submitted successfully: %s\n", id)
			if job, lErr := agentjobs.DefaultDispatcher.Ledger.Load(id); lErr == nil {
				fmt.Fprintf(cmd.OutOrStdout(), "    Agent/Model:     %s/%s\n", job.Agent, job.Model)
			}
			return nil
		},
	}
	f := cmd.Flags()
	addTicketFlag(cmd, &spec.Ticket, "")
	f.StringVar(&spec.Agent, "agent", spec.Agent, "Agent: agy, aider, copilot, ...")
	addModelFlag(cmd, &spec.Model, "")
	f.Lookup("model").Usage = "LLM model name, or auto to pick by complexity"
	f.StringSliceVar(&spec.Scope, "scope", nil, "Paths the agent may change (comma-separated)")
	f.StringVar(&spec.PromptFile, "prompt-file", "", "File with the prompt")
	f.StringVar(&spec.Prompt, "prompt", "", "Prompt text")
	f.StringVar(&spec.Worktree, "worktree", "", "Run in this checkout")
	f.BoolVar(&spec.Isolate, "isolate", false, "Run in the ticket's dendrite-work/ copy")
	f.IntVar(&spec.Complexity, "complexity", spec.Complexity, "Task complexity (1-10); picks the model tier")
	requireFlags(cmd, "ticket")
	cmd.MarkFlagsOneRequired("prompt-file", "prompt")
	cmd.MarkFlagsMutuallyExclusive("prompt-file", "prompt")
	cmd.MarkFlagsMutuallyExclusive("worktree", "isolate")
	return cmd
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
		var latest *agentjobs.Job
		for _, j := range matches {
			if latest == nil || j.CreatedAt.After(latest.CreatedAt) || (j.CreatedAt.Equal(latest.CreatedAt) && j.ID > latest.ID) {
				latest = j
			}
		}
		return runJobsShow(resolvedLedger, latest.ID, out)
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

func newJobsWaitCmd() *cobra.Command {
	var ticket string
	timeout, interval := 30*time.Minute, 2*time.Second
	cmd := &cobra.Command{
		Use:   "wait",
		Short: "Block until the matching jobs finish",
		Args:  noArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			if timeout <= 0 || interval <= 0 {
				return usageErrorf("--timeout and --interval must be positive")
			}
			return runJobsWait(cmd.Context(), agentjobs.NewLedger(""), ticket, timeout, interval, cmd.OutOrStdout())
		},
	}
	addTicketFlag(cmd, &ticket, "")
	cmd.Flags().DurationVar(&timeout, "timeout", timeout, "Give up after this long")
	cmd.Flags().DurationVar(&interval, "interval", interval, "Polling interval")
	return cmd
}

// runJobsWait returns once every matching job has a done marker newer than when waiting began.
func runJobsWait(ctx context.Context, l *agentjobs.Ledger, ticket string, timeout, interval time.Duration, out io.Writer) error {
	var targets []*agentjobs.Job
	if ticket != "" {
		resolvedLedger, matches, err := agentjobs.ResolveTicketLedger(l, ticket)
		if err != nil || len(matches) == 0 {
			return fmt.Errorf("no matching jobs found for ticket %s", ticket)
		}
		l = resolvedLedger
		targets = matches
	} else {
		all, err := l.List()
		if err != nil {
			return fmt.Errorf("list jobs: %w", err)
		}
		targets = all
		if len(targets) == 0 {
			fmt.Fprintln(out, "[-] No matching jobs found")
			return nil
		}
	}

	initialDone := make(map[string]time.Time)
	for _, j := range targets {
		if fi, err := os.Stat(l.DoneMarkerPath(j.ID)); err == nil {
			initialDone[j.ID] = fi.ModTime()
		}
	}
	if len(initialDone) == len(targets) {
		for _, j := range targets {
			fmt.Fprintf(out, "[✓] Job finished: %s\n", j.ID)
		}
		return nil
	}

	deadline := time.NewTimer(timeout)
	defer deadline.Stop()
	tick := time.NewTicker(interval)
	defer tick.Stop()
	finished := make(map[string]bool)
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-deadline.C:
			return fmt.Errorf("wait timed out after %v", timeout)
		case <-tick.C:
		}
		for _, j := range targets {
			if finished[j.ID] {
				continue
			}
			fi, err := os.Stat(l.DoneMarkerPath(j.ID))
			if err != nil {
				continue
			}
			if prev, had := initialDone[j.ID]; !had || fi.ModTime().After(prev) {
				finished[j.ID] = true
				fmt.Fprintf(out, "[✓] Job finished: %s\n", j.ID)
			}
		}
		pending := 0
		for _, j := range targets {
			if _, had := initialDone[j.ID]; !had && !finished[j.ID] {
				pending++
			}
		}
		if pending == 0 && len(finished) > 0 {
			return nil
		}
	}
}

func newJobsDaemonCmd() *cobra.Command {
	var once, install bool
	interval := agentjobs.DefaultDaemonInterval
	cmd := &cobra.Command{
		Use:   "daemon",
		Short: "Run the stall watcher (foreground), once, or install it as a LaunchAgent",
		Args:  noArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			if install {
				plist, err := agentjobs.InstallLaunchAgent("", "")
				if err != nil {
					return fmt.Errorf("install launch agent: %w", err)
				}
				fmt.Fprintf(cmd.OutOrStdout(), "[✓] LaunchAgent installed to %s\n", plist)
				return nil
			}
			w := agentjobs.NewWatcher(nil, nil)
			if once {
				return w.Tick(cmd.Context())
			}
			if interval <= 0 {
				return usageErrorf("--interval must be positive")
			}
			return runJobsDaemon(cmd.Context(), w, interval, cmd.ErrOrStderr())
		},
	}
	cmd.Flags().BoolVar(&once, "once", false, "Run one watcher pass and exit")
	cmd.Flags().BoolVar(&install, "install", false, "Install the watcher as a macOS LaunchAgent")
	cmd.Flags().DurationVar(&interval, "interval", interval, "Time between watcher passes")
	cmd.MarkFlagsMutuallyExclusive("once", "install")
	return cmd
}

// runJobsDaemon ticks the watcher until ctx is cancelled; tick errors are logged, not fatal.
func runJobsDaemon(ctx context.Context, w *agentjobs.Watcher, interval time.Duration, errOut io.Writer) error {
	tick := func() {
		if err := w.Tick(ctx); err != nil && !errors.Is(err, context.Canceled) {
			fmt.Fprintf(errOut, "[-] daemon tick error: %v\n", err)
		}
	}
	tick()
	t := time.NewTicker(interval)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return nil
		case <-t.C:
			tick()
		}
	}
}
