package env

import (
	"bufio"
	"crypto/rand"
	_ "embed"
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

// embeddedDefaults is a copy of TemplatePath compiled into the binary, so the defaults are known
// outside a checkout too (a test keeps the two identical).
//
//go:embed defaults.env
var embeddedDefaults string

// readTemplate returns the template lines: the checkout's own file when there is one, otherwise the
// embedded copy.
func readTemplate(dir string) ([]string, error) {
	lines, err := readLines(filepath.Join(dir, TemplatePath))
	if errors.Is(err, os.ErrNotExist) {
		return strings.Split(strings.TrimRight(embeddedDefaults, "\n"), "\n"), nil
	}
	return lines, err
}

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

// LocalFile is a checkout's own untracked overrides: owner-only, outranking the global file.
const LocalFile = ".env.local"

// SecretsOptions says how the user's credentials are created or updated.
type SecretsOptions struct {
	Dir string // project root holding the committed .env, .env.local and TemplatePath
	// GlobalPath is $DENDRITE_HOME/.env, where credentials are configured once for every project.
	// It is the write target unless Local is set (or GlobalPath is empty).
	GlobalPath string
	Local      bool              // write to this checkout's .env.local instead
	Set        map[string]string // explicit values, always applied
	Generate   bool              // random values for managed keys still at their default
	Rotate     bool              // with Generate: replace every managed key, custom ones included
	// Prompt, when set, is asked for each managed key that is not set explicitly; it returns the
	// new value, or "" to keep the current one. It receives only the key, never a value.
	Prompt func(key string, isDefault bool) (string, error)
}

// SecretsResult names what happened. It never carries a value.
type SecretsResult struct {
	Path    string // the file that was written
	Created bool   // that file did not exist and was written
	Added   []string
	Changed []string // keys whose value was replaced
	// StillDefault lists managed keys that remain at their published default.
	StillDefault []string
	// ShadowedByLocal lists keys written to the global file that this checkout's .env.local overrides.
	ShadowedByLocal []string
}

// layers loads the files in increasing precedence: the committed .env (published defaults), the
// global file, then this checkout's .env.local.
func layers(dir, globalPath string) (proj, global, local []string, err error) {
	read := func(path string, missingOK bool) ([]string, error) {
		if path == "" {
			return nil, nil
		}
		l, err := readLines(path)
		if errors.Is(err, os.ErrNotExist) && missingOK {
			return nil, nil
		}
		return l, err
	}
	if proj, err = read(filepath.Join(dir, ".env"), true); err != nil {
		return
	}
	if global, err = read(globalPath, true); err != nil {
		return
	}
	local, err = read(filepath.Join(dir, LocalFile), true)
	return
}

// EnsureSecrets keeps the layers apart. The committed .env holds the published defaults and is
// never edited here (it is created from the template only when missing). The user's own values
// go to the global file ($DENDRITE_HOME/.env: configure once, every project uses it) or, with
// Local, to this checkout's .env.local. Existing values are never overwritten, so re-running is
// safe, and only the keys being set are touched in a file that holds other settings.
func EnsureSecrets(opts SecretsOptions) (*SecretsResult, error) {
	tmplLines, err := readTemplate(opts.Dir)
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
	if _, err := os.Stat(projPath); errors.Is(err, os.ErrNotExist) && isCheckout(opts.Dir) { // a checkout without the committed defaults
		if err := os.WriteFile(projPath, []byte(strings.Join(tmplLines, "\n")+"\n"), 0o644); err != nil {
			return nil, err
		}
	}
	projLines, globalLines, localLines, err := layers(opts.Dir, opts.GlobalPath)
	if err != nil {
		return nil, err
	}

	target := opts.GlobalPath
	if opts.Local || target == "" {
		target = filepath.Join(opts.Dir, LocalFile)
	}
	targetLocal := target == filepath.Join(opts.Dir, LocalFile)
	lines, existed := globalLines, globalLines != nil
	if targetLocal {
		lines, existed = localLines, localLines != nil
	}
	res := &SecretsResult{Path: target}

	proj, global, local := parseKV(projLines), parseKV(globalLines), parseKV(localLines)
	effective := map[string]string{}
	for _, m := range []map[string]string{proj, global, local} { // the template is not a layer
		for k, v := range m {
			if v != "" || hasKey(local, k) {
				effective[k] = v
			}
		}
	}
	written := parseKV(lines)
	dirty := false
	set := func(k, v string) {
		if written[k] == v {
			return
		}
		if effective[k] != v {
			res.Changed = append(res.Changed, k)
		}
		lines = setLine(lines, k, v)
		written[k], dirty = v, true
		if !targetLocal && hasKey(local, k) {
			res.ShadowedByLocal = append(res.ShadowedByLocal, k)
		} else {
			effective[k] = v
		}
	}

	// A managed credential no layer defines (an older checkout) gets its published default.
	for _, k := range ManagedSecrets {
		if effective[k] == "" && defaults[k] != "" {
			lines = setLine(lines, k, defaults[k])
			written[k], effective[k], dirty = defaults[k], defaults[k], true
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
		if !existed && targetLocal {
			lines = append([]string{"# Local overrides for this checkout: untracked, owner-only. Written by `dendrite soma secrets ensure --local`."}, lines...)
		}
		res.Created = !existed
		if err := writeLines(target, lines); err != nil {
			return nil, err
		}
	}

	for _, k := range ManagedSecrets {
		if effective[k] == defaults[k] {
			res.StillDefault = append(res.StillDefault, k)
		}
	}
	sort.Strings(res.Added)
	sort.Strings(res.Changed)
	sort.Strings(res.ShadowedByLocal)
	return res, nil
}

// SecretState says, for each managed key, whether the value that applies is the published default,
// custom or missing. It never returns a value.
func SecretState(dir, globalPath string) (map[string]string, error) {
	tmplLines, err := readTemplate(dir)
	if err != nil {
		return nil, err
	}
	defaults := parseKV(tmplLines)
	proj, global, local, err := layers(dir, globalPath)
	if err != nil {
		return nil, err
	}
	effective := map[string]string{}
	for _, m := range []map[string]string{parseKV(proj), parseKV(global), parseKV(local)} {
		for k, v := range m {
			if v != "" {
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

// isCheckout reports whether dir is a Dendrite checkout (it carries the committed template), as
// opposed to any directory the command happens to run from.
func isCheckout(dir string) bool {
	_, err := os.Stat(filepath.Join(dir, TemplatePath))
	return err == nil
}
