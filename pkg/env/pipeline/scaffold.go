package pipeline

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"gopkg.in/yaml.v3"

	"Dendrite/pkg/directive"
	"Dendrite/pkg/fsutil"
)

const (
	// WorkflowPath is the generated workflow, relative to the project root.
	WorkflowPath = ".github/workflows/ci.yml"
	// legacyQualityPath is the hand-written quality workflow the generated quality job replaces.
	legacyQualityPath = ".github/workflows/quality.yml"
	archiveDir        = ".github/dendrite"
)

// Result says what Scaffold wrote and what it left alone, paths relative to the project root.
type Result struct {
	Created, Updated, Notes []string
}

// Scaffold writes the project's workflow. A workflow without the managed header was written by a person and
// is kept, unless adopt is set: then it is archived in .github/dendrite/ci.previous.yml (with the hand-written
// quality.yml next to it) and replaced, and the notes name the jobs of the old file the new one lacks.
func Scaffold(root string, set directive.Set, adopt, dryRun bool) (Result, error) {
	var res Result
	ext, err := LoadExtension(root)
	if err != nil {
		return res, err
	}
	comps, notes := Detect(root)
	res.Notes = notes
	want := Render(comps, set, ext)
	path := filepath.Join(root, filepath.FromSlash(WorkflowPath))
	existing, readErr := os.ReadFile(path)
	if readErr != nil && !os.IsNotExist(readErr) {
		return res, readErr
	}
	if readErr == nil && !ownsWorkflow(existing) {
		if !adopt {
			res.Notes = append(res.Notes, "kept the hand-written "+WorkflowPath+": it does not carry the managed header, so Dendrite does not overwrite it (`dendrite soma prepare --adopt-ci` takes it over)")
			return res, nil
		}
		adopted, err := adoptWorkflow(root, existing, want, dryRun)
		res.Notes = append(res.Notes, adopted...)
		if err != nil {
			return res, err
		}
	}
	if readErr == nil && bytes.Equal(existing, want) {
		return res, nil
	}
	if !dryRun {
		if err := fsutil.WriteFileAtomic(path, want); err != nil {
			return res, fmt.Errorf("write %s: %w", WorkflowPath, err)
		}
	}
	if readErr != nil {
		res.Created = append(res.Created, filepath.FromSlash(WorkflowPath))
	} else {
		res.Updated = append(res.Updated, filepath.FromSlash(WorkflowPath))
	}
	return res, nil
}

// adoptWorkflow archives the hand-written workflows the generated one replaces and says what moved.
func adoptWorkflow(root string, old, generated []byte, dryRun bool) ([]string, error) {
	notes := []string{"adopted the hand-written " + WorkflowPath + ", archived in " + archiveDir + "/ci.previous.yml"}
	if err := archive(root, archiveDir+"/ci.previous.yml", old, dryRun); err != nil {
		return notes, err
	}
	if lost := missingJobs(old, generated); len(lost) > 0 {
		notes = append(notes, "the previous workflow had jobs the generated one lacks ("+strings.Join(lost, ", ")+"); move what the project still needs into "+ExtensionPath)
	}
	quality, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(legacyQualityPath)))
	if err != nil || ownsWorkflow(quality) {
		return notes, nil
	}
	if err := archive(root, archiveDir+"/quality.previous.yml", quality, dryRun); err != nil {
		return notes, err
	}
	if !dryRun {
		if err := os.Remove(filepath.Join(root, filepath.FromSlash(legacyQualityPath))); err != nil {
			return notes, err
		}
	}
	return append(notes, "replaced the hand-written "+legacyQualityPath+" with the generated quality job (archived in "+archiveDir+"/quality.previous.yml); scripts it called are no longer used"), nil
}

func archive(root, rel string, data []byte, dryRun bool) error {
	if dryRun {
		return nil
	}
	return fsutil.WriteFileAtomic(filepath.Join(root, filepath.FromSlash(rel)), data)
}

func jobNames(data []byte) map[string]bool {
	var doc struct {
		Jobs map[string]any `yaml:"jobs"`
	}
	names := map[string]bool{}
	if yaml.Unmarshal(data, &doc) != nil {
		return names
	}
	for n := range doc.Jobs {
		names[n] = true
	}
	return names
}

func missingJobs(old, generated []byte) []string {
	have := jobNames(generated)
	var lost []string
	for n := range jobNames(old) {
		if !have[n] {
			lost = append(lost, n)
		}
	}
	sort.Strings(lost)
	return lost
}
