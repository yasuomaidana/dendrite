package coverage

import (
	"bytes"
	"context"
	"errors"
	"math"
	"os"
	"path/filepath"
	"testing"
)

func TestLoadBaseline(t *testing.T) {
	tmpDir := t.TempDir()

	// Missing file
	_, err := LoadBaseline(filepath.Join(tmpDir, "missing.json"))
	if err == nil {
		t.Fatal("expected error for missing baseline file")
	}

	// Direct map format
	directPath := filepath.Join(tmpDir, "direct.json")
	if err := os.WriteFile(directPath, []byte(`{"cmd/dendrite": 1.6, "pkg/quality": 100.0}`), 0644); err != nil {
		t.Fatal(err)
	}
	base, err := LoadBaseline(directPath)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if base["cmd/dendrite"] != 1.6 || base["pkg/quality"] != 100.0 {
		t.Fatalf("unexpected baseline content: %v", base)
	}

	// Structured format
	structPath := filepath.Join(tmpDir, "struct.json")
	if err := os.WriteFile(structPath, []byte(`{"packages": {"cmd/dendrite": 1.6}}`), 0644); err != nil {
		t.Fatal(err)
	}
	baseStruct, err := LoadBaseline(structPath)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if baseStruct["cmd/dendrite"] != 1.6 {
		t.Fatalf("unexpected structured baseline: %v", baseStruct)
	}

	// Empty direct map
	emptyPath := filepath.Join(tmpDir, "empty.json")
	if err := os.WriteFile(emptyPath, []byte(`{}`), 0644); err != nil {
		t.Fatal(err)
	}
	baseEmpty, err := LoadBaseline(emptyPath)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(baseEmpty) != 0 {
		t.Fatalf("expected empty baseline, got: %v", baseEmpty)
	}

	// Invalid JSON
	badPath := filepath.Join(tmpDir, "bad.json")
	if err := os.WriteFile(badPath, []byte(`{ invalid-json }`), 0644); err == nil {
		_, badErr := LoadBaseline(badPath)
		if badErr == nil {
			t.Fatal("expected error for bad JSON")
		}
	}
}

func TestSaveBaseline(t *testing.T) {
	tmpDir := t.TempDir()

	// Normal save with directory creation
	targetPath := filepath.Join(tmpDir, "sub", "dir", "baseline.json")
	data := map[string]float64{
		"cmd/dendrite": 1.6,
		"pkg/quality":  100.0,
	}
	if err := SaveBaseline(targetPath, data); err != nil {
		t.Fatalf("SaveBaseline failed: %v", err)
	}

	loaded, err := LoadBaseline(targetPath)
	if err != nil {
		t.Fatalf("LoadBaseline failed on saved file: %v", err)
	}
	if loaded["cmd/dendrite"] != 1.6 || loaded["pkg/quality"] != 100.0 {
		t.Fatalf("unexpected loaded data: %v", loaded)
	}

	// Save with dir = "." or ""
	localPath := filepath.Join(tmpDir, "local.json")
	if err := SaveBaseline(localPath, data); err != nil {
		t.Fatalf("SaveBaseline in dir failed: %v", err)
	}

	// Fail directory creation when a regular file blocks the path
	blockerFile := filepath.Join(tmpDir, "blocker")
	if err := os.WriteFile(blockerFile, []byte("data"), 0644); err != nil {
		t.Fatal(err)
	}
	impossiblePath := filepath.Join(blockerFile, "child", "baseline.json")
	if err := SaveBaseline(impossiblePath, data); err == nil {
		t.Fatal("expected error when directory creation is blocked by a file")
	}

	// Fail JSON marshal with NaN
	badData := map[string]float64{"bad": math.NaN()}
	if err := SaveBaseline(filepath.Join(tmpDir, "nan.json"), badData); err == nil {
		t.Fatal("expected error for NaN JSON marshal")
	}

	// Fail write file when target is an existing directory
	dirAsTarget := filepath.Join(tmpDir, "existing_dir")
	if err := os.Mkdir(dirAsTarget, 0755); err != nil {
		t.Fatal(err)
	}
	if err := SaveBaseline(dirAsTarget, data); err == nil {
		t.Fatal("expected error writing to directory path")
	}
}

func TestDetectModulePath(t *testing.T) {
	tmpDir := t.TempDir()

	// Missing go.mod
	if p := DetectModulePath(tmpDir); p != "" {
		t.Fatalf("expected empty for missing go.mod, got %s", p)
	}

	// Valid go.mod
	if err := os.WriteFile(filepath.Join(tmpDir, "go.mod"), []byte("module Dendrite\n\ngo 1.25\n"), 0644); err != nil {
		t.Fatal(err)
	}
	if p := DetectModulePath(tmpDir); p != "Dendrite" {
		t.Fatalf("expected Dendrite, got %s", p)
	}

	// go.mod without module declaration
	if err := os.WriteFile(filepath.Join(tmpDir, "go.mod"), []byte("// comment only\n"), 0644); err != nil {
		t.Fatal(err)
	}
	if p := DetectModulePath(tmpDir); p != "" {
		t.Fatalf("expected empty, got %s", p)
	}
}

