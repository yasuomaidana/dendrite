package triage

import (
	"strings"
)

// DefaultSkillMatchers returns the standard set of skills recognized in the project.
func DefaultSkillMatchers() []SkillMatcher {
	return []SkillMatcher{
		{
			Name:        "ticket-lifecycle-developer",
			Description: "Guides end-to-end development lifecycle for Plane v2 tickets, TDD specs, and status synchronization.",
			Keywords:    []string{"ticket", "plane", "issue", "den-", "status", "milestone", "lifecycle", "acceptance criteria"},
			Guideline:   "Follow the 8-phase ticket lifecycle: author TDD tests first, check Plane status authoritatively, ensure statement coverage >= 85%, and sync status.",
		},
		{
			Name:        "pattern-complexity-auditor",
			Description: "Audits idiomatic Go/Rust design patterns, cyclomatic complexity (<= 15), and cognitive complexity (<= 20).",
			Keywords:    []string{"complexity", "cyclomatic", "refactor", "cognitive", "pattern", "clean code", "code smell", "solid"},
			Guideline:   "Enforce cyclomatic complexity <= 15 per function, extract helpers, use early returns, and follow idiomatic interface composition.",
		},
		{
			Name:        "crate-compatibility",
			Description: "Audits dependencies and external packages for version conflicts and cross-library compatibility.",
			Keywords:    []string{"dependency", "dependencies", "version", "upgrade", "go.mod", "cargo", "conflict", "compatibility", "package"},
			Guideline:   "Verify mutual compatibility between dependencies, ensure reproducible lockfiles, and check breaking API changes.",
		},
		{
			Name:        "ai-practices-auditor",
			Description: "Audits AI prompt templates, schema compliance, token economy, and deterministic guardrails.",
			Keywords:    []string{"token", "prompt", "guardrail", "schema", "llm", "deterministic", "ai practice"},
			Guideline:   "Optimize prompt token footprint, avoid prompt bloat, enforce JSON-RPC 2.0 schemas, and adhere to deterministic tool outputs.",
		},
		{
			Name:        "scrum-master-alignment",
			Description: "Evaluates alignment of tests, coverage metrics, and features with targeted user acceptance criteria.",
			Keywords:    []string{"acceptance criteria", "user story", "scrum", "definition of done", "coverage target", "business goal"},
			Guideline:   "Map test scenarios directly to user acceptance criteria (AC-1..N) and verify minimum 85% test coverage before completion.",
		},
		{
			Name:        "compliance",
			Description: "Enterprise Architecture and compliance reference framework.",
			Keywords:    []string{"compliance", "enterprise", "governance", "audit", "security review", "rbac", "soc2"},
			Guideline:   "Adhere to zero-ingress container network policies, ephemeral scratch volumes, and auditable git commit provenance.",
		},
		{
			Name:        "dendrite-workflow",
			Description: "Dendrite-first workflow for every model: session resume/checkpoint, dendrite-work/ working copies, dendrite commit, and delegation through Dendrite.",
			Keywords:    []string{"worktree", "clone", "working copy", "dendrite-work", "isolate", "checkpoint", "session resume", "delegate", "antigravity", "agy"},
			Guideline:   "Create isolated checkouts only with `dendrite work new <ticket>` (never hand-made clones or sibling worktrees), commit with `dendrite commit`, delegate to other models with `dendrite agent jobs submit --isolate`, and route LLM calls through the LiteLLM gateway.",
		},
	}
}

// MatchSkills identifies relevant project skills based on prompt keywords.
func MatchSkills(prompt string, matchers []SkillMatcher) []SkillMatcher {
	if len(matchers) == 0 {
		matchers = DefaultSkillMatchers()
	}

	lowerPrompt := strings.ToLower(prompt)
	var matched []SkillMatcher

	for _, sm := range matchers {
		for _, kw := range sm.Keywords {
			if strings.Contains(lowerPrompt, kw) {
				matched = append(matched, sm)
				break
			}
		}
	}
	return matched
}
