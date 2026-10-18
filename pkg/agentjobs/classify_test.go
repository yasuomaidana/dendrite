package agentjobs

import (
	"testing"
)

func TestClassify_RealSnippetsFromArchitectureDoc(t *testing.T) {
	tests := []struct {
		name     string
		outcome  Outcome
		expected Class
	}{
		{
			name: "DEN-982 first run: exit 3 + EOF + network issue",
			outcome: Outcome{
				ExitCode:       3,
				LogTail:        `AGY_ERROR {"short_error":"…request failed: Post …: EOF","status":"UNKNOWN"…} network issue connecting to the server`,
				ChangedInScope: false,
			},
			expected: TRANSIENT,
		},
		{
			name: "audit batches / DEN-900 first run: permission auto-denied",
			outcome: Outcome{
				ExitCode:       0,
				LogTail:        `jetski: no output produced — a tool required the "command" permission … auto-denied`,
				ChangedInScope: false,
			},
			expected: PERMISSION,
		},
		{
			name: "permission with quoted command without auto-denied token",
			outcome: Outcome{
				ExitCode:       1,
				LogTail:        `Action blocked: required the "command" permission to proceed`,
				ChangedInScope: false,
			},
			expected: PERMISSION,
		},
		{
			name: "permission with unquoted command without auto-denied token",
			outcome: Outcome{
				ExitCode:       1,
				LogTail:        `Action blocked: required the command permission to proceed`,
				ChangedInScope: false,
			},
			expected: PERMISSION,
		},
		{
			name: "DEN-910 first run: exit 0 + launched a subagent and no file changes",
			outcome: Outcome{
				ExitCode:       0,
				LogTail:        `I have launched a research subagent to investigate the ticket.`,
				ChangedInScope: false,
			},
			expected: EARLY_EXIT,
		},
		{
			name: "most jobs: exit 0 + changes inside file scope",
			outcome: Outcome{
				ExitCode:       0,
				LogTail:        `Completed all requested modifications in pkg/agentjobs/classify.go`,
				ChangedInScope: true,
			},
			expected: DONE,
		},
		{
			name: "Gemini / AGY: RESOURCE_EXHAUSTED",
			outcome: Outcome{
				ExitCode:       1,
				LogTail:        `googleapi: Error 403: RESOURCE_EXHAUSTED, project quota limit exceeded`,
				ChangedInScope: false,
			},
			expected: QUOTA,
		},
		{
			name: "Gemini / AGY: HTTP 429",
			outcome: Outcome{
				ExitCode:       1,
				LogTail:        `HTTP 429 Too Many Requests: please wait before retrying`,
				ChangedInScope: false,
			},
			expected: QUOTA,
		},
		{
			name: "Gemini / AGY: quota keyword",
			outcome: Outcome{
				ExitCode:       1,
				LogTail:        `Error: Daily quota exhausted for this account`,
				ChangedInScope: false,
			},
			expected: QUOTA,
		},
		{
			name: "Gemini / AGY: exhausted your capacity",
			outcome: Outcome{
				ExitCode:       1,
				LogTail:        `You have exhausted your capacity for model gemini-1.5-pro in region us-central1`,
				ChangedInScope: false,
			},
			expected: QUOTA,
		},
		{
			name: "LiteLLM: RateLimitError",
			outcome: Outcome{
				ExitCode:       1,
				LogTail:        `litellm.RateLimitError: Rate limit reached for model gpt-4o`,
				ChangedInScope: false,
			},
			expected: QUOTA,
		},
		{
			name: "LiteLLM: BudgetExceededError",
			outcome: Outcome{
				ExitCode:       1,
				LogTail:        `litellm.BudgetExceededError: Virtual key max spend cap exceeded`,
				ChangedInScope: false,
			},
			expected: QUOTA,
		},
		{
			name: "Local engines: connection refused",
			outcome: Outcome{
				ExitCode:       1,
				LogTail:        `dial tcp 127.0.0.1:11434: connect: connection refused`,
				ChangedInScope: false,
			},
			expected: TRANSIENT,
		},
		{
			name: "Internal quota_manager log line (with changes in scope) MUST NOT classify as QUOTA",
			outcome: Outcome{
				ExitCode:       0,
				LogTail:        `2026/10/17 18:00:00 quota_manager.go:42: doRefreshQuota: checking remaining capacity`,
				ChangedInScope: true,
			},
			expected: DONE,
		},
		{
			name: "Internal quota_manager log line (no changes in scope) MUST NOT classify as QUOTA",
			outcome: Outcome{
				ExitCode:       0,
				LogTail:        `quota_manager.go:105 doRefreshQuota finished without errors`,
				ChangedInScope: false,
			},
			expected: EARLY_EXIT,
		},
		{
			name: "quota_manager reporting genuine quota exhaustion MUST classify as QUOTA",
			outcome: Outcome{
				ExitCode:       1,
				LogTail:        `quota_manager.go:88: doRefreshQuota: organization quota exceeded`,
				ChangedInScope: false,
			},
			expected: QUOTA,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := Classify(tc.outcome)
			if got != tc.expected {
				t.Errorf("Classify() = %q, want %q", got, tc.expected)
			}
		})
	}
}

