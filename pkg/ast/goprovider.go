package ast

import (
	"bytes"
	"fmt"
	"go/ast"
	"go/parser"
	"go/printer"
	"go/token"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
)

// GoASTProvider provides AST parsing, signature folding, and git hunk mapping for Go source files.
type GoASTProvider struct{}

// NewGoASTProvider creates a new GoASTProvider instance.
func NewGoASTProvider() *GoASTProvider {
	return &GoASTProvider{}
}

// Compile-time interface implementation check.
var _ ASTProvider = (*GoASTProvider)(nil)

type foldRange struct {
	start int
	end   int
}

// FoldSignatures prunes function bodies for non-kept functions, keeping only signatures and doc comments.
func (p *GoASTProvider) FoldSignatures(src []byte, keepSymbols []string) ([]byte, error) {
	if len(bytes.TrimSpace(src)) == 0 {
		return nil, ErrEmptySource
	}

	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, "", src, parser.ParseComments)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrInvalidSyntax, err)
	}

	keepMap := buildKeepMap(keepSymbols)
	ranges := collectFoldRanges(fset, file, keepMap, len(src))
	if len(ranges) == 0 {
		return src, nil
	}

	return applyFoldReplacements(src, ranges), nil
}

func buildKeepMap(symbols []string) map[string]bool {
	m := make(map[string]bool, len(symbols))
	for _, s := range symbols {
		trimmed := strings.TrimSpace(s)
		if trimmed != "" {
			m[trimmed] = true
			m[strings.TrimPrefix(trimmed, "*")] = true
		}
	}
	return m
}

func collectFoldRanges(fset *token.FileSet, file *ast.File, keepMap map[string]bool, srcLen int) []foldRange {
	var ranges []foldRange
	for _, decl := range file.Decls {
		fn, ok := decl.(*ast.FuncDecl)
		if !ok || fn.Body == nil {
			continue
		}
		if isFuncKept(fn, keepMap) {
			continue
		}
		start := fset.Position(fn.Body.Lbrace).Offset
		end := fset.Position(fn.Body.Rbrace).Offset
		if start >= 0 && end < srcLen && start <= end {
			ranges = append(ranges, foldRange{start: start, end: end})
		}
	}
	return ranges
}

func isFuncKept(fn *ast.FuncDecl, keepMap map[string]bool) bool {
	if keepMap[fn.Name.Name] {
		return true
	}
	recvType := receiverTypeName(fn)
	if recvType == "" {
		return false
	}
	return matchReceiverSymbol(recvType, fn.Name.Name, keepMap)
}

func receiverTypeName(fn *ast.FuncDecl) string {
	if fn.Recv == nil || len(fn.Recv.List) == 0 {
		return ""
	}
	return extractReceiverName(fn.Recv.List[0].Type)
}

func matchReceiverSymbol(recvType, name string, keepMap map[string]bool) bool {
	variants := []string{
		recvType + "." + name,
		"(*" + recvType + ")." + name,
		"(" + recvType + ")." + name,
		"*" + recvType + "." + name,
	}
	for _, v := range variants {
		if keepMap[v] {
			return true
		}
	}
	return false
}

func extractReceiverName(expr ast.Expr) string {
	switch t := expr.(type) {
	case *ast.StarExpr:
		if ident, ok := t.X.(*ast.Ident); ok {
			return ident.Name
		}
	case *ast.Ident:
		return t.Name
	case *ast.IndexExpr:
		if ident, ok := t.X.(*ast.Ident); ok {
			return ident.Name
		}
	}
	return ""
}

// PrunedBodyPlaceholder replaces folded function bodies.
const PrunedBodyPlaceholder = "{ /* pruned */ }"

func applyFoldReplacements(src []byte, ranges []foldRange) []byte {
	sort.Slice(ranges, func(i, j int) bool {
		return ranges[i].start > ranges[j].start
	})

	// A comment placeholder, never a panic/zero-value stub: folded context is shown to LLMs and must not
	// teach them stub bodies (the anti-stub gate flags those).
	prunedStub := []byte(PrunedBodyPlaceholder)
	out := append([]byte(nil), src...)

	for _, r := range ranges {
		if r.start < 0 || r.end >= len(out) || r.start > r.end {
			continue
		}
		prefix := out[:r.start]
		suffix := out[r.end+1:]
		newOut := make([]byte, 0, len(prefix)+len(prunedStub)+len(suffix))
		newOut = append(newOut, prefix...)
		newOut = append(newOut, prunedStub...)
		newOut = append(newOut, suffix...)
		out = newOut
	}
	return out
}

// LastCompleteNode returns the byte offset of the end of the last complete top-level declaration.
func (p *GoASTProvider) LastCompleteNode(src []byte) (int, error) {
	if len(bytes.TrimSpace(src)) == 0 {
		return 0, ErrEmptySource
	}

	fset := token.NewFileSet()
	file, _ := parser.ParseFile(fset, "", src, 0)
	if file == nil || len(file.Decls) == 0 {
		return 0, ErrNoCompleteNode
	}

	return findLastValidDeclOffset(fset, file.Decls, src)
}

