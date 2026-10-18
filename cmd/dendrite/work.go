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
	"Dendrite/pkg/workcopy"
)

// newWorkManager is a seam so tests can point the CLI at a temporary repository.
var newWorkManager = func() *workcopy.Manager { return workcopy.New(".") }

func handleWorkCommand(ctx context.Context, args []string) error {
	return runWorkCommand(ctx, args, os.Stdout)
}

func runWorkCommand(ctx context.Context, args []string, out io.Writer) error {
	if len(args) < 1 {
		printWorkUsage(out)
		return nil
	}

	switch args[0] {
	case "new", "create":
		return runWorkNew(ctx, args[1:], out)
	case "list", "ls":
		return runWorkList(ctx, args[1:], out)
	case "done", "remove", "rm":
		return runWorkDone(ctx, args[1:], out)
	case "gc", "prune":
		return runWorkGC(ctx, args[1:], out)
	case "adopt":
		return runWorkAdopt(ctx, args[1:], out)
	case "help", "--help", "-h":
		printWorkUsage(out)
		return nil
	default:
		return fmt.Errorf("unknown work command %q. Usage: dendrite work <new|list|done|gc|adopt> [flags...]", args[0])
	}
}

func printWorkUsage(out io.Writer) {
	fmt.Fprintln(out, "Usage: dendrite work <new|list|done|gc|adopt> [flags...]")
	fmt.Fprintln(out, "\nModel-owned working copies live in dendrite-work/ next to the main checkout ($DENDRITE_WORK_DIR overrides).")
	fmt.Fprintln(out, "\nCommands:")
	fmt.Fprintln(out, "  new <name> [--branch b] [--base ref]  Create an isolated working copy (branch agent/<name>)")
	fmt.Fprintln(out, "  list [--json]                         List working copies and their cleanup status")
	fmt.Fprintln(out, "  done <name|path> [--force]            Remove a copy (refuses uncommitted/unpushed work unless --force)")
	fmt.Fprintln(out, "  gc [--dry-run]                        Remove copies that are merged (or whose upstream is gone) and clean")
	fmt.Fprintln(out, "  adopt [--dry-run]                     Move this repo's worktrees that live outside dendrite-work/ into it")
}

// splitPositional separates positional arguments from flags so `done NAME --force`
// and `done --force NAME` both parse. valueFlags take the following argument.
func splitPositional(args []string, valueFlags ...string) (positional, flags []string) {
	takesValue := make(map[string]bool, len(valueFlags))
	for _, f := range valueFlags {
		takesValue[f] = true
	}
	for i := 0; i < len(args); i++ {
		a := args[i]
		if !strings.HasPrefix(a, "-") {
			positional = append(positional, a)
			continue
		}
		flags = append(flags, a)
		if takesValue[strings.TrimLeft(a, "-")] && i+1 < len(args) {
			i++
			flags = append(flags, args[i])
		}
	}
	return positional, flags
}

func runWorkNew(ctx context.Context, args []string, out io.Writer) error {
	fs := flag.NewFlagSet("work-new", flag.ContinueOnError)
	fs.SetOutput(out)
	branch := fs.String("branch", "", "Branch to create (default agent/<name>)")
	base := fs.String("base", "", "Commit-ish to start from (default HEAD)")
	positional, flags := splitPositional(args, "branch", "base")
	if err := fs.Parse(flags); err != nil {
		return err
	}
	if len(positional) != 1 {
		return fmt.Errorf("usage: dendrite work new <name> [--branch b] [--base ref]")
	}

	path, err := newWorkManager().Create(ctx, positional[0], *branch, *base)
	if err != nil {
		return err
	}
	fmt.Fprintf(out, "[✓] Working copy created: %s\n", path)
	fmt.Fprintf(out, "    cd %s\n", path)
	return nil
}

func runWorkList(ctx context.Context, args []string, out io.Writer) error {
	fs := flag.NewFlagSet("work-list", flag.ContinueOnError)
	fs.SetOutput(out)
	asJSON := fs.Bool("json", false, "Output in JSON format")
	if err := fs.Parse(args); err != nil {
		return err
	}

	m := newWorkManager()
	copies, err := m.List(ctx)
	if err != nil {
		return err
	}
	if *asJSON {
		if copies == nil {
			copies = []workcopy.Copy{}
		}
		data, _ := json.MarshalIndent(copies, "", "  ")
		fmt.Fprintln(out, string(data))
		return nil
	}

	root, err := m.Root(ctx)
	if err != nil {
		return err
	}
	fmt.Fprintf(out, "[+] Working copies in %s (%d):\n", root, len(copies))
	for _, c := range copies {
		fmt.Fprintf(out, "  - %-40s %-30s %s\n", c.Path, c.Branch, copyStatus(c))
	}
	return nil
}

