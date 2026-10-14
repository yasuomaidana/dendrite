package reflect

import (
	"bufio"
	"encoding/json"
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

// ExtractBlueprint scans a reference documentation directory and builds a DocBlueprint.
func ExtractBlueprint(refDocsDir string) (*DocBlueprint, error) {
	if strings.TrimSpace(refDocsDir) == "" {
		return nil, ErrEmptyReferenceDir
	}
	info, err := os.Stat(refDocsDir)
	if err != nil || !info.IsDir() {
		return nil, ErrRefDirNotFound
	}

	blueprint := &DocBlueprint{
		ReferenceDir: refDocsDir,
		Categories:   []string{},
		Templates:    make(map[string]DocFileTemplate),
	}

	categorySet := make(map[string]struct{})

	err = filepath.Walk(refDocsDir, func(path string, fileInfo os.FileInfo, walkErr error) error {
		if walkErr != nil {
			return nil
		}
		if fileInfo.IsDir() {
			rel, _ := filepath.Rel(refDocsDir, path)
			if rel != "." && !strings.HasPrefix(rel, ".") {
				parts := strings.Split(rel, string(filepath.Separator))
				categorySet[parts[0]] = struct{}{}
			}
			return nil
		}

		ext := strings.ToLower(filepath.Ext(path))
		if ext == ".md" || ext == ".mdx" {
			tmpl, parseErr := parseDocTemplate(refDocsDir, path)
			if parseErr == nil {
				blueprint.Templates[tmpl.RelativePath] = tmpl
			}
		}

		if filepath.Base(path) == "sidebars.json" || filepath.Base(path) == "sidebars.js" {
			data, readErr := os.ReadFile(path)
			if readErr == nil {
				blueprint.SidebarJSON = string(data)
			}
		}
		return nil
	})

	if err != nil {
		return nil, err
	}

	for cat := range categorySet {
		blueprint.Categories = append(blueprint.Categories, cat)
	}
	sort.Strings(blueprint.Categories)

	return blueprint, nil
}

// parseDocTemplate extracts frontmatter, headings, and mermaid diagrams from a single markdown file.
func parseDocTemplate(baseDir, filePath string) (DocFileTemplate, error) {
	relPath, err := filepath.Rel(baseDir, filePath)
	if err != nil {
		relPath = filepath.Base(filePath)
	}

	contentBytes, err := os.ReadFile(filePath)
	if err != nil {
		return DocFileTemplate{}, err
	}
	raw := string(contentBytes)

	tmpl := DocFileTemplate{
		RelativePath: relPath,
		Category:     extractCategory(relPath),
		Frontmatter:  extractFrontmatter(raw),
		Headings:     extractHeadings(raw),
		MermaidStyle: extractMermaidStyle(raw),
		RawContent:   raw,
	}

	return tmpl, nil
}

func extractCategory(relPath string) string {
	parts := strings.Split(filepath.ToSlash(relPath), "/")
	if len(parts) > 1 {
		return parts[0]
	}
	return "general"
}

func extractFrontmatter(raw string) map[string]string {
	fm := make(map[string]string)
	if !strings.HasPrefix(raw, "---") {
		return fm
	}

	lines := strings.Split(raw, "\n")
	if len(lines) < 2 {
		return fm
	}

	inFM := false
	for i, line := range lines {
		trimmed := strings.TrimSpace(line)
		if i == 0 && trimmed == "---" {
			inFM = true
			continue
		}
		if inFM && trimmed == "---" {
			break
		}
		if inFM && strings.Contains(line, ":") {
			parts := strings.SplitN(line, ":", 2)
			key := strings.TrimSpace(parts[0])
			val := strings.Trim(strings.TrimSpace(parts[1]), `"'[]`)
			fm[key] = val
		}
	}
	return fm
}

func extractHeadings(raw string) []HeadingTemplate {
	var headings []HeadingTemplate
	scanner := bufio.NewScanner(strings.NewReader(raw))
	for scanner.Scan() {
		line := scanner.Text()
		if strings.HasPrefix(line, "#") {
			level := 0
			for _, ch := range line {
				if ch == '#' {
					level++
				} else {
					break
				}
			}
			title := strings.TrimSpace(line[level:])
			headings = append(headings, HeadingTemplate{
				Level: level,
				Title: title,
			})
		}
	}
	return headings
}

func extractMermaidStyle(raw string) *DiagramStyle {
	if !strings.Contains(raw, "```mermaid") {
		return nil
	}
	idx := strings.Index(raw, "```mermaid")
	sub := raw[idx+len("```mermaid"):]
	endIdx := strings.Index(sub, "```")
	if endIdx == -1 {
		return nil
	}
	block := strings.TrimSpace(sub[:endIdx])
	lines := strings.Split(block, "\n")
	if len(lines) == 0 {
		return nil
	}

	diagType := strings.TrimSpace(lines[0])
	var sampleNodes []string
	for _, l := range lines[1:] {
		trimmed := strings.TrimSpace(l)
		if trimmed != "" && !strings.HasPrefix(trimmed, "%%") {
			sampleNodes = append(sampleNodes, trimmed)
		}
	}

	return &DiagramStyle{
		DiagramType: diagType,
		SampleNodes: sampleNodes,
	}
}

// HarvestTargetSymbols harvests exported symbols from target Go source code.
func HarvestTargetSymbols(sourceDir string) ([]TargetSymbol, error) {
	if strings.TrimSpace(sourceDir) == "" {
		return nil, ErrEmptyTargetDir
	}

	var symbols []TargetSymbol
	fset := token.NewFileSet()

	err := filepath.Walk(sourceDir, func(path string, info os.FileInfo, walkErr error) error {
		if walkErr != nil || info.IsDir() || !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
			return nil
		}

		node, parseErr := parser.ParseFile(fset, path, nil, parser.ParseComments)
		if parseErr != nil {
			return nil
		}

		pkgName := node.Name.Name

		for _, decl := range node.Decls {
			switch d := decl.(type) {
			case *ast.FuncDecl:
				if d.Name.IsExported() {
					symbols = append(symbols, TargetSymbol{
						Package: pkgName,
						Name:    d.Name.Name,
						Kind:    "func",
						Doc:     strings.TrimSpace(d.Doc.Text()),
					})
				}
			case *ast.GenDecl:
				for _, spec := range d.Specs {
					if ts, ok := spec.(*ast.TypeSpec); ok && ts.Name.IsExported() {
						kind := "type"
						switch ts.Type.(type) {
						case *ast.StructType:
							kind = "struct"
						case *ast.InterfaceType:
							kind = "interface"
						}
						symbols = append(symbols, TargetSymbol{
							Package: pkgName,
							Name:    ts.Name.Name,
							Kind:    kind,
							Doc:     strings.TrimSpace(d.Doc.Text()),
						})
					}
				}
			}
		}
		return nil
	})

	if err != nil {
		return nil, err
	}

	sort.Slice(symbols, func(i, j int) bool {
		if symbols[i].Package == symbols[j].Package {
			return symbols[i].Name < symbols[j].Name
		}
		return symbols[i].Package < symbols[j].Package
	})

	return symbols, nil
}

