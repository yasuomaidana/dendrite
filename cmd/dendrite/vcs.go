package main

import (
	"context"
	"fmt"
	"strings"

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
	fmt.Println("Usage: dendrite vcs <check|guard|status|evacuate>")
	fmt.Println("\nCommands:")
	fmt.Println("  check, guard, status   Verify branch policy (halts and evacuates main/master to develop)")
	fmt.Println("  evacuate               Force switch to develop branch")
}

func indentLines(text, indent string) string {
	lines := strings.Split(text, "\n")
	for i, l := range lines {
		lines[i] = indent + l
	}
	return strings.Join(lines, "\n")
}
