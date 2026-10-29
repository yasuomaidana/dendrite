// Package directive is the project's quality directives: the limits (production lines per file, coverage,
// duplication, security, who may set Done) that CI, the quality gate and the agents all enforce. They live in
// .github/dendrite/directives.toml, tracked, so CI can read them; `dendrite directive` changes them.
package directive

import (
	"fmt"
	"strings"
)

const (
	// Path is where a project keeps its directives, relative to its root.
	Path = ".github/dendrite/directives.toml"
	// Header opens every file Dendrite generates from the directives.
	Header = "# managed by dendrite — edit with `dendrite directive \"<rule>\"`"
	// SchemaVersion is the version of the file shape.
	SchemaVersion = 1
)

// Set is the whole directives file.
type Set struct {
	Schema      int         `toml:"schema"`
	Loc         Loc         `toml:"loc"`
	Coverage    Coverage    `toml:"coverage"`
	Duplication Duplication `toml:"duplication"`
	Lint        Lint        `toml:"lint"`
	Security    Security    `toml:"security"`
	Done        Done        `toml:"done"`
	Languages   Languages   `toml:"languages"`
}

// Loc limits the production lines of a file (tests, comments and blank lines do not count).
type Loc struct {
	MaxProductionLines int      `toml:"max_production_lines"`
	WarnAt             int      `toml:"warn_at"`
	Exclude            []string `toml:"exclude"`
}

// Coverage is the test coverage a project must keep.
type Coverage struct {
	MinPercent float64  `toml:"min_percent"`
	Scope      string   `toml:"scope"`
	Exclude    []string `toml:"exclude"`
}

// Duplication is the share of duplicated code a project tolerates.
type Duplication struct {
	MaxPercent   float64 `toml:"max_percent"`
	MinTokens    int     `toml:"min_tokens"`
	MinLines     int     `toml:"min_lines"`
	IncludeTests bool    `toml:"include_tests"`
	// Strategies are how repeated code is removed, in the order to try them: see StrategyAdvice.
	Strategies []string `toml:"strategies"`
}

// strategyAdvice says what each duplication strategy means.
var strategyAdvice = map[string]string{
	"helper_function": "extract the repeated logic into a helper function the copies call",
	"macro":           "where a function cannot express it (repeated items, impls, match arms, test bodies), generate the copies with a macro (macro_rules!, a decorator, a template)",
	"rstest":          "in Rust tests use the rstest crate (rstest = \"0.28.0\"): #[case] for constants, #[values] for matrices, #[fixture] for setup",
}

// StrategyAdvice is the sentence that tells an agent how this project removes duplicated code.
func (d Duplication) StrategyAdvice() string {
	var parts []string
	for _, s := range d.Strategies {
		if a, ok := strategyAdvice[s]; ok {
			parts = append(parts, a)
		}
	}
	if len(parts) == 0 {
		return ""
	}
	return "To remove repeated code: " + strings.Join(parts, "; ") + "."
}

// Lint picks how strict the generated lint configurations are.
type Lint struct {
	Strictness string `toml:"strictness"`
}

// Security says what fails the security job.
type Security struct {
	FailOn  string `toml:"fail_on"`
	Secrets bool   `toml:"secrets"`
}

// Done says whether an agent may set a ticket to Done when every gate passes.
type Done struct {
	AutoDone          bool     `toml:"auto_done"`
	HumanReviewLabels []string `toml:"human_review_labels"`
}

// Languages is the order in which Dendrite proposes languages for new code.
type Languages struct {
	Preference []string `toml:"preference"`
}

// Defaults are the directives of a project that has none.
func Defaults() Set {
	return Set{
		Schema:      SchemaVersion,
		Loc:         Loc{MaxProductionLines: 500, WarnAt: 450, Exclude: []string{"**/generated/**"}},
		Coverage:    Coverage{MinPercent: 100, Scope: "line", Exclude: []string{}},
		Duplication: Duplication{MaxPercent: 0, MinTokens: 50, MinLines: 5, IncludeTests: true, Strategies: []string{"helper_function", "macro", "rstest"}},
		Lint:        Lint{Strictness: "strict"},
		Security:    Security{FailOn: "high", Secrets: true},
		Done:        Done{AutoDone: true, HumanReviewLabels: []string{"needs-human-review"}},
		Languages:   Languages{Preference: []string{"rust", "python", "typescript", "go", "kotlin", "c", "cpp"}},
	}
}

// minProductionLines keeps a limit from being set so low that no file could pass.
const minProductionLines = 50

// Validate reports the first rule the directives break.
func (s Set) Validate() error {
	check := func(ok bool, format string, args ...any) error {
		if ok {
			return nil
		}
		return fmt.Errorf(format, args...)
	}
	rules := []error{
		check(s.Schema == SchemaVersion, "schema must be %d, the file says %d", SchemaVersion, s.Schema),
		check(s.Loc.MaxProductionLines >= minProductionLines, "loc.max_production_lines must be at least %d", minProductionLines),
		check(s.Loc.WarnAt >= 0 && s.Loc.WarnAt <= s.Loc.MaxProductionLines, "loc.warn_at (%d) must be between 0 and loc.max_production_lines (%d)", s.Loc.WarnAt, s.Loc.MaxProductionLines),
		check(s.Coverage.MinPercent >= 0 && s.Coverage.MinPercent <= 100, "coverage.min_percent must be between 0 and 100"),
		check(oneOf(s.Coverage.Scope, "line", "statement", "branch"), "coverage.scope must be line, statement or branch"),
		check(s.Duplication.MaxPercent >= 0 && s.Duplication.MaxPercent <= 100, "duplication.max_percent must be between 0 and 100"),
		check(s.Duplication.MinTokens >= 10, "duplication.min_tokens must be at least 10"),
		check(s.Duplication.MinLines >= 2, "duplication.min_lines must be at least 2"),
		check(knownStrategies(s.Duplication.Strategies), "duplication.strategies must be among helper_function, macro and rstest"),
		check(oneOf(s.Lint.Strictness, "relaxed", "standard", "strict"), "lint.strictness must be relaxed, standard or strict"),
		check(oneOf(s.Security.FailOn, "low", "medium", "high", "critical"), "security.fail_on must be low, medium, high or critical"),
		check(len(s.Languages.Preference) > 0, "languages.preference needs at least one language"),
	}
	for _, err := range rules {
		if err != nil {
			return err
		}
	}
	return nil
}

func knownStrategies(list []string) bool {
	for _, s := range list {
		if _, ok := strategyAdvice[s]; !ok {
			return false
		}
	}
	return true
}

func oneOf(v string, allowed ...string) bool {
	for _, a := range allowed {
		if strings.EqualFold(v, a) {
			return true
		}
	}
	return false
}
