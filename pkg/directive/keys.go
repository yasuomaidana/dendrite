package directive

import (
	"fmt"
	"math"
	"sort"
	"strconv"
	"strings"
)

// keyDef is one setting a rule can change: its value kind, what it means and how to store and read it.
type keyDef struct {
	kind string // int, float, bool, string, strings
	doc  string
	set  func(*Set, any)
	get  func(Set) any
}

// keys is every setting, by its dotted name (table.key).
var keys = map[string]keyDef{
	"loc.max_production_lines":           {"int", "most production lines a file may have (tests, comments and blank lines do not count), at least 50", func(s *Set, v any) { s.Loc.MaxProductionLines = v.(int) }, func(s Set) any { return s.Loc.MaxProductionLines }},
	"loc.warn_at":                        {"int", "production lines above which a file warns, not above loc.max_production_lines", func(s *Set, v any) { s.Loc.WarnAt = v.(int) }, func(s Set) any { return s.Loc.WarnAt }},
	"loc.exclude":                        {"strings", "globs of files whose lines are not counted", func(s *Set, v any) { s.Loc.Exclude = v.([]string) }, func(s Set) any { return s.Loc.Exclude }},
	"coverage.min_percent":               {"float", "minimum test coverage, 0 to 100", func(s *Set, v any) { s.Coverage.MinPercent = v.(float64) }, func(s Set) any { return s.Coverage.MinPercent }},
	"coverage.scope":                     {"string", "line, statement or branch", func(s *Set, v any) { s.Coverage.Scope = v.(string) }, func(s Set) any { return s.Coverage.Scope }},
	"coverage.exclude":                   {"strings", "globs of files not measured", func(s *Set, v any) { s.Coverage.Exclude = v.([]string) }, func(s Set) any { return s.Coverage.Exclude }},
	"duplication.max_percent":            {"float", "share of duplicated code tolerated, 0 to 100 (0 means none)", func(s *Set, v any) { s.Duplication.MaxPercent = v.(float64) }, func(s Set) any { return s.Duplication.MaxPercent }},
	"duplication.min_tokens":             {"int", "tokens a repeated block needs to count, at least 10", func(s *Set, v any) { s.Duplication.MinTokens = v.(int) }, func(s Set) any { return s.Duplication.MinTokens }},
	"duplication.min_lines":              {"int", "lines a repeated block needs to count, at least 2", func(s *Set, v any) { s.Duplication.MinLines = v.(int) }, func(s Set) any { return s.Duplication.MinLines }},
	"duplication.include_tests":          {"bool", "whether test code is checked for duplication", func(s *Set, v any) { s.Duplication.IncludeTests = v.(bool) }, func(s Set) any { return s.Duplication.IncludeTests }},
	"duplication.strategies":             {"strings", "how repeated code is removed, in order: helper_function, macro, rstest (Rust tests use the rstest crate)", func(s *Set, v any) { s.Duplication.Strategies = v.([]string) }, func(s Set) any { return s.Duplication.Strategies }},
	"complexity.max_cyclomatic":          {"int", "most decision points a function may have, at least 1", func(s *Set, v any) { s.Complexity.MaxCyclomatic = v.(int) }, func(s Set) any { return s.Complexity.MaxCyclomatic }},
	"complexity.max_cognitive":           {"int", "highest cognitive complexity score a function may have, at least 1", func(s *Set, v any) { s.Complexity.MaxCognitive = v.(int) }, func(s Set) any { return s.Complexity.MaxCognitive }},
	"complexity.exclude":                 {"strings", "globs of files skipped by the complexity check", func(s *Set, v any) { s.Complexity.Exclude = v.([]string) }, func(s Set) any { return s.Complexity.Exclude }},
	"structure.max_files_per_dir":        {"int", "most source files one directory may hold before splitting into sub-packages, at least 1", func(s *Set, v any) { s.Structure.MaxFilesPerDir = v.(int) }, func(s Set) any { return s.Structure.MaxFilesPerDir }},
	"structure.max_dir_production_lines": {"int", "most production lines one directory may hold across its files, at least 1", func(s *Set, v any) { s.Structure.MaxDirProductionLines = v.(int) }, func(s Set) any { return s.Structure.MaxDirProductionLines }},
	"structure.max_prefix_group":         {"int", "most files in one directory that may share a leading word before becoming a sub-package, at least 2", func(s *Set, v any) { s.Structure.MaxPrefixGroup = v.(int) }, func(s Set) any { return s.Structure.MaxPrefixGroup }},
	"structure.require_docs":             {"bool", "whether every crate, module and repository needs its documentation files", func(s *Set, v any) { s.Structure.RequireDocs = v.(bool) }, func(s Set) any { return s.Structure.RequireDocs }},
	"structure.docs_files":               {"strings", "the documentation file names every crate, module and repository needs, e.g. README.md and CHANGELOG.md", func(s *Set, v any) { s.Structure.DocsFiles = v.([]string) }, func(s Set) any { return s.Structure.DocsFiles }},
	"structure.exclude":                  {"strings", "globs of paths skipped by the directory structure check", func(s *Set, v any) { s.Structure.Exclude = v.([]string) }, func(s Set) any { return s.Structure.Exclude }},
	"lint.strictness":                    {"string", "relaxed, standard or strict", func(s *Set, v any) { s.Lint.Strictness = v.(string) }, func(s Set) any { return s.Lint.Strictness }},
	"security.fail_on":                   {"string", "lowest severity that fails the security job: low, medium, high or critical", func(s *Set, v any) { s.Security.FailOn = v.(string) }, func(s Set) any { return s.Security.FailOn }},
	"security.secrets":                   {"bool", "whether leaked secrets are scanned for", func(s *Set, v any) { s.Security.Secrets = v.(bool) }, func(s Set) any { return s.Security.Secrets }},
	"done.auto_done":                     {"bool", "true lets an agent set a ticket to Done when every gate passes; false leaves it to a person", func(s *Set, v any) { s.Done.AutoDone = v.(bool) }, func(s Set) any { return s.Done.AutoDone }},
	"done.human_review_labels":           {"strings", "ticket labels that always need a person", func(s *Set, v any) { s.Done.HumanReviewLabels = v.([]string) }, func(s Set) any { return s.Done.HumanReviewLabels }},
	"languages.preference":               {"strings", "languages in the order Dendrite proposes them for new code", func(s *Set, v any) { s.Languages.Preference = v.([]string) }, func(s Set) any { return s.Languages.Preference }},
}

