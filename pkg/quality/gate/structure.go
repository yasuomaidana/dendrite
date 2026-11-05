package gate

import (
	"context"
	"fmt"
	"strings"
)

// checkStructure fails a directory that is a pile of files (too many modules, too many lines, a group of files
// sharing a name prefix) and a project unit without its documentation; scoped to a change, only the findings
// that involve a file it touched.
func checkStructure(ctx context.Context, o Options) Check {
	var rep struct {
		Totals struct {
			Dirs, Files, Failures int
		}
		Findings []struct {
			Path    string
			Rule    string
			Value   int
			Limit   int
			Files   []string
			Message string
		}
	}
	s := o.Set.Structure
	params := map[string]interface{}{
		"max_files_per_dir": s.MaxFilesPerDir, "max_dir_production_lines": s.MaxDirProductionLines,
		"max_prefix_group": s.MaxPrefixGroup, "require_docs": s.RequireDocs, "docs_files": s.DocsFiles,
		"exclude": s.Exclude,
	}
	if err := o.Sidecar.Quality(ctx, o.Dir, "quality.structure", params, &rep); err != nil {
		return failed("structure", err)
	}
	var findings []string
	for _, f := range rep.Findings {
		if touchesAnyFile(o.Scope, f.Path, f.Files) {
			findings = append(findings, fmt.Sprintf("%s [%s %d, limit %d]: %s (%s)", f.Path, f.Rule, f.Value, f.Limit, f.Message, strings.Join(f.Files, ", ")))
		}
	}
	summary := fmt.Sprintf("%d directories, %d files: %d findings", rep.Totals.Dirs, rep.Totals.Files, len(findings))
	return advise(verdict("structure", summary, findings), structureAdvice)
}
