package main

import (
	"Dendrite/pkg/statedir"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strconv"
	"strings"

	"github.com/yasuomaidana/dendrite-mcp-hub/pkg/polyglot/contract"
	"Dendrite/pkg/store"
	"Dendrite/pkg/story/fork"
)

var (
	defaultStoryMonitor  = contract.NewContractMonitor(nil)
	defaultCLIController = fork.NewForkController(
		".",
		statedir.Resolve(".dendrite/worktrees"),
		fork.NewOSGitExecutor(),
		fork.NewDefaultBenchmarkHarness(),
		store.NewStore(".dendrite/store", "docs/adr"),
	)
)

func handleStoryCommand(ctx context.Context, action string, args []string) {
	switch action {
	case "next":
		handlePlaneCommand(ctx, "next", args)
		return
	case "start":
		if len(args) == 0 {
			fmt.Println("Usage: dendrite story start \"<story-name>\" [--sync-contracts] [--schema-dir <dir>]")
			return
		}
		handleStoryStart(ctx, args[0], args[1:])
	case "check-contracts", "sync-contracts":
		handleStoryCheckContracts(ctx, args)
	case "fork":
		if len(args) == 0 {
			fmt.Println("Usage: dendrite story fork \"<story>\" --variants \"v1,v2\" [--base <commit>]")
			return
		}
		handleStoryFork(ctx, args[0], args[1:])
	case "benchmark", "bench":
		if len(args) == 0 {
			fmt.Println("Usage: dendrite story benchmark \"<story>\" [--iterations <n>] [--vus <n>] [--json]")
			return
		}
		handleStoryBenchmark(ctx, args[0], args[1:])
	case "resolve":
		if len(args) == 0 {
			fmt.Println("Usage: dendrite story resolve \"<story>\" --winner \"<name>\"")
			return
		}
		handleStoryResolve(ctx, args[0], args[1:])
	case "abort":
		if len(args) == 0 {
			fmt.Println("Usage: dendrite story abort \"<story>\"")
			return
		}
		handleStoryAbort(ctx, args[0])
	case "status":
		if len(args) == 0 {
			fmt.Println("Usage: dendrite story status \"<story>\"")
			return
		}
		handleStoryStatus(ctx, args[0])
	default:
		printStoryUsage()
	}
}

func handleStoryFork(ctx context.Context, storyID string, args []string) {
	var variants []string
	var baseCommit string

	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "--variants":
			if i+1 < len(args) {
				raw := args[i+1]
				parts := strings.Split(raw, ",")
				for _, p := range parts {
					item := strings.TrimSpace(p)
					if item != "" {
						variants = append(variants, item)
					}
				}
				i++
			}
		case "--base":
			if i+1 < len(args) {
				baseCommit = args[i+1]
				i++
			}
		}
	}

	if len(variants) < 2 {
		fmt.Println("[-] Error: --variants flag is required with at least 2 comma-separated variants (e.g. --variants \"v1,v2\")")
		os.Exit(1)
	}

	fmt.Printf("[+] Forking story '%s' into %d speculative variants (%s)...\n", storyID, len(variants), strings.Join(variants, ", "))
	manifest, guard, err := defaultCLIController.Fork(ctx, storyID, variants, baseCommit)
	if err != nil {
		fmt.Printf("[-] Fork failed: %v\n", err)
		os.Exit(1)
	}
	guard.Dismiss() // Leave worktrees active for developer inspection/implementation

	fmt.Printf("[✓] Story '%s' successfully forked into isolated Git worktrees:\n", storyID)
	for _, v := range manifest.Variants {
		fmt.Printf("    * Variant '%s':\n", v.Name)
		fmt.Printf("      - Branch:   %s\n", v.Branch)
		fmt.Printf("      - Worktree: %s\n", v.WorktreePath)
	}
	fmt.Printf("[i] Base commit: %s (branch: %s)\n", manifest.BaseCommit, manifest.BaseBranch)
}

