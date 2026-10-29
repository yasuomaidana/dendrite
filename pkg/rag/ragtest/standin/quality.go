package main

import (
	"encoding/json"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// quality answers the quality.* methods with simple, real computations over the project's files, enough to
// exercise a client: non-blank lines of .go and .rs files for loc, files of identical non-blank content for
// clones, and one violation per Rust file holding the marker REPEATED_TEST for rstest.
func quality(method, root string, raw json.RawMessage) map[string]interface{} {
	var q struct {
		MaxProductionLines int      `json:"max_production_lines"`
		WarnAt             int      `json:"warn_at"`
		MaxPercent         float64  `json:"max_percent"`
		MinLines           int      `json:"min_lines"`
		Exclude            []string `json:"exclude"`
	}
	_ = json.Unmarshal(raw, &q)
	type file struct {
		path, text string
		lines      int
	}
	var files []file
	_ = filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() || !(strings.HasSuffix(path, ".go") || strings.HasSuffix(path, ".rs")) {
			return nil
		}
		data, _ := os.ReadFile(path)
		rel, _ := filepath.Rel(root, path)
		n := 0
		for _, l := range strings.Split(string(data), "\n") {
			if strings.TrimSpace(l) != "" {
				n++
			}
		}
		files = append(files, file{filepath.ToSlash(rel), strings.TrimSpace(string(data)), n})
		return nil
	})
	switch method {
	case "quality.loc":
		if q.MaxProductionLines == 0 {
			q.MaxProductionLines, q.WarnAt = 500, 450
		}
		rows, failures := []map[string]interface{}{}, 0
		for _, f := range files {
			status := "ok"
			if f.lines > q.MaxProductionLines {
				status, failures = "fail", failures+1
			} else if f.lines > q.WarnAt {
				status = "warn"
			}
			rows = append(rows, map[string]interface{}{"path": f.path, "language": "go", "production": f.lines, "test": 0, "comment": 0, "blank": 0, "status": status, "parse_errors": false})
		}
		return map[string]interface{}{"schema": 1, "max_production_lines": q.MaxProductionLines, "warn_at": q.WarnAt,
			"totals": map[string]int{"files": len(files), "failures": failures}, "files": rows}
	case "quality.clones":
		byText := map[string][]string{}
		for _, f := range files {
			if f.lines >= q.MinLines {
				byText[f.text] = append(byText[f.text], f.path)
			}
		}
		groups, dup, total := []map[string]interface{}{}, 0, 0
		for _, f := range files {
			total += f.lines
		}
		paths := make([]string, 0, len(byText))
		for text := range byText {
			paths = append(paths, byText[text][0])
		}
		sort.Strings(paths)
		for _, first := range paths {
			for text, same := range byText {
				if len(same) < 2 || same[0] != first {
					continue
				}
				locs := []map[string]interface{}{}
				for _, p := range same {
					locs = append(locs, map[string]interface{}{"path": p, "start_line": 1, "end_line": len(strings.Split(text, "\n"))})
					dup += len(strings.Split(text, "\n"))
				}
				groups = append(groups, map[string]interface{}{"id": len(groups) + 1, "tokens": 100, "locations": locs})
			}
		}
		percent := 0.0
		if total > 0 {
			percent = float64(dup) * 100 / float64(total)
		}
		return map[string]interface{}{"schema": 1, "max_percent": q.MaxPercent, "totals": map[string]interface{}{"files": len(files), "lines": total, "duplicated_lines": dup, "percent": percent, "groups": len(groups)}, "groups": groups}
	default: // quality.rstest
		rust, violations := 0, []map[string]interface{}{}
		for _, f := range files {
			if strings.HasSuffix(f.path, ".rs") {
				rust++
				if strings.Contains(f.text, "REPEATED_TEST") {
					violations = append(violations, map[string]interface{}{"kind": "repeated_test", "path": f.path, "line": 1, "message": "a and b differ only in constants"})
				}
			}
		}
		return map[string]interface{}{"schema": 1, "files": rust, "violations": violations}
	}
}
