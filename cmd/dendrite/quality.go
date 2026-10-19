package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"
	"strings"

	"Dendrite/pkg/quality"
	"Dendrite/pkg/quality/coverage"
	"Dendrite/pkg/tracker/hitl"
)

var (
	osExit      = os.Exit
	runCoverage = coverage.Run
)

func handleQualityCommand(ctx context.Context, args []string) {
	if len(args) == 0 {
		printQualityUsage()
		return
	}

	switch args[0] {
	case "coverage":
		handleQualityCoverage(ctx, args[1:])
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
		if err := runHITLSync(ctx, args[1:], os.Stdout); err != nil {
			fmt.Printf("[-] HITL sync failed: %v\n", err)
		}

	default:
		printQualityUsage()
	}
}

func handleQualityCoverage(ctx context.Context, args []string) {
	ratchet := false
	update := false
	baselinePath := ".dendrite/coverage_baseline.json"
	var targetPackages []string

	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "--ratchet":
			ratchet = true
		case "--update":
			update = true
		case "--baseline":
			if i+1 < len(args) {
				baselinePath = args[i+1]
				i++
			}
		case "--help", "-h":
			printCoverageUsage()
			return
		default:
			if !strings.HasPrefix(args[i], "-") {
				targetPackages = append(targetPackages, args[i])
			}
		}
	}

	res, err := runCoverage(ctx, coverage.Options{
		BaselinePath: baselinePath,
		Ratchet:      ratchet,
		Update:       update,
		Packages:     targetPackages,
	})
	if err != nil {
		fmt.Fprintf(os.Stderr, "[-] Coverage error: %v\n", err)
		osExit(1)
		return
	}
	if !res.Passed {
		osExit(1)
		return
	}
}

func printQualityUsage() {
	fmt.Println("Usage: dendrite quality <audit|hitl-sync|coverage> [args...]")
	fmt.Println("\nCommands:")
	fmt.Println("  audit [package]    Run anti-mock scan and 100% coverage profiler on package")
	fmt.Println("  hitl-sync [--dry-run] [--json] [--repo owner/repo]")
	fmt.Println("                     Poll the GitHub issue of every Plane To Verify ticket: closed as completed -> Done, comments -> feedback")
	fmt.Println("  coverage [flags]   Per-package coverage ratchet and baseline management (--ratchet, --update)")
}

func printCoverageUsage() {
	fmt.Println("Usage: dendrite quality coverage [--ratchet] [--update] [--baseline <path>] [packages...]")
	fmt.Println("\nFlags:")
	fmt.Println("  --ratchet           Fail (exit 1) if any package drops below its baseline")
	fmt.Println("  --update            Automatically raise baseline with improvements and initialize new packages at 100%")
	fmt.Println("  --baseline <path>   Path to coverage baseline JSON file (default .dendrite/coverage_baseline.json)")
}

// runHITLSync polls the verification issue of every Plane ticket in To Verify.
func runHITLSync(ctx context.Context, args []string, out io.Writer) error {
	fs := flag.NewFlagSet("hitl-sync", flag.ContinueOnError)
	fs.SetOutput(out)
	repo := fs.String("repo", "", "Target GitHub repository owner/repo (default: the origin remote)")
	dryRun := fs.Bool("dry-run", false, "Report approvals without moving tickets to Done")
	asJSON := fs.Bool("json", false, "Output results as JSON")
	if err := fs.Parse(args); err != nil {
		return err
	}

	syncer, err := newHITLSyncer(ctx, *repo)
	if err != nil {
		return err
	}
	results, err := syncer.HITLSync(ctx, *dryRun)
	if err != nil {
		return err
	}
	if *asJSON {
		if results == nil {
			results = []hitl.HITLResult{}
		}
		data, _ := json.MarshalIndent(results, "", "  ")
		fmt.Fprintln(out, string(data))
		return nil
	}

	fmt.Fprintf(out, "[+] Human-in-the-Loop verification in %s (%d tickets To Verify)\n", syncer.GitHub.Repo, len(results))
	for _, r := range results {
		fmt.Fprintf(out, "    * %s\n", describeHITLResult(r, *dryRun))
	}
	return nil
}

func describeHITLResult(r hitl.HITLResult, dryRun bool) string {
	issue := "no issue"
	if r.IssueNumber > 0 {
		issue = fmt.Sprintf("#%d", r.IssueNumber)
	}
	line := fmt.Sprintf("[%s] %s (%s): %s", r.TicketID, r.Outcome, issue, r.Title)
	switch {
	case r.Error != "":
		line += " [failed: " + r.Error + "]"
	case r.Outcome == hitl.OutcomeApproved && r.Applied:
		line += " -> moved to Done in Plane"
	case r.Outcome == hitl.OutcomeApproved && dryRun:
		line += " -> would move to Done"
	case r.Outcome == hitl.OutcomeFeedback:
		last := r.Feedback[len(r.Feedback)-1]
		line += fmt.Sprintf(" -> %d comment(s), latest by @%s: %s", len(r.Feedback), last.Author, firstLine(last.Body))
	case r.Outcome == hitl.OutcomeNoIssue:
		line += " -> run `dendrite github-sync` to open its verification issue"
	}
	return line
}

func firstLine(s string) string {
	s = strings.TrimSpace(s)
	if i := strings.IndexByte(s, '\n'); i >= 0 {
		s = s[:i]
	}
	if len(s) > 100 {
		s = s[:100] + "…"
	}
	return s
}
