package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"

	"Dendrite/pkg/tracker/github"
	"Dendrite/pkg/tracker/hitl"
)

// newHITLSyncer is a seam so tests can point the commands at a local Plane and gh.
var newHITLSyncer = func(ctx context.Context, repo string) (*hitl.Syncer, error) {
	return hitl.NewFromEnv(ctx, repo, ".")
}

func handleGitHubSyncCommand(ctx context.Context, args []string) {
	if err := runGitHubSync(ctx, args, os.Stdout); err != nil {
		fmt.Printf("[-] GitHub synchronization failed: %v\n", err)
	}
}

// runGitHubSync mirrors Plane's verification state onto GitHub issues: To Verify
// tickets get a HITL verification issue, Done tickets have theirs closed.
func runGitHubSync(ctx context.Context, args []string, out io.Writer) error {
	fs := flag.NewFlagSet("github-sync", flag.ContinueOnError)
	fs.SetOutput(out)
	repo := fs.String("repo", "", "Target GitHub repository owner/repo (default: the origin remote)")
	fs.StringVar(repo, "r", "", "Shorthand for --repo")
	dryRun := fs.Bool("dry-run", false, "Show planned GitHub changes without applying them")
	asJSON := fs.Bool("json", false, "Output the sync result as JSON")
	if err := fs.Parse(args); err != nil {
		return err
	}

	syncer, err := newHITLSyncer(ctx, *repo)
	if err != nil {
		return err
	}
	res, err := syncer.GitHubSync(ctx, *dryRun)
	if err != nil {
		return err
	}
	if *asJSON {
		data, _ := json.MarshalIndent(res, "", "  ")
		fmt.Fprintln(out, string(data))
		return nil
	}

	mode := "Synchronized"
	if res.DryRun {
		mode = "Dry run for"
	}
	fmt.Fprintf(out, "[+] %s verification issues in %s (%d tickets, %d To Verify)\n", mode, res.Repo, res.Tickets, len(res.Pending))
	for _, d := range res.Duplicates {
		fmt.Fprintf(out, "[!] Skipped duplicate ticket ID %s: rename it in Plane\n", d)
	}
	if len(res.Actions) == 0 {
		fmt.Fprintln(out, "[✓] GitHub is already in sync with Plane; nothing to change")
		return nil
	}
	for _, a := range res.Actions {
		fmt.Fprintf(out, "    * %s\n", describeAction(a, res.DryRun))
	}
	if len(res.Errors) > 0 {
		return fmt.Errorf("%d of %d GitHub changes failed", len(res.Errors), len(res.Actions))
	}
	return nil
}

func describeAction(a github.Action, dryRun bool) string {
	issue := fmt.Sprintf("#%d", a.IssueNumber)
	if a.IssueNumber == 0 {
		issue = "new issue"
	}
	prefix := ""
	if dryRun {
		prefix = "would "
	}
	line := fmt.Sprintf("%s%s %s (%s): %s", prefix, a.Kind, a.TicketID, issue, a.Reason)
	if a.Error != "" {
		line += " [failed: " + a.Error + "]"
	}
	return line
}
