package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"strings"

	"github.com/spf13/cobra"

	"Dendrite/pkg/quality"
	"Dendrite/pkg/quality/coverage"
)

var runCoverage = coverage.Run

// newAllostasisCmd builds `dendrite allostasis`: anti-mock scan and per-package statement coverage ratchet.
func newAllostasisCmd() *cobra.Command {
	cmd := newGroupCmd("allostasis", "Anti-mock scan, 100% coverage profiler, and per-package coverage ratchet", "glia",
		"quality", "homeostatic-audit", "allostasis-audit", "qlty")
	cmd.Example = "dendrite allostasis audit pkg/vcs\ndendrite allostasis coverage --ratchet"
	cmd.AddCommand(newAllostasisAuditCmd(), newAllostasisCoverageCmd())
	return cmd
}

func newAllostasisAuditCmd() *cobra.Command {
	return &cobra.Command{
		Use:     "audit [package]",
		Short:   "Run anti-mock scan and 100% coverage profiler on a package",
		Args:    maxArgs(1),
		Example: "dendrite allostasis audit\ndendrite allostasis audit pkg/vcs",
		RunE: func(c *cobra.Command, args []string) error {
			pkgPath := "pkg/vcs"
			if len(args) > 0 {
				pkgPath = args[0]
			}
			return runAllostasisAudit(c.Context(), pkgPath, c.OutOrStdout())
		},
	}
}

type allostasisCoverageOptions struct {
	Ratchet  bool
	Update   bool
	Baseline string
}

func newAllostasisCoverageCmd() *cobra.Command {
	var o allostasisCoverageOptions
	cmd := &cobra.Command{
		Use:     "coverage [packages...]",
		Short:   "Per-package coverage ratchet and baseline management",
		Example: "dendrite allostasis coverage --ratchet\ndendrite allostasis coverage --update pkg/vcs",
		RunE: func(c *cobra.Command, args []string) error {
			return runAllostasisCoverage(c.Context(), o, args)
		},
	}
	f := cmd.Flags()
	f.BoolVar(&o.Ratchet, "ratchet", false, "Fail if any package drops below its baseline")
	f.BoolVar(&o.Update, "update", false, "Raise baseline with improvements and initialize new packages at 100%")
	f.StringVar(&o.Baseline, "baseline", ".dendrite/coverage_baseline.json", "Path to coverage baseline JSON file")
	return cmd
}

func runAllostasisAudit(ctx context.Context, pkgPath string, out io.Writer) error {
	fmt.Fprintf(out, "[+] Running Anti-Mock & 100%% Coverage Quality Audit on %s...\n", pkgPath)
	hasMocks, mockLocs, mErr := quality.ScanForMocks(".", pkgPath)
	if mErr != nil {
		return fmt.Errorf("mock scan error: %w", mErr)
	}
	if hasMocks {
		fmt.Fprintf(out, "[!] Zero-Mock Policy Violation: Detected %d mock pattern(s):\n", len(mockLocs))
		for _, loc := range mockLocs {
			fmt.Fprintf(out, "    * %s\n", loc)
		}
		return fmt.Errorf("zero-mock policy violation: detected %d mock pattern(s) in %s", len(mockLocs), pkgPath)
	}
	fmt.Fprintln(out, "[✓] Zero-Mock Policy: Clean (Zero mocks or stubs detected).")

	uncovered, pct, failed, testOut := quality.RunCoverageAudit(ctx, ".", pkgPath)
	if failed {
		return fmt.Errorf("test execution failed:\n%s", strings.TrimSpace(testOut))
	}
	fmt.Fprintf(out, "[✓] Statement Coverage: %.1f%%\n", pct)
	if len(uncovered) > 0 {
		fmt.Fprintf(out, "[!] %d Statement Block(s) Uncovered (< 100%%):\n", len(uncovered))
		for i, b := range uncovered {
			if i >= 5 {
				fmt.Fprintf(out, "    ... and %d more blocks\n", len(uncovered)-5)
				break
			}
			fmt.Fprintf(out, "    * %s\n", b)
		}
	} else {
		fmt.Fprintln(out, "[✓] 100% Statement Coverage achieved!")
	}
	return nil
}

func runAllostasisCoverage(ctx context.Context, o allostasisCoverageOptions, targetPackages []string) error {
	res, err := runCoverage(ctx, coverage.Options{
		BaselinePath: o.Baseline,
		Ratchet:      o.Ratchet,
		Update:       o.Update,
		Packages:     targetPackages,
	})
	if err != nil {
		return fmt.Errorf("coverage error: %w", err)
	}
	if !res.Passed {
		return errors.New("coverage ratchet failed")
	}
	return nil
}

func handleQualityCommand(ctx context.Context, args []string) {
	_ = invokeCmd(ctx, newAllostasisCmd(), args...)
}
