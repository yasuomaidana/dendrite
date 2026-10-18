package workspace

import (
	"Dendrite/pkg/projectid"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"sync"
	"time"
)

var validProjectNameRegex = regexp.MustCompile(`^[a-zA-Z0-9_-]+$`)

// Registry manages project workspaces and context switching.
type Registry struct {
	configPath  string
	baseDataDir string
	mu          sync.RWMutex
	config      RegistryConfig
}

// NewRegistry constructs a new workspace Registry.
func NewRegistry(configPath, baseDataDir string) (*Registry, error) {
	if configPath == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			home = "."
		}
		configPath = filepath.Join(home, ".dendrite", "config.json")
	}
	if baseDataDir == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			home = "."
		}
		baseDataDir = filepath.Join(home, ".dendrite", "projects")
	}

	r := &Registry{
		configPath:  configPath,
		baseDataDir: baseDataDir,
		config: RegistryConfig{
			Projects: make(map[string]ProjectEntry),
		},
	}

	if err := r.Load(); err != nil && !os.IsNotExist(err) {
		return nil, err
	}

	return r, nil
}

// Load reads the workspace registry configuration from disk.
func (r *Registry) Load() error {
	r.mu.Lock()
	defer r.mu.Unlock()

	data, err := os.ReadFile(r.configPath)
	if err != nil {
		return err
	}

	var cfg RegistryConfig
	if err := json.Unmarshal(data, &cfg); err != nil {
		return err
	}

	if cfg.Projects == nil {
		cfg.Projects = make(map[string]ProjectEntry)
	}
	r.config = cfg
	return nil
}

// Save writes the workspace registry configuration atomically to disk.
func (r *Registry) Save() error {
	dir := filepath.Dir(r.configPath)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return err
	}

	data, err := json.MarshalIndent(r.config, "", "  ")
	if err != nil {
		return err
	}

	tmpFile := fmt.Sprintf("%s.tmp.%d", r.configPath, time.Now().UnixNano())
	if err := os.WriteFile(tmpFile, data, 0644); err != nil {
		return err
	}

	return os.Rename(tmpFile, r.configPath)
}

// AddProject registers a new codebase workspace into Dendrite and prepares its isolated directories.
func (r *Registry) AddProject(name, projectPath string) (*ProjectEntry, error) {
	cleanName := strings.TrimSpace(name)
	if !validProjectNameRegex.MatchString(cleanName) {
		return nil, ErrInvalidProjectName
	}

	cleanPath := filepath.Clean(strings.TrimSpace(projectPath))
	info, err := os.Stat(cleanPath)
	if err != nil || !info.IsDir() {
		return nil, ErrInvalidProjectPath
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	if _, exists := r.config.Projects[cleanName]; exists {
		return nil, ErrProjectAlreadyExists
	}

	projectID, err := projectid.Ensure(cleanPath)
	if err != nil {
		return nil, fmt.Errorf("minting project id: %w", err)
	}

	dataDir := filepath.Join(r.baseDataDir, cleanName)
	scratchDir := filepath.Join(dataDir, "scratch")
	graphDBPath := filepath.Join(dataDir, "graphify.db")
	spendCachePath := filepath.Join(dataDir, "spend.json")

	if err := os.MkdirAll(scratchDir, 0755); err != nil {
		return nil, err
	}

	// Initialize spend.json cache with virtual key mapping
	initialSpend := map[string]interface{}{
		"project":           cleanName,
		"virtual_key_alias": "project-" + cleanName,
		"total_spend_usd":   0.0,
		"total_tokens":      0,
		"updated_at":        time.Now().Format(time.RFC3339),
	}
	spendBytes, _ := json.MarshalIndent(initialSpend, "", "  ")
	_ = os.WriteFile(spendCachePath, spendBytes, 0644)

	entry := ProjectEntry{
		Name:            cleanName,
		Path:            cleanPath,
		ID:              projectID,
		State:           StateRegistered,
		DataDir:         dataDir,
		GraphDBPath:     graphDBPath,
		SpendCachePath:  spendCachePath,
		ScratchDir:      scratchDir,
		VirtualKeyAlias: "project-" + cleanName,
		RegisteredAt:    time.Now(),
	}

	r.config.Projects[cleanName] = entry

	if err := r.Save(); err != nil {
		return nil, err
	}

	return &entry, nil
}

// SwitchProject shifts the active workspace context to the designated project.
func (r *Registry) SwitchProject(name string) (*ProjectEntry, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	target, exists := r.config.Projects[name]
	if !exists {
		return nil, ErrProjectNotFound
	}

	if r.config.ActiveProject == name && target.State == StateActive {
		return &target, nil
	}

	// Suspend currently active project
	if r.config.ActiveProject != "" && r.config.ActiveProject != name {
		if current, ok := r.config.Projects[r.config.ActiveProject]; ok {
			if err := ValidateTransition(current.State, StateSuspended); err == nil {
				current.State = StateSuspended
				r.config.Projects[r.config.ActiveProject] = current
			}
		}
	}

	if err := ValidateTransition(target.State, StateActive); err != nil {
		return nil, err
	}

	target.State = StateActive
	target.LastSwitchedAt = time.Now()
	r.config.Projects[name] = target
	r.config.ActiveProject = name

	if err := r.Save(); err != nil {
		return nil, err
	}

	return &target, nil
}

// GetActiveProject retrieves the currently active project workspace.
func (r *Registry) GetActiveProject() (*ProjectEntry, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	if r.config.ActiveProject == "" {
		return nil, ErrNoActiveProject
	}

	entry, exists := r.config.Projects[r.config.ActiveProject]
	if !exists {
		return nil, ErrNoActiveProject
	}

	return &entry, nil
}

// GetProject looks up a project by name.
func (r *Registry) GetProject(name string) (*ProjectEntry, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	entry, exists := r.config.Projects[name]
	if !exists {
		return nil, ErrProjectNotFound
	}
	return &entry, nil
}

// ListProjects returns all registered projects sorted by name.
func (r *Registry) ListProjects() []ProjectEntry {
	r.mu.RLock()
	defer r.mu.RUnlock()

	var list []ProjectEntry
	for _, p := range r.config.Projects {
		list = append(list, p)
	}

	sort.Slice(list, func(i, j int) bool {
		return list[i].Name < list[j].Name
	})

	return list
}

// RemoveProject unregisters a project from the registry and optionally purges its data.
func (r *Registry) RemoveProject(name string, purgeData bool) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	entry, exists := r.config.Projects[name]
	if !exists {
		return ErrProjectNotFound
	}

	if r.config.ActiveProject == name {
		r.config.ActiveProject = ""
	}

	delete(r.config.Projects, name)

	if purgeData && entry.DataDir != "" {
		_ = os.RemoveAll(entry.DataDir)
	}

	return r.Save()
}
