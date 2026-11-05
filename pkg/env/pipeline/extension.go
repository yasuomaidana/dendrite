package pipeline

import (
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"gopkg.in/yaml.v3"
)

// ExtensionPath is the project-owned file that adds project-specific work to the generated workflow. Dendrite
// never writes it.
const ExtensionPath = ".github/dendrite/pipeline.yml"

// Extension is what a project adds to the generated workflow.
type Extension struct {
	// Env is added to the workflow's environment.
	Env map[string]string `yaml:"env"`
	// PreTest steps run in every component job before its tests (sidecar builds, services).
	PreTest []map[string]any `yaml:"pre_test"`
	// Jobs are extra jobs, added as written.
	Jobs map[string]map[string]any `yaml:"jobs"`
	// SkipCoverageFloor leaves the coverage threshold to the project, for one with its own ratchet.
	SkipCoverageFloor bool `yaml:"skip_coverage_floor"`
	// LoomGate replaces the installed loom-gate with a command, for the repository that builds it.
	LoomGate string `yaml:"loom_gate"`
}

// LoadExtension reads the project's extension; a project without one has an empty extension.
func LoadExtension(root string) (Extension, error) {
	var ext Extension
	data, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(ExtensionPath)))
	if os.IsNotExist(err) {
		return ext, nil
	}
	if err != nil {
		return ext, err
	}
	dec := yaml.NewDecoder(bytesReader(data))
	dec.KnownFields(true)
	if err := dec.Decode(&ext); err != nil && !errors.Is(err, io.EOF) {
		return Extension{}, fmt.Errorf("%s: %w", ExtensionPath, err)
	}
	return ext, nil
}
