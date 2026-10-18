package assert

import (
	"fmt"
	"sort"

	"Dendrite/pkg/ast"
)

// CheckComplexity checks whether any function exceeds the cyclomatic complexity threshold.
func CheckComplexity(funcs []ast.FunctionComplexity, threshold int) (Verdict, []string) {
	if threshold <= 0 {
		threshold = DefaultMaxCyclomaticGate
	}

	var reasons []string
	for _, f := range funcs {
		if f.Complexity > threshold {
			reason := fmt.Sprintf("function %s has cyclomatic complexity %d exceeding gate of %d", f.Name, f.Complexity, threshold)
			reasons = append(reasons, reason)
		}
	}

	if len(reasons) > 0 {
		sort.Strings(reasons)
		return ComplexityExceeded, reasons
	}

	return VerifiedComplete, nil
}

// CheckSourceComplexity parses Go source code and evaluates function cyclomatic complexity against threshold.
func CheckSourceComplexity(source string, threshold int) (Verdict, map[string]int, []string, error) {
	funcs, err := ast.AnalyzeFuncComplexity(source)
	if err != nil {
		return "", nil, nil, fmt.Errorf("analyzing source complexity: %w", err)
	}

	cycloMap := make(map[string]int, len(funcs))
	for _, f := range funcs {
		cycloMap[f.Name] = f.Complexity
	}

	verdict, reasons := CheckComplexity(funcs, threshold)
	return verdict, cycloMap, reasons, nil
}

// CheckPackageComplexity parses all non-test Go files in pkgDir and evaluates cyclomatic complexity.
func CheckPackageComplexity(pkgDir string, threshold int) (Verdict, map[string]int, []string, error) {
	funcs, err := ast.AnalyzePackageComplexity(pkgDir)
	if err != nil {
		return "", nil, nil, fmt.Errorf("analyzing package complexity: %w", err)
	}

	cycloMap := make(map[string]int, len(funcs))
	for _, f := range funcs {
		cycloMap[f.Name] = f.Complexity
	}

	verdict, reasons := CheckComplexity(funcs, threshold)
	return verdict, cycloMap, reasons, nil
}

// EvaluateComplexityMap evaluates an existing function-to-complexity map against the threshold.
func EvaluateComplexityMap(maxCyclo map[string]int, threshold int) (Verdict, []string) {
	if threshold <= 0 {
		threshold = DefaultMaxCyclomaticGate
	}

	var reasons []string
	for fnName, cyclo := range maxCyclo {
		if cyclo > threshold {
			reason := fmt.Sprintf("function %s has cyclomatic complexity %d exceeding gate of %d", fnName, cyclo, threshold)
			reasons = append(reasons, reason)
		}
	}

	if len(reasons) > 0 {
		sort.Strings(reasons)
		return ComplexityExceeded, reasons
	}

	return VerifiedComplete, nil
}
