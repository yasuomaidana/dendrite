package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"strings"

	"Dendrite/pkg/tracker/github"
	"Dendrite/pkg/tracker/hitl"

	"github.com/spf13/cobra"
)

// newHITLSyncer is a seam so tests can point the commands at a local Plane and gh.
var newHITLSyncer = func(ctx context.Context, repo string) (*hitl.Syncer, error) {
	return hitl.NewFromEnv(ctx, repo, ".")
}

// githubSyncOptions are the flags shared by github-sync and hitl-sync.
type githubSyncOptions struct {
	Repo    string
	DryRun  bool
	Tickets string
}

func (o *githubSyncOptions) bind(cmd *cobra.Command, dryRunHelp string) {
	f := cmd.Flags()
	f.StringVarP(&o.Repo, "repo", "r", "", "GitHub repository owner/name (default: the origin remote)")
	f.BoolVar(&o.DryRun, "dry-run", false, dryRunHelp)
	f.StringVar(&o.Tickets, "ticket", "", "Limit the sync to these ticket IDs (comma-separated)")
}

// newMyelinGitHubSyncCmd builds `myelin github-sync`: mirror Plane's verification state onto
// GitHub issues, and with --hitl then poll those issues for human verification.
func newMyelinGitHubSyncCmd() *cobra.Command {
	var o githubSyncOptions
	var withHITL bool
	cmd := &cobra.Command{
		Use:     "github-sync",
		Aliases: []string{"sync-github", "saltatory-sync", "gsync"},
		Short:   "Open a GitHub verification issue for every Plane To Verify ticket and close those of Done tickets (--hitl then polls them)",
		Args:    noArgs,
		Example: "dendrite myelin github-sync --dry-run\ndendrite myelin github-sync --ticket DEN-123\ndendrite myelin github-sync --hitl",
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := runGitHubSync(cmd.Context(), o, cmd.OutOrStdout()); err != nil {
				return err
			}
			if !withHITL {
				return nil
			}
			return runHITLSync(cmd.Context(), o, cmd.OutOrStdout())
		},
	}
	o.bind(cmd, "Show the planned issue changes without modifying GitHub")
	cmd.Flags().BoolVar(&withHITL, "hitl", false, "Then poll the issues for human-in-the-loop approval or feedback")
	return cmd
}

// runGitHubSync mirrors Plane's verification state onto GitHub issues: To Verify
// tickets get a HITL verification issue, Done tickets have theirs closed.
func runGitHubSync(ctx context.Context, o githubSyncOptions, out io.Writer) error {
	syncer, err := newHITLSyncer(ctx, o.Repo)
	if err != nil {
		return err
	}
	syncer.Only = splitTicketIDs(o.Tickets)
	res, err := syncer.GitHubSync(ctx, o.DryRun)
	if err != nil {
		return err
	}
	if globals.JSON {
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

// newAllostasisHITLSyncCmd builds `allostasis hitl-sync` (also `quality hitl-sync`).
func newAllostasisHITLSyncCmd() *cobra.Command {
	var o githubSyncOptions
	cmd := &cobra.Command{
		Use:     "hitl-sync",
		Aliases: []string{"sync-hitl"},
		Short:   "Poll the GitHub issue of every Plane To Verify ticket; closed as completed moves it to Done",
		Args:    noArgs,
		Example: "dendrite allostasis hitl-sync --dry-run\ndendrite allostasis hitl-sync --ticket DEN-123",
		RunE: func(cmd *cobra.Command, args []string) error {
			return runHITLSync(cmd.Context(), o, cmd.OutOrStdout())
		},
	}
	o.bind(cmd, "Report approvals without moving tickets to Done")
	return cmd
}

// runHITLSync polls the verification issue of every Plane ticket in To Verify.
func runHITLSync(ctx context.Context, o githubSyncOptions, out io.Writer) error {
	syncer, err := newHITLSyncer(ctx, o.Repo)
	if err != nil {
		return err
	}
	syncer.Only = splitTicketIDs(o.Tickets)
	results, err := syncer.HITLSync(ctx, o.DryRun)
	if err != nil {
		return err
	}
	if globals.JSON {
		if results == nil {
			results = []hitl.HITLResult{}
		}
		data, _ := json.MarshalIndent(results, "", "  ")
		fmt.Fprintln(out, string(data))
		return nil
	}

	fmt.Fprintf(out, "[+] Human-in-the-Loop verification in %s (%d tickets To Verify)\n", syncer.GitHub.Repo, len(results))
	for _, r := range results {
		fmt.Fprintf(out, "    * %s\n", describeHITLResult(r, o.DryRun))
	}
	return nil
}

// splitTicketIDs parses a comma-separated --ticket value.
func splitTicketIDs(raw string) []string {
	var ids []string
	for _, id := range strings.Split(raw, ",") {
		if id = strings.TrimSpace(id); id != "" {
			ids = append(ids, id)
		}
	}
	return ids
}
