package main

import (
	"context"
	"fmt"

	"Dendrite/pkg/vcs"
)

func handleVCSCommand(ctx context.Context, args []string) {
	if len(args) == 0 {
		printVCSUsage()
		return
	}

	guard := vcs.NewGitGuard(".", vcs.ModeStandard)

	switch args[0] {
	case "check", "guard", "status":
		branch, err := guard.PreFlightBranchCheck(ctx)
		if err != nil {
			fmt.Printf("[!] VCS Guard Alert: %v\n", err)
			fmt.Printf("    - Evacuated to: %s\n", branch)
			return
		}
		fmt.Printf("[✓] VCS Guard Active:\n")
		fmt.Printf("    - Active Branch:  %s\n", branch)
		fmt.Printf("    - Policy:         Forbidden on main/master; defaults to develop\n")

	case "sync-remote", "remote-sync":
		fmt.Println("[+] Checking and synchronizing remote fixes and commits...")
		watcher := vcs.NewRemoteSyncWatcher(guard, nil)
		status, err := watcher.SyncRemoteFixes(ctx, "")
		if err != nil {
			fmt.Printf("[-] Remote sync error: %v\n", err)
			return
		}
		fmt.Printf("[✓] Remote Sync Status for branch %s:\n", status.Branch)
		fmt.Printf("    - Divergence: Ahead %d, Behind %d\n", status.Ahead, status.Behind)
		fmt.Printf("    - Details:    %s\n", status.Details)
		if len(status.RemoteCommits) > 0 {
			fmt.Printf("    - Incoming Commits (%d):\n", len(status.RemoteCommits))
			for _, c := range status.RemoteCommits {
				fmt.Printf("      * %s\n", c)
			}
		}
		if len(status.ReconciledTickets) > 0 {
			fmt.Printf("    - Reconciled Tickets: %v\n", status.ReconciledTickets)
		}

	case "evacuate":
		err := guard.EnsureAndCheckoutDevelop(ctx)
		if err != nil {
			fmt.Printf("[-] Failed to checkout develop: %v\n", err)
			return
		}
		fmt.Println("[✓] Successfully evacuated to develop branch.")

	default:
		printVCSUsage()
	}
}

func printVCSUsage() {
	fmt.Println("Usage: dendrite vcs <check|guard|status|sync-remote|evacuate>")
	fmt.Println("\nCommands:")
	fmt.Println("  check, guard, status   Verify branch policy (halts and evacuates main/master to develop)")
	fmt.Println("  sync-remote            Fetch and rebase remote fixes with commit reconciliation")
	fmt.Println("  evacuate               Force switch to develop branch")
}
