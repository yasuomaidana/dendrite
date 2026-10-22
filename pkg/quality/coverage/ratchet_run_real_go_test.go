package coverage

import (
	"bytes"
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func TestRun_RealGoModule(t *testing.T) {
	tmpDir := t.TempDir()

	// Write go.mod
	goMod := "module example.com/ratchetpkg\n\ngo 1.22\n"
	if err := os.WriteFile(filepath.Join(tmpDir, "go.mod"), []byte(goMod), 0644); err != nil {
		t.Fatal(err)
	}

	// Write package code
	code := `package ratchetpkg

func Add(a, b int) int {
	return a + b
}
`
	if err := os.WriteFile(filepath.Join(tmpDir, "calc.go"), []byte(code), 0644); err != nil {
		t.Fatal(err)
	}

	testCode := `package ratchetpkg

import "testing"

func TestAdd(t *testing.T) {
	if Add(2, 3) != 5 {
		t.Fail()
	}
}
`
	if err := os.WriteFile(filepath.Join(tmpDir, "calc_test.go"), []byte(testCode), 0644); err != nil {
		t.Fatal(err)
	}

	baselineFile := filepath.Join(tmpDir, ".dendrite", "coverage_baseline.json")
	var stdout, stderr bytes.Buffer

	// Step 1: Initial run with --update to generate baseline
	res1, err := Run(context.Background(), Options{
		RepoDir:      tmpDir,
		BaselinePath: baselineFile,
		Ratchet:      true,
		Update:       true,
		Stdout:       &stdout,
		Stderr:       &stderr,
	})
	if err != nil {
		t.Fatalf("initial run failed: %v", err)
	}
	if !res1.Passed {
		t.Fatalf("expected initial run to pass, stderr: %s", stderr.String())
	}

	// Verify baseline file created
	loaded, err := LoadBaseline(baselineFile)
	if err != nil {
		t.Fatalf("failed to load generated baseline: %v", err)
	}
	if loaded["."] != 100.0 {
		t.Fatalf("expected 100.0%% for root package, got: %v", loaded)
	}

	// Step 2: Second run with ratchet passes cleanly
	stdout.Reset()
	stderr.Reset()
	res2, err := Run(context.Background(), Options{
		RepoDir:      tmpDir,
		BaselinePath: baselineFile,
		Ratchet:      true,
		Update:       true,
		Stdout:       &stdout,
		Stderr:       &stderr,
	})
	if err != nil {
		t.Fatalf("second run failed: %v", err)
	}
	if !res2.Passed {
		t.Fatalf("expected second run to pass, stderr: %s", stderr.String())
	}
}

func TestRun_CommandFailure(t *testing.T) {
	tmpDir := t.TempDir()
	runner := func(ctx context.Context, dir string, name string, args ...string) ([]byte, error) {
		return []byte("FAIL\tDendrite/pkg/broken\t0.015s\n"), errors.New("exit status 1")
	}

	var stdout, stderr bytes.Buffer
	res, err := Run(context.Background(), Options{
		RepoDir:       tmpDir,
		BaselinePath:  filepath.Join(tmpDir, "baseline.json"),
		CommandRunner: runner,
		Stdout:        &stdout,
		Stderr:        &stderr,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if res.Passed {
		t.Fatal("expected failure on test command error")
	}
	if !bytes.Contains(stderr.Bytes(), []byte("Test execution failed")) {
		t.Fatalf("expected error details in stderr: %s", stderr.String())
	}
}

func TestRun_RatchetViolations(t *testing.T) {
	tmpDir := t.TempDir()
	baselineFile := filepath.Join(tmpDir, "baseline.json")

	// Existing baseline with 90.0%
	if err := SaveBaseline(baselineFile, map[string]float64{"pkg/existing": 90.0}); err != nil {
		t.Fatal(err)
	}

	// Current output shows drop to 80.0%, plus a new package at 75.0%
	runner := func(ctx context.Context, dir string, name string, args ...string) ([]byte, error) {
		out := "ok  \tDendrite/pkg/existing\t0.01s\tcoverage: 80.0% of statements\n" +
			"ok  \tDendrite/pkg/brandnew\t0.01s\tcoverage: 75.0% of statements\n"
		return []byte(out), nil
	}

	var stdout, stderr bytes.Buffer
	res, err := Run(context.Background(), Options{
		RepoDir:       tmpDir,
		BaselinePath:  baselineFile,
		Ratchet:       true,
		Update:        false,
		CommandRunner: runner,
		Stdout:        &stdout,
		Stderr:        &stderr,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if res.Passed {
		t.Fatal("expected ratchet to fail due to coverage drops")
	}
	if len(res.Offenders) != 2 {
		t.Fatalf("expected 2 offenders, got %d: %v", len(res.Offenders), res.Offenders)
	}
	if !bytes.Contains(stderr.Bytes(), []byte("Coverage ratchet violation")) {
		t.Fatalf("expected violation output: %s", stderr.String())
	}
}

func TestRun_SaveBaselineError(t *testing.T) {
	tmpDir := t.TempDir()
	blockerFile := filepath.Join(tmpDir, "file_blocking_dir")
	if err := os.WriteFile(blockerFile, []byte("x"), 0644); err != nil {
		t.Fatal(err)
	}
	impossiblePath := filepath.Join(blockerFile, "baseline.json")

	runner := func(ctx context.Context, dir string, name string, args ...string) ([]byte, error) {
		return []byte("ok  \tDendrite/pkg/a\t0.01s\tcoverage: 100.0% of statements\n"), nil
	}

	var stdout, stderr bytes.Buffer
	_, err := Run(context.Background(), Options{
		RepoDir:       tmpDir,
		BaselinePath:  impossiblePath,
		Update:        true,
		CommandRunner: runner,
		Stdout:        &stdout,
		Stderr:        &stderr,
	})
	if err == nil {
		t.Fatal("expected error when saving baseline to impossible path")
	}
}

func TestRun_CorruptBaselineLoadError(t *testing.T) {
	tmpDir := t.TempDir()
	corruptFile := filepath.Join(tmpDir, "corrupt.json")
	if err := os.WriteFile(corruptFile, []byte("NOT_JSON"), 0644); err != nil {
		t.Fatal(err)
	}

	var stdout, stderr bytes.Buffer
	_, err := Run(context.Background(), Options{
		RepoDir:      tmpDir,
		BaselinePath: corruptFile,
		Stdout:       &stdout,
		Stderr:       &stderr,
	})
	if err == nil {
		t.Fatal("expected error on corrupt baseline")
	}
}

func scriptedGoTest(output string) func(ctx context.Context, dir string, name string, args ...string) ([]byte, error) {
	return func(ctx context.Context, dir string, name string, args ...string) ([]byte, error) {
		return []byte(output), nil
	}
}

func TestRun_AppliesDefaults(t *testing.T) {
	t.Chdir(t.TempDir())
	res, err := Run(context.Background(), Options{
		CommandRunner: scriptedGoTest("ok  \tDendrite/pkg/a\t0.1s\tcoverage: 100.0% of statements\n"),
	})
	if err != nil || res == nil || !res.Passed {
		t.Fatalf("expected passing run with defaults, got res=%v err=%v", res, err)
	}
	if _, statErr := os.Stat(".dendrite/coverage_baseline.json"); !os.IsNotExist(statErr) {
		t.Fatalf("a plain run must not write the default baseline, stat err=%v", statErr)
	}
}

func TestRun_CorruptBaselineFails(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "baseline.json")
	if err := os.WriteFile(path, []byte("{not json"), 0o644); err != nil {
		t.Fatal(err)
	}
	var out, errOut bytes.Buffer
	if _, err := Run(context.Background(), Options{RepoDir: dir, BaselinePath: path, Ratchet: true, Stdout: &out, Stderr: &errOut, CommandRunner: scriptedGoTest("")}); err == nil {
		t.Fatal("expected an error for a corrupt baseline")
	}
}

func TestRun_UpdateSaveErrorIsReturned(t *testing.T) {
	dir := t.TempDir()
	blocker := filepath.Join(dir, "file")
	if err := os.WriteFile(blocker, nil, 0o644); err != nil {
		t.Fatal(err)
	}
	var out, errOut bytes.Buffer
	_, err := Run(context.Background(), Options{
		RepoDir: dir, BaselinePath: filepath.Join(blocker, "baseline.json"), Update: true,
		Stdout: &out, Stderr: &errOut,
		CommandRunner: scriptedGoTest("ok  \tDendrite/pkg/a\t0.1s\tcoverage: 50.0% of statements\n"),
	})
	if err == nil {
		t.Fatal("expected save error when the baseline path is under a regular file")
	}
}

func TestRun_NewPackageMustStartAt100(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "go.mod"), []byte("module Dendrite\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dir, "baseline.json")
	if err := SaveBaseline(path, map[string]float64{"pkg/a": 100}); err != nil {
		t.Fatal(err)
	}
	var out, errOut bytes.Buffer
	res, err := Run(context.Background(), Options{
		RepoDir: dir, BaselinePath: path, Ratchet: true, Stdout: &out, Stderr: &errOut,
		CommandRunner: scriptedGoTest("ok  \tDendrite/pkg/a\t0.1s\tcoverage: 100.0% of statements\nok  \tDendrite/pkg/b\t0.1s\tcoverage: 60.0% of statements\n"),
	})
	if err != nil || res.Passed {
		t.Fatalf("expected ratchet failure for a new package below 100%%, got res=%v err=%v", res, err)
	}
	if !bytes.Contains(errOut.Bytes(), []byte("new packages must start at 100.0%")) {
		t.Fatalf("expected new-package message, got: %s", errOut.String())
	}
}

func TestRun_UpdateCannotWriteBaseline(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "baseline.json")
	if err := SaveBaseline(path, map[string]float64{"pkg/a": 50}); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(path, 0o444); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(dir, 0o555); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(dir, 0o755) })
	var out, errOut bytes.Buffer
	_, err := Run(context.Background(), Options{
		RepoDir: dir, BaselinePath: path, Update: true, Stdout: &out, Stderr: &errOut,
		CommandRunner: scriptedGoTest("ok  \tDendrite/pkg/a\t0.1s\tcoverage: 60.0% of statements\n"),
	})
	if err == nil {
		t.Fatal("expected an error when the baseline cannot be written")
	}
}

func TestRun_ExistingPackageDropReported(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "go.mod"), []byte("module Dendrite\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dir, "baseline.json")
	if err := SaveBaseline(path, map[string]float64{"pkg/a": 90}); err != nil {
		t.Fatal(err)
	}
	var out, errOut bytes.Buffer
	res, err := Run(context.Background(), Options{
		RepoDir: dir, BaselinePath: path, Ratchet: true, Stdout: &out, Stderr: &errOut,
		CommandRunner: scriptedGoTest("ok  \tDendrite/pkg/a\t0.1s\tcoverage: 80.0% of statements\n"),
	})
	if err != nil || res.Passed {
		t.Fatalf("expected ratchet failure, got res=%v err=%v", res, err)
	}
	if !bytes.Contains(errOut.Bytes(), []byte("pkg/a: 80.0% < 90.0% (dropped by 10.0%)")) {
		t.Fatalf("expected drop report, got: %s", errOut.String())
	}
}