func TestClassify_OrderOfPrecedence(t *testing.T) {
	tests := []struct {
		name     string
		outcome  Outcome
		expected Class
	}{
		{
			name: "QUOTA takes precedence over PERMISSION",
			outcome: Outcome{
				ExitCode:       1,
				LogTail:        `RateLimitError: rate limit reached but also auto-denied`,
				ChangedInScope: false,
			},
			expected: QUOTA,
		},
		{
			name: "QUOTA takes precedence over TRANSIENT (exit 3)",
			outcome: Outcome{
				ExitCode:       3,
				LogTail:        `RESOURCE_EXHAUSTED: server returned code 429`,
				ChangedInScope: false,
			},
			expected: QUOTA,
		},
		{
			name: "QUOTA takes precedence over DONE",
			outcome: Outcome{
				ExitCode:       1,
				LogTail:        `Changes made successfully but BudgetExceededError hit`,
				ChangedInScope: true,
			},
			expected: QUOTA,
		},
		{
			name: "PERMISSION takes precedence over TRANSIENT (exit 3)",
			outcome: Outcome{
				ExitCode:       3,
				LogTail:        `command execution was auto-denied by safety guard`,
				ChangedInScope: false,
			},
			expected: PERMISSION,
		},
		{
			name: "PERMISSION takes precedence over EARLY_EXIT",
			outcome: Outcome{
				ExitCode:       0,
				LogTail:        `jetski: tool required the "command" permission and was auto-denied`,
				ChangedInScope: false,
			},
			expected: PERMISSION,
		},
		{
			name: "PERMISSION takes precedence over DONE",
			outcome: Outcome{
				ExitCode:       0,
				LogTail:        `tool required the command permission to commit`,
				ChangedInScope: true,
			},
			expected: PERMISSION,
		},
		{
			name: "TRANSIENT takes precedence over EARLY_EXIT",
			outcome: Outcome{
				ExitCode:       3,
				LogTail:        `no output produced`,
				ChangedInScope: false,
			},
			expected: TRANSIENT,
		},
		{
			name: "TRANSIENT takes precedence over DONE",
			outcome: Outcome{
				ExitCode:       1,
				LogTail:        `changes made but network issue connecting to the server`,
				ChangedInScope: true,
			},
			expected: TRANSIENT,
		},
		{
			name: "EARLY_EXIT (subagent delegation) takes precedence over DONE",
			outcome: Outcome{
				ExitCode:       0,
				LogTail:        `I launched a subagent to work on the files.`,
				ChangedInScope: true,
			},
			expected: EARLY_EXIT,
		},
		{
			name: "EARLY_EXIT: exit 0 without changes in scope",
			outcome: Outcome{
				ExitCode:       0,
				LogTail:        `Checked code, no changes needed.`,
				ChangedInScope: false,
			},
			expected: EARLY_EXIT,
		},
		{
			name: "DONE: exit 0 with changes in scope",
			outcome: Outcome{
				ExitCode:       0,
				LogTail:        `Successfully modified files.`,
				ChangedInScope: true,
			},
			expected: DONE,
		},
		{
			name: "Unclassified outcome",
			outcome: Outcome{
				ExitCode:       1,
				LogTail:        `fatal error: syntax error in main.go:12`,
				ChangedInScope: false,
			},
			expected: "",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := Classify(tc.outcome)
			if got != tc.expected {
				t.Errorf("Classify() = %q, want %q", got, tc.expected)
			}
		})
	}
}

