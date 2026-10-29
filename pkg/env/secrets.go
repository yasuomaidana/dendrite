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

// LocalFile is where a user's own values live: untracked, owner-only, highest precedence.
const LocalFile = ".env.local"

// SecretsOptions says how the local overrides are created or updated.
type SecretsOptions struct {
	Dir      string            // project root holding .env, .env.local and TemplatePath
	Set      map[string]string // explicit values, always applied
	Generate bool              // random values for managed keys still at their default
	Rotate   bool              // with Generate: replace every managed key, custom ones included
	// Global is the parsed $DENDRITE_HOME/.env, used only to warn when the project layer shadows it.
	Global map[string]string
	// Prompt, when set, is asked for each managed key that is not set explicitly; it returns the
	// new value, or "" to keep the current one. It receives only the key, never a value.
	Prompt func(key string, isDefault bool) (string, error)
}

// SecretsResult names what happened. It never carries a value.
type SecretsResult struct {
	Path    string // the local overrides file
	Created bool   // .env.local did not exist and was written
	Added   []string
	Changed []string // keys whose value was replaced
	// StillDefault lists managed keys that remain at their published default.
	StillDefault []string
	// ShadowsGlobal lists managed keys the global .env sets differently from what applies here.
	ShadowsGlobal []string
}

// EnsureSecrets keeps three layers apart. The committed .env holds the published defaults and is
// never edited here (it is created from the template only when missing). A user's own values go to
// .env.local, which outranks it. Existing values are never overwritten, so re-running is safe.
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

	projPath := filepath.Join(opts.Dir, ".env")
	projLines, err := readLines(projPath)
	if errors.Is(err, os.ErrNotExist) { // a checkout without the committed defaults
		projLines = append([]string(nil), tmplLines...)
		err = os.WriteFile(projPath, []byte(strings.Join(projLines, "\n")+"\n"), 0o644)
	}
	if err != nil {
		return nil, err
	}

	localPath := filepath.Join(opts.Dir, LocalFile)
	res := &SecretsResult{Path: localPath}
	localLines, err := readLines(localPath)
	localExisted := err == nil
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return nil, err
	}
	local := parseKV(localLines)
	effective := map[string]string{}
	for _, m := range []map[string]string{defaults, parseKV(projLines), local} {
		for k, v := range m {
			effective[k] = v
		}
	}
	dirty := false
	set := func(k, v string) {
		if effective[k] == v && hasKey(local, k) {
			return
		}
		if effective[k] != v {
			res.Changed = append(res.Changed, k)
		}
		localLines = setLine(localLines, k, v)
		local[k], effective[k], dirty = v, v, true
	}

	// Keys the template has that neither .env nor .env.local defines (an older checkout).
	proj := parseKV(projLines)
	for _, l := range tmplLines {
		if k, v, ok := splitKV(l); ok && !hasKey(proj, k) && !hasKey(local, k) {
			localLines = setLine(localLines, k, v)
			local[k], effective[k], dirty = v, v, true
			res.Added = append(res.Added, k)
		}
	}

	for _, k := range ManagedSecrets {
		isDefault := effective[k] == defaults[k]
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

	if dirty {
		if !localExisted {
			localLines = append([]string{"# Local overrides: untracked, owner-only, outrank .env. Written by `dendrite soma secrets ensure`."}, localLines...)
			res.Created = true
		}
		if err := writeLines(localPath, localLines); err != nil {
			return nil, err
		}
	}

	for _, k := range ManagedSecrets {
		if effective[k] == defaults[k] {
			res.StillDefault = append(res.StillDefault, k)
		}
		if g := opts.Global[k]; g != "" && g != effective[k] {
			res.ShadowsGlobal = append(res.ShadowsGlobal, k)
		}
	}
	sort.Strings(res.Added)
	sort.Strings(res.Changed)
	return res, nil
}

// SecretState says, for each managed key, whether the value that applies is the published default,
// custom or missing. It never returns a value.
func SecretState(dir string) (map[string]string, error) {
	tmplLines, err := readLines(filepath.Join(dir, TemplatePath))
	if err != nil {
		return nil, err
	}
	defaults := parseKV(tmplLines)
	effective := map[string]string{}
	for _, name := range []string{".env", LocalFile} {
		lines, err := readLines(filepath.Join(dir, name))
		if err != nil && !errors.Is(err, os.ErrNotExist) {
			return nil, err
		}
		for k, v := range parseKV(lines) {
			if v != "" || name == LocalFile {
				effective[k] = v
			}
		}
	}
	out := make(map[string]string, len(ManagedSecrets))
	for _, k := range ManagedSecrets {
		switch v := effective[k]; {
		case v == "":
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
