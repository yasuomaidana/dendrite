package modal

import (
	"bytes"
	"errors"
	"strings"
	"testing"
)

func TestDirectoryPickerModal_RenderCLI(t *testing.T) {
	m := NewDirectoryPickerModal("matcher-core", "project-a", "")

	// 1. Valid input
	in := strings.NewReader("/path/to/matcher-core\n")
	var out bytes.Buffer
	dir, err := m.RenderCLI(in, &out)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if dir != "/path/to/matcher-core" {
		t.Errorf("expected /path/to/matcher-core, got %s", dir)
	}
	if !strings.Contains(out.String(), "project-a -> matcher-core") {
		t.Errorf("missing inferred edge in output: %s", out.String())
	}

	// 2. Empty input without default
	inEmpty := strings.NewReader("\n")
	var outEmpty bytes.Buffer
	_, err = m.RenderCLI(inEmpty, &outEmpty)
	if !errors.Is(err, ErrEmptyDirectoryPath) {
		t.Errorf("expected ErrEmptyDirectoryPath, got %v", err)
	}

	// 3. User cancel
	inCancel := strings.NewReader("cancel\n")
	var outCancel bytes.Buffer
	_, err = m.RenderCLI(inCancel, &outCancel)
	if !errors.Is(err, ErrPromptCancelled) {
		t.Errorf("expected ErrPromptCancelled, got %v", err)
	}

	// 4. Default path fallback
	mWithDefault := NewDirectoryPickerModal("matcher-core", "project-a", "/default/path")
	inDefault := strings.NewReader("\n")
	var outDefault bytes.Buffer
	dirDef, err := mWithDefault.RenderCLI(inDefault, &outDefault)
	if err != nil {
		t.Fatalf("unexpected error with default: %v", err)
	}
	if dirDef != "/default/path" {
		t.Errorf("expected /default/path, got %s", dirDef)
	}

	// 5. RenderJSON
	jsonData, err := m.RenderJSON()
	if err != nil {
		t.Fatalf("RenderJSON failed: %v", err)
	}
	if !strings.Contains(string(jsonData), "matcher-core") {
		t.Errorf("missing member name in JSON: %s", string(jsonData))
	}
}
