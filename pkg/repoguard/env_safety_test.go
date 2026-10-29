// Package repoguard holds repository-level guards that run with go test.
package repoguard

import (
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// The committed .env exists so people can see the stack's default credentials. That is only safe
// while it holds nothing but those published, non-sensitive defaults. These tests fail the build
// the moment it holds anything else; real values belong in .env.local or $DENDRITE_HOME/.env.

// vendorTokens are provider credentials. They must be empty in any committed env file.
var vendorTokens = []string{"PLANE_API_KEY", "GEMINI_API_KEY", "COPILOT_API_KEY", "HF_TOKEN"}

// keyShapes match what real provider credentials look like. The published defaults deliberately
// contain none of them (they are words such as dendrite_password or sk-dendrite-local-master).
var keyShapes = []*regexp.Regexp{
	regexp.MustCompile(`sk-[A-Za-z0-9]{20,}`),
	regexp.MustCompile(`AIza[0-9A-Za-z_-]{30,}`),
	regexp.MustCompile(`gh[pousr]_[A-Za-z0-9]{30,}`),
	regexp.MustCompile(`xox[abp]-[A-Za-z0-9-]{10,}`),
	regexp.MustCompile(`AKIA[0-9A-Z]{16}`),
	regexp.MustCompile(`lin_api_[A-Za-z0-9]{20,}`),
	regexp.MustCompile(`^[0-9a-fA-F]{32,}$`),
	regexp.MustCompile(`-----BEGIN [A-Z ]*PRIVATE KEY`),
}

func repoRoot(t *testing.T) string {
	t.Helper()
	out, err := exec.Command("git", "rev-parse", "--show-toplevel").Output()
	if err != nil {
		t.Skipf("not a git checkout: %v", err)
	}
	return strings.TrimSpace(string(out))
}

func parse(t *testing.T, file string) map[string]string {
	t.Helper()
	data, err := os.ReadFile(file)
	if err != nil {
		t.Fatal(err)
	}
	kv := map[string]string{}
	for _, l := range strings.Split(string(data), "\n") {
		l = strings.TrimSpace(l)
		if k, v, ok := strings.Cut(l, "="); ok && l != "" && !strings.HasPrefix(l, "#") {
			kv[strings.TrimSpace(k)] = strings.TrimSpace(v)
		}
	}
	return kv
}

func TestCommittedEnvHoldsOnlyThePublishedDefaults(t *testing.T) {
	root := repoRoot(t)
	example := parse(t, filepath.Join(root, "stacks", ".env.example"))
	committed := parse(t, filepath.Join(root, ".env"))

	for k, v := range committed {
		if want, ok := example[k]; !ok || want != v {
			t.Errorf(".env sets %s differently from stacks/.env.example; the committed .env must be a copy of the published defaults (put your own value in .env.local)", k)
		}
	}
	for k := range example {
		if _, ok := committed[k]; !ok {
			t.Errorf(".env lacks %s from stacks/.env.example; copy the template over it", k)
		}
	}
}

func TestPublishedDefaultsContainNothingSensitive(t *testing.T) {
	root := repoRoot(t)
	for _, f := range []string{filepath.Join("stacks", ".env.example"), ".env"} {
		for k, v := range parse(t, filepath.Join(root, f)) {
			for _, re := range keyShapes {
				if re.MatchString(v) {
					t.Errorf("%s: %s looks like a real credential (%s); published defaults must be obviously local, e.g. sk-dendrite-local-master", f, k, re)
				}
			}
		}
		kv := parse(t, filepath.Join(root, f))
		for _, k := range vendorTokens {
			if kv[k] != "" {
				t.Errorf("%s: %s must be empty in a committed file; keep vendor tokens in $DENDRITE_HOME/.env", f, k)
			}
		}
	}
}

func TestOnlyTheCommittedEnvAndItsTemplateAreTracked(t *testing.T) {
	root := repoRoot(t)
	out, err := exec.Command("git", "-C", root, "ls-files").Output()
	if err != nil {
		t.Skipf("git ls-files: %v", err)
	}
	for _, f := range strings.Split(strings.TrimSpace(string(out)), "\n") {
		base := path.Base(f)
		isEnv := base == ".env" || strings.HasPrefix(base, ".env.")
		allowed := f == ".env" || strings.HasSuffix(base, ".example") || strings.HasSuffix(base, ".sample")
		if isEnv && !allowed {
			t.Errorf("%s is tracked: only the root .env (published defaults) and .env.example files may be; git rm --cached it", f)
		}
	}
}
