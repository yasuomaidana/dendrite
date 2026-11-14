package directive

import (
	"bytes"
	"encoding/json"
	"fmt"
	"path/filepath"
	"strconv"
	"strings"

	"Dendrite/pkg/fsutil"
)

// Artifact is a file derived only from the directives. A file Dendrite did not write is never touched.
type Artifact struct {
	// Path is relative to the project root.
	Path string
	// Content renders the file for the project at dir and its directives; nil means the project has no use for it.
	Content func(dir string, s Set) []byte
	// Owns reports whether an existing file was written by Dendrite (and so may be rewritten).
	Owns func(existing []byte) bool
}

// artifacts are the files `dendrite directive` keeps in step with the directives. Later tickets register the CI
// workflow, the Makefile and the lint configurations here.
var artifacts = []Artifact{
	{Path: "codecov.yml", Content: forSet(codecovYML), Owns: hasHeader},
	{Path: ".jscpd.json", Content: forSet(jscpdJSON), Owns: func(b []byte) bool { return bytes.Contains(b, []byte(jscpdMarker)) }},
}

// Register adds an artifact to what Render keeps current.
func Register(a Artifact) { artifacts = append(artifacts, a) }

// RenderResult says what Render did to each artifact.
type RenderResult struct {
	Written []string // created or rewritten
	Created []string // the part of Written that did not exist before
	Kept    []string // an existing file Dendrite did not write, left alone
}

// Render writes every artifact for the directives into the project at dir.
func Render(dir string, s Set) (RenderResult, error) { return render(dir, s, true) }

// Plan says what Render would write, without writing.
func Plan(dir string, s Set) (RenderResult, error) { return render(dir, s, false) }

func render(dir string, s Set, write bool) (RenderResult, error) {
	var res RenderResult
	for _, a := range artifacts {
		if err := renderOne(dir, s, a, write, &res); err != nil {
			return res, err
		}
	}
	return res, nil
}

func renderOne(dir string, s Set, a Artifact, write bool, res *RenderResult) error {
	want := a.Content(dir, s)
	if want == nil {
		return nil
	}
	path := filepath.Join(dir, filepath.FromSlash(a.Path))
	existing, existed, err := fsutil.ReadIfPresent(path)
	if err != nil {
		return err
	}
	switch {
	case existed && !a.Owns(existing):
		res.Kept = append(res.Kept, a.Path)
		return nil
	case existed && bytes.Equal(existing, want):
		return nil
	}
	return emit(path, a.Path, want, existed, write, res)
}

func emit(path, rel string, want []byte, existed, write bool, res *RenderResult) error {
	if write {
		if err := fsutil.WriteFileAtomic(path, want); err != nil {
			return fmt.Errorf("write %s: %w", rel, err)
		}
	}
	res.Written = append(res.Written, rel)
	if !existed {
		res.Created = append(res.Created, rel)
	}
	return nil
}

// forSet adapts a renderer that depends only on the directives.
func forSet(f func(Set) []byte) func(string, Set) []byte {
	return func(_ string, s Set) []byte { return f(s) }
}

func hasHeader(b []byte) bool { return bytes.HasPrefix(b, []byte(Header)) }

func percent(f float64) string { return strconv.FormatFloat(f, 'f', -1, 64) + "%" }

func codecovYML(s Set) []byte {
	return []byte(strings.Join([]string{
		Header,
		"coverage:",
		"  status:",
		"    project:",
		"      default:",
		"        target: " + percent(s.Coverage.MinPercent),
		"        threshold: 0%",
		"    patch:",
		"      default:",
		"        target: " + percent(s.Coverage.MinPercent),
		"comment:",
		`  layout: "condensed_header, condensed_files, condensed_footer"`,
		"",
	}, "\n"))
}

// jscpdMarker tells a generated .jscpd.json from a hand-written one; JSON has no comments.
const jscpdMarker = "dendrite directives"

func jscpdJSON(s Set) []byte {
	ignore := []string{"**/target/**", "**/node_modules/**", "**/vendor/**", "**/dist/**", "**/.git/**"}
	if !s.Duplication.IncludeTests {
		ignore = append(ignore, "**/tests/**", "**/*_test.*", "**/*.test.*", "**/*.spec.*")
	}
	doc := map[string]any{
		"_managed_by": jscpdMarker + " (" + Path + ")",
		"threshold":   s.Duplication.MaxPercent,
		"minTokens":   s.Duplication.MinTokens,
		"minLines":    s.Duplication.MinLines,
		"gitignore":   true,
		"reporters":   []string{"console"},
		"ignore":      ignore,
	}
	out, _ := json.MarshalIndent(doc, "", "  ")
	return append(out, '\n')
}
