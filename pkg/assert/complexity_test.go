package assert

import (
	"Dendrite/pkg/coreclient/coretest"
	"os"
	"path/filepath"
	"testing"

	"Dendrite/pkg/coreclient"
	"Dendrite/pkg/testkit"
)

func TestMain(m *testing.M) { coretest.Main(m) }

const highComplexitySource = `package fixture

func ExceededFunc(x int) int {
	return 1
}
`

const boundaryComplexitySource = `package fixture

func BoundaryFunc(x int) int {
	return 1
}
`

func TestCheckSourceComplexity_ExceededGate(t *testing.T) {
	verdict, cycloMap, reasons, err := CheckSourceComplexity(highComplexitySource, 10)
	testkit.Check(t, err)

	testkit.Equal(t, "verdict", verdict, ComplexityExceeded)
	testkit.Equal(t, "cyclo", cycloMap["ExceededFunc"], 11)

	if len(reasons) == 0 {
		t.Fatalf("expected failure reasons, got none")
	}
}

func TestCheckSourceComplexity_BoundaryAndSafe(t *testing.T) {
	verdict, cycloMap, reasons, err := CheckSourceComplexity(boundaryComplexitySource, 10)
	testkit.Check(t, err)

	testkit.Equal(t, "verdict", verdict, VerifiedComplete)
	testkit.Equal(t, "cyclo", cycloMap["BoundaryFunc"], 10)
	if len(reasons) != 0 {
		t.Fatalf("expected 0 reasons, got %v", reasons)
	}
}

func TestCheckPackageComplexity(t *testing.T) {
	dir := t.TempDir()
	sourcePath := filepath.Join(dir, "service.go")
	testkit.WriteFile(t, sourcePath, highComplexitySource, 0644)
	// Subdirectories and _test.go files must be skipped
	testkit.WriteFile(t, filepath.Join(dir, "service_test.go"), highComplexitySource, 0644)
	testkit.WriteFile(t, filepath.Join(dir, "notes.txt"), "some notes", 0644)
	subDir := filepath.Join(dir, "nested")
	if err := os.MkdirAll(subDir, 0755); err != nil {
		t.Fatal(err)
	}

	verdict, cycloMap, _, err := CheckPackageComplexity(dir, 10)
	testkit.Check(t, err)

	testkit.Equal(t, "verdict", verdict, ComplexityExceeded)
	testkit.Equal(t, "cyclo", cycloMap["ExceededFunc"], 11)
}

func TestCheckPackageComplexity_ReadError(t *testing.T) {
	dir := t.TempDir()
	filePath := filepath.Join(dir, "unreadable.go")
	testkit.WriteFile(t, filePath, "content", 0000)
	t.Cleanup(func() { _ = os.Chmod(filePath, 0644) })

	// If unreadable file cannot be opened by ReadFile
	if _, err := os.ReadFile(filePath); err != nil {
		_, _, _, perr := CheckPackageComplexity(dir, 10)
		if perr == nil {
			t.Fatal("expected error reading unreadable file")
		}
	}
}

func TestCheckPackageComplexity_AnalyzeError(t *testing.T) {
	dir := t.TempDir()
	filePath := filepath.Join(dir, "broken.go")
	testkit.WriteFile(t, filePath, "func broken(", 0644)

	_, _, _, err := CheckPackageComplexity(dir, 10)
	if err == nil {
		t.Fatal("expected error analyzing broken syntax in package file")
	}
}

func TestEvaluateComplexityMap(t *testing.T) {
	exceededMap := map[string]int{
		"Simple":   2,
		"TooBig":   11,
		"Moderate": 6,
	}
	verdict, reasons := EvaluateComplexityMap(exceededMap, 10)
	testkit.Equal(t, "verdict", verdict, ComplexityExceeded)
	testkit.Equal(t, "reasons length", len(reasons), 1)

	safeMap := map[string]int{
		"Simple":   2,
		"Boundary": 10,
	}
	safeVerdict, safeReasons := EvaluateComplexityMap(safeMap, 10)
	testkit.Equal(t, "safe verdict", safeVerdict, VerifiedComplete)
	testkit.Equal(t, "safe reasons length", len(safeReasons), 0)
}

func TestCheckSourceComplexity_SyntaxError(t *testing.T) {
	badSource := `package fixture func broken(`
	_, _, _, err := CheckSourceComplexity(badSource, 10)
	if err == nil {
		t.Fatalf("expected error on invalid Go source syntax")
	}
}

func TestCheckComplexity_Branches(t *testing.T) {
	// Case 1: funcs empty, threshold <= 0 falls back to DefaultMaxCyclomaticGate (10)
	verdict, reasons := CheckComplexity(nil, 0)
	testkit.Equal(t, "verdict", verdict, VerifiedComplete)
	testkit.Equal(t, "reasons length", len(reasons), 0)

	// Case 2: func exceeds default threshold (10) when threshold <= 0
	funcsExceed := []coreclient.FunctionMetrics{
		{Symbol: "HighFunc", Cyclomatic: 15},
		{Symbol: "SafeFunc", Cyclomatic: 2},
	}
	verdictExceed, reasonsExceed := CheckComplexity(funcsExceed, -1)
	testkit.Equal(t, "verdict exceed", verdictExceed, ComplexityExceeded)
	testkit.Equal(t, "reasons exceed length", len(reasonsExceed), 1)

	// Case 3: threshold > 0 safe
	funcsSafe := []coreclient.FunctionMetrics{
		{Symbol: "SafeFunc", Cyclomatic: 4},
	}
	verdictSafe, reasonsSafe := CheckComplexity(funcsSafe, 5)
	testkit.Equal(t, "verdict safe", verdictSafe, VerifiedComplete)
	testkit.Equal(t, "reasons safe length", len(reasonsSafe), 0)
}

func TestCheckPackageComplexity_DirError(t *testing.T) {
	nonExistentDir := filepath.Join(t.TempDir(), "nonexistent")
	_, _, _, err := CheckPackageComplexity(nonExistentDir, 10)
	if err == nil {
		t.Fatalf("expected error for non-existent package directory")
	}
}

func TestEvaluateComplexityMap_DefaultThreshold(t *testing.T) {
	exceededMap := map[string]int{
		"High": 12,
		"Low":  3,
	}
	verdict, reasons := EvaluateComplexityMap(exceededMap, 0)
	testkit.Equal(t, "verdict", verdict, ComplexityExceeded)
	testkit.Equal(t, "reasons length", len(reasons), 1)

	safeMap := map[string]int{
		"Low": 5,
	}
	verdictSafe, reasonsSafe := EvaluateComplexityMap(safeMap, -1)
	testkit.Equal(t, "safe verdict", verdictSafe, VerifiedComplete)
	testkit.Equal(t, "safe reasons length", len(reasonsSafe), 0)
}
