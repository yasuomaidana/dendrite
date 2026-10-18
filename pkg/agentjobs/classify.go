package agentjobs

import (
	"regexp"
	"strings"
)

// Outcome represents the execution outcome of an agent invocation.
type Outcome struct {
	ExitCode       int
	LogTail        string
	ChangedInScope bool
}

// Signature maps a matching predicate to a classification Class.
type Signature struct {
	Class       Class
	Description string
	Match       func(o Outcome) bool
}

var (
	reEOF = regexp.MustCompile(`(?i)\bEOF\b`)
	re5xx = regexp.MustCompile(`(?i)(\b5xx\b|\bhttp\s*5\d{2}\b|\bstatus[:\s]+5\d{2}\b|internal\s+server\s+error|bad\s+gateway|service\s+unavailable|gateway\s+timeout)`)
)

// matchQuota checks for quota exhaustion signatures (Gemini/AGY, LiteLLM, Copilot),
// ensuring internal quota manager log lines like "quota_manager.go ... doRefreshQuota"
// are excluded unless an actual quota exhaustion error is present.
func matchQuota(o Outcome) bool {
	// HTTP agents (oxillama, LiteLLM) report the response status as the exit code.
	if o.ExitCode == 429 {
		return true
	}
	if o.LogTail == "" {
		return false
	}

	tailLower := strings.ToLower(o.LogTail)

	// Unambiguous quota exhaustion signatures
	if strings.Contains(tailLower, "resource_exhausted") ||
		strings.Contains(tailLower, "429") ||
		strings.Contains(tailLower, "exhausted your capacity") ||
		strings.Contains(tailLower, "ratelimiterror") ||
		strings.Contains(tailLower, "rate limit") ||
		strings.Contains(tailLower, "budgetexceedederror") {
		return true
	}

	// Line-by-line inspection for "quota" keyword to filter benign quota_manager logs
	lines := strings.Split(o.LogTail, "\n")
	for _, line := range lines {
		l := strings.ToLower(line)
		if !strings.Contains(l, "quota") {
			continue
		}

		// Strip benign quota manager tokens
		cleaned := strings.ReplaceAll(l, "quota_manager.go", "")
		cleaned = strings.ReplaceAll(cleaned, "quota_manager", "")
		cleaned = strings.ReplaceAll(cleaned, "dorefreshquota", "")

		if strings.Contains(cleaned, "quota") {
			return true
		}
	}

	return false
}

// matchPermission checks if execution was aborted due to missing command permissions.
func matchPermission(o Outcome) bool {
	tailLower := strings.ToLower(o.LogTail)
	if strings.Contains(tailLower, "auto-denied") {
		return true
	}
	if strings.Contains(tailLower, `required the "command" permission`) ||
		strings.Contains(tailLower, "required the command permission") {
		return true
	}
	return false
}

// matchTransient checks for network, EOF, or HTTP 5xx transient server issues, or exit code 3.
func matchTransient(o Outcome) bool {
	if o.ExitCode == 3 {
		return true
	}
	if reEOF.MatchString(o.LogTail) {
		return true
	}
	tailLower := strings.ToLower(o.LogTail)
	// "stream was interrupted" is reported by AGY with "retryable": false, yet the task can be continued.
	if strings.Contains(tailLower, "network issue") ||
		strings.Contains(tailLower, "connection refused") ||
		strings.Contains(tailLower, "stream was interrupted") {
		return true
	}
	if re5xx.MatchString(o.LogTail) {
		return true
	}
	return false
}

// matchEarlyExit checks for premature exits (exit 0 without changes in scope, or delegation to subagents).
func matchEarlyExit(o Outcome) bool {
	tailLower := strings.ToLower(o.LogTail)
	if strings.Contains(tailLower, "launched a") && strings.Contains(tailLower, "subagent") {
		return true
	}
	if o.ExitCode == 0 && !o.ChangedInScope {
		return true
	}
	return false
}

// matchDone checks for successful completion with changes inside file scope.
func matchDone(o Outcome) bool {
	return o.ExitCode == 0 && o.ChangedInScope
}

// Signatures is the authoritative ordered table of outcome signatures.
// Precedence order: QUOTA, PERMISSION, TRANSIENT, EARLY_EXIT, DONE.
var Signatures = []Signature{
	{
		Class:       QUOTA,
		Description: "Quota exhaustion (RESOURCE_EXHAUSTED, 429, RateLimitError, BudgetExceededError, exhausted capacity)",
		Match:       matchQuota,
	},
	{
		Class:       PERMISSION,
		Description: "Permission auto-denial (auto-denied command execution)",
		Match:       matchPermission,
	},
	{
		Class:       TRANSIENT,
		Description: "Transient failures (exit code 3, EOF, network issues, HTTP 5xx)",
		Match:       matchTransient,
	},
	{
		Class:       EARLY_EXIT,
		Description: "Early exits (exit 0 without scope changes, or subagent delegation)",
		Match:       matchEarlyExit,
	},
	{
		Class:       DONE,
		Description: "Successful completion with scope changes (exit 0 with scope changes)",
		Match:       matchDone,
	},
}

// Classify categorizes an agent execution outcome against the ordered signatures table.
func Classify(o Outcome) Class {
	// A clean exit without an agent error envelope is a finished turn: quota/5xx words in the
	// agent's own summary (e.g. an agent implementing quota handling) are content, not failures.
	clean := o.ExitCode == 0 && !strings.Contains(o.LogTail, "AGY_ERROR")
	for _, sig := range Signatures {
		if clean && (sig.Class == QUOTA || sig.Class == TRANSIENT) {
			continue
		}
		if sig.Match(o) {
			return sig.Class
		}
	}
	return ""
}
