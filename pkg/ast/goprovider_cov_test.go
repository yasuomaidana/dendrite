package ast

import (
	"errors"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestMatchReceiverSymbol_AllVariants(t *testing.T) {
	tests := []struct {
		name     string
		recvType string
		funcName string
		keepMap  map[string]bool
		want     bool
	}{
		{
			name:     "plain Type.Name match",
			recvType: "Worker",
			funcName: "Run",
			keepMap:  map[string]bool{"Worker.Run": true},
			want:     true,
		},
		{
			name:     "pointer (*Type).Name match",
			recvType: "Worker",
			funcName: "Run",
			keepMap:  map[string]bool{"(*Worker).Run": true},
			want:     true,
		},
		{
			name:     "parenthesized (Type).Name match",
			recvType: "Worker",
			funcName: "Run",
			keepMap:  map[string]bool{"(Worker).Run": true},
			want:     true,
		},
		{
			name:     "star prefix *Type.Name match",
			recvType: "Worker",
			funcName: "Run",
			keepMap:  map[string]bool{"*Worker.Run": true},
			want:     true,
		},
		{
			name:     "unmatched variant",
			recvType: "Worker",
			funcName: "Run",
			keepMap:  map[string]bool{"Other.Run": true},
			want:     false,
		},
		{
			name:     "empty keep map",
			recvType: "Worker",
			funcName: "Run",
			keepMap:  map[string]bool{},
			want:     false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := matchReceiverSymbol(tt.recvType, tt.funcName, tt.keepMap)
			if got != tt.want {
				t.Fatalf("matchReceiverSymbol(%q, %q, %v) = %v; want %v", tt.recvType, tt.funcName, tt.keepMap, got, tt.want)
			}
		})
	}
}

func TestFoldSignatures_ReceiverVariantsAndBuildKeepMap(t *testing.T) {
	src := []byte(`package sample

type Worker struct{}

func (w *Worker) PointerMethod() {
	println("pointer")
}

func (w Worker) ValueMethod() {
	println("value")
}

func UnrelatedFunc() {
	println("unrelated")
}

// Declaration without body (assembly/linkname signature)
func AssemblySignature()
`)

	provider := NewGoASTProvider()

	// Test keeping by pointer variant "(*Worker).PointerMethod" and with empty string / whitespace in keep symbols
	kept, err := provider.FoldSignatures(src, []string{"   ", "", "(*Worker).PointerMethod"})
	if err != nil {
		t.Fatalf("FoldSignatures failed: %v", err)
	}
	if _, parseErr := parser.ParseFile(token.NewFileSet(), "", kept, parser.ParseComments); parseErr != nil {
		t.Fatalf("folded source failed to parse: %v", parseErr)
	}
	keptStr := string(kept)
	if !strings.Contains(keptStr, `println("pointer")`) {
		t.Errorf("expected PointerMethod body to be kept")
	}
	if strings.Contains(keptStr, `println("value")`) {
		t.Errorf("expected ValueMethod body to be pruned")
	}
	if strings.Contains(keptStr, `println("unrelated")`) {
		t.Errorf("expected UnrelatedFunc body to be pruned")
	}

	// Test keeping by value variant "(Worker).ValueMethod"
	keptVal, err := provider.FoldSignatures(src, []string{"(Worker).ValueMethod"})
	if err != nil {
		t.Fatalf("FoldSignatures failed: %v", err)
	}
	keptValStr := string(keptVal)
	if !strings.Contains(keptValStr, `println("value")`) {
		t.Errorf("expected ValueMethod body to be kept")
	}
	if strings.Contains(keptValStr, `println("pointer")`) {
		t.Errorf("expected PointerMethod body to be pruned")
	}

	// Test keeping by "*Worker.PointerMethod"
	keptStar, err := provider.FoldSignatures(src, []string{"*Worker.PointerMethod"})
	if err != nil {
		t.Fatalf("FoldSignatures failed: %v", err)
	}
	if !strings.Contains(string(keptStar), `println("pointer")`) {
		t.Errorf("expected PointerMethod body to be kept with *Worker.PointerMethod")
	}
}

