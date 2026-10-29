package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"Dendrite/pkg/agentjobs"
	"Dendrite/pkg/statedir"
	"Dendrite/pkg/ticketlock"
)

// newSessionCmd builds `dendrite session`: orchestrator checkpoint and resume.
func newSessionCmd() *cobra.Command {
	cmd := newGroupCmd("session", "Session checkpoint and resume for multi-agent coordination", "cleft", "handoff")
	cmd.Example = "dendrite session resume\ndendrite session checkpoint --next DEN-123 --note \"waiting on review\""
	cmd.AddCommand(newSessionCheckpointCmd(), newSessionResumeCmd())
	return cmd
}

// sessionCheckpointOptions holds the flags of `session checkpoint`.
type sessionCheckpointOptions struct {
	Worktree  string
	Path      string
	Decisions []string
	Next      []string
	Notes     []string
}

func newSessionCheckpointCmd() *cobra.Command {
	var o sessionCheckpointOptions
	cmd := &cobra.Command{
		Use:     "checkpoint",
		Short:   "Record session state into .dendrite/session/handoff.md",
		Args:    noArgs,
		Example: "dendrite session checkpoint --worktree ../wt --decision \"keep X\" --next DEN-123 --note \"blocked on CI\"",
		RunE: func(cmd *cobra.Command, args []string) error {
			return runSessionCheckpoint(cmd.Context(), o, cmd.OutOrStdout())
		},
	}
	f := cmd.Flags()
	f.StringVar(&o.Worktree, "worktree", "", "Worktree directory")
	f.StringVar(&o.Path, "path", "", "Destination handoff file path")
	f.StringArrayVar(&o.Decisions, "decision", nil, "Pending decision (repeatable)")
	f.StringArrayVar(&o.Next, "next", nil, "Next ticket or task (repeatable)")
	f.StringArrayVar(&o.Notes, "note", nil, "Session note (repeatable)")
	return cmd
}

func newSessionResumeCmd() *cobra.Command {
	var path string
	cmd := &cobra.Command{
		Use:     "resume",
		Short:   "Resume from the handoff checkpoint and report job status deltas",
		Args:    noArgs,
		Example: "dendrite session resume\ndendrite session resume --json --path handoff.md",
		RunE: func(cmd *cobra.Command, args []string) error {
			return runSessionResume(cmd.Context(), path, globals.JSON, cmd.OutOrStdout())
		},
	}
	cmd.Flags().StringVar(&path, "path", "", "Handoff file path")
	return cmd
}

func runSessionCheckpoint(ctx context.Context, o sessionCheckpointOptions, out io.Writer) error {
	noteList := o.Notes
	combinedNotes := strings.Join(noteList, "\n")
	if len(noteList) > 1 {
		combinedNotes = "- " + strings.Join(noteList, "\n- ")
	}

	opts := agentjobs.CheckpointOptions{
		Worktree:         o.Worktree,
		Ticket:           claimedTicket(o.Worktree),
		PendingDecisions: o.Decisions,
		Next:             o.Next,
		Notes:            combinedNotes,
	}

	ledger := agentjobs.NewLedger("")
	cp, err := agentjobs.BuildCheckpoint(ledger, opts)
	if err != nil {
		return fmt.Errorf("checkpoint failed: %w", err)
	}

	destPath := o.Path
	if err := agentjobs.WriteCheckpoint(destPath, cp); err != nil {
		return fmt.Errorf("write checkpoint failed: %w", err)
	}

	if globals.JSON {
		data, _ := json.MarshalIndent(cp, "", "  ")
		fmt.Fprintln(out, string(data))
		autoCleanWorkCopies(ctx, io.Discard)
		return nil
	}

	resolvedPath := destPath
	if resolvedPath == "" {
		resolvedPath = agentjobs.DefaultHandoffPath
	}
	resolvedPath = statedir.Resolve(resolvedPath)

	fmt.Fprintf(out, "[✓] Session checkpoint written to %s\n", resolvedPath)
	fmt.Fprintf(out, "    - Branch:               %s\n", cp.Branch)
	fmt.Fprintf(out, "    - Worktree:             %s\n", cp.Worktree)
	fmt.Fprintf(out, "    - In-Flight Jobs:       %d\n", len(cp.InFlight))
	fmt.Fprintf(out, "    - Verified Uncommitted: %d\n", len(cp.VerifiedUncommitted))
	if len(cp.PendingDecisions) > 0 {
		fmt.Fprintf(out, "    - Pending Decisions:    %d\n", len(cp.PendingDecisions))
	}
	if len(cp.Next) > 0 {
		fmt.Fprintf(out, "    - Next:                 %s\n", strings.Join(cp.Next, ", "))
	}

	autoCleanWorkCopies(ctx, out)
	return nil
}

