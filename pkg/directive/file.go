package directive

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/pelletier/go-toml/v2"

	"Dendrite/pkg/fsutil"
)

// Decode reads a directives file. Keys the file leaves out keep their defaults; a key that does not exist is
// an error, so a typo is not silently ignored.
func Decode(data []byte) (Set, error) {
	s := Defaults()
	dec := toml.NewDecoder(bytes.NewReader(data))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&s); err != nil {
		var strict *toml.StrictMissingError
		if errors.As(err, &strict) {
			return Set{}, fmt.Errorf("unknown key in the directives: %s", strings.TrimSpace(strict.String()))
		}
		return Set{}, fmt.Errorf("not a valid directives file: %w", err)
	}
	return s, s.Validate()
}

// Exists reports whether the project at dir has a directives file.
func Exists(dir string) bool {
	_, err := os.Stat(filepath.Join(dir, filepath.FromSlash(Path)))
	return err == nil
}

// Load reads the directives of the project at dir. A project with no file has the defaults.
func Load(dir string) (Set, error) {
	data, err := os.ReadFile(filepath.Join(dir, filepath.FromSlash(Path)))
	if errors.Is(err, os.ErrNotExist) {
		return Defaults(), nil
	}
	if err != nil {
		return Set{}, err
	}
	s, err := Decode(data)
	if err != nil {
		return Set{}, fmt.Errorf("%s: %w", Path, err)
	}
	return s, nil
}

// Save writes the directives of the project at dir.
func Save(dir string, s Set) error {
	if err := s.Validate(); err != nil {
		return err
	}
	return fsutil.WriteFileAtomic(filepath.Join(dir, filepath.FromSlash(Path)), Encode(s))
}

// EnsureDefaults writes the default directives when the project has none and reports whether it did.
func EnsureDefaults(dir string) (bool, error) {
	if Exists(dir) {
		return false, nil
	}
	return true, Save(dir, Defaults())
}

func quoted(list []string) string {
	parts := make([]string, len(list))
	for i, v := range list {
		parts[i] = strconv.Quote(v)
	}
	return "[" + strings.Join(parts, ", ") + "]"
}

func number(f float64) string {
	s := strconv.FormatFloat(f, 'f', -1, 64)
	if !strings.Contains(s, ".") {
		s += ".0"
	}
	return s
}

// Encode renders the directives in the canonical order, so the same settings always give the same text and a
// change is a small diff.
func Encode(s Set) []byte {
	var b strings.Builder
	line := func(format string, args ...any) { fmt.Fprintf(&b, format+"\n", args...) }
	line("%s", Header)
	line("# What CI, the quality gate and the agents enforce. See docs/specs/quality.md in dendrite-rag-rs.")
	line("schema = %d", s.Schema)
	line("")
	line("[loc]")
	line("max_production_lines = %d", s.Loc.MaxProductionLines)
	line("warn_at = %d", s.Loc.WarnAt)
	line("exclude = %s", quoted(s.Loc.Exclude))
	line("")
	line("[coverage]")
	line("min_percent = %s", number(s.Coverage.MinPercent))
	line("scope = %s", strconv.Quote(s.Coverage.Scope))
	line("exclude = %s", quoted(s.Coverage.Exclude))
	line("")
	line("[duplication]")
	line("max_percent = %s", number(s.Duplication.MaxPercent))
	line("min_tokens = %d", s.Duplication.MinTokens)
	line("min_lines = %d", s.Duplication.MinLines)
	line("include_tests = %t", s.Duplication.IncludeTests)
	line("")
	line("[lint]")
	line("strictness = %s", strconv.Quote(s.Lint.Strictness))
	line("")
	line("[security]")
	line("fail_on = %s", strconv.Quote(s.Security.FailOn))
	line("secrets = %t", s.Security.Secrets)
	line("")
	line("[done]")
	line("auto_done = %t", s.Done.AutoDone)
	line("human_review_labels = %s", quoted(s.Done.HumanReviewLabels))
	line("")
	line("[languages]")
	line("preference = %s", quoted(s.Languages.Preference))
	return []byte(b.String())
}
