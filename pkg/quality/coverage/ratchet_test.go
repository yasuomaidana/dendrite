package coverage

import (
	"bytes"
	"context"
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
