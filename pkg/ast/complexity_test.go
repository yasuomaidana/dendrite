package ast

import (
	"os"
	"path/filepath"
	"testing"
)

func TestCyclomaticComplexity_Constructs(t *testing.T) {
	source := `package fixture

type Engine struct{}

func (e *Engine) PointerMethod(a, b bool) int {
	if a && b {
		return 1
	}
	return 0
}

func (e Engine) ValueMethod(items []int) int {
	count := 0
	for _, it := range items {
		if it > 0 || it < -10 {
			count++
		}
	}
	return count
}

func SwitchAndSelect(ch <-chan int, code int) int {
	res := 0
	switch code {
	case 1:
		res = 10
	case 2:
		res = 20
	default:
		res = 0
	}

	for i := 0; i < 5; i++ {
		res += i
	}

	select {
	case val := <-ch:
		res += val
	default:
		res += 99
	}

	return res
}
`

	funcs, err := AnalyzeFuncComplexity(source)
	if err != nil {
		t.Fatalf("unexpected error analyzing funcs: %v", err)
	}

	if len(funcs) != 3 {
		t.Fatalf("expected 3 functions, got %d", len(funcs))
	}

	funcMap := make(map[string]int)
	for _, f := range funcs {
		funcMap[f.Name] = f.Complexity
	}

	// PointerMethod:
	// 1 (base) + 1 (if) + 1 (&&) = 3
	if funcMap["(*Engine).PointerMethod"] != 3 {
		t.Fatalf("expected (*Engine).PointerMethod complexity 3, got %d", funcMap["(*Engine).PointerMethod"])
	}

	// ValueMethod:
	// 1 (base) + 1 (range) + 1 (if) + 1 (||) = 4
	if funcMap["(Engine).ValueMethod"] != 4 {
		t.Fatalf("expected (Engine).ValueMethod complexity 4, got %d", funcMap["(Engine).ValueMethod"])
	}

	// SwitchAndSelect:
	// 1 (base) + 2 (case 1, case 2, default is 0) + 1 (for loop) + 1 (select case, default is 0) = 5
	if funcMap["SwitchAndSelect"] != 5 {
		t.Fatalf("expected SwitchAndSelect complexity 5, got %d", funcMap["SwitchAndSelect"])
	}

	if MaxComplexity(funcs) != 5 {
		t.Fatalf("expected MaxComplexity 5, got %d", MaxComplexity(funcs))
	}
}

func TestCyclomaticComplexity_NilSafeguards(t *testing.T) {
	if CyclomaticComplexity(nil) != 1 {
		t.Fatalf("expected 1 for nil FuncDecl")
	}
	if nodeComplexity(nil) != 1 {
		t.Fatalf("expected 1 for nil node")
	}
	if MaxComplexity(nil) != 0 {
		t.Fatalf("expected 0 for empty slice")
	}
}

func TestAnalyzeFileAndPackageComplexity(t *testing.T) {
	dir := t.TempDir()
	sourceA := `package demo

func WorkA() int {
	if true {
		return 1
	}
	return 0
}
`
	sourceB := `package demo

func WorkB() int {
	for i := 0; i < 2; i++ {
	}
	return 2
}
`
	testSource := `package demo

import "testing"

func TestIgnoredInPackage(t *testing.T) {
	if true {}
}
`

	fileA := filepath.Join(dir, "a.go")
	fileB := filepath.Join(dir, "b.go")
	testFile := filepath.Join(dir, "a_test.go")

	_ = os.WriteFile(fileA, []byte(sourceA), 0644)
	_ = os.WriteFile(fileB, []byte(sourceB), 0644)
	_ = os.WriteFile(testFile, []byte(testSource), 0644)

	// File analysis
	fileFuncs, err := AnalyzeFileComplexity(fileA)
	if err != nil || len(fileFuncs) != 1 {
		t.Fatalf("failed analyzing file: %v, funcs: %v", err, fileFuncs)
	}
	if fileFuncs[0].Name != "WorkA" || fileFuncs[0].Complexity != 2 {
		t.Fatalf("unexpected file func result: %v", fileFuncs[0])
	}

	// Package analysis excludes test files
	pkgFuncs, err := AnalyzePackageComplexity(dir)
	if err != nil {
		t.Fatalf("failed analyzing package: %v", err)
	}
	if len(pkgFuncs) != 2 {
		t.Fatalf("expected 2 package funcs (ignoring _test.go), got %d", len(pkgFuncs))
	}

	// Error handling
	_, errBadFile := AnalyzeFileComplexity(filepath.Join(dir, "missing.go"))
	if errBadFile == nil {
		t.Fatalf("expected error for missing file")
	}

	_, errBadPkg := AnalyzePackageComplexity(filepath.Join(dir, "nonexistent_dir"))
	if errBadPkg == nil {
		t.Fatalf("expected error for missing package dir")
	}

	_, errBadSource := AnalyzeFuncComplexity("package broken\nfunc broken(")
	if errBadSource == nil {
		t.Fatalf("expected error for invalid source")
	}
}

func TestAnalyzeSourceComplexity_Legacy(t *testing.T) {
	snippet := `
func Calculate(x int) int {
	if x > 0 {
		for i := 0; i < x; i++ {
			if i%2 == 0 && x > 10 {
				return i
			}
		}
	}
	return 0
}
`
	cyclo, cog := AnalyzeSourceComplexity(snippet)
	if cyclo <= 1 {
		t.Fatalf("expected cyclomatic > 1, got %d", cyclo)
	}
	if cog <= 0 {
		t.Fatalf("expected cognitive > 0, got %d", cog)
	}
}
