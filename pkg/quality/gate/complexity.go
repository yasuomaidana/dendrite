package gate

import (
	"context"
	"fmt"
)

// checkComplexity fails a function whose cyclomatic or cognitive complexity is above the directive; scoped to a
// change, only the functions that overlap lines it touched.
func checkComplexity(ctx context.Context, o Options) Check {
	var rep struct {
		Totals struct {
			Files, Functions, Failures int
		}
		Findings []struct {
			Path       string
			Symbol     string
			StartLine  int `json:"start_line"`
			EndLine    int `json:"end_line"`
			Cyclomatic int
			Cognitive  int
		}
	}
	params := map[string]interface{}{
		"max_cyclomatic": o.Set.Complexity.MaxCyclomatic, "max_cognitive": o.Set.Complexity.MaxCognitive,
		"exclude": o.Set.Complexity.Exclude,
	}
	if err := o.Sidecar.Quality(ctx, o.Dir, "quality.complexity", params, &rep); err != nil {
		return failed("complexity", err)
	}
	var findings []string
	for _, f := range rep.Findings {
		if touchesSpan(o.Scope, f.Path, f.StartLine, f.EndLine) {
			findings = append(findings, fmt.Sprintf("%s:%d %s: cyclomatic %d, cognitive %d", f.Path, f.StartLine, f.Symbol, f.Cyclomatic, f.Cognitive))
		}
	}
	summary := fmt.Sprintf("%d functions checked, %d over cyclomatic %d or cognitive %d",
		rep.Totals.Functions, len(findings), o.Set.Complexity.MaxCyclomatic, o.Set.Complexity.MaxCognitive)
	return advise(verdict("complexity", summary, findings), complexityAdvice)
}
