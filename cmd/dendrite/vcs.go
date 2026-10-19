package main

import (
	"context"
	"fmt"
	"io"

	"github.com/spf13/cobra"

	"Dendrite/pkg/vcs"
)

// newMyelinCmd builds `dendrite myelin`: Git branch guardrails and remote/GitHub synchronization.
func newMyelinCmd() *cobra.Command {
	cmd := newGroupCmd("myelin", "Git branch guardrails prohibiting main pushes and ensuring rebase integrity", "soma",
		"vcs", "hillock-guard", "myelin-guard", "guard")
	cmd.Example = "dendrite myelin check\ndendrite myelin evacuate\ndendrite myelin sync-remote\ndendrite myelin github-sync --repo yasuomaidana/dendrite"

	checkCmd := &cobra.Command{
		Use:     "check",
		Aliases: []string{"guard", "status"},
		Short:   "Verify branch policy (halts and evacuates main/master to develop)",
		Args:    noArgs,
		RunE: func(c *cobra.Command, args []string) error {
			return runMyelinCheck(c.Context(), c.OutOrStdout())
		},
	}

	evacuateCmd := &cobra.Command{
		Use:   "evacuate",
		Short: "Force switch to develop branch",
		Args:  noArgs,
		RunE: func(c *cobra.Command, args []string) error {
			return runMyelinEvacuate(c.Context(), c.OutOrStdout())
		},
	}

	syncRemoteCmd := &cobra.Command{
		Use:     "sync-remote",
		Aliases: []string{"remote-sync"},
		Short:   "Fetch and rebase remote fixes with commit reconciliation",
		Args:    noArgs,
		RunE: func(c *cobra.Command, args []string) error {
			return runMyelinSyncRemote(c.Context(), c.OutOrStdout())
		},
	}

	cmd.AddCommand(checkCmd, evacuateCmd, syncRemoteCmd, newMyelinGitHubSyncCmd())
	return cmd
}

func runMyelinCheck(ctx context.Context, out io.Writer) error {
	guard := vcs.NewGitGuard(".", vcs.ModeStandard)
	branch, err := guard.PreFlightBranchCheck(ctx)
	if err != nil {
		fmt.Fprintf(out, "[!] VCS Guard Alert: %v\n", err)
		if branch != "" {
			fmt.Fprintf(out, "    - Evacuated to: %s\n", branch)
		}
		return fmt.Errorf("vcs guard alert: %w", err)
	}
	fmt.Fprintf(out, "[✓] VCS Guard Active:\n")
	fmt.Fprintf(out, "    - Active Branch:  %s\n", branch)
	fmt.Fprintf(out, "    - Policy:         Forbidden on main/master; defaults to develop\n")
	return nil
}

func runMyelinEvacuate(ctx context.Context, out io.Writer) error {
	guard := vcs.NewGitGuard(".", vcs.ModeStandard)
	if err := guard.EnsureAndCheckoutDevelop(ctx); err != nil {
		return fmt.Errorf("failed to checkout develop: %w", err)
	}
	fmt.Fprintln(out, "[✓] Successfully evacuated to develop branch.")
	return nil
}

func runMyelinSyncRemote(ctx context.Context, out io.Writer) error {
	guard := vcs.NewGitGuard(".", vcs.ModeStandard)
	fmt.Fprintln(out, "[+] Checking and synchronizing remote fixes and commits...")
	watcher := vcs.NewRemoteSyncWatcher(guard, nil)
	status, err := watcher.SyncRemoteFixes(ctx, "")
	if err != nil {
		return fmt.Errorf("remote sync error: %w", err)
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

func handleVCSCommand(ctx context.Context, args []string) {
	_ = invokeCmd(ctx, newMyelinCmd(), args...)
}
