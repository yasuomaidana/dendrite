package immune

import (
	"encoding/json"
	"fmt"
)

// Issue is one finding of the scanner, reduced to what a reader of the gate output needs.
type Issue struct {
	ID          string `json:"id"`
	Category    string `json:"category"`
	Pattern     string `json:"pattern"`
	Severity    string `json:"severity"`
	File        string `json:"file"`
	Line        int    `json:"line"`
	Remediation string `json:"remediation,omitempty"`
}

// Report is the gate's answer for one scanned path.
type Report struct {
	Path           string  `json:"path"`
	Skill          string  `json:"skill"`
	ScannerVersion string  `json:"scanner_version"`
	Score          int     `json:"score"`
	Severity       string  `json:"severity"`
	Verdict        Verdict `json:"verdict"`
	Issues         []Issue `json:"issues"`
}

// rawReport is the part of SkillSpector's JSON report the gate reads.
type rawReport struct {
	Skill struct {
		Name string `json:"name"`
	} `json:"skill"`
	Risk struct {
		Score    *int   `json:"score"`
		Severity string `json:"severity"`
	} `json:"risk_assessment"`
	Issues []struct {
		ID          string `json:"id"`
		Category    string `json:"category"`
		Pattern     string `json:"pattern"`
		Severity    string `json:"severity"`
		Remediation string `json:"remediation"`
		Location    struct {
			File      string `json:"file"`
			StartLine int    `json:"start_line"`
		} `json:"location"`
	} `json:"issues"`
}

// ParseReport reads SkillSpector's JSON report. A report without a risk score is an error, never a pass.
func ParseReport(data []byte, t Thresholds) (Report, error) {
	var raw rawReport
	if err := json.Unmarshal(data, &raw); err != nil {
		return Report{}, fmt.Errorf("scanner report is not valid JSON: %w", err)
	}
	if raw.Risk.Score == nil {
		return Report{}, fmt.Errorf("scanner report has no risk_assessment.score")
	}
	r := Report{Skill: raw.Skill.Name, Score: *raw.Risk.Score, Severity: raw.Risk.Severity, Issues: []Issue{}}
	r.Verdict = t.Classify(r.Score)
	for _, i := range raw.Issues {
		r.Issues = append(r.Issues, Issue{
			ID: i.ID, Category: i.Category, Pattern: i.Pattern, Severity: i.Severity,
			File: i.Location.File, Line: i.Location.StartLine, Remediation: i.Remediation,
		})
	}
	return r, nil
}
