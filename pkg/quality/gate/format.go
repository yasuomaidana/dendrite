package gate

import (
	"encoding/json"
	"fmt"
	"strings"
)

// JSON is the report as indented JSON.
func (r Report) JSON() string {
	out, _ := json.MarshalIndent(r, "", "  ")
	return string(out)
}

var marks = map[Status]string{Pass: "PASS", Fail: "FAIL", Skipped: "skip", Error: "ERROR"}

// Text is one line per check and its findings, for a terminal.
func (r Report) Text() string {
	var b strings.Builder
	for _, c := range r.Checks {
		fmt.Fprintf(&b, "%-5s %-9s %s\n", marks[c.Status], c.Name, c.Summary)
		for _, f := range c.Findings {
			fmt.Fprintf(&b, "        - %s\n", f)
		}
	}
	fmt.Fprintf(&b, "quality gate (%s): %s\n", r.Scope, map[bool]string{true: "passed", false: "FAILED"}[r.Passed])
	return b.String()
}

// Markdown is the report for a ticket comment or a pull request.
func (r Report) Markdown() string {
	var b strings.Builder
	fmt.Fprintf(&b, "Quality gate (%s): **%s**\n\n", r.Scope, map[bool]string{true: "passed", false: "failed"}[r.Passed])
	for _, c := range r.Checks {
		fmt.Fprintf(&b, "- `%s` %s: %s\n", c.Name, c.Status, c.Summary)
		for _, f := range c.Findings {
			fmt.Fprintf(&b, "  - %s\n", f)
		}
	}
	return b.String()
}