func TestExtractReceiverName_AllBranches(t *testing.T) {
	tests := []struct {
		name string
		expr ast.Expr
		want string
	}{
		{
			name: "StarExpr with Ident",
			expr: &ast.StarExpr{X: ast.NewIdent("Service")},
			want: "Service",
		},
		{
			name: "StarExpr with non-Ident nested star",
			expr: &ast.StarExpr{X: &ast.StarExpr{X: ast.NewIdent("Service")}},
			want: "",
		},
		{
			name: "Ident directly",
			expr: ast.NewIdent("Service"),
			want: "Service",
		},
		{
			name: "IndexExpr with Ident",
			expr: &ast.IndexExpr{
				X:     ast.NewIdent("Container"),
				Index: ast.NewIdent("T"),
			},
			want: "Container",
		},
		{
			name: "IndexExpr with non-Ident X selector",
			expr: &ast.IndexExpr{
				X: &ast.SelectorExpr{
					X:   ast.NewIdent("pkg"),
					Sel: ast.NewIdent("Container"),
				},
				Index: ast.NewIdent("T"),
			},
			want: "",
		},
		{
			name: "Unsupported expression BasicLit",
			expr: &ast.BasicLit{Value: `"bad"`},
			want: "",
		},
		{
			name: "Nil expression",
			expr: nil,
			want: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := extractReceiverName(tt.expr)
			if got != tt.want {
				t.Fatalf("extractReceiverName() = %q; want %q", got, tt.want)
			}
		})
	}

	fnNoRecv := &ast.FuncDecl{Name: ast.NewIdent("Hello")}
	if got := receiverTypeName(fnNoRecv); got != "" {
		t.Fatalf("receiverTypeName(fnNoRecv) = %q; want empty string", got)
	}
	fnEmptyRecv := &ast.FuncDecl{Name: ast.NewIdent("Hello"), Recv: &ast.FieldList{List: []*ast.Field{}}}
	if got := receiverTypeName(fnEmptyRecv); got != "" {
		t.Fatalf("receiverTypeName(fnEmptyRecv) = %q; want empty string", got)
	}
}

func TestApplyFoldReplacements_BoundsAndInvalidRanges(t *testing.T) {
	src := []byte("package p\nfunc A() {\n\tprintln(1)\n}\n")
	ranges := []foldRange{
		{start: -10, end: 5},
		{start: 5, end: 1000},
		{start: 20, end: 10},
		{start: 19, end: 34},
	}

	out := applyFoldReplacements(src, ranges)
	outStr := string(out)
	if !strings.Contains(outStr, PrunedBodyPlaceholder) {
		t.Fatalf("expected output to contain pruned placeholder, got: %s", outStr)
	}
	if strings.Contains(outStr, "println(1)") {
		t.Fatalf("expected body to be pruned, got: %s", outStr)
	}
}