// claimedTicket is the ticket claimed by the working copy in dir ("" is the current directory).
func claimedTicket(dir string) string {
	if dir == "" {
		dir = "."
	}
	c, _ := ticketlock.NewStore().ClaimOfDir(dir)
	return c.Ticket
}

// mintSessionToken prunes expired session tokens and writes the token of this session, which the
// write lock (`dendrite hook pre-edit`) requires. It returns a one-line summary for the user.
func mintSessionToken() (string, error) {
	store, now := ticketlock.NewStore(), time.Now()
	store.PruneTokens(now)
	cwd, _ := os.Getwd()
	tok, err := store.MintToken(ticketlock.TokenOptions{Worktree: cwd}, now)
	if err != nil {
		return "", err
	}
	line := fmt.Sprintf("session: %s (agent %s", tok.SessionID, tok.Agent)
	if tok.ModelTier != "" {
		line += ", tier " + tok.ModelTier
	}
	line += ")"
	if t := claimedTicket(""); t != "" {
		line += ", claimed ticket " + t
	}
	return line, nil
}

func runSessionResume(ctx context.Context, targetPath string, asJSON bool, out io.Writer) error {
	ledger := agentjobs.NewLedger("")
	session, tokErr := mintSessionToken()
	if asJSON {
		report, err := agentjobs.ResumeState(targetPath, ledger)
		if err != nil {
			return fmt.Errorf("resume failed: %w", err)
		}
		data, err := json.MarshalIndent(report, "", "  ")
		if err != nil {
			return fmt.Errorf("marshal resume report: %w", err)
		}
		fmt.Fprintln(out, string(data))
		autoCleanWorkCopies(ctx, io.Discard)
		return nil
	}

	text, err := agentjobs.Resume(targetPath, ledger)
	if err != nil {
		return fmt.Errorf("resume failed: %w", err)
	}

	fmt.Fprint(out, text)
	if tokErr != nil {
		fmt.Fprintf(out, "[!] session token not written: %v\n", tokErr)
	} else {
		fmt.Fprintf(out, "[i] %s\n", session)
	}
	autoCleanWorkCopies(ctx, out)
	warnStaleIndex(out)
	warnWatcherDown(ledger, time.Now(), out)
	printSynapsePeers(".", out)
	return nil
}

// warnWatcherDown says so when delegated jobs are in flight and no stall watcher is looking at them, so a
// stuck agent does not run unnoticed.
func warnWatcherDown(l *agentjobs.Ledger, now time.Time, out io.Writer) {
	jobs, err := l.List()
	if err != nil {
		return
	}
	active := 0
	for _, j := range jobs {
		switch j.Status {
		case agentjobs.StatusQueued, agentjobs.StatusRunning, agentjobs.StatusParked, agentjobs.StatusFailedRetryable:
			active++
		}
	}
	if _, alive := l.WatcherAlive(now); active > 0 && !alive {
		fmt.Fprintf(out, "[!] %d job(s) in flight and no stall watcher is running: start it with `dendrite bouton jobs daemon` (submitting a job also starts one)\n", active)
	}
}