func TestNormalizePackage(t *testing.T) {
	if p := normalizePackage("pkg/foo", ""); p != "pkg/foo" {
		t.Fatalf("expected pkg/foo, got %s", p)
	}
	if p := normalizePackage("Dendrite", "Dendrite"); p != "." {
		t.Fatalf("expected ., got %s", p)
	}
	if p := normalizePackage("Dendrite/pkg/foo", "Dendrite"); p != "pkg/foo" {
		t.Fatalf("expected pkg/foo, got %s", p)
	}
	if p := normalizePackage("other/pkg/foo", "Dendrite"); p != "other/pkg/foo" {
		t.Fatalf("expected other/pkg/foo, got %s", p)
	}
}

func TestRoundPct(t *testing.T) {
	if r := roundPct(85.24); r != 85.2 {
		t.Fatalf("expected 85.2, got %v", r)
	}
	if r := roundPct(85.26); r != 85.3 {
		t.Fatalf("expected 85.3, got %v", r)
	}
}

func TestParseCoverageOutput(t *testing.T) {
	output := `
=== RUN   TestSample
--- PASS: TestSample (0.00s)
PASS
ok  	Dendrite/cmd/dendrite	0.123s	coverage: 1.6% of statements
ok  	Dendrite/pkg/quality	(cached)	coverage: 85.2% of statements
ok  	Dendrite/pkg/empty	0.005s	coverage: [no statements]
?   	Dendrite/pkg/notest	[no test files]
	Dendrite/pkg/untested		coverage: 0.0% of statements
FAIL	Dendrite/pkg/broken	0.015s
FAIL: TestFailure (0.01s)
--- FAIL: TestInner (0.00s)
`
	res, failed := ParseCoverageOutput(output, "Dendrite")

	if res["cmd/dendrite"].CoveragePct != 1.6 {
		t.Fatalf("expected 1.6%% for cmd/dendrite, got %v", res["cmd/dendrite"])
	}
	if res["pkg/quality"].CoveragePct != 85.2 {
		t.Fatalf("expected 85.2%% for pkg/quality, got %v", res["pkg/quality"])
	}
	if !res["pkg/empty"].NoStatements || res["pkg/empty"].CoveragePct != 100.0 {
		t.Fatalf("expected 100%% no statements, got %v", res["pkg/empty"])
	}
	if !res["pkg/notest"].NoTests || res["pkg/notest"].CoveragePct != 0.0 {
		t.Fatalf("expected 0%% no tests, got %v", res["pkg/notest"])
	}
	if !res["pkg/untested"].NoTests || res["pkg/untested"].CoveragePct != 0.0 {
		t.Fatalf("expected Go 1.22+ untested package at 0%%, got %v", res["pkg/untested"])
	}
	if len(failed) != 1 || failed[0] != "pkg/broken" {
		t.Fatalf("expected pkg/broken in failed, got %v", failed)
	}
}

func TestCalculateProgress(t *testing.T) {
	// Empty map
	at100, total, avg := CalculateProgress(map[string]PackageCoverage{})
	if at100 != 0 || total != 0 || avg != 0.0 {
		t.Fatalf("expected zeros, got %d, %d, %v", at100, total, avg)
	}

	covs := map[string]PackageCoverage{
		"pkg/a": {CoveragePct: 100.0},
		"pkg/b": {CoveragePct: 80.0},
		"pkg/c": {CoveragePct: 90.0},
	}
	at100, total, avg = CalculateProgress(covs)
	if at100 != 1 {
		t.Fatalf("expected 1 package at 100%%, got %d", at100)
	}
	if total != 3 {
		t.Fatalf("expected 3 packages total, got %d", total)
	}
	if avg != 90.0 {
		t.Fatalf("expected average 90.0%%, got %v", avg)
	}
}

