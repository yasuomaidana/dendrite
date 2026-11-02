package assert

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"Dendrite/pkg/coreclient"
)

// CheckComplexity checks whether any function exceeds the cyclomatic complexity threshold.
func CheckComplexity(funcs []coreclient.FunctionMetrics, threshold int) (Verdict, []string) {
	if threshold <= 0 {
		threshold = DefaultMaxCyclomaticGate
	}

	var reasons []string
	for _, f := range funcs {
		if f.Cyclomatic > threshold {
			reason := fmt.Sprintf("function %s has cyclomatic complexity %d exceeding gate of %d", f.Symbol, f.Cyclomatic, threshold)
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
	funcs, err := coreclient.Default().Complexity(context.Background(), "go", source)
	if err != nil {
		return "", nil, nil, fmt.Errorf("analyzing source complexity: %w", err)
	}

	cycloMap := make(map[string]int, len(funcs))
	for _, f := range funcs {
		cycloMap[f.Symbol] = f.Cyclomatic
	}

	verdict, reasons := CheckComplexity(funcs, threshold)
	return verdict, cycloMap, reasons, nil
}

// CheckPackageComplexity parses all non-test Go files in pkgDir and evaluates cyclomatic complexity.
func CheckPackageComplexity(pkgDir string, threshold int) (Verdict, map[string]int, []string, error) {
	entries, err := os.ReadDir(pkgDir)
	if err != nil {
		return "", nil, nil, fmt.Errorf("reading package directory %s: %w", pkgDir, err)
	}

	client := coreclient.Default()
	ctx := context.Background()
	var allFuncs []coreclient.FunctionMetrics

	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".go") || strings.HasSuffix(entry.Name(), "_test.go") {
			continue
		}
		filePath := filepath.Join(pkgDir, entry.Name())
		data, err := os.ReadFile(filePath)
		if err != nil {
			return "", nil, nil, fmt.Errorf("reading file %s: %w", filePath, err)
		}
		funcs, err := client.Complexity(ctx, "go", string(data))
		if err != nil {
			return "", nil, nil, fmt.Errorf("analyzing file %s: %w", filePath, err)
		}
		allFuncs = append(allFuncs, funcs...)
	}

	cycloMap := make(map[string]int, len(allFuncs))
	for _, f := range allFuncs {
		cycloMap[f.Symbol] = f.Cyclomatic
	}

	verdict, reasons := CheckComplexity(allFuncs, threshold)
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
