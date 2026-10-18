package ast

import (
	"bufio"
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"strings"
)

// FunctionComplexity captures the McCabe cyclomatic complexity metric for a function.
type FunctionComplexity struct {
	Name       string `json:"name"`
	Complexity int    `json:"complexity"`
	File       string `json:"file,omitempty"`
	Line       int    `json:"line,omitempty"`
}

// CyclomaticComplexity computes McCabe cyclomatic complexity for an AST function declaration.
func CyclomaticComplexity(fn *ast.FuncDecl) int {
	if fn == nil || fn.Body == nil {
		return 1
	}
	return nodeComplexity(fn.Body)
}

func nodeComplexity(body ast.Node) int {
	if body == nil {
		return 1
	}

	complexity := 1
	ast.Inspect(body, func(n ast.Node) bool {
		switch node := n.(type) {
		case *ast.IfStmt:
			complexity++
		case *ast.ForStmt, *ast.RangeStmt:
			complexity++
		case *ast.CaseClause:
			if len(node.List) > 0 {
				complexity++
			}
		case *ast.CommClause:
			if node.Comm != nil {
				complexity++
			}
		case *ast.BinaryExpr:
			if node.Op == token.LAND || node.Op == token.LOR {
				complexity++
			}
		}
		return true
	})
	return complexity
}

// AnalyzeFuncComplexity parses Go source code and returns cyclomatic complexity for all declared functions.
func AnalyzeFuncComplexity(source string) ([]FunctionComplexity, error) {
	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, "", source, parser.ParseComments)
	if err != nil {
		return nil, fmt.Errorf("parsing Go source: %w", err)
	}

	return extractFunctions(fset, file), nil
}

// AnalyzeFileComplexity parses a Go file from disk and returns cyclomatic complexity for all functions.
func AnalyzeFileComplexity(filename string) ([]FunctionComplexity, error) {
	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, filename, nil, parser.ParseComments)
	if err != nil {
		return nil, fmt.Errorf("parsing Go file %s: %w", filename, err)
	}

	return extractFunctions(fset, file), nil
}

// AnalyzePackageComplexity parses all non-test Go source files in a directory.
func AnalyzePackageComplexity(pkgDir string) ([]FunctionComplexity, error) {
	fset := token.NewFileSet()
	filter := func(fi os.FileInfo) bool {
		return !strings.HasSuffix(fi.Name(), "_test.go") && strings.HasSuffix(fi.Name(), ".go")
	}

	pkgs, err := parser.ParseDir(fset, pkgDir, filter, parser.ParseComments)
	if err != nil {
		return nil, fmt.Errorf("parsing package directory %s: %w", pkgDir, err)
	}

	var results []FunctionComplexity
	for _, pkg := range pkgs {
		for _, file := range pkg.Files {
			results = append(results, extractFunctions(fset, file)...)
		}
	}
	return results, nil
}

func extractFunctions(fset *token.FileSet, file *ast.File) []FunctionComplexity {
	var results []FunctionComplexity
	for _, decl := range file.Decls {
		fn, ok := decl.(*ast.FuncDecl)
		if !ok {
			continue
		}

		pos := fset.Position(fn.Pos())
		results = append(results, FunctionComplexity{
			Name:       funcDeclName(fn),
			Complexity: CyclomaticComplexity(fn),
			File:       pos.Filename,
			Line:       pos.Line,
		})
	}
	return results
}

func funcDeclName(fn *ast.FuncDecl) string {
	if fn.Recv == nil || len(fn.Recv.List) == 0 {
		return fn.Name.Name
	}

	recv := fn.Recv.List[0].Type
	var typeName string
	switch t := recv.(type) {
	case *ast.StarExpr:
		if id, ok := t.X.(*ast.Ident); ok {
			typeName = "*" + id.Name
		}
	case *ast.Ident:
		typeName = t.Name
	}

	if typeName != "" {
		return fmt.Sprintf("(%s).%s", typeName, fn.Name.Name)
	}
	return fn.Name.Name
}

// MaxComplexity returns the highest complexity found in a slice of FunctionComplexity.
func MaxComplexity(funcs []FunctionComplexity) int {
	max := 0
	for _, f := range funcs {
		if f.Complexity > max {
			max = f.Complexity
		}
	}
	return max
}

// AnalyzeSourceComplexity computes cyclomatic and cognitive complexity for source code snippets.
// Maintained for backward compatibility with heuristic callers.
func AnalyzeSourceComplexity(source string) (cyclomatic int, cognitive int) {
	cyclomatic = 1 // Standard McCabe baseline
	cognitive = 0
	nestingLevel := 0

	scanner := bufio.NewScanner(strings.NewReader(source))

	for scanner.Scan() {
		rawLine := scanner.Text()
		line := strings.TrimSpace(rawLine)

		// Ignore blank lines and comments
		if line == "" || strings.HasPrefix(line, "//") || strings.HasPrefix(line, "#") || strings.HasPrefix(line, "/*") {
			continue
		}

		// Adjust nesting level based on braces
		openBraces := strings.Count(line, "{")
		closeBraces := strings.Count(line, "}")

		// Check for branching constructs
		hasBranch := false
		if strings.HasPrefix(line, "if ") || strings.Contains(line, " if ") ||
			strings.HasPrefix(line, "for ") || strings.Contains(line, " for ") ||
			strings.HasPrefix(line, "while ") || strings.Contains(line, " while ") ||
			strings.HasPrefix(line, "case ") ||
			strings.HasPrefix(line, "catch ") || strings.Contains(line, " catch ") {
			hasBranch = true
			cyclomatic++
			cognitive += 1 + nestingLevel
		}

		// Logical operators add to cyclomatic and cognitive complexity
		logicalOps := strings.Count(line, "&&") + strings.Count(line, "||")
		if logicalOps > 0 {
			cyclomatic += logicalOps
			cognitive += logicalOps
		}

		// Update nesting level
		nestingLevel += openBraces - closeBraces
		if nestingLevel < 0 {
			nestingLevel = 0
		}

		_ = hasBranch
	}

	return cyclomatic, cognitive
}
