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
	paths, files, err := readGoSources(pkgDir)
	if err != nil {
		return "", nil, nil, err
	}
	allFuncs, err := measurePackage(pkgDir, paths, files)
	if err != nil {
		return "", nil, nil, err
	}

	cycloMap := make(map[string]int, len(allFuncs))
	for _, f := range allFuncs {
		cycloMap[f.Symbol] = f.Cyclomatic
	}

	verdict, reasons := CheckComplexity(allFuncs, threshold)
	return verdict, cycloMap, reasons, nil
}

// readGoSources reads the non-test Go files of pkgDir: their paths and, in the same order, their sources.
func readGoSources(pkgDir string) ([]string, []coreclient.SourceFile, error) {
	entries, err := os.ReadDir(pkgDir)
	if err != nil {
		return nil, nil, fmt.Errorf("reading package directory %s: %w", pkgDir, err)
	}
	var paths []string
	var files []coreclient.SourceFile
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".go") || strings.HasSuffix(entry.Name(), "_test.go") {
			continue
		}
		filePath := filepath.Join(pkgDir, entry.Name())
		data, err := os.ReadFile(filePath)
		if err != nil {
			return nil, nil, fmt.Errorf("reading file %s: %w", filePath, err)
		}
		paths = append(paths, filePath)
		files = append(files, coreclient.SourceFile{Lang: "go", Source: string(data)})
	}
	return paths, files, nil
}

// measurePackage sends the files as one request, so dendrite-core measures them on all its cores, and returns
// every function; a file the sidecar rejects fails the check under its path.
func measurePackage(pkgDir string, paths []string, files []coreclient.SourceFile) ([]coreclient.FunctionMetrics, error) {
	results, err := coreclient.Default().ComplexityBatch(context.Background(), files)
	if err != nil {
		return nil, fmt.Errorf("analyzing package %s: %w", pkgDir, err)
	}
	var funcs []coreclient.FunctionMetrics
	for i, r := range results {
		if r.Error != "" {
			return nil, fmt.Errorf("analyzing file %s: %s", paths[i], r.Error)
		}
		funcs = append(funcs, r.Functions...)
	}
	return funcs, nil
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