func TestCompareAndRatchet(t *testing.T) {
	// Case 1: Initial creation (baselineExists == false)
	current := map[string]PackageCoverage{
		"cmd/dendrite": {CoveragePct: 1.6},
		"pkg/quality":  {CoveragePct: 100.0},
	}
	offenders, newBase := CompareAndRatchet(current, nil, false, false)
	if len(offenders) != 0 {
		t.Fatalf("expected no offenders on initial creation, got %v", offenders)
	}
	if newBase["cmd/dendrite"] != 1.6 || newBase["pkg/quality"] != 100.0 {
		t.Fatalf("unexpected new baseline: %v", newBase)
	}

	// Case 2: Baseline exists, test regressions, improvements, 100% clamping, and new packages
	baseline := map[string]float64{
		"pkg/stable":      80.0,
		"pkg/perfect":     100.0,
		"pkg/overflow":    105.0, // Should be clamped to 100.0
		"pkg/regressed":   90.0,
		"pkg/disappeared": 50.0, // Not in current, should be preserved
	}

	current2 := map[string]PackageCoverage{
		"pkg/stable":    {CoveragePct: 85.0}, // Improvement
		"pkg/perfect":   {CoveragePct: 95.0}, // Regression from 100%
		"pkg/overflow":  {CoveragePct: 100.0},
		"pkg/regressed": {CoveragePct: 80.0},  // Regression
		"pkg/new_bad":   {CoveragePct: 80.0},  // New package, must start at 100%
		"pkg/new_good":  {CoveragePct: 100.0}, // New package at 100%
	}

	// Subcase 2A: update == false
	offenders2, newBase2 := CompareAndRatchet(current2, baseline, true, false)
	// Offenders expected: pkg/perfect (95 < 100), pkg/regressed (80 < 90), pkg/new_bad (80 < 100)
	if len(offenders2) != 3 {
		t.Fatalf("expected 3 offenders without update, got %d: %v", len(offenders2), offenders2)
	}
	if newBase2["pkg/stable"] != 80.0 {
		t.Fatalf("expected pkg/stable to remain 80.0 when update is false, got %v", newBase2["pkg/stable"])
	}
	if newBase2["pkg/disappeared"] != 50.0 {
		t.Fatalf("expected pkg/disappeared preserved at 50.0, got %v", newBase2["pkg/disappeared"])
	}
	if newBase2["pkg/overflow"] != 100.0 {
		t.Fatalf("expected pkg/overflow clamped to 100.0, got %v", newBase2["pkg/overflow"])
	}

	// Subcase 2B: update == true
	offenders3, newBase3 := CompareAndRatchet(current2, baseline, true, true)
	if len(offenders3) != 3 {
		t.Fatalf("expected 3 offenders with update, got %d: %v", len(offenders3), offenders3)
	}
	// pkg/stable improved from 80.0 to 85.0 -> raised!
	if newBase3["pkg/stable"] != 85.0 {
		t.Fatalf("expected pkg/stable raised to 85.0, got %v", newBase3["pkg/stable"])
	}
	// pkg/perfect dropped from 100% to 95% -> must stay at 100%!
	if newBase3["pkg/perfect"] != 100.0 {
		t.Fatalf("expected pkg/perfect to stay at 100.0, got %v", newBase3["pkg/perfect"])
	}
	// pkg/regressed dropped from 90 to 80 -> baseline never lowered!
	if newBase3["pkg/regressed"] != 90.0 {
		t.Fatalf("expected pkg/regressed baseline to stay 90.0, got %v", newBase3["pkg/regressed"])
	}
	// pkg/new_bad -> new packages must start at 100%
	if newBase3["pkg/new_bad"] != 100.0 {
		t.Fatalf("expected pkg/new_bad to start at 100.0, got %v", newBase3["pkg/new_bad"])
	}
	// pkg/new_good -> starts at 100%
	if newBase3["pkg/new_good"] != 100.0 {
		t.Fatalf("expected pkg/new_good at 100.0, got %v", newBase3["pkg/new_good"])
	}
}

func TestDefaultCommandRunner(t *testing.T) {
	ctx := context.Background()

	// Empty dir
	out, err := DefaultCommandRunner(ctx, "", "go", "version")
	if err != nil {
		t.Fatalf("command failed: %v", err)
	}
	if !bytes.Contains(out, []byte("go version")) {
		t.Fatalf("unexpected output: %s", string(out))
	}

	// Non-empty dir
	tmpDir := t.TempDir()
	out2, err := DefaultCommandRunner(ctx, tmpDir, "go", "version")
	if err != nil {
		t.Fatalf("command with dir failed: %v", err)
	}
	if !bytes.Contains(out2, []byte("go version")) {
		t.Fatalf("unexpected output with dir: %s", string(out2))
	}
}

func TestRun_DefaultOptions(t *testing.T) {
	tmpDir := t.TempDir()
	calledArgs := false

	runner := func(ctx context.Context, dir string, name string, args ...string) ([]byte, error) {
		calledArgs = true
		return []byte("ok  \tpkg/a\t0.01s\tcoverage: 100.0% of statements\n"), nil
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
		t.Fatalf("Run failed: %v", err)
	}
	if !calledArgs {
		t.Fatal("runner was not invoked")
	}
	if !res.Passed {
		t.Fatal("expected run to pass")
	}
}

func TestRun_MissingBaselineRatchetNoUpdate(t *testing.T) {
	tmpDir := t.TempDir()
	var stdout, stderr bytes.Buffer

	res, err := Run(context.Background(), Options{
		RepoDir:      tmpDir,
		BaselinePath: filepath.Join(tmpDir, "missing.json"),
		Ratchet:      true,
		Update:       false,
		Stdout:       &stdout,
		Stderr:       &stderr,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if res.Passed {
		t.Fatal("expected failure when baseline is missing in ratchet mode without update")
	}
	if !bytes.Contains(stderr.Bytes(), []byte("Coverage baseline not found")) {
		t.Fatalf("expected error message in stderr, got: %s", stderr.String())
	}
}

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