// PlanReflection contextualizes reference blueprint templates against target source symbols.
func PlanReflection(blueprint *DocBlueprint, targetSourceDir, targetDocsDir string) (*ReflectionPlan, error) {
	if blueprint == nil || len(blueprint.Templates) == 0 {
		return nil, ErrInvalidBlueprint
	}
	if strings.TrimSpace(targetDocsDir) == "" {
		return nil, ErrEmptyTargetDir
	}

	targetSymbols, _ := HarvestTargetSymbols(targetSourceDir)

	plan := &ReflectionPlan{
		TargetDocsDir: targetDocsDir,
		Files:         make([]PlannedDocFile, 0, len(blueprint.Templates)),
		Warnings:      []string{},
	}

	var filePaths []string

	for _, tmpl := range blueprint.Templates {
		docFile := generatePlannedDoc(tmpl, targetSymbols)
		plan.Files = append(plan.Files, docFile)
		filePaths = append(filePaths, docFile.RelativePath)
	}

	sort.Slice(plan.Files, func(i, j int) bool {
		return plan.Files[i].RelativePath < plan.Files[j].RelativePath
	})

	plan.SidebarConfig = generateSidebarJSON(filePaths)

	return plan, nil
}

func generatePlannedDoc(tmpl DocFileTemplate, symbols []TargetSymbol) PlannedDocFile {
	targetFM := make(map[string]string)
	for k, v := range tmpl.Frontmatter {
		targetFM[k] = v
	}
	targetFM["repository"] = "target-workspace"

	var sb strings.Builder
	sb.WriteString("---\n")
	var keys []string
	for k := range targetFM {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		sb.WriteString(fmt.Sprintf("%s: %s\n", k, targetFM[k]))
	}
	sb.WriteString("---\n\n")

	for _, h := range tmpl.Headings {
		prefix := strings.Repeat("#", h.Level)
		sb.WriteString(fmt.Sprintf("%s %s\n\n", prefix, h.Title))

		if h.Level == 2 && strings.Contains(strings.ToLower(h.Title), "technical") || strings.Contains(strings.ToLower(h.Title), "specification") {
			if len(symbols) > 0 {
				sb.WriteString("### Verified Target Domain Symbols\n\n")
				limit := 6
				if len(symbols) < limit {
					limit = len(symbols)
				}
				for i := 0; i < limit; i++ {
					sym := symbols[i]
					sb.WriteString(fmt.Sprintf("- `%s.%s` (%s)\n", sym.Package, sym.Name, sym.Kind))
				}
				sb.WriteString("\n")
			}
		}
	}

	if tmpl.MermaidStyle != nil {
		sb.WriteString("```mermaid\n")
		diagType := tmpl.MermaidStyle.DiagramType
		if diagType == "" {
			diagType = "flowchart TD"
		}
		sb.WriteString(diagType + "\n")

		if len(symbols) >= 2 {
			sb.WriteString(fmt.Sprintf("    A[\"%s.%s\"] --> B[\"%s.%s\"]\n",
				symbols[0].Package, symbols[0].Name, symbols[1].Package, symbols[1].Name))
		} else {
			sb.WriteString("    NodeA[\"Core Module\"] --> NodeB[\"Engine Service\"]\n")
		}
		sb.WriteString("```\n")
	}

	return PlannedDocFile{
		RelativePath: tmpl.RelativePath,
		Frontmatter:  targetFM,
		Content:      sb.String(),
	}
}