func handleStoryBenchmark(ctx context.Context, storyID string, args []string) {
	iterations := 100
	vus := 1
	jsonOutput := false

	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "--iterations":
			if i+1 < len(args) {
				if n, err := strconv.Atoi(args[i+1]); err == nil && n > 0 {
					iterations = n
				}
				i++
			}
		case "--vus":
			if i+1 < len(args) {
				if n, err := strconv.Atoi(args[i+1]); err == nil && n > 0 {
					vus = n
				}
				i++
			}
		case "--json":
			jsonOutput = true
		}
	}

	fmt.Printf("[+] Executing differential benchmark across worktrees for story '%s' (%d iterations)...\n", storyID, iterations)
	opts := fork.BenchmarkOptions{
		Iterations:   iterations,
		VirtualUsers: vus,
	}

	scorecard, err := defaultCLIController.Benchmark(ctx, storyID, opts)
	if err != nil {
		fmt.Printf("[-] Benchmark failed: %v\n", err)
		os.Exit(1)
	}

	if jsonOutput {
		data, _ := json.MarshalIndent(scorecard, "", "  ")
		fmt.Println(string(data))
		return
	}

	fmt.Printf("\n=== Differential Architecture Benchmark Scorecard (%s) ===\n", storyID)
	fmt.Printf("Base Commit: %s | Evaluated At: %s\n\n", scorecard.BaseCommit, scorecard.Timestamp.Format("2006-01-02 15:04:05 UTC"))

	fmt.Printf("%-20s %-12s %-14s %-12s %-10s %-10s\n", "VARIANT", "TTFT", "TPS (OPS/SEC)", "MEMORY (MB)", "CPU (%)", "STATUS")
	fmt.Println(strings.Repeat("-", 80))
	for name, m := range scorecard.Metrics {
		if m.Error != "" {
			fmt.Printf("%-20s %-12s %-14s %-12s %-10s ERROR: %s\n", name, "-", "-", "-", "-", m.Error)
			continue
		}
		status := "PASS"
		if name == scorecard.RecommendedWinner {
			status = "WINNER ★"
		}
		fmt.Printf("%-20s %-12v %-14.1f %-12.2f %-10.1f %-10s\n", name, m.TTFT, m.TPS, m.MemoryMB, m.CPUPercent, status)
	}
	fmt.Println(strings.Repeat("-", 80))
	fmt.Printf("[★] Recommended Winner: '%s'\n", scorecard.RecommendedWinner)
	fmt.Printf("[i] Evaluation Summary: %s\n", scorecard.Reason)
	fmt.Printf("\n[+] To merge the winner and purge alternates, execute:\n")
	fmt.Printf("    dendrite story resolve \"%s\" --winner \"%s\"\n", storyID, scorecard.RecommendedWinner)
}

func handleStoryResolve(ctx context.Context, storyID string, args []string) {
	var winner string
	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "--winner":
			if i+1 < len(args) {
				winner = args[i+1]
				i++
			}
		}
	}

	if winner == "" {
		fmt.Println("[-] Error: --winner flag is required (e.g. --winner \"v2\")")
		os.Exit(1)
	}

	fmt.Printf("[+] Resolving story '%s' to winning variant '%s'...\n", storyID, winner)
	res, err := defaultCLIController.Resolve(ctx, storyID, winner)
	if err != nil {
		fmt.Printf("[-] Resolve failed: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("[✓] Winning variant '%s' successfully merged into base repository.\n", winner)
	fmt.Printf("    * Merged Branch:    %s\n", res.MergedBranch)
	fmt.Printf("    * Purged Alternates: %s\n", strings.Join(res.PurgedVariants, ", "))
	fmt.Printf("    * Saved Store Key:  %s\n", res.StoreKey)
	fmt.Printf("[✓] All alternate worktrees pruned and git lock indexes cleaned.\n")
}

func handleStoryAbort(ctx context.Context, storyID string) {
	fmt.Printf("[+] Aborting story '%s' and cleaning up speculative worktrees...\n", storyID)
	err := defaultCLIController.Abort(ctx, storyID)
	if err != nil {
		fmt.Printf("[-] Abort failed: %v\n", err)
		os.Exit(1)
	}
	fmt.Printf("[✓] Story '%s' worktrees and branches successfully purged.\n", storyID)
}

func handleStoryStatus(ctx context.Context, storyID string) {
	manifest, err := defaultCLIController.GetManifest(storyID)
	if err != nil {
		fmt.Printf("[-] Story '%s' has no active fork or manifest: %v\n", storyID, err)
		os.Exit(1)
	}

	fmt.Printf("=== Speculative Story Status: %s ===\n", storyID)
	fmt.Printf("Status:      %s\n", manifest.Status)
	fmt.Printf("Base Branch: %s (%s)\n", manifest.BaseBranch, manifest.BaseCommit)
	fmt.Printf("Created At:  %s\n", manifest.CreatedAt.Format("2006-01-02 15:04:05 UTC"))
	if manifest.Winner != "" {
		fmt.Printf("Winner:      %s\n", manifest.Winner)
	}
	fmt.Printf("Variants (%d):\n", len(manifest.Variants))
	for _, v := range manifest.Variants {
		fmt.Printf("  - %-15s (Branch: %s | Worktree: %s)\n", v.Name, v.Branch, v.WorktreePath)
	}
}

func handleStoryStart(ctx context.Context, storyName string, args []string) {
	syncContracts := false
	schemaDirs := []string{"schemas", "proto", "api"}
	codeDirs := map[contract.TargetLanguage]string{
		contract.LangGo:         "pkg",
		contract.LangRust:       "crates",
		contract.LangTypeScript: "src",
		contract.LangPython:     "python",
	}

	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "--sync-contracts":
			syncContracts = true
		case "--schema-dir":
			if i+1 < len(args) {
				schemaDirs = []string{args[i+1]}
				i++
			}
		case "--code-dir":
			if i+1 < len(args) {
				parts := strings.Split(args[i+1], "=")
				if len(parts) == 2 {
					codeDirs[contract.TargetLanguage(parts[0])] = parts[1]
				}
				i++
			}
		}
	}

	fmt.Printf("[✓] Story '%s' initialized.\n", storyName)

	if syncContracts {
		fmt.Println("[+] Synchronizing Quad-IDE cross-language contracts...")
		report, err := defaultStoryMonitor.CheckDrift(schemaDirs, codeDirs)
		if err != nil {
			fmt.Printf("[-] Schema scan notice: %v\n", err)
			return
		}

		printDriftReport(report)
		if report.TotalViolations > 0 {
			_ = defaultStoryMonitor.NotifyIDE(ctx, report)
			fmt.Println("[!] Emitted diagnostic markers to connected JetBrains IDE.")
		} else {
			fmt.Println("[✓] Zero contract drift detected across Quad-IDE models.")
		}
	}
}

