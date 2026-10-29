package env

import (
	"bufio"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// TemplatePath is the committed file that documents every default value.
const TemplatePath = "stacks/.env.example"

// ManagedSecrets are the credentials a user may want to change at install. Their defaults are
// published in TemplatePath on purpose: they are local-development values.
var ManagedSecrets = []string{
	"POSTGRES_PASSWORD",
	"LITELLM_MASTER_KEY",
	"LITELLM_SALT_KEY",
	"PLANE_DB_PASSWORD",
	"PLANE_SECRET_KEY",
	"PLANE_ADMIN_PASSWORD",
	"PENPOT_DB_PASSWORD",
	"PENPOT_SECRET_KEY",
	"GRAFANA_ADMIN_PASSWORD",
}

// SecretsOptions says how .env is created or updated.
type SecretsOptions struct {
	Dir      string            // project root holding .env and TemplatePath
	Set      map[string]string // explicit values, always applied
	Generate bool              // random values for managed keys still at their default
	Rotate   bool              // with Generate: replace every managed key, custom ones included
	// Prompt, when set, is asked for each managed key that is not set explicitly; it returns the
	// new value, or "" to keep the current one. It receives only the key, never a value.
	Prompt func(key string, isDefault bool) (string, error)
}

// SecretsResult names what happened. It never carries a value.
type SecretsResult struct {
	Path    string
	Created bool
	Added   []string // keys a newer template introduced
	Changed []string // keys whose value was replaced
	// StillDefault lists managed keys that remain at their published default.
	StillDefault []string
}

// EnsureSecrets creates <Dir>/.env from the template when missing and keeps every existing value,
// so re-running it is safe. It then applies Set, Generate and Prompt, in that order of precedence.
func EnsureSecrets(opts SecretsOptions) (*SecretsResult, error) {
	tmplLines, err := readLines(filepath.Join(opts.Dir, TemplatePath))
	if err != nil {
		return nil, fmt.Errorf("read %s: %w", TemplatePath, err)
	}
	defaults := parseKV(tmplLines)
	for k := range opts.Set {
		if _, ok := defaults[k]; !ok && !contains(ManagedSecrets, k) {
			return nil, fmt.Errorf("unknown setting %q: not in %s", k, TemplatePath)
		}
	}

	path := filepath.Join(opts.Dir, ".env")
	res := &SecretsResult{Path: path}
	lines, err := readLines(path)
	switch {
	case errors.Is(err, os.ErrNotExist):
		lines, res.Created = append([]string(nil), tmplLines...), true
	case err != nil:
		return nil, err
	}
	current := parseKV(lines)

	// Keys the template has and .env lacks (a newer template, or a .env written by hand).
	for _, l := range tmplLines {
		if k, v, ok := splitKV(l); ok {
			if _, have := current[k]; !have {
				lines = append(lines, k+"="+v)
				current[k] = v
				if !res.Created {
					res.Added = append(res.Added, k)
				}
			}
		}
	}

	set := func(k, v string) {
		if current[k] == v {
			return
		}
		lines = setLine(lines, k, v)
		current[k] = v
		if !res.Created && !contains(res.Added, k) {
			res.Changed = append(res.Changed, k)
		}
	}
	for _, k := range ManagedSecrets {
		isDefault := current[k] == defaults[k]
		switch {
		case hasKey(opts.Set, k):
			set(k, opts.Set[k])
		case opts.Generate && (isDefault || opts.Rotate):
			set(k, generate(k))
		case opts.Prompt != nil:
			v, err := opts.Prompt(k, isDefault)
			if err != nil {
				return nil, err
			}
			if v != "" {
				set(k, v)
			}
		}
	}
	for k, v := range opts.Set { // non-managed keys the caller set explicitly
		if !contains(ManagedSecrets, k) {
			set(k, v)
		}
	}

	for _, k := range ManagedSecrets {
		if current[k] == defaults[k] {
			res.StillDefault = append(res.StillDefault, k)
		}
	}
	sort.Strings(res.Added)
	sort.Strings(res.Changed)
	return res, writeLines(path, lines)
}

// SecretState says, for each managed key, whether it is the published default, custom or missing.
func SecretState(dir string) (map[string]string, error) {
	tmplLines, err := readLines(filepath.Join(dir, TemplatePath))
	if err != nil {
		return nil, err
	}
	defaults := parseKV(tmplLines)
	envLines, err := readLines(filepath.Join(dir, ".env"))
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return nil, err
	}
	current := parseKV(envLines)
	out := make(map[string]string, len(ManagedSecrets))
	for _, k := range ManagedSecrets {
		switch v, ok := current[k]; {
		case !ok || v == "":
			out[k] = "missing"
		case v == defaults[k]:
			out[k] = "default"
		default:
			out[k] = "custom"
		}
	}
	return out, nil
}

func generate(key string) string {
	b := make([]byte, 16)
	_, _ = rand.Read(b)
	v := hex.EncodeToString(b)
	if strings.HasPrefix(key, "LITELLM_") { // LiteLLM requires the sk- prefix
		return "sk-" + v
	}
	return v
}

func readLines(path string) ([]string, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	var out []string
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 0, 64*1024), 1<<20)
	for sc.Scan() {
		out = append(out, sc.Text())
	}
	return out, sc.Err()
}

// writeLines replaces the file atomically with owner-only permissions: it holds credentials.
func writeLines(path string, lines []string) error {
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, []byte(strings.Join(lines, "\n")+"\n"), 0o600); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}

func splitKV(line string) (k, v string, ok bool) {
	line = strings.TrimSpace(line)
	if line == "" || strings.HasPrefix(line, "#") {
		return "", "", false
	}
	k, v, ok = strings.Cut(line, "=")
	return strings.TrimSpace(k), strings.TrimSpace(v), ok && k != ""
}

func parseKV(lines []string) map[string]string {
	m := map[string]string{}
	for _, l := range lines {
		if k, v, ok := splitKV(l); ok {
			m[k] = v
		}
	}
	return m
}

func setLine(lines []string, key, value string) []string {
	for i, l := range lines {
		if k, _, ok := splitKV(l); ok && k == key {
			lines[i] = key + "=" + value
			return lines
		}
	}
	return append(lines, key+"="+value)
}

func hasKey(m map[string]string, k string) bool { _, ok := m[k]; return ok }

func contains(list []string, s string) bool {
	for _, x := range list {
		if x == s {
			return true
		}
	}
	return false
}
