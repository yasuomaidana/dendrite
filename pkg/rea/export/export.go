package export

import (
	"errors"
	"fmt"
	"path/filepath"
	"sort"
	"strings"

	"Dendrite/pkg/fsutil"
	"Dendrite/pkg/rea/analysis"
)

// ErrNeedsOutput means no output directory was given: an export never guesses where to write.
var ErrNeedsOutput = errors.New("an output directory is required: pass --output")

// ErrUnknownLanguage means the language asked for has no renderer; the message lists the ones that have.
var ErrUnknownLanguage = errors.New("unsupported target language")

// languages maps each supported language to how it is written.
var languages = map[string]language{"rust": rustLang, "go": goLang, "python": pyLang, "typescript": tsLang}

// Languages lists the supported target languages, sorted.
func Languages() []string {
	names := make([]string, 0, len(languages))
	for n := range languages {
		names = append(names, n)
	}
	sort.Strings(names)
	return names
}

// Normalize returns the supported language named by name (case-insensitive, with the usual short names), or an error
// that lists the supported ones.
func Normalize(name string) (string, error) {
	n := strings.ToLower(strings.TrimSpace(name))
	switch n {
	case "golang":
		n = "go"
	case "py":
		n = "python"
	case "ts":
		n = "typescript"
	}
	if _, ok := languages[n]; !ok {
		return "", fmt.Errorf("%w %q: use one of %s", ErrUnknownLanguage, name, strings.Join(Languages(), ", "))
	}
	return n, nil
}

// Write writes the structure of a finished analysis as a data module in language, <output>/<id>.<ext>, and returns
// its path. Any format can be exported: the module holds what the analysis found, whatever the target was.
func Write(svc *analysis.Service, id, language, output string) (string, error) {
	lang, err := Normalize(language)
	if err != nil {
		return "", err
	}
	if strings.TrimSpace(output) == "" {
		return "", ErrNeedsOutput
	}
	st, img, err := svc.Inspected(id)
	if err != nil {
		return "", err
	}
	if st.Stage != analysis.StageDone {
		return "", analysis.ErrNotDone
	}
	fns, err := svc.EngineFunctions(id)
	if err != nil {
		return "", err
	}
	m := NewModel(id, st.Binary, img, fns)
	tree, err := m.tree()
	if err != nil {
		return "", fmt.Errorf("analysis %s: %w", id, err)
	}
	l := languages[lang]
	path := filepath.Join(output, id+"."+l.ext) // id is a validated file name, so the file stays under output
	return path, fsutil.WriteFileAtomic(path, []byte(l.module(m, tree)))
}
