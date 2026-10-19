package main

import (
	"bytes"
	"context"
	"errors"
	"strings"
	"testing"

	"Dendrite/pkg/quality/coverage"
)

// withCoverageRunner swaps runCoverage for the duration of the test.
func withCoverageRunner(t *testing.T, fn func(context.Context, coverage.Options) (*coverage.Result, error)) {
	t.Helper()
	orig := runCoverage
	runCoverage = fn
	t.Cleanup(func() { runCoverage = orig })
}

func TestAllostasisCoverage_FlagsAndSuccess(t *testing.T) {
	var captured coverage.Options
	withCoverageRunner(t, func(_ context.Context, opts coverage.Options) (*coverage.Result, error) {
		captured = opts
		return &coverage.Result{Passed: true}, nil
	})

	var out bytes.Buffer
	err := runWith(context.Background(), &out, newAllostasisCmd(),
		"allostasis", "coverage", "--ratchet", "--update", "--baseline", "custom_baseline.json", "pkg/sample")
	if err != nil {
		t.Fatalf("coverage: %v\n%s", err, out.String())
	}
	if !captured.Ratchet || !captured.Update {
		t.Fatalf("expected ratchet and update, got %+v", captured)
	}
	if captured.BaselinePath != "custom_baseline.json" {
		t.Fatalf("expected custom_baseline.json, got %s", captured.BaselinePath)
	}
	if len(captured.Packages) != 1 || captured.Packages[0] != "pkg/sample" {
		t.Fatalf("expected [pkg/sample], got %v", captured.Packages)
	}
}

func TestAllostasisCoverage_DefaultBaseline(t *testing.T) {
	var captured coverage.Options
	withCoverageRunner(t, func(_ context.Context, opts coverage.Options) (*coverage.Result, error) {
		captured = opts
		return &coverage.Result{Passed: true}, nil
	})

	var out bytes.Buffer
	if err := runWith(context.Background(), &out, newAllostasisCmd(), "allostasis", "coverage"); err != nil {
		t.Fatalf("coverage: %v", err)
	}
	if captured.BaselinePath != ".dendrite/coverage_baseline.json" {
		t.Fatalf("unexpected default baseline %q", captured.BaselinePath)
	}
}

func TestAllostasisCoverage_UnknownFlagRejected(t *testing.T) {
	withCoverageRunner(t, func(context.Context, coverage.Options) (*coverage.Result, error) {
		t.Fatal("runner must not be called on a flag error")
		return nil, nil
	})
	var out bytes.Buffer
	if err := runWith(context.Background(), &out, newAllostasisCmd(), "allostasis", "coverage", "--unknown-flag"); err == nil {
		t.Fatal("expected an unknown-flag error")
	}
}

func TestAllostasisCoverage_RunError(t *testing.T) {
	withCoverageRunner(t, func(context.Context, coverage.Options) (*coverage.Result, error) {
		return nil, errors.New("simulated runner failure")
	})
	var out bytes.Buffer
	err := runWith(context.Background(), &out, newAllostasisCmd(), "allostasis", "coverage", "--ratchet")
	if err == nil || !strings.Contains(err.Error(), "simulated runner failure") {
		t.Fatalf("expected runner failure, got %v", err)
	}
}

func TestAllostasisCoverage_RatchetFailed(t *testing.T) {
	withCoverageRunner(t, func(context.Context, coverage.Options) (*coverage.Result, error) {
		return &coverage.Result{Passed: false}, nil
	})
	var out bytes.Buffer
	err := runWith(context.Background(), &out, newAllostasisCmd(), "allostasis", "coverage", "--ratchet")
	if err == nil || !strings.Contains(err.Error(), "ratchet failed") {
		t.Fatalf("expected ratchet failure, got %v", err)
	}
}

func TestAllostasisCmd_LegacyQualityEntryPoint(t *testing.T) {
	calls := 0
	withCoverageRunner(t, func(context.Context, coverage.Options) (*coverage.Result, error) {
		calls++
		return &coverage.Result{Passed: true}, nil
	})
	handleQualityCommand(context.Background(), []string{"coverage", "--ratchet"})
	if calls != 1 {
		t.Fatalf("expected the legacy entry point to run coverage once, ran %d", calls)
	}
}