func copyStatus(c workcopy.Copy) string {
	var parts []string
	if c.Story {
		parts = append(parts, "story")
	}
	if c.Dirty {
		parts = append(parts, "dirty")
	}
	if c.Ahead > 0 {
		parts = append(parts, fmt.Sprintf("%d unpushed", c.Ahead))
	}
	if c.Untouched {
		parts = append(parts, "no commits")
	}
	if c.MergedInto != "" {
		parts = append(parts, "merged into "+c.MergedInto)
	}
	if c.UpstreamGone {
		parts = append(parts, "upstream gone")
	}
	if len(parts) == 0 {
		return "active"
	}
	return strings.Join(parts, ", ")
}

func runWorkDone(ctx context.Context, args []string, out io.Writer) error {
	fs := flag.NewFlagSet("work-done", flag.ContinueOnError)
	fs.SetOutput(out)
	force := fs.Bool("force", false, "Remove even with uncommitted or unpushed work, and delete the branch")
	positional, flags := splitPositional(args)
	if err := fs.Parse(flags); err != nil {
		return err
	}
	if len(positional) != 1 {
		return fmt.Errorf("usage: dendrite work done <name|path> [--force]")
	}

	report, err := newWorkManager().Remove(ctx, positional[0], workcopy.RemoveOptions{Force: *force})
	if err != nil {
		return err
	}
	fmt.Fprintf(out, "[✓] %s %s (%s)\n", report.Action, report.Path, report.Reason)
	return nil
}

func runWorkGC(ctx context.Context, args []string, out io.Writer) error {
	fs := flag.NewFlagSet("work-gc", flag.ContinueOnError)
	fs.SetOutput(out)
	dryRun := fs.Bool("dry-run", false, "Report what would be removed without deleting")
	if err := fs.Parse(args); err != nil {
		return err
	}

	reports, err := newWorkManager().GC(ctx, workcopy.GCOptions{
		DryRun: *dryRun,
		InUse:  agentjobs.NewLedger("").WorktreeInUse(),
	})
	if err != nil {
		return err
	}
	printWorkReports(out, reports, true)
	return nil
}

func runWorkAdopt(ctx context.Context, args []string, out io.Writer) error {
	fs := flag.NewFlagSet("work-adopt", flag.ContinueOnError)
	fs.SetOutput(out)
	dryRun := fs.Bool("dry-run", false, "Report what would be moved without moving")
	if err := fs.Parse(args); err != nil {
		return err
	}

	reports, err := newWorkManager().Adopt(ctx, workcopy.GCOptions{
		DryRun: *dryRun,
		InUse:  agentjobs.NewLedger("").WorktreeInUse(),
	})
	if err != nil {
		return err
	}
	if len(reports) == 0 {
		fmt.Fprintln(out, "[✓] No worktrees outside dendrite-work/ to adopt")
		return nil
	}
	printWorkReports(out, reports, true)
	return nil
}

// printWorkReports prints one line per report; with verbose=false only removals are shown.
func printWorkReports(out io.Writer, reports []workcopy.Report, verbose bool) {
	for _, r := range reports {
		if !verbose && r.Action != "removed" {
			continue
		}
		mark := "[✓]"
		if r.Action == "kept" {
			mark = "[=]"
		}
		fmt.Fprintf(out, "%s %s %s (%s)\n", mark, r.Action, r.Path, r.Reason)
	}
}

// autoCleanWorkCopies removes merged, clean working copies after a session
// command. It never fails the caller: problems are printed as warnings.
func autoCleanWorkCopies(ctx context.Context, out io.Writer) {
	reports, err := newWorkManager().GC(ctx, workcopy.GCOptions{InUse: agentjobs.NewLedger("").WorktreeInUse()})
	if err != nil {
		fmt.Fprintf(out, "[!] Working-copy cleanup skipped: %v\n", err)
		return
	}
	printWorkReports(out, reports, false)
}