func TestFindLastValidDeclOffset_EdgeCases(t *testing.T) {
	fset := token.NewFileSet()
	src := []byte("package sample\nfunc Valid() {}\n")

	badDecl := &ast.BadDecl{From: token.NoPos, To: token.NoPos}

	file := fset.AddFile("test.go", 1, 200)
	posBeyond := file.Pos(150)
	outOfBoundsDecl := &ast.FuncDecl{
		Name: &ast.Ident{Name: "OutOfBounds", NamePos: posBeyond},
		Type: &ast.FuncType{Func: posBeyond, Params: &ast.FieldList{Opening: posBeyond, Closing: posBeyond}},
		Body: &ast.BlockStmt{Lbrace: posBeyond, Rbrace: posBeyond},
	}

	posZero := file.Pos(1)
	zeroOffsetDecl := &ast.FuncDecl{
		Name: &ast.Ident{Name: "Zero", NamePos: posZero},
		Type: &ast.FuncType{Func: posZero, Params: &ast.FieldList{Opening: posZero, Closing: posZero}},
		Body: &ast.BlockStmt{Lbrace: posZero, Rbrace: posZero},
	}

	decls := []ast.Decl{badDecl, zeroOffsetDecl, outOfBoundsDecl}
	offset, err := findLastValidDeclOffset(fset, decls, src)
	if !errors.Is(err, ErrNoCompleteNode) {
		t.Fatalf("expected ErrNoCompleteNode, got offset=%d, err=%v", offset, err)
	}

	brokenSrc := []byte("package sample\nvar a = (")
	brokenFile := fset.AddFile("broken.go", fset.Base(), 100)
	brokenPos := brokenFile.Pos(len(brokenSrc))
	unparsableDecl := &ast.FuncDecl{
		Name: &ast.Ident{Name: "Unparsable", NamePos: brokenPos},
		Type: &ast.FuncType{Func: brokenPos, Params: &ast.FieldList{Opening: brokenPos, Closing: brokenPos}},
		Body: &ast.BlockStmt{Lbrace: brokenPos, Rbrace: brokenPos},
	}
	offsetBroken, errBroken := findLastValidDeclOffset(fset, []ast.Decl{unparsableDecl}, brokenSrc)
	if !errors.Is(errBroken, ErrNoCompleteNode) {
		t.Fatalf("expected ErrNoCompleteNode for unparsable decl, got offset=%d, err=%v", offsetBroken, errBroken)
	}
}

func TestResolveDiffText_BranchesAndChangedSymbolsError(t *testing.T) {
	tempDir := t.TempDir()

	provider := NewGoASTProvider()
	_, err := provider.ChangedSymbols(tempDir, "HEAD")
	if err == nil {
		t.Fatal("expected error calling ChangedSymbols on non-git directory")
	}

	diffContent := `diff --git a/foo.go b/foo.go
--- a/foo.go
+++ b/foo.go
@@ -1,1 +1,2 @@
 func Hello() {
+	println("hi")
 }
`
	relDiffName := "my_relative.diff"
	if err := os.WriteFile(filepath.Join(tempDir, relDiffName), []byte(diffContent), 0644); err != nil {
		t.Fatalf("failed to write relative diff: %v", err)
	}

	gotDiff, err := resolveDiffText(tempDir, relDiffName)
	if err != nil {
		t.Fatalf("resolveDiffText failed for relative diff path: %v", err)
	}
	if gotDiff != diffContent {
		t.Fatalf("expected diff content %q, got %q", diffContent, gotDiff)
	}

	unreadableName := filepath.Join(tempDir, "unreadable.diff")
	if err := os.WriteFile(unreadableName, []byte("diff"), 0644); err != nil {
		t.Fatalf("write unreadable diff: %v", err)
	}
	if err := os.Chmod(unreadableName, 0000); err == nil {
		defer os.Chmod(unreadableName, 0644)
		_, _ = resolveDiffText(tempDir, unreadableName)
	}

	gitDir := t.TempDir()
	initCmds := [][]string{
		{"git", "init"},
		{"git", "config", "user.name", "Tester"},
		{"git", "config", "user.email", "tester@dendrite.dev"},
	}
	for _, c := range initCmds {
		cmd := exec.Command(c[0], c[1:]...)
		cmd.Dir = gitDir
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git setup failed: %v, out: %s", err, string(out))
		}
	}
	if err := os.WriteFile(filepath.Join(gitDir, "main.go"), []byte("package main\n"), 0644); err != nil {
		t.Fatalf("write main.go: %v", err)
	}
	for _, c := range [][]string{{"git", "add", "main.go"}, {"git", "commit", "-m", "init"}} {
		cmd := exec.Command(c[0], c[1:]...)
		cmd.Dir = gitDir
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git commit failed: %v, out: %s", err, string(out))
		}
	}
	if err := os.WriteFile(filepath.Join(gitDir, "main.go"), []byte("package main\n// modified\n"), 0644); err != nil {
		t.Fatalf("modify main.go: %v", err)
	}

	diffEmptyRef, err := resolveDiffText(gitDir, "")
	if err != nil {
		t.Fatalf("resolveDiffText with empty baseRef failed: %v", err)
	}
	if !strings.Contains(diffEmptyRef, "modified") {
		t.Fatalf("expected git diff to contain modified text, got: %s", diffEmptyRef)
	}

	_, errInvalidRef := resolveDiffText(gitDir, "nonexistent-branch-xyz")
	if errInvalidRef == nil {
		t.Fatal("expected error for nonexistent git branch in resolveDiffText")
	}
}

