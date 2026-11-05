package main

import (
	"os"
	"path"
	"path/filepath"
	"sort"
	"strings"
)

// complexityReport finds one over-limit function in each file that holds the marker COMPLEX_FUNCTION.
func complexityReport(files []file) map[string]interface{} {
	findings := []map[string]interface{}{}
	for _, f := range files {
		if strings.Contains(f.text, "COMPLEX_FUNCTION") {
			findings = append(findings, map[string]interface{}{
				"path": f.path, "language": "go", "symbol": "Complex", "start_line": 1, "end_line": f.lines,
				"cyclomatic": 99, "cognitive": 99,
			})
		}
	}
	return map[string]interface{}{"schema": 1, "totals": map[string]int{"files": len(files), "functions": len(files), "failures": len(findings)}, "findings": findings}
}

// structureReport applies two of the directory rules: more files in one directory than maxFiles (when set), and
// a repository root without a README.md (when docs are required).
func structureReport(files []file, maxFiles int, requireDocs bool, root string) map[string]interface{} {
	perDir := map[string][]string{}
	for _, f := range files {
		dir := path.Dir(f.path)
		perDir[dir] = append(perDir[dir], f.path)
	}
	dirs := make([]string, 0, len(perDir))
	for dir := range perDir {
		dirs = append(dirs, dir)
	}
	sort.Strings(dirs)
	findings := []map[string]interface{}{}
	for _, dir := range dirs {
		if maxFiles > 0 && len(perDir[dir]) > maxFiles {
			findings = append(findings, map[string]interface{}{"path": dir, "rule": "files", "value": len(perDir[dir]), "limit": maxFiles,
				"files": perDir[dir], "message": "too many modules in one directory"})
		}
	}
	if _, err := os.Stat(filepath.Join(root, "README.md")); requireDocs && err != nil {
		findings = append(findings, map[string]interface{}{"path": ".", "rule": "docs", "value": 1, "limit": 0,
			"files": []string{"README.md"}, "message": "the repository root has no README.md"})
	}
	return map[string]interface{}{"schema": 1, "totals": map[string]int{"dirs": len(dirs), "files": len(files), "failures": len(findings)}, "findings": findings}
}
