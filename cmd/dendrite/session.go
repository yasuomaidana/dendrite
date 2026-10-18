package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"
	"strings"

	"Dendrite/pkg/agentjobs"
	"Dendrite/pkg/statedir"
)

type stringFlagSlice []string

func (s *stringFlagSlice) String() string {
	return strings.Join(*s, ", ")
}

func (s *stringFlagSlice) Set(val string) error {
	*s = append(*s, val)
	return nil
}

func handleSessionCommand(ctx context.Context, args []string) error {
	return runSessionCommand(ctx, args, os.Stdout)
}

func runSessionCommand(ctx context.Context, args []string, out io.Writer) error {
	if len(args) < 1 {
		printSessionUsage(out)
		return nil
	}

	switch args[0] {
	case "checkpoint":
		return runSessionCheckpoint(ctx, args[1:], out)
	case "resume":
		return runSessionResume(ctx, args[1:], out)
	case "help", "--help", "-h":
		printSessionUsage(out)
		return nil
	default:
		return fmt.Errorf("unknown session command %q. Usage: dendrite session <checkpoint|resume> [flags...]", args[0])
	}
}

func printSessionUsage(out io.Writer) {
	fmt.Fprintln(out, "Usage: dendrite session <checkpoint|resume> [flags...]")
	fmt.Fprintln(out, "\nCommands:")
	fmt.Fprintln(out, "  checkpoint [--worktree dir] [--decision \"...\"]... [--next \"DEN-XXX\"]... [--note \"...\"]")
	fmt.Fprintln(out, "             Record session state into .dendrite/session/handoff.md")
	fmt.Fprintln(out, "  resume [--json] [--path file]")
	fmt.Fprintln(out, "             Resume session from handoff checkpoint and report job status deltas")
}

func runSessionCheckpoint(ctx context.Context, args []string, out io.Writer) error {
	fs := flag.NewFlagSet("session-checkpoint", flag.ContinueOnError)
	fs.SetOutput(out)

	var decisions stringFlagSlice
	var nextList stringFlagSlice
	var noteList stringFlagSlice

	worktree := fs.String("worktree", "", "Worktree directory")
	pathFlag := fs.String("path", "", "Destination handoff file path")
	asJSON := fs.Bool("json", false, "Output checkpoint in JSON format")

	fs.Var(&decisions, "decision", "Pending decision (repeatable)")
	fs.Var(&decisions, "decisions", "Pending decisions (repeatable)")
	fs.Var(&nextList, "next", "Next ticket or task (repeatable)")
	fs.Var(&noteList, "note", "Session note (repeatable)")
	fs.Var(&noteList, "notes", "Session notes (repeatable)")

	if err := fs.Parse(args); err != nil {
		return err
	}

	combinedNotes := strings.Join(noteList, "\n")
	if len(noteList) > 1 {
		combinedNotes = "- " + strings.Join(noteList, "\n- ")
	}

	opts := agentjobs.CheckpointOptions{
		Worktree:         *worktree,
		PendingDecisions: decisions,
		Next:             nextList,
		Notes:            combinedNotes,
	}

	ledger := agentjobs.NewLedger("")
	cp, err := agentjobs.BuildCheckpoint(ledger, opts)
	if err != nil {
		return fmt.Errorf("checkpoint failed: %w", err)
	}

	destPath := *pathFlag
	if err := agentjobs.WriteCheckpoint(destPath, cp); err != nil {
		return fmt.Errorf("write checkpoint failed: %w", err)
	}

	if *asJSON {
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

func runSessionResume(ctx context.Context, args []string, out io.Writer) error {
	fs := flag.NewFlagSet("session-resume", flag.ContinueOnError)
	fs.SetOutput(out)

	asJSON := fs.Bool("json", false, "Output handoff state and delta in JSON format")
	pathFlag := fs.String("path", "", "Handoff file path")
	fileFlag := fs.String("file", "", "Handoff file path")

	if err := fs.Parse(args); err != nil {
		return err
	}

	targetPath := *pathFlag
	if targetPath == "" {
		targetPath = *fileFlag
	}

	ledger := agentjobs.NewLedger("")
	if *asJSON {
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
	autoCleanWorkCopies(ctx, out)
	return nil
}
