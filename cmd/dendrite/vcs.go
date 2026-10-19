package main

import (
	"context"
	"fmt"
	"io"

	"Dendrite/pkg/vcs"

	"github.com/spf13/cobra"
)

// newMyelinCmd builds `dendrite myelin` (alias vcs): branch guardrails, remote sync and GitHub issue sync.
func newMyelinCmd() *cobra.Command {
	cmd := newGroupCmd("myelin", "Git branch guardrails prohibiting main pushes and ensuring rebase integrity", "axon",
		"vcs", "hillock-guard", "myelin-guard", "guard")
	cmd.Example = "dendrite myelin check\n" +
		"dendrite myelin sync-remote\n" +
		"dendrite myelin github-sync --repo owner/repo --hitl"

	guard := func() *vcs.GitGuard { return vcs.NewGitGuard(".", vcs.ModeStandard) }
	cmd.AddCommand(
		&cobra.Command{
			Use:     "check",
			Aliases: []string{"guard", "status"},
			Short:   "Check the branch policy; on main/master, move to develop and exit 1",
			Args:    noArgs,
			RunE: func(cmd *cobra.Command, args []string) error {
				return runMyelinCheck(cmd.Context(), guard(), cmd.OutOrStdout())
			},
		},
		&cobra.Command{
			Use:     "sync-remote",
			Aliases: []string{"remote-sync"},
			Short:   "Fetch and rebase remote fixes and reconcile their tickets",
			Args:    noArgs,
			RunE: func(cmd *cobra.Command, args []string) error {
				return runMyelinSyncRemote(cmd.Context(), guard(), cmd.OutOrStdout())
			},
		},
		&cobra.Command{
			Use:   "evacuate",
			Short: "Check out develop, creating it when missing",
			Args:  noArgs,
			RunE: func(cmd *cobra.Command, args []string) error {
				if err := guard().EnsureAndCheckoutDevelop(cmd.Context()); err != nil {
					return fmt.Errorf("checkout develop: %w", err)
				}
				fmt.Fprintln(cmd.OutOrStdout(), "[✓] Successfully evacuated to develop branch.")
				return nil
			},
		},
		newMyelinGitHubSyncCmd(),
	)
	return cmd
}

func runMyelinCheck(ctx context.Context, g *vcs.GitGuard, out io.Writer) error {
	branch, err := g.PreFlightBranchCheck(ctx)
	if err != nil {
		fmt.Fprintf(out, "[!] VCS Guard Alert: %v\n", err)
		fmt.Fprintf(out, "    - Evacuated to: %s\n", branch)
		return fmt.Errorf("branch policy violated: %w", err)
	}
	fmt.Fprintln(out, "[✓] VCS Guard Active:")
	fmt.Fprintf(out, "    - Active Branch:  %s\n", branch)
	fmt.Fprintln(out, "    - Policy:         Forbidden on main/master; defaults to develop")
	return nil
}

func runMyelinSyncRemote(ctx context.Context, g *vcs.GitGuard, out io.Writer) error {
	fmt.Fprintln(out, "[+] Checking and synchronizing remote fixes and commits...")
	status, err := vcs.NewRemoteSyncWatcher(g, nil).SyncRemoteFixes(ctx, "")
	if err != nil {
		return fmt.Errorf("remote sync: %w", err)
	}
	if globals.JSON {
		return writeJSON(out, status)
	}
	fmt.Fprintf(out, "[✓] Remote Sync Status for branch %s:\n", status.Branch)
	fmt.Fprintf(out, "    - Divergence: Ahead %d, Behind %d\n", status.Ahead, status.Behind)
	fmt.Fprintf(out, "    - Details:    %s\n", status.Details)
	if len(status.RemoteCommits) > 0 {
		fmt.Fprintf(out, "    - Incoming Commits (%d):\n", len(status.RemoteCommits))
		for _, c := range status.RemoteCommits {
			fmt.Fprintf(out, "      * %s\n", c)
		}
	}
	if len(status.ReconciledTickets) > 0 {
		fmt.Fprintf(out, "    - Reconciled Tickets: %v\n", status.ReconciledTickets)
	}
	return nil
}
