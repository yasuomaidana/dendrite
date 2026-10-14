package triage

import (
	"strings"
)

var choreKeywords = []string{
	"format", "formatting", "lint", "linter", "typo", "docstring", "comment",
	"unit test vector", "unit test boilerplate", "test case vector", "test boilerplate",
	"boilerplate", "syntax error", "ast query", "symbol query", "explain error",
	"rename variable", "generate mock", "mock struct", "fix typo", "add struct tags",
	"json tag", "yaml tag", "getter", "setter", "simple test",
}

var macroKeywords = []string{
	"macro architecture", "system design", "distributed consensus", "threat model",
	"protocol specification", "formal verification", "formal proof", "deadlock proof",
	"cross-package contract", "cross-system migration", "strategic architecture",
	"enterprise architecture redesign", "cryptographic protocol",
}

// ClassifyTaskComplexity determines whether a prompt represents a routine chore,
// an architectural macro design, or a hybrid feature.
func ClassifyTaskComplexity(prompt string) (ExecutionTier, string) {
	lower := strings.ToLower(prompt)

	// Check macro keywords first
	for _, kw := range macroKeywords {
		if strings.Contains(lower, kw) {
			return TierCloud, "Task involves macro-system architecture or complex cross-system design; assigned to high-cognition cloud model."
		}
	}

	// Check chore keywords
	choreCount := 0
	for _, kw := range choreKeywords {
		if strings.Contains(lower, kw) {
			choreCount++
		}
	}

	if choreCount > 0 {
		return TierLocal, "Task identified as routine chore/developer maintenance; routed to zero-cost local inference to save cloud tokens."
	}

	// Default to Hybrid (local preprocessing, skill selection, context compaction, then cloud execution)
	return TierHybrid, "Task involves feature implementation or multi-step logic; utilizing hybrid pipeline with local triage and context compaction."
}
