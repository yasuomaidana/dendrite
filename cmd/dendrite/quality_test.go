package main

import (
	"context"
	"errors"
	"testing"

	"Dendrite/pkg/quality/coverage"
)

func TestHandleQualityCoverage_FlagsAndSuccess(t *testing.T) {
	origRun := runCoverage
	origExit := osExit
	defer func() {
		runCoverage = origRun
		osExit = origExit
	}()

	var capturedOpts coverage.Options
	runCoverage = func(ctx context.Context, opts coverage.Options) (*coverage.Result, error) {
		capturedOpts = opts
		return &coverage.Result{Passed: true}, nil
	}

	exitCalled := false
	osExit = func(code int) {
		exitCalled = true
	}

	ctx := context.Background()
	args := []string{
		"--ratchet",
		"--update",
		"--baseline",
		"custom_baseline.json",
		"--unknown-flag",
		"pkg/sample",
	}

	handleQualityCoverage(ctx, args)

	if exitCalled {
		t.Fatal("expected osExit not to be called on success")
	}
	if !capturedOpts.Ratchet {
		t.Fatal("expected Ratchet to be true")
	}
	if !capturedOpts.Update {
		t.Fatal("expected Update to be true")
	}
	if capturedOpts.BaselinePath != "custom_baseline.json" {
		t.Fatalf("expected custom_baseline.json, got %s", capturedOpts.BaselinePath)
	}
	if len(capturedOpts.Packages) != 1 || capturedOpts.Packages[0] != "pkg/sample" {
		t.Fatalf("expected [pkg/sample], got %v", capturedOpts.Packages)
	}
}

func TestHandleQualityCoverage_HelpFlags(t *testing.T) {
	ctx := context.Background()

	// Should return early and not invoke runCoverage or osExit
	handleQualityCoverage(ctx, []string{"--help"})
	handleQualityCoverage(ctx, []string{"-h"})
}

func TestHandleQualityCoverage_RunError(t *testing.T) {
	origRun := runCoverage
	origExit := osExit
	defer func() {
		runCoverage = origRun
		osExit = origExit
	}()

	runCoverage = func(ctx context.Context, opts coverage.Options) (*coverage.Result, error) {
		return nil, errors.New("simulated runner failure")
	}

	exitCode := 0
	osExit = func(code int) {
		exitCode = code
	}

	handleQualityCoverage(context.Background(), []string{"--ratchet"})

	if exitCode != 1 {
		t.Fatalf("expected exit code 1 on run error, got %d", exitCode)
	}
}

func TestHandleQualityCoverage_RatchetFailed(t *testing.T) {
	origRun := runCoverage
	origExit := osExit
	defer func() {
		runCoverage = origRun
		osExit = origExit
	}()

	runCoverage = func(ctx context.Context, opts coverage.Options) (*coverage.Result, error) {
		return &coverage.Result{Passed: false}, nil
	}

	exitCode := 0
	osExit = func(code int) {
		exitCode = code
	}

	handleQualityCoverage(context.Background(), []string{"--ratchet"})

	if exitCode != 1 {
		t.Fatalf("expected exit code 1 when ratchet fails, got %d", exitCode)
	}
}

func TestHandleQualityCommand(t *testing.T) {
	origRun := runCoverage
	origExit := osExit
	defer func() {
		runCoverage = origRun
		osExit = origExit
	}()

	runCoverage = func(ctx context.Context, opts coverage.Options) (*coverage.Result, error) {
		return &coverage.Result{Passed: true}, nil
	}
	osExit = func(code int) {}

	ctx := context.Background()

	// 1. Empty args
	handleQualityCommand(ctx, []string{})

	// 2. Unknown command
	handleQualityCommand(ctx, []string{"unrecognized-subcommand"})

	// 3. Coverage command
	handleQualityCommand(ctx, []string{"coverage", "--ratchet"})

	// 4. Audit command
	handleQualityCommand(ctx, []string{"audit", "pkg/quality/coverage"})

	// 5. Audit command default pkg
	handleQualityCommand(ctx, []string{"audit"})

	// 6. HITL sync command (offline: local Plane server and scripted gh)
	offlineSyncer(t)
	handleQualityCommand(ctx, []string{"hitl-sync"})
}

func TestPrintUsageFunctions(t *testing.T) {
	printQualityUsage()
	printCoverageUsage()
}
