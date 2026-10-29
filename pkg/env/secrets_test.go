package env

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const tmpl = `# template
POSTGRES_PASSWORD=pg_default
LITELLM_MASTER_KEY=sk-default-master
LITELLM_SALT_KEY=sk-default-salt
PLANE_DB_PASSWORD=plane_default
PLANE_SECRET_KEY=plane_secret_default
PLANE_ADMIN_PASSWORD=admin_default
PENPOT_DB_PASSWORD=penpot_default
PENPOT_SECRET_KEY=penpot_secret_default
GRAFANA_ADMIN_PASSWORD=grafana_default
LITELLM_PORT=4000
`

func project(t *testing.T, envFile string) string {
	t.Helper()
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, "stacks"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, TemplatePath), []byte(tmpl), 0o644); err != nil {
		t.Fatal(err)
	}
	if envFile != "" {
		if err := os.WriteFile(filepath.Join(dir, ".env"), []byte(envFile), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	return dir
}

func read(t *testing.T, dir string) map[string]string {
	t.Helper()
	lines, err := readLines(filepath.Join(dir, ".env"))
	if err != nil {
		t.Fatal(err)
	}
	return parseKV(lines)
}

func TestEnsureCreatesEnvFromTheTemplateWithOwnerOnlyPermissions(t *testing.T) {
	dir := project(t, "")
	res, err := EnsureSecrets(SecretsOptions{Dir: dir})
	if err != nil || !res.Created {
		t.Fatalf("res=%+v err=%v", res, err)
	}
	if got := read(t, dir)["LITELLM_MASTER_KEY"]; got != "sk-default-master" {
		t.Errorf("master key = %q, want the published default", got)
	}
	info, _ := os.Stat(filepath.Join(dir, ".env"))
	if info.Mode().Perm() != 0o600 {
		t.Errorf(".env mode = %v, want 0600", info.Mode().Perm())
	}
	if len(res.StillDefault) != len(ManagedSecrets) {
		t.Errorf("still default = %v, want all %d", res.StillDefault, len(ManagedSecrets))
	}
}

func TestEnsureKeepsCustomValuesAndAddsKeysTheTemplateIntroduced(t *testing.T) {
	// A .env from before LITELLM_SALT_KEY existed, with a password the user chose.
	dir := project(t, "POSTGRES_PASSWORD=mine\nLITELLM_MASTER_KEY=sk-mine\nEXTRA=keep\n")
	res, err := EnsureSecrets(SecretsOptions{Dir: dir})
	if err != nil {
		t.Fatal(err)
	}
	got := read(t, dir)
	if got["POSTGRES_PASSWORD"] != "mine" || got["LITELLM_MASTER_KEY"] != "sk-mine" || got["EXTRA"] != "keep" {
		t.Errorf("custom values changed: %v", got)
	}
	if got["LITELLM_SALT_KEY"] != "sk-default-salt" {
		t.Errorf("salt key = %q, want the default added", got["LITELLM_SALT_KEY"])
	}
	if res.Created || !contains(res.Added, "LITELLM_SALT_KEY") || contains(res.Added, "POSTGRES_PASSWORD") {
		t.Errorf("result = %+v", res)
	}
	if contains(res.StillDefault, "POSTGRES_PASSWORD") {
		t.Error("a custom password was reported as default")
	}
}

func TestEnsureSetChangesOnlyTheNamedKeysAndRejectsUnknownOnes(t *testing.T) {
	dir := project(t, "")
	res, err := EnsureSecrets(SecretsOptions{Dir: dir, Set: map[string]string{"POSTGRES_PASSWORD": "chosen", "LITELLM_PORT": "4100"}})
	if err != nil {
		t.Fatal(err)
	}
	got := read(t, dir)
	if got["POSTGRES_PASSWORD"] != "chosen" || got["LITELLM_PORT"] != "4100" || got["PLANE_DB_PASSWORD"] != "plane_default" {
		t.Errorf("env = %v", got)
	}
	if contains(res.StillDefault, "POSTGRES_PASSWORD") {
		t.Error("changed key still reported as default")
	}
	if _, err := EnsureSecrets(SecretsOptions{Dir: dir, Set: map[string]string{"NOT_A_SETTING": "x"}}); err == nil {
		t.Error("an unknown key was accepted")
	}
}

func TestGenerateReplacesDefaultsButNotCustomValuesUnlessRotating(t *testing.T) {
	dir := project(t, "POSTGRES_PASSWORD=mine\n")
	res, err := EnsureSecrets(SecretsOptions{Dir: dir, Generate: true})
	if err != nil {
		t.Fatal(err)
	}
	got := read(t, dir)
	if got["POSTGRES_PASSWORD"] != "mine" {
		t.Error("--generate overwrote a custom value")
	}
	if !strings.HasPrefix(got["LITELLM_MASTER_KEY"], "sk-") || got["LITELLM_MASTER_KEY"] == "sk-default-master" {
		t.Errorf("master key = %q, want a generated sk- key", got["LITELLM_MASTER_KEY"])
	}
	if got["PLANE_SECRET_KEY"] == "plane_secret_default" || len(got["PLANE_SECRET_KEY"]) < 32 {
		t.Errorf("plane secret not generated: %q", got["PLANE_SECRET_KEY"])
	}
	if len(res.StillDefault) != 0 {
		t.Errorf("still default after generate: %v", res.StillDefault)
	}

	if _, err := EnsureSecrets(SecretsOptions{Dir: dir, Generate: true, Rotate: true}); err != nil {
		t.Fatal(err)
	}
	if read(t, dir)["POSTGRES_PASSWORD"] == "mine" {
		t.Error("--rotate left a custom value in place")
	}
}

func TestPromptIsAskedOnlyForKeysNotSetExplicitlyAndEmptyKeepsTheCurrentValue(t *testing.T) {
	dir := project(t, "")
	var asked []string
	_, err := EnsureSecrets(SecretsOptions{
		Dir: dir,
		Set: map[string]string{"POSTGRES_PASSWORD": "explicit"},
		Prompt: func(key string, isDefault bool) (string, error) {
			asked = append(asked, key)
			if key == "GRAFANA_ADMIN_PASSWORD" {
				return "typed", nil
			}
			return "", nil
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if contains(asked, "POSTGRES_PASSWORD") {
		t.Error("prompted for a key that --set already decided")
	}
	got := read(t, dir)
	if got["GRAFANA_ADMIN_PASSWORD"] != "typed" || got["PLANE_DB_PASSWORD"] != "plane_default" {
		t.Errorf("env = %v", got)
	}
}

func TestReRunningEnsureChangesNothing(t *testing.T) {
	dir := project(t, "")
	if _, err := EnsureSecrets(SecretsOptions{Dir: dir, Generate: true}); err != nil {
		t.Fatal(err)
	}
	before, _ := os.ReadFile(filepath.Join(dir, ".env"))
	res, err := EnsureSecrets(SecretsOptions{Dir: dir})
	if err != nil {
		t.Fatal(err)
	}
	after, _ := os.ReadFile(filepath.Join(dir, ".env"))
	if string(before) != string(after) || len(res.Added)+len(res.Changed) != 0 {
		t.Errorf("a second run changed the file or reported changes: %+v", res)
	}
}

func TestSecretStateNeverReturnsValues(t *testing.T) {
	dir := project(t, "POSTGRES_PASSWORD=mine\nLITELLM_MASTER_KEY=sk-default-master\n")
	st, err := SecretState(dir)
	if err != nil {
		t.Fatal(err)
	}
	if st["POSTGRES_PASSWORD"] != "custom" || st["LITELLM_MASTER_KEY"] != "default" || st["LITELLM_SALT_KEY"] != "missing" {
		t.Errorf("state = %v", st)
	}
	for _, v := range st {
		if v != "custom" && v != "default" && v != "missing" {
			t.Errorf("state leaks %q", v)
		}
	}
}
