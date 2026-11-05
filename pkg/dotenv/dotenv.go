// Package dotenv loads API keys and vendor configuration, which are configured once in
// $DENDRITE_HOME/.env: configure once, every project uses it. Highest first: an exported
// variable, the project's .env.local (untracked, this checkout only), the global file, and
// last the project's committed .env, which only supplies defaults.
package dotenv

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"Dendrite/pkg/projectid"
)

// FileName is the name of both the global and the project configuration file.
const FileName = ".env"

// LocalFileName is the project's untracked override file, for what must differ in one checkout
// only. It outranks the global file; the project's committed .env ranks below it.
const LocalFileName = ".env.local"

// EnvDisable set to "off" stops Load from reading any file (hermetic tests).
const EnvDisable = "DENDRITE_DOTENV"

var (
	mu sync.Mutex
	// loaded holds the variables Load set, so Lookup can tell them from exported ones.
	loaded = map[string]bool{}
)

// GlobalPath is $DENDRITE_HOME/.env, the one place keys and vendor settings are configured.
func GlobalPath() (string, error) {
	home, err := projectid.HomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, FileName), nil
}

// ProjectRoot is the nearest ancestor of dir (itself included) holding .git, or dir
// itself when it is in no repository.
func ProjectRoot(dir string) string {
	abs, err := filepath.Abs(dir)
	if err != nil {
		return dir
	}
	for d := abs; ; d = filepath.Dir(d) {
		if _, err := os.Stat(filepath.Join(d, ".git")); err == nil {
			return d
		}
		if filepath.Dir(d) == d {
			return abs
		}
	}
}

// Parse reads KEY=VALUE lines; blank lines, # comments and an "export " prefix are
// allowed, and one pair of surrounding quotes is removed from a value.
func Parse(data string) map[string]string {
	out := map[string]string{}
	for _, line := range strings.Split(data, "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		key, value, ok := strings.Cut(strings.TrimPrefix(line, "export "), "=")
		key = strings.TrimSpace(key)
		if !ok || key == "" {
			continue
		}
		value = strings.TrimSpace(value)
		if len(value) >= 2 && (value[0] == '"' || value[0] == '\'') && value[len(value)-1] == value[0] {
			value = value[1 : len(value)-1]
		}
		out[key] = value
	}
	return out
}

// files returns the parsed files for dir, highest precedence first (.env.local, global, the
// project's committed .env); a missing file is skipped.
func files(dir string) ([]string, []map[string]string, error) {
	root := ProjectRoot(dir)
	project := filepath.Join(root, FileName)
	paths := []string{filepath.Join(root, LocalFileName)}
	if global, err := GlobalPath(); err == nil && global != project {
		paths = append(paths, global)
	}
	paths = append(paths, project)
	var found []string
	var vars []map[string]string
	for _, p := range paths {
		data, err := os.ReadFile(p)
		if errors.Is(err, fs.ErrNotExist) {
			continue
		}
		if err != nil {
			return nil, nil, fmt.Errorf("dotenv: %w", err)
		}
		found = append(found, p)
		vars = append(vars, Parse(string(data)))
	}
	return found, vars, nil
}

// Load sets every variable of the project's .env (found from dir), then of the global
// $DENDRITE_HOME/.env, that the environment does not already set, and returns the files
// it read. Empty values are skipped so they never mask a global one.
func Load(dir string) ([]string, error) {
	if os.Getenv(EnvDisable) == "off" {
		return nil, nil
	}
	found, vars, err := files(dir)
	if err != nil {
		return nil, err
	}
	mu.Lock()
	defer mu.Unlock()
	for _, m := range vars {
		for k, v := range m {
			if v == "" || os.Getenv(k) != "" {
				continue
			}
			if err := os.Setenv(k, v); err != nil {
				return found, fmt.Errorf("dotenv: set %s: %w", k, err)
			}
			loaded[k] = true
		}
	}
	return found, nil
}

// Lookup resolves key for the project at dir, which may differ from the one Load ran
// in: an exported value, then the project's .env, then the global .env, then whatever
// Load set.
func Lookup(dir, key string) string {
	mu.Lock()
	fromFile := loaded[key]
	mu.Unlock()
	current := os.Getenv(key)
	if current != "" && !fromFile {
		return current
	}
	if os.Getenv(EnvDisable) != "off" {
		if _, vars, err := files(dir); err == nil {
			for _, m := range vars {
				if v := m[key]; v != "" {
					return v
				}
			}
		}
	}
	return current
}