func generateSidebarJSON(files []string) string {
	sort.Strings(files)
	sidebar := map[string]interface{}{
		"docsSidebar": files,
	}
	data, _ := json.MarshalIndent(sidebar, "", "  ")
	return string(data)
}

// ApplyReflection executes a reflection plan by writing files to the target directory.
func ApplyReflection(plan *ReflectionPlan) (*ReflectionResult, error) {
	if plan == nil {
		return nil, ErrInvalidBlueprint
	}

	start := time.Now()
	var written []string

	for _, file := range plan.Files {
		destPath := filepath.Join(plan.TargetDocsDir, file.RelativePath)
		dir := filepath.Dir(destPath)
		if err := os.MkdirAll(dir, 0755); err != nil {
			return nil, err
		}

		if err := os.WriteFile(destPath, []byte(file.Content), 0644); err != nil {
			return nil, err
		}
		written = append(written, destPath)
	}

	synced := false
	if plan.SidebarConfig != "" {
		sidebarPath := filepath.Join(plan.TargetDocsDir, "sidebars.json")
		if err := os.WriteFile(sidebarPath, []byte(plan.SidebarConfig), 0644); err == nil {
			synced = true
		}
	}

	return &ReflectionResult{
		FilesWritten:  written,
		SidebarSynced: synced,
		DurationMs:    time.Since(start).Milliseconds(),
	}, nil
}

// ReflectTaxonomy extracts a blueprint from refDocsDir, contextualizes it against targetSourceDir,
// and writes the reflected documentation suite to targetDocsDir.
func ReflectTaxonomy(refDocsDir, targetSourceDir, targetDocsDir string) (*ReflectionResult, error) {
	blueprint, err := ExtractBlueprint(refDocsDir)
	if err != nil {
		return nil, err
	}

	plan, err := PlanReflection(blueprint, targetSourceDir, targetDocsDir)
	if err != nil {
		return nil, err
	}

	return ApplyReflection(plan)
}