func TestClassify_TransientHTTP5xxAndMarkers(t *testing.T) {
	tests := []struct {
		name     string
		logTail  string
		expected Class
	}{
		{name: "HTTP 500", logTail: "request failed: HTTP 500 Internal Server Error", expected: TRANSIENT},
		{name: "AGY stream interrupted (real, DEN-910 fix run)", logTail: `AGY_ERROR: {"short_error":"The stream was interrupted. Please continue the task you were working on.","retryable":false}`, expected: TRANSIENT},
		{name: "502 Bad Gateway", logTail: "Gateway failed: 502 Bad Gateway", expected: TRANSIENT},
		{name: "503 Service Unavailable", logTail: "Error: 503 Service Unavailable", expected: TRANSIENT},
		{name: "504 Gateway Timeout", logTail: "upstream response: 504 Gateway Timeout", expected: TRANSIENT},
		{name: "HTTP 5xx marker", logTail: "server returned HTTP 5xx series code", expected: TRANSIENT},
		{name: "status 500", logTail: "API response status: 500", expected: TRANSIENT},
		{name: "bad gateway phrase", logTail: "bad gateway from edge proxy", expected: TRANSIENT},
		{name: "service unavailable phrase", logTail: "service unavailable currently", expected: TRANSIENT},
		{name: "gateway timeout phrase", logTail: "gateway timeout reached", expected: TRANSIENT},
		{name: "internal server error phrase", logTail: "internal server error encountered", expected: TRANSIENT},
		{name: "EOF marker", logTail: "read tcp 127.0.0.1:4000: EOF", expected: TRANSIENT},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := Classify(Outcome{ExitCode: 1, LogTail: tc.logTail, ChangedInScope: false})
			if got != tc.expected {
				t.Errorf("Classify(%q) = %q, want %q", tc.logTail, got, tc.expected)
			}
		})
	}
}

func TestClassify_EmptyLogTail(t *testing.T) {
	// Empty log tail with ExitCode 0 and ChangedInScope false -> EARLY_EXIT
	if got := Classify(Outcome{ExitCode: 0, LogTail: "", ChangedInScope: false}); got != EARLY_EXIT {
		t.Errorf("Classify(exit 0, empty log, no changes) = %q, want %q", got, EARLY_EXIT)
	}

	// Empty log tail with ExitCode 0 and ChangedInScope true -> DONE
	if got := Classify(Outcome{ExitCode: 0, LogTail: "", ChangedInScope: true}); got != DONE {
		t.Errorf("Classify(exit 0, empty log, changes) = %q, want %q", got, DONE)
	}

	// Empty log tail with ExitCode 3 -> TRANSIENT
	if got := Classify(Outcome{ExitCode: 3, LogTail: "", ChangedInScope: false}); got != TRANSIENT {
		t.Errorf("Classify(exit 3, empty log) = %q, want %q", got, TRANSIENT)
	}

	// Empty log tail with ExitCode 1 -> unclassified
	if got := Classify(Outcome{ExitCode: 1, LogTail: "", ChangedInScope: false}); got != "" {
		t.Errorf("Classify(exit 1, empty log) = %q, want empty", got)
	}
}

func TestClassify_HTTPStatus429AndRateLimitPhrase(t *testing.T) {
	if got := Classify(Outcome{ExitCode: 429}); got != QUOTA {
		t.Errorf("exit code 429 = %q, want QUOTA", got)
	}
	if got := Classify(Outcome{ExitCode: 1, LogTail: `{"error":"rate limit"}`}); got != QUOTA {
		t.Errorf("rate limit body = %q, want QUOTA", got)
	}
}
