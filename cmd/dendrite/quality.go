package main

import (
	"context"
	"fmt"

	"Dendrite/pkg/quality"
	"Dendrite/pkg/tracker/github"
)

func handleQualityCommand(ctx context.Context, args []string) {
	if len(args) == 0 {
		printQualityUsage()
		return
	}

	switch args[0] {
	case "audit":
		pkgPath := "pkg/vcs"
		if len(args) > 1 {
			pkgPath = args[1]
		}
		fmt.Printf("[+] Running Anti-Mock & 100%% Coverage Quality Audit on %s...\n", pkgPath)
		hasMocks, mockLocs, mErr := quality.ScanForMocks(".", pkgPath)
		if mErr != nil {
			fmt.Printf("[-] Mock scan error: %v\n", mErr)
			return
		}
		if hasMocks {
			fmt.Printf("[!] Zero-Mock Policy Violation: Detected %d mock pattern(s):\n", len(mockLocs))
			for _, loc := range mockLocs {
				fmt.Printf("    * %s\n", loc)
			}
		} else {
			fmt.Println("[✓] Zero-Mock Policy: Clean (Zero mocks or stubs detected).")
		}

		uncovered, pct, failed, out := quality.RunCoverageAudit(ctx, ".", pkgPath)
		if failed {
			fmt.Printf("[-] Test execution failed:\n%s\n", out)
			return
		}
		fmt.Printf("[✓] Statement Coverage: %.1f%%\n", pct)
		if len(uncovered) > 0 {
			fmt.Printf("[!] %d Statement Block(s) Uncovered (< 100%%):\n", len(uncovered))
			for i, b := range uncovered {
				if i >= 5 {
					fmt.Printf("    ... and %d more blocks\n", len(uncovered)-5)
					break
				}
				fmt.Printf("    * %s\n", b)
			}
		} else {
			fmt.Println("[✓] 100% Statement Coverage achieved!")
		}

	case "hitl-sync", "sync-hitl":
		fmt.Println("[+] Polling GitHub Issues for Human-in-the-Loop Verification...")
		poller := quality.NewGitHubHITLPoller("yasuomaidana/dendrite", &github.DefaultCommandExecutor{})
		tickets := []*quality.IssueSpec{
			{ID: "DEN-701", GitHubIssueNum: 1, State: quality.StateToVerify},
			{ID: "DEN-702", GitHubIssueNum: 2, State: quality.StateToVerify},
			{ID: "DEN-703", GitHubIssueNum: 3, State: quality.StateToVerify},
			{ID: "DEN-704", GitHubIssueNum: 4, State: quality.StateToVerify},
			{ID: "DEN-705", GitHubIssueNum: 5, State: quality.StateToVerify},
		}

		for _, t := range tickets {
			followUp, err := poller.SyncValidationState(ctx, t)
			if err != nil {
				fmt.Printf("[-] Failed to sync %s: %v\n", t.ID, err)
				continue
			}
			if followUp != nil {
				fmt.Printf("[!] Human feedback received on %s! Demoted to TO_DO. Follow-up: %s\n", t.ID, followUp.ID)
			} else {
				fmt.Printf("    * [%s] Current State: %s (GitHub #%d)\n", t.ID, t.State, t.GitHubIssueNum)
			}
		}

	default:
		printQualityUsage()
	}
}

func printQualityUsage() {
	fmt.Println("Usage: dendrite quality <audit|hitl-sync> [args...]")
	fmt.Println("\nCommands:")
	fmt.Println("  audit [package]    Run anti-mock scan and 100% coverage profiler on package")
	fmt.Println("  hitl-sync          Poll GitHub issues for closed approvals (DONE) or new feedback")
}