func findLastValidDeclOffset(fset *token.FileSet, decls []ast.Decl, src []byte) (int, error) {
	for i := len(decls) - 1; i >= 0; i-- {
		decl := decls[i]
		if !decl.End().IsValid() {
			continue
		}
		offset := fset.Position(decl.End()).Offset
		if offset <= 0 || offset > len(src) {
			continue
		}
		if isParsableGoSource(src[:offset]) {
			return offset, nil
		}
	}
	return 0, ErrNoCompleteNode
}

func isParsableGoSource(snippet []byte) bool {
	fset := token.NewFileSet()
	_, err := parser.ParseFile(fset, "", snippet, 0)
	return err == nil
}

// ChangedSymbols maps every hunk of `git diff -U0 <baseRef>` (or of a unified diff file when
// baseRef names one) to the innermost enclosing top-level declaration in the new file version.
func (p *GoASTProvider) ChangedSymbols(repoDir string, baseRef string) ([]SymbolSlice, error) {
	diffText, err := resolveDiffText(repoDir, baseRef)
	if err != nil {
		return nil, err
	}
	return p.ChangedSymbolsFromDiff(repoDir, diffText)
}

// ChangedSymbolsFromDiff maps the hunks of diffText to declarations. Hunk ranges are new-file line
// numbers, so each Go file is parsed from repoDir (the working tree, i.e. the new version). When a
// file cannot be read or parsed, the symbol is taken from the hunk heading or the hunk body.
func (p *GoASTProvider) ChangedSymbolsFromDiff(repoDir string, diffText string) ([]SymbolSlice, error) {
	parsed := map[string]*parsedGoFile{}
	var slices []SymbolSlice
	for _, h := range parseDiffHunks(diffText) {
		pf, seen := parsed[h.file]
		if !seen {
			pf = parseGoFile(repoDir, h.file)
			parsed[h.file] = pf
		}
		var slice SymbolSlice
		if pf != nil {
			slice = pf.symbolFor(h)
		} else {
			slice = symbolFromHunkText(h)
		}
		slice.File = h.file
		slice.HunkStart, slice.HunkEnd = h.start, h.end
		slice.HunkRange = fmt.Sprintf("+%d,%d", h.newStart, h.newCount)
		slices = append(slices, slice)
	}
	return slices, nil
}

// diffHunk is one "@@ -a,b +c,d @@ heading" block of a unified diff.
type diffHunk struct {
	file     string
	newStart int
	newCount int
	start    int // first new-file line touched
	end      int // last new-file line touched
	heading  string
	body     []string
}

var hunkHeaderRegex = regexp.MustCompile(`^@@ -\d+(?:,\d+)? \+(\d+)(?:,(\d+))? @@ ?(.*)$`)

func parseDiffHunks(diffText string) []diffHunk {
	var hunks []diffHunk
	file := ""
	for _, line := range strings.Split(diffText, "\n") {
		switch {
		case strings.HasPrefix(line, "+++ "):
			target := strings.TrimSpace(strings.TrimPrefix(line, "+++ "))
			if target == "/dev/null" {
				file = ""
			} else {
				file = strings.TrimPrefix(target, "b/")
			}
		case strings.HasPrefix(line, "--- "), strings.HasPrefix(line, "diff "), strings.HasPrefix(line, "index "):
			// file headers carry no hunk content
		case file != "" && strings.HasPrefix(line, "@@ "):
			m := hunkHeaderRegex.FindStringSubmatch(line)
			if m == nil {
				continue
			}
			start, _ := strconv.Atoi(m[1])
			count := 1
			if m[2] != "" {
				count, _ = strconv.Atoi(m[2])
			}
			end := start + count - 1
			if count == 0 {
				end = start
			}
			hunks = append(hunks, diffHunk{file: file, newStart: start, newCount: count, start: start, end: end, heading: strings.TrimSpace(m[3])})
		case file != "" && len(hunks) > 0 && hunks[len(hunks)-1].file == file:
			hunks[len(hunks)-1].body = append(hunks[len(hunks)-1].body, line)
		}
	}
	return hunks
}

// parsedGoFile is a parsed new-version Go file used to resolve hunk line ranges.
type parsedGoFile struct {
	fset    *token.FileSet
	file    *ast.File
	pkgLine int
}

func parseGoFile(repoDir, file string) *parsedGoFile {
	if !strings.HasSuffix(file, ".go") {
		return nil
	}
	src, err := os.ReadFile(filepath.Join(repoDir, file))
	if err != nil {
		return nil
	}
	fset := token.NewFileSet()
	f, err := parser.ParseFile(fset, file, src, parser.ParseComments)
	if err != nil {
		return nil
	}
	return &parsedGoFile{fset: fset, file: f, pkgLine: fset.Position(f.Package).Line}
}