// Keys lists every setting name, sorted.
func Keys() []string {
	out := make([]string, 0, len(keys))
	for k := range keys {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

// Describe lists every setting with its kind and meaning, one per line, for prompts and help.
func Describe() string {
	var b strings.Builder
	for _, k := range Keys() {
		fmt.Fprintf(&b, "%s (%s): %s\n", k, keys[k].kind, keys[k].doc)
	}
	return b.String()
}

// Change is one setting moved to a new value.
type Change struct {
	Key    string `json:"key"`
	Value  any    `json:"value"`
	Reason string `json:"rationale,omitempty"`
}

func (c Change) String() string { return fmt.Sprintf("%s = %v", c.Key, c.Value) }

// Apply sets key to value (a Go value of the key's kind, or a JSON number for int and float keys) and
// validates the result. On error the set is unchanged.
func (s *Set) Apply(key string, value any) error {
	def, ok := keys[key]
	if !ok {
		return fmt.Errorf("unknown directive %q (known: %s)", key, strings.Join(Keys(), ", "))
	}
	v, err := convert(def.kind, value)
	if err != nil {
		return fmt.Errorf("%s: %w", key, err)
	}
	next := *s
	def.set(&next, v)
	if err := next.Validate(); err != nil {
		return err
	}
	*s = next
	return nil
}

// ApplyAll applies several changes and validates the result once, so the order of changes that only make
// sense together (a lower limit and a lower warning level) does not matter. On error the set is unchanged.
func (s *Set) ApplyAll(changes []Change) error {
	next := *s
	for _, c := range changes {
		def, ok := keys[c.Key]
		if !ok {
			return fmt.Errorf("unknown directive %q (known: %s)", c.Key, strings.Join(Keys(), ", "))
		}
		v, err := convert(def.kind, c.Value)
		if err != nil {
			return fmt.Errorf("%s: %w", c.Key, err)
		}
		def.set(&next, v)
	}
	if err := next.Validate(); err != nil {
		return err
	}
	*s = next
	return nil
}

func convert(kind string, v any) (any, error) {
	switch kind {
	case "int":
		switch n := v.(type) {
		case int:
			return n, nil
		case float64:
			if n == math.Trunc(n) {
				return int(n), nil
			}
		}
		return nil, fmt.Errorf("want a whole number, got %v", v)
	case "float":
		switch n := v.(type) {
		case int:
			return float64(n), nil
		case float64:
			return n, nil
		}
		return nil, fmt.Errorf("want a number, got %v", v)
	case "bool":
		if b, ok := v.(bool); ok {
			return b, nil
		}
		return nil, fmt.Errorf("want true or false, got %v", v)
	case "string":
		if s, ok := v.(string); ok && strings.TrimSpace(s) != "" {
			return strings.ToLower(strings.TrimSpace(s)), nil
		}
		return nil, fmt.Errorf("want a word, got %v", v)
	default: // strings
		switch list := v.(type) {
		case []string:
			return list, nil
		case []any:
			out := make([]string, 0, len(list))
			for _, item := range list {
				text, ok := item.(string)
				if !ok {
					return nil, fmt.Errorf("want a list of text, got %v", v)
				}
				out = append(out, text)
			}
			return out, nil
		}
		return nil, fmt.Errorf("want a list, got %v", v)
	}
}

// ParseValue reads the text a person typed (`95`, `95%`, `true`, `a,b`) as a value of the key's kind.
func ParseValue(key, text string) (any, error) {
	def, ok := keys[key]
	if !ok {
		return nil, fmt.Errorf("unknown directive %q (known: %s)", key, strings.Join(Keys(), ", "))
	}
	text = strings.TrimSpace(text)
	switch def.kind {
	case "int":
		n, err := strconv.Atoi(text)
		return n, wrap(key, err)
	case "float":
		n, err := strconv.ParseFloat(strings.TrimSuffix(text, "%"), 64)
		return n, wrap(key, err)
	case "bool":
		switch strings.ToLower(text) {
		case "true", "yes", "on":
			return true, nil
		case "false", "no", "off":
			return false, nil
		}
		return nil, fmt.Errorf("%s: want true or false, got %q", key, text)
	case "string":
		return text, nil
	default:
		var out []string
		for _, part := range strings.Split(text, ",") {
			if part = strings.TrimSpace(part); part != "" {
				out = append(out, part)
			}
		}
		return out, nil
	}
}

func wrap(key string, err error) error {
	if err != nil {
		return fmt.Errorf("%s: %w", key, err)
	}
	return nil
}
