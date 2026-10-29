package directive

import (
	"regexp"
	"strconv"
)

var (
	maxLines    = regexp.MustCompile(`(?i)\b(?:max(?:imum)?|at most|up to|no (?:file )?(?:over|above|longer than)|limit(?:ed)? (?:files? )?to)\s+(\d+)\s+(?:production\s+)?(?:lines?|loc)\b`)
	coverageNum = regexp.MustCompile(`(?i)(\d+(?:\.\d+)?)\s*(?:%|percent)\s*(?:of\s+)?(?:code\s+|test\s+)?coverage|coverage\s+(?:of|at|to|above|at least|>=)\s*(\d+(?:\.\d+)?)\s*(?:%|percent)?`)
	warnAt      = regexp.MustCompile(`(?i)\bwarn(?:ing|s)?\s+(?:at|from|above|over)\s+(\d+)\b`)
	noDuplicate = regexp.MustCompile(`(?i)\b(?:zero|no|without|0\s*%?)\s+(?:duplicat\w*|clones?|copy[- ]?paste)`)
	dupPercent  = regexp.MustCompile(`(?i)(?:up to|at most|max(?:imum)?|allow)\s+(\d+(?:\.\d+)?)\s*(?:%|percent)\s+(?:of\s+)?(?:code\s+)?duplicat\w*`)
	autoDoneOn  = regexp.MustCompile(`(?i)\b(?:allow|let|enable)s?\s+agents?\s+(?:to\s+)?(?:set\s+)?done|agents?\s+may\s+set\s+done|auto[- ]?done\s+(?:on|enabled)`)
	autoDoneOff = regexp.MustCompile(`(?i)\brequire\s+human\s+(?:review|sign[- ]?off)|agents?\s+(?:never|must not|may not|cannot)\s+set\s+done|(?:disable|turn off)\s+auto[- ]?done|auto[- ]?done\s+off|humans?\s+sign\s+off`)
)

// Parse reads the rules it knows from a sentence ("enforce max 500 lines of code ignoring tests", "enforce 95%
// code coverage", "enforce zero duplicated code", "allow agents to set done", "require human review"). One
// sentence can hold several. No change means the sentence is not one it knows.
func Parse(text string) []Change {
	var out []Change
	if m := maxLines.FindStringSubmatch(text); m != nil {
		n, _ := strconv.Atoi(m[1])
		out = append(out, Change{Key: "loc.max_production_lines", Value: n, Reason: "a limit on lines per file"})
		if m := warnAt.FindStringSubmatch(text); m != nil {
			w, _ := strconv.Atoi(m[1])
			out = append(out, Change{Key: "loc.warn_at", Value: w, Reason: "where the warning starts"})
		} else {
			out = append(out, Change{Key: "loc.warn_at", Value: n * 9 / 10, Reason: "warn at 90% of the limit"})
		}
	}
	if m := coverageNum.FindStringSubmatch(text); m != nil {
		n, _ := strconv.ParseFloat(firstNonEmpty(m[1], m[2]), 64)
		out = append(out, Change{Key: "coverage.min_percent", Value: n, Reason: "a coverage floor"})
	}
	if m := dupPercent.FindStringSubmatch(text); m != nil {
		n, _ := strconv.ParseFloat(m[1], 64)
		out = append(out, Change{Key: "duplication.max_percent", Value: n, Reason: "a duplication allowance"})
	} else if noDuplicate.MatchString(text) {
		out = append(out, Change{Key: "duplication.max_percent", Value: 0.0, Reason: "no duplicated code"})
	}
	switch {
	case autoDoneOff.MatchString(text):
		out = append(out, Change{Key: "done.auto_done", Value: false, Reason: "a person signs off"})
	case autoDoneOn.MatchString(text):
		out = append(out, Change{Key: "done.auto_done", Value: true, Reason: "agents may set Done"})
	}
	return out
}

func firstNonEmpty(a, b string) string {
	if a != "" {
		return a
	}
	return b
}