func TestParseDiffHunks_ComprehensiveBranches(t *testing.T) {
	diffText := `random header noise before diff
diff --git a/deleted.go b/deleted.go
--- a/deleted.go
+++ /dev/null
@@ -1,5 +0,0 @@
-func Deleted() {}
diff --git a/sample.go b/sample.go
--- a/sample.go
+++ b/sample.go
@@ malformed hunk header @@
@@ -1 +5 @@ func SingleLine()
+added single line
@@ -2,3 +6,0 @@
-removed lines
@@ -10,2 +10,2 @@ func Normal()
-old
+new
`
	hunks := parseDiffHunks(diffText)
	if len(hunks) != 3 {
		t.Fatalf("expected 3 valid hunks parsed, got %d", len(hunks))
	}

	if hunks[0].newStart != 5 || hunks[0].newCount != 1 || hunks[0].start != 5 || hunks[0].end != 5 {
		t.Errorf("Hunk 0 unexpected ranges: %+v", hunks[0])
	}
	if hunks[0].heading != "func SingleLine()" {
		t.Errorf("Hunk 0 unexpected heading: %q", hunks[0].heading)
	}

	if hunks[1].newStart != 6 || hunks[1].newCount != 0 || hunks[1].start != 6 || hunks[1].end != 6 {
		t.Errorf("Hunk 1 unexpected ranges for count=0: %+v", hunks[1])
	}

	if hunks[2].newStart != 10 || hunks[2].newCount != 2 || hunks[2].start != 10 || hunks[2].end != 11 {
		t.Errorf("Hunk 2 unexpected ranges: %+v", hunks[2])
	}
}

func TestParseGoFile_AllBranches(t *testing.T) {
	tempDir := t.TempDir()

	if pf := parseGoFile(tempDir, "README.md"); pf != nil {
		t.Fatalf("expected nil for non-go file, got %+v", pf)
	}

	if pf := parseGoFile(tempDir, "missing.go"); pf != nil {
		t.Fatalf("expected nil for missing file, got %+v", pf)
	}

	badGo := filepath.Join(tempDir, "bad.go")
	if err := os.WriteFile(badGo, []byte("package broken\nfunc ("), 0644); err != nil {
		t.Fatalf("failed to write bad.go: %v", err)
	}
	if pf := parseGoFile(tempDir, "bad.go"); pf != nil {
		t.Fatalf("expected nil for unparsable Go file, got %+v", pf)
	}

	goodGo := filepath.Join(tempDir, "good.go")
	if err := os.WriteFile(goodGo, []byte("package good\n\nfunc Ok() {}\n"), 0644); err != nil {
		t.Fatalf("failed to write good.go: %v", err)
	}
	pf := parseGoFile(tempDir, "good.go")
	if pf == nil {
		t.Fatal("expected non-nil parsedGoFile for valid file")
	}
	if pf.pkgLine != 1 {
		t.Fatalf("expected pkgLine == 1, got %d", pf.pkgLine)
	}
}
