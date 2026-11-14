package coverage

import (
	"regexp"
	"sort"
	"strings"
)

// Floor lists the packages whose coverage is below min percent, worst first. A package with no statements, and
// a package exempt from measuring, is never listed.
func Floor(current map[string]PackageCoverage, min float64, exempt func(pkg string) bool) []Regression {
	var below []Regression
	for pkg, cov := range current {
		if cov.NoStatements || (exempt != nil && exempt(pkg)) {
			continue
		}
		if pct := roundPct(cov.CoveragePct); pct < min-0.001 {
			below = append(below, Regression{Package: pkg, CurrentPct: pct, BaselinePct: min})
		}
	}
	sort.Slice(below, func(i, j int) bool {
		if below[i].CurrentPct != below[j].CurrentPct {
			return below[i].CurrentPct < below[j].CurrentPct
		}
		return below[i].Package < below[j].Package
	})
	return below
}

// IsExempt reports whether a package is left out of the coverage floor: test helpers (a path segment ending in
// "test", like planetest or ragtest, or named testdata) and packages matching one of the directive's globs
// (`**` crosses directories, `*` does not).
func IsExempt(pkg string, globs []string) bool {
	for _, segment := range strings.Split(pkg, "/") {
		if strings.HasSuffix(segment, "test") || segment == "testdata" {
			return true
		}
	}
	for _, g := range globs {
		if globRegexp(g).MatchString(pkg) {
			return true
		}
	}
	return false
}

func globRegexp(glob string) *regexp.Regexp {
	var b strings.Builder
	b.WriteString("^")
	for i := 0; i < len(glob); i++ {
		switch {
		case strings.HasPrefix(glob[i:], "**/"):
			b.WriteString("(?:.*/)?")
			i += 2
		case strings.HasPrefix(glob[i:], "**"):
			b.WriteString(".*")
			i++
		case glob[i] == '*':
			b.WriteString("[^/]*")
		default:
			b.WriteString(regexp.QuoteMeta(string(glob[i])))
		}
	}
	b.WriteString("$")
	return regexp.MustCompile(b.String())
}

// withoutExempt drops the offenders that are exempt from measuring (test helpers and the directive's globs): a
// helper package is not a package that must start at 100%.
func withoutExempt(offenders []Regression, globs []string) []Regression {
	kept := offenders[:0:0]
	for _, o := range offenders {
		if !IsExempt(o.Package, globs) {
			kept = append(kept, o)
		}
	}
	return kept
}
