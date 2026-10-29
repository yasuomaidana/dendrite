package main

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"os/exec"
	"strings"

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
		newJobsWatchCmd(),
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
			ensureJobWatcher(agentjobs.DefaultDispatcher, cmd.ErrOrStderr())
			if globals.JSON {
				return writeJSON(cmd.OutOrStdout(), map[string]string{"id": id})
			}
			fmt.Fprintf(cmd.OutOrStdout(), "[✓] Job submitted successfully: %s\n", id)
			if job, lErr := agentjobs.DefaultDispatcher.Ledger.Load(id); lErr == nil {
				fmt.Fprintf(cmd.OutOrStdout(), "    Agent/Model:     %s/%s\n", job.Agent, job.Model)
			}
			fmt.Fprintf(cmd.OutOrStdout(), "    Follow it:       dendrite bouton jobs watch %s\n", id)
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

// ensureJobWatcher makes sure a stall watcher runs over the ledger and says so on errOut, so a job is never
// left to run unwatched. A failure is a warning: the job itself is already submitted.
func ensureJobWatcher(d *agentjobs.Dispatcher, errOut io.Writer) {
	pid, err := d.EnsureWatcher()
	switch {
	case err != nil:
		fmt.Fprintf(errOut, "[!] No stall watcher is running and one could not be started: %v\n    Start it with `dendrite bouton jobs daemon`.\n", err)
	case pid > 0:
		fmt.Fprintf(errOut, "[i] Stall watcher started (pid %d, log %s)\n", pid, d.Ledger.WatcherLogPath())
	}
}
