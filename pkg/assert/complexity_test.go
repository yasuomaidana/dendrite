package assert

import (
	"os"
	"path/filepath"
	"testing"

	"Dendrite/pkg/ast"
)

const highComplexitySource = `package fixture

func ExceededFunc(x int) int {
	if x == 1 {
		return 1
	}
	if x == 2 {
		return 2
	}
	if x == 3 {
		return 3
	}
	if x == 4 {
		return 4
	}
	if x == 5 {
		return 5
	}
	if x == 6 {
		return 6
	}
	if x == 7 {
		return 7
	}
	if x == 8 {
		return 8
	}
	if x == 9 {
		return 9
	}
	if x == 10 {
		return 10
	}
	return 0
}
`

const boundaryComplexitySource = `package fixture

func BoundaryFunc(x int) int {
	if x == 1 {
		return 1
	}
	if x == 2 {
		return 2
	}
	if x == 3 {
		return 3
	}
	if x == 4 {
		return 4
	}
	if x == 5 {
		return 5
	}
	if x == 6 {
		return 6
	}
	if x == 7 {
		return 7
	}
	if x == 8 {
		return 8
	}
	if x == 9 {
		return 9
	}
	return 0
}
`

func TestCheckSourceComplexity_ExceededGate(t *testing.T) {
	verdict, cycloMap, reasons, err := CheckSourceComplexity(highComplexitySource, 10)
	if err != nil {
		t.Fatalf("unexpected error analyzing complexity: %v", err)
	}

	if verdict != ComplexityExceeded {
		t.Fatalf("expected verdict %s, got %s", ComplexityExceeded, verdict)
	}

	if cycloMap["ExceededFunc"] != 11 {
		t.Fatalf("expected cyclomatic complexity 11 for ExceededFunc, got %d", cycloMap["ExceededFunc"])
	}

	if len(reasons) == 0 {
		t.Fatalf("expected failure reasons, got none")
	}
}

func TestCheckSourceComplexity_BoundaryAndSafe(t *testing.T) {
	// Boundary: 9 branches + 1 baseline = 10 (gate is <= 10)
	verdict, cycloMap, reasons, err := CheckSourceComplexity(boundaryComplexitySource, 10)
	if err != nil {
		t.Fatalf("unexpected error analyzing complexity: %v", err)
	}

	if verdict != VerifiedComplete {
		t.Fatalf("expected verdict %s for complexity 10, got %s (%v)", VerifiedComplete, verdict, reasons)
	}

	if cycloMap["BoundaryFunc"] != 10 {
		t.Fatalf("expected complexity 10 for BoundaryFunc, got %d", cycloMap["BoundaryFunc"])
	}
}

func TestCheckPackageComplexity(t *testing.T) {
	dir := t.TempDir()
	sourcePath := filepath.Join(dir, "service.go")
	if err := os.WriteFile(sourcePath, []byte(highComplexitySource), 0644); err != nil {
		t.Fatalf("failed writing package file: %v", err)
	}

	verdict, cycloMap, _, err := CheckPackageComplexity(dir, 10)
	if err != nil {
		t.Fatalf("unexpected error analyzing package: %v", err)
	}
	if verdict != ComplexityExceeded {
		t.Fatalf("expected ComplexityExceeded for package, got: %s", verdict)
	}
	if cycloMap["ExceededFunc"] != 11 {
		t.Fatalf("expected ExceededFunc in map with complexity 11, got: %d", cycloMap["ExceededFunc"])
	}
}

func TestEvaluateComplexityMap(t *testing.T) {
	// Exceeded
	exceededMap := map[string]int{
		"Simple":   2,
		"TooBig":   11,
		"Moderate": 6,
	}
	verdict, reasons := EvaluateComplexityMap(exceededMap, 10)
	if verdict != ComplexityExceeded || len(reasons) != 1 {
		t.Fatalf("expected ComplexityExceeded with 1 reason, got %s (%v)", verdict, reasons)
	}

	// Safe
	safeMap := map[string]int{
		"Simple":   2,
		"Boundary": 10,
	}
	safeVerdict, safeReasons := EvaluateComplexityMap(safeMap, 10)
	if safeVerdict != VerifiedComplete || len(safeReasons) != 0 {
		t.Fatalf("expected VerifiedComplete for safe map, got %s (%v)", safeVerdict, safeReasons)
	}
}

func TestCheckSourceComplexity_SyntaxError(t *testing.T) {
	badSource := `package fixture func broken(`
	_, _, _, err := CheckSourceComplexity(badSource, 10)
	if err == nil {
		t.Fatalf("expected error on invalid Go source syntax")
	}
}

func TestCheckComplexity_Branches(t *testing.T) {
	// threshold <= 0 falls back to DefaultMaxCyclomaticGate (10)
	// Case 1: funcs empty, threshold 0
	verdict, reasons := CheckComplexity(nil, 0)
	if verdict != VerifiedComplete || len(reasons) != 0 {
		t.Fatalf("expected VerifiedComplete for nil funcs with threshold 0, got %s (%v)", verdict, reasons)
	}

	// Case 2: func exceeds default threshold (10) when threshold <= 0
	funcsExceed := []ast.FunctionComplexity{
		{Name: "HighFunc", Complexity: 15},
		{Name: "SafeFunc", Complexity: 2},
	}
	verdictExceed, reasonsExceed := CheckComplexity(funcsExceed, -1)
	if verdictExceed != ComplexityExceeded || len(reasonsExceed) != 1 {
		t.Fatalf("expected ComplexityExceeded with 1 reason for threshold <= 0, got %s (%v)", verdictExceed, reasonsExceed)
	}

	// Case 3: threshold > 0 safe
	funcsSafe := []ast.FunctionComplexity{
		{Name: "SafeFunc", Complexity: 4},
	}
	verdictSafe, reasonsSafe := CheckComplexity(funcsSafe, 5)
	if verdictSafe != VerifiedComplete || len(reasonsSafe) != 0 {
		t.Fatalf("expected VerifiedComplete with threshold 5, got %s (%v)", verdictSafe, reasonsSafe)
	}
}

func TestCheckPackageComplexity_Error(t *testing.T) {
	nonExistentDir := filepath.Join(t.TempDir(), "nonexistent")
	_, _, _, err := CheckPackageComplexity(nonExistentDir, 10)
	if err == nil {
		t.Fatalf("expected error for non-existent package directory")
	}
}

func TestEvaluateComplexityMap_DefaultThreshold(t *testing.T) {
	// threshold <= 0 fallback to DefaultMaxCyclomaticGate (10)
	exceededMap := map[string]int{
		"High": 12,
		"Low":  3,
	}
	verdict, reasons := EvaluateComplexityMap(exceededMap, 0)
	if verdict != ComplexityExceeded || len(reasons) != 1 {
		t.Fatalf("expected ComplexityExceeded with threshold 0, got %s (%v)", verdict, reasons)
	}

	safeMap := map[string]int{
		"Low": 5,
	}
	verdictSafe, reasonsSafe := EvaluateComplexityMap(safeMap, -1)
	if verdictSafe != VerifiedComplete || len(reasonsSafe) != 0 {
		t.Fatalf("expected VerifiedComplete with threshold -1, got %s (%v)", verdictSafe, reasonsSafe)
	}
}
