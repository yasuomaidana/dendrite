package gate

import (
	"context"
	"fmt"
	"path"
	"strings"
)

const (
	complexityAdvice = "split the function, extract helpers, replace nested branches with early returns or tables"
	structureAdvice  = "move modules that share a leading word into a sub-package named after it, group files by responsibility"
)

func failed(name string, err error) Check {
	return Check{Name: name, Status: Error, Summary: err.Error()}
}

func verdict(name, summary string, findings []string) Check {
	if len(findings) > 0 {
		return Check{Name: name, Status: Fail, Summary: summary, Findings: findings}
	}
	return Check{Name: name, Status: Pass, Summary: summary}
}

func advise(c Check, advice string) Check {
	if c.Status == Fail {
		c.Advice = advice
	}
	return c
}

func touchesSpan(s *Scope, file string, start, end int) bool {
	if s == nil {
		return true
	}
	if start <= 0 {
		return s.HasFile(file)
	}
	return s.Touches(file, start, max(start, end))
}

func touchesAnyFile(s *Scope, dir string, files []string) bool {
	if s == nil {
		return true
	}
	for _, f := range files {
		if s.HasFile(f) || (!strings.Contains(f, "/") && s.HasFile(path.Join(dir, f))) {
			return true
		}
	}
	return false
}

// checkLoc fails a file with more production lines than the directive allows; scoped to a change, only the
// files it touched.
func checkLoc(ctx context.Context, o Options) Check {
	var rep struct {
		Totals struct{ Files int }
		Files  []struct {
			Path       string
			Production int
			Status     string
		}
	}
	params := map[string]interface{}{
		"max_production_lines": o.Set.Loc.MaxProductionLines, "warn_at": o.Set.Loc.WarnAt, "exclude": o.Set.Loc.Exclude,
	}
	if err := o.Sidecar.Quality(ctx, o.Dir, "quality.loc", params, &rep); err != nil {
		return failed("loc", err)
	}
	var findings []string
	checked := 0
	for _, f := range rep.Files {
		if o.Scope != nil && !o.Scope.HasFile(f.Path) {
			continue
		}
		checked++
		if f.Status == "fail" {
			findings = append(findings, fmt.Sprintf("%s: %d production lines, the limit is %d", f.Path, f.Production, o.Set.Loc.MaxProductionLines))
		}
	}
	return verdict("loc", fmt.Sprintf("%d files checked, %d over %d production lines", checked, len(findings), o.Set.Loc.MaxProductionLines), findings)
}

// cloneGroup is one block of repeated code and where it appears.
type cloneGroup struct {
	ID        int
	Locations []struct {
		Path      string `json:"path"`
		StartLine int    `json:"start_line"`
		EndLine   int    `json:"end_line"`
	}
}

// finding describes the group and says whether it belongs to the change: any place in lines the change added,
// or every group when there is no scope.
func (g cloneGroup) finding(s *Scope) (text string, touched bool) {
	var places []string
	touched = s == nil
	for _, l := range g.Locations {
		places = append(places, fmt.Sprintf("%s:%d-%d", l.Path, l.StartLine, l.EndLine))
		if s != nil && s.Touches(l.Path, l.StartLine, l.EndLine) {
			touched = true
		}
	}
	return fmt.Sprintf("the same code in %s", strings.Join(places, ", ")), touched
}

// checkClones fails when repeated code exceeds the allowance; scoped to a change, when a repeated block has a
// place in lines the change added.
func checkClones(ctx context.Context, o Options) Check {
	var rep struct {
		Totals struct {
			Percent float64
			Groups  int
		}
		Groups []cloneGroup
	}
	params := map[string]interface{}{
		"max_percent": o.Set.Duplication.MaxPercent, "min_lines": o.Set.Duplication.MinLines,
		"min_tokens": o.Set.Duplication.MinTokens, "include_tests": o.Set.Duplication.IncludeTests,
	}
	if err := o.Sidecar.Quality(ctx, o.Dir, "quality.clones", params, &rep); err != nil {
		return failed("clones", err)
	}
	var findings []string
	for _, g := range rep.Groups {
		if finding, touched := g.finding(o.Scope); touched {
			findings = append(findings, finding)
		}
	}
	if o.Scope == nil && rep.Totals.Percent <= o.Set.Duplication.MaxPercent {
		findings = nil // within the allowance
	}
	summary := fmt.Sprintf("%.2f%% of the code is repeated (allowed %.2f%%) in %d groups", rep.Totals.Percent, o.Set.Duplication.MaxPercent, rep.Totals.Groups)
	if o.Scope != nil {
		summary = fmt.Sprintf("%d repeated blocks involve lines this change added", len(findings))
	}
	c := verdict("clones", summary, findings)
	if c.Status == Fail {
		c.Advice = o.Set.Duplication.StrategyAdvice()
	}
	return c
}

// checkRstest applies to Rust projects only.
func checkRstest(ctx context.Context, o Options) Check {
	var rep struct {
		Files      int
		Violations []struct {
			Kind, Path, Message string
			Line                int
		}
	}
	if err := o.Sidecar.Quality(ctx, o.Dir, "quality.rstest", nil, &rep); err != nil {
		return failed("rstest", err)
	}
	if rep.Files == 0 {
		return Check{Name: "rstest", Status: Skipped, Summary: "no Rust files"}
	}
	var findings []string
	for _, v := range rep.Violations {
		if o.Scope == nil || o.Scope.HasFile(v.Path) {
			findings = append(findings, fmt.Sprintf("%s:%d [%s] %s", v.Path, v.Line, v.Kind, v.Message))
		}
	}
	return verdict("rstest", fmt.Sprintf("%d Rust files checked, %d violations", rep.Files, len(findings)), findings)
}

// baselineFile is where the coverage ratchet keeps its per-package baseline.
