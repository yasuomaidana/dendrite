package modelcatalog

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/pelletier/go-toml/v2"
)

// attemptGap is the shortest time between two background refresh attempts, so a failing refresh
// does not start again for every call that finds the catalog stale.
const attemptGap = 5 * time.Minute

const (
	attemptFile = "model-catalog.attempt"
	configFile  = "models.toml"
)

// Manager builds, stores and serves the catalog kept in Dir.
type Manager struct {
	Dir     string
	TTL     time.Duration
	Now     func() time.Time
	Getenv  func(string) string
	Sources []Source
	// LocalPicker returns the gateway's local model (GPU engine, reachable); optional.
	LocalPicker func(ctx context.Context) (string, bool)
	// Spawn starts a detached `dendrite cleft models refresh`; optional.
	Spawn func() error
}

// New returns a manager over dir with the given sources.
func New(dir string, sources ...Source) *Manager {
	return &Manager{Dir: dir, TTL: TTL, Now: time.Now, Getenv: os.Getenv, Sources: sources}
}

// Refresh asks every source, merges the answers, computes the roles and stores the catalog. A source
// that fails is noted and skipped; when every source fails the stored catalog is left alone.
func (m *Manager) Refresh(ctx context.Context) (Catalog, error) {
	var lists [][]Model
	var notes []string
	for _, s := range m.Sources {
		models, err := s.List(ctx)
		if err != nil {
			notes = append(notes, fmt.Sprintf("%s: %v", s.Name(), err))
			continue
		}
		lists = append(lists, models)
	}
	if len(lists) == 0 {
		return Catalog{}, fmt.Errorf("modelcatalog: no source answered (%s)", strings.Join(notes, "; "))
	}
	cfg := m.config()
	models := Merge(lists...)
	local := ""
	if m.LocalPicker != nil {
		local, _ = m.LocalPicker(ctx)
	}
	c := Catalog{FetchedAt: m.Now(), Models: models, Roles: ComputeRoles(models, local), Notes: notes}
	for role, model := range cfg.Roles {
		c.Roles[role] = model
	}
	return c, c.Save(m.Dir)
}

// RefreshIfStale refreshes when the catalog is missing or older than the TTL and reports whether it did.
// A failed attempt is not repeated for attemptGap, so a daemon calling this every tick does not
// hammer the providers.
func (m *Manager) RefreshIfStale(ctx context.Context) (bool, error) {
	c, err := Load(m.Dir)
	if err == nil && !c.Stale(m.Now(), m.TTL) {
		return false, nil
	}
	if !m.markAttempt() {
		return false, nil
	}
	_, err = m.Refresh(ctx)
	return true, err
}

// Role returns the model for role: an override, else the catalog's pick, else fallback. A missing or
// stale catalog starts one background refresh and is still answered from what is stored.
func (m *Manager) Role(role, fallback string) string {
	if v := m.override(role); v != "" {
		return v
	}
	c, err := Load(m.Dir)
	if err != nil || c.Stale(m.Now(), m.TTL) {
		m.kickRefresh()
	}
	if v := c.Roles[role]; v != "" {
		return v
	}
	return fallback
}

func envName(role string) string {
	return "DENDRITE_MODEL_" + strings.ToUpper(strings.ReplaceAll(role, "-", "_"))
}

type config struct {
	Roles   map[string]string `toml:"roles"`
	Catalog struct {
		IncludePreview bool `toml:"include_preview"`
	} `toml:"catalog"`
}

// config reads the user's models.toml; a missing or broken file is no configuration.
func (m *Manager) config() config {
	var cfg config
	if data, err := os.ReadFile(filepath.Join(m.Dir, configFile)); err == nil {
		_ = toml.Unmarshal(data, &cfg)
	}
	return cfg
}

// override is the user's choice for role: DENDRITE_MODEL_<ROLE>, then models.toml [roles].
func (m *Manager) override(role string) string {
	if v := strings.TrimSpace(m.Getenv(envName(role))); v != "" {
		return v
	}
	return m.config().Roles[role]
}

// IncludePreview reports whether models.toml asks for preview models in the catalog.
func (m *Manager) IncludePreview() bool { return m.config().Catalog.IncludePreview }

// markAttempt records a refresh attempt and reports false when one was made within attemptGap.
func (m *Manager) markAttempt() bool {
	path := filepath.Join(m.Dir, attemptFile)
	if fi, err := os.Stat(path); err == nil && m.Now().Sub(fi.ModTime()) < attemptGap {
		return false
	}
	if os.MkdirAll(m.Dir, 0o755) != nil || os.WriteFile(path, nil, 0o644) != nil {
		return false
	}
	_ = os.Chtimes(path, m.Now(), m.Now())
	return true
}

// kickRefresh starts a detached refresh unless one was started within attemptGap.
func (m *Manager) kickRefresh() {
	if m.Spawn != nil && m.markAttempt() {
		_ = m.Spawn()
	}
}

// selfPath is this program's path; tests point it at a script.
var selfPath = os.Executable

// SpawnSelf starts `<this binary> cleft models refresh` without waiting for it.
func SpawnSelf() error {
	exe, err := selfPath()
	if err != nil {
		return err
	}
	cmd := exec.Command(exe, "cleft", "models", "refresh", "--quiet")
	if err := cmd.Start(); err != nil {
		return err
	}
	return cmd.Process.Release()
}