func handleStoryCheckContracts(ctx context.Context, args []string) {
	schemaDirs := []string{"schemas", "proto", "api"}
	codeDirs := map[contract.TargetLanguage]string{
		contract.LangGo:         "pkg",
		contract.LangRust:       "crates",
		contract.LangTypeScript: "src",
		contract.LangPython:     "python",
	}
	jsonOutput := false

	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "--schema-dir":
			if i+1 < len(args) {
				schemaDirs = []string{args[i+1]}
				i++
			}
		case "--json":
			jsonOutput = true
		}
	}

	report, err := defaultStoryMonitor.CheckDrift(schemaDirs, codeDirs)
	if err != nil {
		fmt.Printf("[-] Failed to check contracts: %v\n", err)
		os.Exit(1)
	}

	if jsonOutput {
		data, _ := json.MarshalIndent(report, "", "  ")
		fmt.Println(string(data))
		return
	}

	printDriftReport(report)
}

func printDriftReport(report *contract.DriftReport) {
	fmt.Printf("=== Quad-IDE Cross-Language Contract Drift Report ===\n")
	fmt.Printf("Scanned Contract Specifications: %d\n", report.ScannedContracts)
	fmt.Printf("Scanned Codebase Models:         %d\n", report.ScannedCodeModels)
	fmt.Printf("Total Drift Violations:          %d\n", report.TotalViolations)
	fmt.Printf("Status:                          ")
	if report.HasErrors {
		fmt.Printf("CRITICAL DRIFT DETECTED\n")
	} else if report.TotalViolations > 0 {
		fmt.Printf("WARNINGS DETECTED\n")
	} else {
		fmt.Printf("PARITY CONFIRMED (CLEAN)\n")
	}

	for i, v := range report.Violations {
		fmt.Printf("\n  [%d] [%s] %s (in %s)\n", i+1, v.Severity, v.Type, v.TargetLang)
		fmt.Printf("      Schema:   %s.%s\n", v.SchemaName, v.FieldName)
		fmt.Printf("      File:     %s:%d\n", v.TargetFile, v.Line)
		fmt.Printf("      Expected: %s\n", v.Expected)
		fmt.Printf("      Actual:   %s\n", v.Actual)
		fmt.Printf("      Message:  %s\n", v.Message)
	}
}

func printStoryUsage() {
	fmt.Println("Usage: dendrite story <command> [args...]")
	fmt.Println("\nAvailable Story Commands:")
	fmt.Println("  start \"<name>\" [--sync-contracts]         Initialize story context with cross-language contract monitoring")
	fmt.Println("  check-contracts [--schema-dir <d>]        Audit schema drift across Protobuf, OpenAPI, Go, Rust, TS, and Python")
	fmt.Println("  fork \"<story>\" --variants \"v1,v2\"        Fork story into isolated Git worktrees for competing variants")
	fmt.Println("  benchmark \"<story>\" [--iterations <n>]   Run differential performance benchmarks across variant worktrees")
	fmt.Println("  resolve \"<story>\" --winner \"<name>\"     Merge winning worktree, persist benchmarks in store, purge alternates")
	fmt.Println("  abort \"<story>\"                           Safely purge speculative worktrees and branches without merging")
	fmt.Println("  status \"<story>\"                          Display current lifecycle state and active worktrees of a story")
}
