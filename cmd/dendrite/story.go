package main

import (
	"context"
	"fmt"
	"io"
	"maps"
	"slices"
	"strings"

	"Dendrite/pkg/store"
	"Dendrite/pkg/story/fork"

	"github.com/spf13/cobra"
	"github.com/yasuomaidana/dendrite-mcp-hub/pkg/polyglot/contract"
)

// newForkController builds the story fork controller on use (worktrees in dendrite-work/<repo>-stories).
func newForkController() *fork.ForkController {
	return fork.NewForkController(".", "", fork.NewOSGitExecutor(), fork.NewDefaultBenchmarkHarness(),
		store.NewStore(".dendrite/store", "docs/adr"))
}

func defaultCodeDirs() map[string]string {
	return map[string]string{
		string(contract.LangGo):         "pkg",
		string(contract.LangRust):       "crates",
		string(contract.LangTypeScript): "src",
		string(contract.LangPython):     "python",
	}
}

// addContractFlags registers --schema-dir and --code-dir and returns their parsed values on use.
func addContractFlags(cmd *cobra.Command) func() ([]string, map[contract.TargetLanguage]string) {
	schemaDirs := []string{"schemas", "proto", "api"}
	codeDirs := defaultCodeDirs()
	cmd.Flags().StringSliceVar(&schemaDirs, "schema-dir", schemaDirs, "Contract schema directories")
	cmd.Flags().StringToStringVar(&codeDirs, "code-dir", codeDirs, "Code directory per language (lang=dir)")
	return func() ([]string, map[contract.TargetLanguage]string) {
		dirs := make(map[contract.TargetLanguage]string, len(codeDirs))
		for lang, dir := range codeDirs {
			dirs[contract.TargetLanguage(lang)] = dir
		}
		return schemaDirs, dirs
	}
}

// newSagaCmd builds `dendrite saga` (alias story): speculative story forks and cross-language contract checks.
func newSagaCmd() *cobra.Command {
	cmd := newGroupCmd("saga", "Autonomous multi-turn story execution and DAG branching", "effectors",
		"story", "stories", "synaptic-saga", "sty")
	cmd.Example = "dendrite saga fork cache --variants lru,arc\n" +
		"dendrite saga benchmark cache --iterations 200\n" +
		"dendrite saga resolve cache --winner arc\n" +
		"dendrite saga check-contracts --json"
	cmd.AddCommand(
		newSagaStartCmd(),
		newSagaCheckContractsCmd(),
		newSagaForkCmd(),
		newSagaBenchmarkCmd(),
		newSagaResolveCmd(),
		&cobra.Command{
			Use:   "abort <story>",
			Short: "Delete a story's variant worktrees and branches without merging",
			Args:  exactArgs(1),
			RunE: func(cmd *cobra.Command, args []string) error {
				fmt.Fprintf(cmd.OutOrStdout(), "[+] Aborting story '%s' and cleaning up speculative worktrees...\n", args[0])
				if err := newForkController().Abort(cmd.Context(), args[0]); err != nil {
					return fmt.Errorf("abort: %w", err)
				}
				fmt.Fprintf(cmd.OutOrStdout(), "[✓] Story '%s' worktrees and branches successfully purged.\n", args[0])
				return nil
			},
		},
		&cobra.Command{
			Use:   "status <story>",
			Short: "Show a story's state and variant worktrees",
			Args:  exactArgs(1),
			RunE: func(cmd *cobra.Command, args []string) error {
				return runSagaStatus(newForkController(), args[0], cmd.OutOrStdout())
			},
		},
	)
	return cmd
}

func newSagaStartCmd() *cobra.Command {
	var syncContracts bool
	cmd := &cobra.Command{
		Use:   "start <story>",
		Short: "Start a story, optionally checking cross-language contracts",
		Args:  exactArgs(1),
	}
	dirs := addContractFlags(cmd)
	cmd.Flags().BoolVar(&syncContracts, "sync-contracts", false, "Check contract drift and mark violations in the IDE")
	cmd.RunE = func(cmd *cobra.Command, args []string) error {
		out := cmd.OutOrStdout()
		fmt.Fprintf(out, "[✓] Story '%s' initialized.\n", args[0])
		if !syncContracts {
			return nil
		}
		fmt.Fprintln(out, "[+] Synchronizing Quad-IDE cross-language contracts...")
		monitor := contract.NewContractMonitor(nil)
		schemas, code := dirs()
		report, err := monitor.CheckDrift(schemas, code)
		if err != nil {
			// The story starts either way; a repo without schemas has nothing to check.
			fmt.Fprintf(out, "[-] Schema scan notice: %v\n", err)
			return nil
		}
		printDriftReport(out, report)
		if report.TotalViolations == 0 {
			fmt.Fprintln(out, "[✓] Zero contract drift detected across Quad-IDE models.")
			return nil
		}
		if err := monitor.NotifyIDE(cmd.Context(), report); err != nil {
			fmt.Fprintf(out, "[!] IDE markers not sent: %v\n", err)
		} else {
			fmt.Fprintln(out, "[!] Emitted diagnostic markers to connected JetBrains IDE.")
		}
		return nil
	}
	return cmd
}

func newSagaCheckContractsCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:     "check-contracts",
		Aliases: []string{"sync-contracts"},
		Short:   "Report schema drift across Protobuf, OpenAPI, Go, Rust, TypeScript and Python (exit 1 on errors)",
		Args:    noArgs,
	}
	dirs := addContractFlags(cmd)
	cmd.RunE = func(cmd *cobra.Command, args []string) error {
		schemas, code := dirs()
		report, err := contract.NewContractMonitor(nil).CheckDrift(schemas, code)
		if err != nil {
			return fmt.Errorf("check contracts: %w", err)
		}
		if globals.JSON {
			if err := writeJSON(cmd.OutOrStdout(), report); err != nil {
				return err
			}
		} else {
			printDriftReport(cmd.OutOrStdout(), report)
		}
		if report.HasErrors {
			return fmt.Errorf("critical contract drift: %d violations", report.TotalViolations)
		}
		return nil
	}
	return cmd
}

func newSagaForkCmd() *cobra.Command {
	var variants []string
	var base string
	cmd := &cobra.Command{
		Use:   "fork <story>",
		Short: "Fork a story into one git worktree per variant",
		Args:  exactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(variants) < 2 {
				return usageErrorf("--variants needs at least two names, e.g. --variants v1,v2")
			}
			out := cmd.OutOrStdout()
			fmt.Fprintf(out, "[+] Forking story '%s' into %d speculative variants (%s)...\n", args[0], len(variants), strings.Join(variants, ", "))
			manifest, guard, err := newForkController().Fork(cmd.Context(), args[0], variants, base)
			if err != nil {
				return fmt.Errorf("fork: %w", err)
			}
			guard.Dismiss() // keep the worktrees for implementation
			fmt.Fprintf(out, "[✓] Story '%s' successfully forked into isolated Git worktrees:\n", args[0])
			for _, v := range manifest.Variants {
				fmt.Fprintf(out, "    * Variant '%s':\n", v.Name)
				fmt.Fprintf(out, "      - Branch:   %s\n", v.Branch)
				fmt.Fprintf(out, "      - Worktree: %s\n", v.WorktreePath)
			}
			fmt.Fprintf(out, "[i] Base commit: %s (branch: %s)\n", manifest.BaseCommit, manifest.BaseBranch)
			return nil
		},
	}
	cmd.Flags().StringSliceVar(&variants, "variants", nil, "Variant names (comma-separated, at least two)")
	cmd.Flags().StringVar(&base, "base", "", "Commit to fork from (default HEAD)")
	requireFlags(cmd, "variants")
	return cmd
}

func newSagaBenchmarkCmd() *cobra.Command {
	opts := fork.BenchmarkOptions{Iterations: 100, VirtualUsers: 1}
	cmd := &cobra.Command{
		Use:     "benchmark <story>",
		Aliases: []string{"bench"},
		Short:   "Benchmark every variant worktree and recommend a winner",
		Args:    exactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if opts.Iterations <= 0 || opts.VirtualUsers <= 0 {
				return usageErrorf("--iterations and --vus must be positive")
			}
			return runSagaBenchmark(cmd.Context(), newForkController(), args[0], opts, cmd.OutOrStdout())
		},
	}
	cmd.Flags().IntVar(&opts.Iterations, "iterations", opts.Iterations, "Iterations per variant")
	cmd.Flags().IntVar(&opts.VirtualUsers, "vus", opts.VirtualUsers, "Concurrent virtual users")
	return cmd
}

func runSagaBenchmark(ctx context.Context, c *fork.ForkController, story string, opts fork.BenchmarkOptions, out io.Writer) error {
	if !globals.JSON {
		fmt.Fprintf(out, "[+] Executing differential benchmark across worktrees for story '%s' (%d iterations)...\n", story, opts.Iterations)
	}
	scorecard, err := c.Benchmark(ctx, story, opts)
	if err != nil {
		return fmt.Errorf("benchmark: %w", err)
	}
	if globals.JSON {
		return writeJSON(out, scorecard)
	}
	fmt.Fprintf(out, "\n=== Differential Architecture Benchmark Scorecard (%s) ===\n", story)
	fmt.Fprintf(out, "Base Commit: %s | Evaluated At: %s\n\n", scorecard.BaseCommit, scorecard.Timestamp.Format("2006-01-02 15:04:05 UTC"))
	fmt.Fprintf(out, "%-20s %-12s %-14s %-12s %-10s %-10s\n", "VARIANT", "TTFT", "TPS (OPS/SEC)", "MEMORY (MB)", "CPU (%)", "STATUS")
	fmt.Fprintln(out, strings.Repeat("-", 80))
	for _, name := range slices.Sorted(maps.Keys(scorecard.Metrics)) {
		m := scorecard.Metrics[name]
		if m.Error != "" {
			fmt.Fprintf(out, "%-20s %-12s %-14s %-12s %-10s ERROR: %s\n", name, "-", "-", "-", "-", m.Error)
			continue
		}
		status := "PASS"
		if name == scorecard.RecommendedWinner {
			status = "WINNER ★"
		}
		fmt.Fprintf(out, "%-20s %-12v %-14.1f %-12.2f %-10.1f %-10s\n", name, m.TTFT, m.TPS, m.MemoryMB, m.CPUPercent, status)
	}
	fmt.Fprintln(out, strings.Repeat("-", 80))
	fmt.Fprintf(out, "[★] Recommended Winner: '%s'\n", scorecard.RecommendedWinner)
	fmt.Fprintf(out, "[i] Evaluation Summary: %s\n", scorecard.Reason)
	fmt.Fprintln(out, "\n[+] To merge the winner and purge alternates, execute:")
	fmt.Fprintf(out, "    dendrite saga resolve %q --winner %q\n", story, scorecard.RecommendedWinner)
	return nil
}

