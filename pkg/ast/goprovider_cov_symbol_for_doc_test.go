package ast

import (
	"go/ast"
	"go/token"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestSymbolFor_DocCommentsAndTrailingHunk(t *testing.T) {
	tempDir := t.TempDir()
	source := `package testpkg

// DocType is a documented type.
type DocType struct {
	Field int
}

// DocFunc is a documented function.
func DocFunc() {
	println("func")
}

// Trailing comment at the end of the file
`
	filePath := filepath.Join(tempDir, "docs.go")
	if err := os.WriteFile(filePath, []byte(source), 0644); err != nil {
		t.Fatalf("failed to write source: %v", err)
	}

	pf := parseGoFile(tempDir, "docs.go")
	if pf == nil {
		t.Fatal("parseGoFile failed")
	}

	hunkGenDoc := diffHunk{file: "docs.go", start: 3, end: 3}
	sliceGen := pf.symbolFor(hunkGenDoc)
	if sliceGen.Symbol != "DocType" || sliceGen.Kind != "type" {
		t.Errorf("expected DocType/type for doc comment hunk, got %s/%s", sliceGen.Symbol, sliceGen.Kind)
	}

	hunkFuncDoc := diffHunk{file: "docs.go", start: 8, end: 8}
	sliceFunc := pf.symbolFor(hunkFuncDoc)
	if sliceFunc.Symbol != "DocFunc" || sliceFunc.Kind != "func" {
		t.Errorf("expected DocFunc/func for doc comment hunk, got %s/%s", sliceFunc.Symbol, sliceFunc.Kind)
	}

	hunkTrailing := diffHunk{file: "docs.go", start: 13, end: 14}
	sliceTrailing := pf.symbolFor(hunkTrailing)
	if sliceTrailing.Symbol != "docs.go" || sliceTrailing.Kind != "file" {
		t.Errorf("expected docs.go/file for trailing hunk, got %s/%s", sliceTrailing.Symbol, sliceTrailing.Kind)
	}
}

func TestDeclSymbol_ValueReceiverImportsConstAndFallback(t *testing.T) {
	tempDir := t.TempDir()
	source := `package sample

import "fmt"

const Version = "1.0.0"

var Count = 42

type Worker struct{}

func (w Worker) ValueMethod() {
	fmt.Println("val")
}

func (w *Worker) PointerMethod() {
	fmt.Println("ptr")
}
`
	if err := os.WriteFile(filepath.Join(tempDir, "declarations.go"), []byte(source), 0644); err != nil {
		t.Fatalf("failed to write file: %v", err)
	}

	pf := parseGoFile(tempDir, "declarations.go")
	if pf == nil {
		t.Fatal("parseGoFile failed")
	}

	var foundImport, foundConst, foundVar, foundValMethod, foundPtrMethod bool
	for _, decl := range pf.file.Decls {
		slice := pf.declSymbol(decl)
		switch slice.Kind {
		case "package":
			if slice.Symbol == "sample" {
				foundImport = true
			}
		case "const":
			if slice.Symbol == "Version" {
				foundConst = true
			}
		case "var":
			if slice.Symbol == "Count" {
				foundVar = true
			}
		case "method":
			if slice.Symbol == "(Worker).ValueMethod" && slice.Receiver == "Worker" {
				foundValMethod = true
			}
			if slice.Symbol == "(*Worker).PointerMethod" && slice.Receiver == "Worker" {
				foundPtrMethod = true
			}
		}
	}

	if !foundImport {
		t.Error("expected import declaration to map to package/sample")
	}
	if !foundConst {
		t.Error("expected const declaration to map to const/Version")
	}
	if !foundVar {
		t.Error("expected var declaration to map to var/Count")
	}
	if !foundValMethod {
		t.Error("expected value receiver method to map to (Worker).ValueMethod")
	}
	if !foundPtrMethod {
		t.Error("expected pointer receiver method to map to (*Worker).PointerMethod")
	}

	fallbackSlice := pf.declSymbol(&ast.BadDecl{})
	if fallbackSlice.Symbol != "declaration" || fallbackSlice.Kind != "file" {
		t.Errorf("expected declaration/file for BadDecl, got %s/%s", fallbackSlice.Symbol, fallbackSlice.Kind)
	}

	emptyGenDecl := &ast.GenDecl{Tok: token.VAR}
	fallbackGenSlice := pf.declSymbol(emptyGenDecl)
	if fallbackGenSlice.Symbol != "declaration" || fallbackGenSlice.Kind != "file" {
		t.Errorf("expected declaration/file for empty GenDecl, got %s/%s", fallbackGenSlice.Symbol, fallbackGenSlice.Kind)
	}
}

func TestSymbolFromHunkText_ComprehensiveBranches(t *testing.T) {
	tests := []struct {
		name     string
		hunk     diffHunk
		wantSym  string
		wantKind string
		wantRecv string
	}{
		{
			name: "heading with standalone function",
			hunk: diffHunk{
				file:    "pkg/util.go",
				heading: "func FormatTime(t time.Time) string",
			},
			wantSym:  "FormatTime",
			wantKind: "func",
		},
		{
			name: "heading with value receiver method",
			hunk: diffHunk{
				file:    "pkg/worker.go",
				heading: "func (w Worker) Execute() error",
			},
			wantSym:  "(Worker).Execute",
			wantKind: "method",
			wantRecv: "Worker",
		},
		{
			name: "heading with pointer receiver method",
			hunk: diffHunk{
				file:    "pkg/worker.go",
				heading: "func (w *Worker) Process()",
			},
			wantSym:  "(*Worker).Process",
			wantKind: "method",
			wantRecv: "Worker",
		},
		{
			name: "heading with type definition",
			hunk: diffHunk{
				file:    "pkg/types.go",
				heading: "type Options struct",
			},
			wantSym:  "Options",
			wantKind: "type",
		},
		{
			name: "body contains func and empty lines",
			hunk: diffHunk{
				file:    "pkg/service.go",
				heading: "",
				body:    []string{"", " // comment", "+func BodyHandler() {"},
			},
			wantSym:  "BodyHandler",
			wantKind: "func",
		},
		{
			name: "body contains type",
			hunk: diffHunk{
				file:    "pkg/models.go",
				heading: "",
				body:    []string{"+type Payload struct {"},
			},
			wantSym:  "Payload",
			wantKind: "type",
		},
		{
			name: "no function or type found anywhere",
			hunk: diffHunk{
				file:    "pkg/config.go",
				heading: "",
				body:    []string{"", "+var Global = 1", "+// comment"},
			},
			wantSym:  "config.go",
			wantKind: "file",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := symbolFromHunkText(tt.hunk)
			if got.Symbol != tt.wantSym || got.Kind != tt.wantKind || got.Receiver != tt.wantRecv {
				t.Fatalf("symbolFromHunkText() = {Symbol: %q, Kind: %q, Receiver: %q}; want {Symbol: %q, Kind: %q, Receiver: %q}",
					got.Symbol, got.Kind, got.Receiver, tt.wantSym, tt.wantKind, tt.wantRecv)
			}
		})
	}
}

func TestFormatFunctionSignature_AllBranches(t *testing.T) {
	fset := token.NewFileSet()

	normalFn := &ast.FuncDecl{
		Name: ast.NewIdent("Compute"),
		Type: &ast.FuncType{
			Params: &ast.FieldList{},
		},
	}
	gotNormal := formatFunctionSignature(fset, normalFn)
	if !strings.Contains(gotNormal, "Compute()") {
		t.Fatalf("expected formatFunctionSignature to format signature, got %q", gotNormal)
	}
}
