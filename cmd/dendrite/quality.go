package main

import (
	"context"
	"fmt"
	"io"

	"Dendrite/pkg/quality"
	"Dendrite/pkg/quality/coverage"

	"github.com/spf13/cobra"
)

// runCoverage is a seam so tests can run the ratchet without `go test ./...`.
var runCoverage = coverage.Run

// newAllostasisCmd builds `dendrite allostasis` (alias quality): the zero-mock scan and the coverage ratchet.
func newAllostasisCmd() *cobra.Command {
	cmd := newGroupCmd("allostasis", "Anti-mock scan, 100% coverage profiler, and HITL verification poller", "glia",
		"quality", "homeostatic-audit", "allostasis-audit", "qlty")
	cmd.Example = "dendrite allostasis coverage --ratchet\n" +
		"dendrite allostasis coverage --update pkg/vcs\n" +
		"dendrite allostasis audit pkg/vcs"
	cmd.AddCommand(
		&cobra.Command{
			Use:   "audit [package]",
			Short: "Scan a package for mocks and stubs and report statement coverage (default pkg/vcs)",
			Args:  maxArgs(1),
			RunE: func(cmd *cobra.Command, args []string) error {
				pkg := "pkg/vcs"
				if len(args) == 1 {
					pkg = args[0]
				}
				return runAllostasisAudit(cmd.Context(), pkg, cmd.OutOrStdout())
			},
		},
		newAllostasisCoverageCmd(),
	)
	return cmd
}

// runAllostasisAudit reports mocks and uncovered blocks; it fails when the package's tests fail.
func runAllostasisAudit(ctx context.Context, pkg string, out io.Writer) error {
	fmt.Fprintf(out, "[+] Running Anti-Mock & 100%% Coverage Quality Audit on %s...\n", pkg)
	hasMocks, mockLocs, err := quality.ScanForMocks(".", pkg)
	if err != nil {
		return fmt.Errorf("mock scan: %w", err)
	}
	if hasMocks {
		fmt.Fprintf(out, "[!] Zero-Mock Policy Violation: Detected %d mock pattern(s):\n", len(mockLocs))
		for _, loc := range mockLocs {
			fmt.Fprintf(out, "    * %s\n", loc)
		}
	} else {
		fmt.Fprintln(out, "[✓] Zero-Mock Policy: Clean (Zero mocks or stubs detected).")
	}

	uncovered, pct, failed, testOut := quality.RunCoverageAudit(ctx, ".", pkg)
	if failed {
		fmt.Fprintln(out, testOut)
		return fmt.Errorf("tests failed in %s", pkg)
	}
	fmt.Fprintf(out, "[✓] Statement Coverage: %.1f%%\n", pct)
	if len(uncovered) == 0 {
		fmt.Fprintln(out, "[✓] 100% Statement Coverage achieved!")
		return nil
	}
	fmt.Fprintf(out, "[!] %d Statement Block(s) Uncovered (< 100%%):\n", len(uncovered))
	for i, b := range uncovered {
		if i >= 5 {
			fmt.Fprintf(out, "    ... and %d more blocks\n", len(uncovered)-5)
			break
		}
		fmt.Fprintf(out, "    * %s\n", b)
	}
	return nil
}

func newAllostasisCoverageCmd() *cobra.Command {
	opts := coverage.Options{BaselinePath: ".dendrite/coverage_baseline.json"}
	cmd := &cobra.Command{
		Use:   "coverage [package...]",
		Short: "Per-package coverage ratchet: --ratchet fails on a drop, --update raises the baseline",
		Args:  cobra.ArbitraryArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			opts.Packages = args
			res, err := runCoverage(cmd.Context(), opts)
			if err != nil {
				return fmt.Errorf("coverage: %w", err)
			}
			if !res.Passed {
				return fmt.Errorf("coverage ratchet failed: a package dropped below its baseline")
			}
			return nil
		},
	}
	cmd.Flags().BoolVar(&opts.Ratchet, "ratchet", false, "Fail when any package drops below its baseline")
	cmd.Flags().BoolVar(&opts.Update, "update", false, "Raise the baseline to the measured coverage and add new packages")
	cmd.Flags().StringVar(&opts.BaselinePath, "baseline", opts.BaselinePath, "Baseline JSON file")
	return cmd
}