func newSagaResolveCmd() *cobra.Command {
	var winner string
	cmd := &cobra.Command{
		Use:   "resolve <story>",
		Short: "Merge the winning variant, store its benchmark, and delete the others",
		Args:  exactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			out := cmd.OutOrStdout()
			fmt.Fprintf(out, "[+] Resolving story '%s' to winning variant '%s'...\n", args[0], winner)
			res, err := newForkController().Resolve(cmd.Context(), args[0], winner)
			if err != nil {
				return fmt.Errorf("resolve: %w", err)
			}
			fmt.Fprintf(out, "[✓] Winning variant '%s' successfully merged into base repository.\n", winner)
			fmt.Fprintf(out, "    * Merged Branch:    %s\n", res.MergedBranch)
			fmt.Fprintf(out, "    * Purged Alternates: %s\n", strings.Join(res.PurgedVariants, ", "))
			fmt.Fprintf(out, "    * Saved Store Key:  %s\n", res.StoreKey)
			fmt.Fprintln(out, "[✓] All alternate worktrees pruned and git lock indexes cleaned.")
			return nil
		},
	}
	cmd.Flags().StringVar(&winner, "winner", "", "Winning variant")
	requireFlags(cmd, "winner")
	return cmd
}

func runSagaStatus(c *fork.ForkController, story string, out io.Writer) error {
	manifest, err := c.GetManifest(story)
	if err != nil {
		return fmt.Errorf("story '%s' has no active fork: %w", story, err)
	}
	if globals.JSON {
		return writeJSON(out, manifest)
	}
	fmt.Fprintf(out, "=== Speculative Story Status: %s ===\n", story)
	fmt.Fprintf(out, "Status:      %s\n", manifest.Status)
	fmt.Fprintf(out, "Base Branch: %s (%s)\n", manifest.BaseBranch, manifest.BaseCommit)
	fmt.Fprintf(out, "Created At:  %s\n", manifest.CreatedAt.Format("2006-01-02 15:04:05 UTC"))
	if manifest.Winner != "" {
		fmt.Fprintf(out, "Winner:      %s\n", manifest.Winner)
	}
	fmt.Fprintf(out, "Variants (%d):\n", len(manifest.Variants))
	for _, v := range manifest.Variants {
		fmt.Fprintf(out, "  - %-15s (Branch: %s | Worktree: %s)\n", v.Name, v.Branch, v.WorktreePath)
	}
	return nil
}

func printDriftReport(out io.Writer, report *contract.DriftReport) {
	fmt.Fprintf(out, "=== Quad-IDE Cross-Language Contract Drift Report ===\n")
	fmt.Fprintf(out, "Scanned Contract Specifications: %d\n", report.ScannedContracts)
	fmt.Fprintf(out, "Scanned Codebase Models:         %d\n", report.ScannedCodeModels)
	fmt.Fprintf(out, "Total Drift Violations:          %d\n", report.TotalViolations)
	fmt.Fprintf(out, "Status:                          ")
	if report.HasErrors {
		fmt.Fprintf(out, "CRITICAL DRIFT DETECTED\n")
	} else if report.TotalViolations > 0 {
		fmt.Fprintf(out, "WARNINGS DETECTED\n")
	} else {
		fmt.Fprintf(out, "PARITY CONFIRMED (CLEAN)\n")
	}

	for i, v := range report.Violations {
		fmt.Fprintf(out, "\n  [%d] [%s] %s (in %s)\n", i+1, v.Severity, v.Type, v.TargetLang)
		fmt.Fprintf(out, "      Schema:   %s.%s\n", v.SchemaName, v.FieldName)
		fmt.Fprintf(out, "      File:     %s:%d\n", v.TargetFile, v.Line)
		fmt.Fprintf(out, "      Expected: %s\n", v.Expected)
		fmt.Fprintf(out, "      Actual:   %s\n", v.Actual)
		fmt.Fprintf(out, "      Message:  %s\n", v.Message)
	}
}

