package reuse

import (
	"Dendrite/pkg/statedir"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
)

// RefManager manages registered source references (Tier 1).
type RefManager struct {
	filePath string
	mu       sync.RWMutex
}

// NewRefManager creates a new RefManager storing state at filePath.
func NewRefManager(filePath string) *RefManager {
	if filePath == "" {
		filePath = statedir.Resolve(".dendrite/refs.json")
	}
	return &RefManager{filePath: statedir.Resolve(filePath)}
}

// ListRefs returns all registered source references.
func (m *RefManager) ListRefs() ([]SourceReference, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	if _, err := os.Stat(m.filePath); os.IsNotExist(err) {
		return m.defaultReferences(), nil
	}

	data, err := os.ReadFile(m.filePath)
	if err != nil {
		return nil, fmt.Errorf("reading refs file: %w", err)
	}

	var refs []SourceReference
	if err := json.Unmarshal(data, &refs); err != nil {
		return nil, fmt.Errorf("unmarshaling refs: %w", err)
	}
	return refs, nil
}

// AddRef registers a new source reference.
func (m *RefManager) AddRef(ref SourceReference) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	refs, err := m.loadRefsLocked()
	if err != nil {
		refs = m.defaultReferences()
	}

	for _, r := range refs {
		if strings.EqualFold(r.ID, ref.ID) {
			return ErrDuplicateReference
		}
	}

	refs = append(refs, ref)
	return m.saveRefsLocked(refs)
}

// RemoveRef removes a source reference by ID.
func (m *RefManager) RemoveRef(id string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	refs, err := m.loadRefsLocked()
	if err != nil {
		return err
	}

	newRefs := make([]SourceReference, 0, len(refs))
	found := false
	for _, r := range refs {
		if strings.EqualFold(r.ID, id) {
			found = true
			continue
		}
		newRefs = append(newRefs, r)
	}

	if !found {
		return ErrReferenceNotFound
	}
	return m.saveRefsLocked(newRefs)
}

func (m *RefManager) loadRefsLocked() ([]SourceReference, error) {
	if _, err := os.Stat(m.filePath); os.IsNotExist(err) {
		return m.defaultReferences(), nil
	}
	data, err := os.ReadFile(m.filePath)
	if err != nil {
		return nil, err
	}
	var refs []SourceReference
	if err := json.Unmarshal(data, &refs); err != nil {
		return nil, err
	}
	return refs, nil
}

func (m *RefManager) saveRefsLocked(refs []SourceReference) error {
	dir := filepath.Dir(m.filePath)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("creating refs directory: %w", err)
	}
	data, err := json.MarshalIndent(refs, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(m.filePath, data, 0644)
}

func (m *RefManager) defaultReferences() []SourceReference {
	return []SourceReference{
		{
			ID:           "ref-carla-sensor",
			Name:         "carla-sensor-rs",
			Repository:   "https://github.com/cool-japan/carla-sensor-rs",
			Language:     "rust",
			Capabilities: []string{"kinematics", "sensor_processing", "spatial_math"},
			License:      "Apache-2.0",
		},
		{
			ID:           "ref-oxipix",
			Name:         "oxipix",
			Repository:   "https://github.com/cool-japan/oxipix",
			Language:     "rust",
			Capabilities: []string{"simd_blending", "color_spaces", "image_processing"},
			License:      "MIT",
		},
		{
			ID:           "ref-docula",
			Name:         "docula",
			Repository:   "https://github.com/cool-japan/docula",
			Language:     "typescript",
			Capabilities: []string{"ast_reflection", "docusaurus_portal", "markdown_render"},
			License:      "MIT",
		},
	}
}