// symbolFor returns the first top-level declaration (in source order) whose span, including its
// doc comment, overlaps the hunk; hunks above the package clause map to the package.
func (pf *parsedGoFile) symbolFor(h diffHunk) SymbolSlice {
	line := func(pos token.Pos) int { return pf.fset.Position(pos).Line }
	for _, decl := range pf.file.Decls {
		start, end := line(decl.Pos()), line(decl.End())
		switch d := decl.(type) {
		case *ast.FuncDecl:
			if d.Doc != nil {
				start = line(d.Doc.Pos())
			}
		case *ast.GenDecl:
			if d.Doc != nil {
				start = line(d.Doc.Pos())
			}
		}
		if h.start <= end && h.end >= start {
			slice := pf.declSymbol(decl)
			slice.StartLine, slice.EndLine = start, end
			return slice
		}
	}
	if h.start <= pf.pkgLine {
		return SymbolSlice{Symbol: pf.file.Name.Name, Kind: "package", StartLine: 1, EndLine: pf.pkgLine}
	}
	return SymbolSlice{Symbol: filepath.Base(h.file), Kind: "file"}
}

func (pf *parsedGoFile) declSymbol(decl ast.Decl) SymbolSlice {
	switch d := decl.(type) {
	case *ast.FuncDecl:
		slice := SymbolSlice{Symbol: d.Name.Name, Kind: "func", Signature: formatFunctionSignature(pf.fset, d)}
		if recv := receiverTypeName(d); recv != "" {
			ptr := ""
			if _, isPtr := d.Recv.List[0].Type.(*ast.StarExpr); isPtr {
				ptr = "*"
			}
			slice.Symbol = fmt.Sprintf("(%s%s).%s", ptr, recv, d.Name.Name)
			slice.Kind = "method"
			slice.Receiver = recv
		}
		return slice
	case *ast.GenDecl:
		if len(d.Specs) == 0 && d.Tok != token.IMPORT {
			break // e.g. an empty grouped "var ()"
		}
		switch d.Tok {
		case token.IMPORT:
			return SymbolSlice{Symbol: pf.file.Name.Name, Kind: "package"}
		case token.TYPE:
			if ts, ok := d.Specs[0].(*ast.TypeSpec); ok {
				return SymbolSlice{Symbol: ts.Name.Name, Kind: "type"}
			}
		case token.VAR, token.CONST:
			if vs, ok := d.Specs[0].(*ast.ValueSpec); ok && len(vs.Names) > 0 {
				return SymbolSlice{Symbol: vs.Names[0].Name, Kind: strings.ToLower(d.Tok.String())}
			}
		}
	}
	return SymbolSlice{Symbol: "declaration", Kind: "file"}
}

var (
	funcLineRegex = regexp.MustCompile(`^func\s*(?:\(\s*\w*\s*(\*?)\s*(\w+)[^)]*\))?\s*(\w+)\s*[\[(]`)
	typeLineRegex = regexp.MustCompile(`^type\s+(\w+)`)
)

// symbolFromHunkText derives a symbol from the hunk heading, else from the first declaration line
// in the hunk body, for files that are not on disk (deleted, or a standalone diff file).
func symbolFromHunkText(h diffHunk) SymbolSlice {
	candidates := []string{h.heading}
	for _, l := range h.body {
		if len(l) > 0 {
			candidates = append(candidates, strings.TrimSpace(l[1:]))
		}
	}
	for _, c := range candidates {
		if m := funcLineRegex.FindStringSubmatch(c); m != nil {
			if m[2] != "" {
				return SymbolSlice{Symbol: fmt.Sprintf("(%s%s).%s", m[1], m[2], m[3]), Kind: "method", Receiver: m[2]}
			}
			return SymbolSlice{Symbol: m[3], Kind: "func"}
		}
		if m := typeLineRegex.FindStringSubmatch(c); m != nil {
			return SymbolSlice{Symbol: m[1], Kind: "type"}
		}
	}
	return SymbolSlice{Symbol: filepath.Base(h.file), Kind: "file"}
}

// resolveDiffText reads baseRef as a unified diff file when it names one; otherwise it runs
// `git diff -U0 <baseRef>` (HEAD when empty) in repoDir.
func resolveDiffText(repoDir, baseRef string) (string, error) {
	if baseRef != "" {
		path := baseRef
		if !filepath.IsAbs(path) {
			path = filepath.Join(repoDir, path)
		}
		if info, err := os.Stat(path); err == nil && !info.IsDir() {
			data, err := os.ReadFile(path)
			if err != nil {
				return "", err
			}
			return string(data), nil
		}
	} else {
		baseRef = "HEAD"
	}
	out, err := exec.Command("git", "-C", repoDir, "diff", "-U0", baseRef).CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("git diff %s failed: %w: %s", baseRef, err, strings.TrimSpace(string(out)))
	}
	return string(out), nil
}

func formatFunctionSignature(fset *token.FileSet, fn *ast.FuncDecl) string {
	stub := &ast.FuncDecl{
		Recv: fn.Recv,
		Name: fn.Name,
		Type: fn.Type,
	}
	var buf bytes.Buffer
	if err := printer.Fprint(&buf, fset, stub); err == nil {
		return buf.String()
	}
	return fn.Name.Name
}
