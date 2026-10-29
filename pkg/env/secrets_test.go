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

// project builds a checkout: the template, optionally the committed .env and a .env.local.
func project(t *testing.T, committed, local string) string {
	t.Helper()
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, "stacks"), 0o755); err != nil {
		t.Fatal(err)
	}
	files := map[string]string{TemplatePath: tmpl}
	if committed != "" {
		files[".env"] = committed
	}
	if local != "" {
		files[LocalFile] = local
	}
	for name, body := range files {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return dir
}

func readFile(t *testing.T, dir, name string) string {
	t.Helper()
	b, err := os.ReadFile(filepath.Join(dir, name))
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

func local(t *testing.T, dir string) map[string]string {
	t.Helper()
	return parseKV(strings.Split(readFile(t, dir, LocalFile), "\n"))
}

func TestEnsureWithTheCommittedDefaultsWritesNothing(t *testing.T) {
	dir := project(t, tmpl, "")
	res, err := EnsureSecrets(SecretsOptions{Dir: dir})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(dir, LocalFile)); err == nil || res.Created {
		t.Error(".env.local was created although nothing needed changing")
	}
	if len(res.StillDefault) != len(ManagedSecrets) {
		t.Errorf("still default = %v, want all %d", res.StillDefault, len(ManagedSecrets))
	}
}

func TestEnsureCreatesTheCommittedEnvOnlyWhenMissing(t *testing.T) {
	dir := project(t, "", "")
	if _, err := EnsureSecrets(SecretsOptions{Dir: dir}); err != nil {
		t.Fatal(err)
	}
	if readFile(t, dir, ".env") != strings.TrimRight(tmpl, "\n")+"\n" {
		t.Error(".env was not created from the template")
	}
}

func TestTheCommittedEnvIsNeverModified(t *testing.T) {
	dir := project(t, tmpl, "")
	before := readFile(t, dir, ".env")
	if _, err := EnsureSecrets(SecretsOptions{Dir: dir, Generate: true, Set: map[string]string{"POSTGRES_PASSWORD": "x"}}); err != nil {
		t.Fatal(err)
	}
	if readFile(t, dir, ".env") != before {
		t.Error("the tracked .env was edited; user values must go to .env.local")
	}
}

func TestSetWritesOnlyToTheLocalFileWithOwnerOnlyPermissions(t *testing.T) {
	dir := project(t, tmpl, "")
	res, err := EnsureSecrets(SecretsOptions{Dir: dir, Set: map[string]string{"POSTGRES_PASSWORD": "chosen", "LITELLM_PORT": "4100"}})
	if err != nil || !res.Created {
		t.Fatalf("res=%+v err=%v", res, err)
	}
	got := local(t, dir)
	if got["POSTGRES_PASSWORD"] != "chosen" || got["LITELLM_PORT"] != "4100" || len(got) != 2 {
		t.Errorf(".env.local = %v, want exactly the two set keys", got)
	}
	info, _ := os.Stat(filepath.Join(dir, LocalFile))
	if info.Mode().Perm() != 0o600 {
		t.Errorf(".env.local mode = %v, want 0600", info.Mode().Perm())
	}
	if contains(res.StillDefault, "POSTGRES_PASSWORD") {
		t.Error("changed key still reported as default")
	}
	if _, err := EnsureSecrets(SecretsOptions{Dir: dir, Set: map[string]string{"NOT_A_SETTING": "x"}}); err == nil {
		t.Error("an unknown key was accepted")
	}
}

func TestEnsureKeepsLocalValuesAndAddsKeysTheTemplateIntroduced(t *testing.T) {
	// A checkout from before LITELLM_SALT_KEY existed: neither file defines it.
	old := strings.ReplaceAll(tmpl, "LITELLM_SALT_KEY=sk-default-salt\n", "")
	dir := project(t, old, "POSTGRES_PASSWORD=mine\n")
	res, err := EnsureSecrets(SecretsOptions{Dir: dir})
	if err != nil {
		t.Fatal(err)
	}
	got := local(t, dir)
	if got["POSTGRES_PASSWORD"] != "mine" || got["LITELLM_SALT_KEY"] != "sk-default-salt" {
		t.Errorf(".env.local = %v", got)
	}
	if !contains(res.Added, "LITELLM_SALT_KEY") || contains(res.Added, "POSTGRES_PASSWORD") {
		t.Errorf("added = %v", res.Added)
	}
	if contains(res.StillDefault, "POSTGRES_PASSWORD") {
		t.Error("a custom password was reported as default")
	}
}

func TestGenerateReplacesDefaultsButNotCustomValuesUnlessRotating(t *testing.T) {
	dir := project(t, tmpl, "POSTGRES_PASSWORD=mine\n")
	res, err := EnsureSecrets(SecretsOptions{Dir: dir, Generate: true})
	if err != nil {
		t.Fatal(err)
	}
	got := local(t, dir)
	if got["POSTGRES_PASSWORD"] != "mine" {
		t.Error("--generate overwrote a custom value")
	}
	if !strings.HasPrefix(got["LITELLM_MASTER_KEY"], "sk-") || got["LITELLM_MASTER_KEY"] == "sk-default-master" {
		t.Errorf("master key = %q, want a generated sk- key", got["LITELLM_MASTER_KEY"])
	}
	if len(got["PLANE_SECRET_KEY"]) < 32 || got["PLANE_SECRET_KEY"] == "plane_secret_default" {
		t.Errorf("plane secret not generated: %q", got["PLANE_SECRET_KEY"])
	}
	if len(res.StillDefault) != 0 {
		t.Errorf("still default after generate: %v", res.StillDefault)
	}

	if _, err := EnsureSecrets(SecretsOptions{Dir: dir, Generate: true, Rotate: true}); err != nil {
		t.Fatal(err)
	}
	if local(t, dir)["POSTGRES_PASSWORD"] == "mine" {
		t.Error("--rotate left a custom value in place")
	}
}

func TestPromptIsAskedOnlyForKeysNotSetExplicitlyAndEmptyKeepsTheCurrentValue(t *testing.T) {
	dir := project(t, tmpl, "")
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
	got := local(t, dir)
	if got["GRAFANA_ADMIN_PASSWORD"] != "typed" || hasKey(got, "PLANE_DB_PASSWORD") {
		t.Errorf(".env.local = %v", got)
	}
}

func TestReRunningEnsureChangesNothing(t *testing.T) {
	dir := project(t, tmpl, "")
	if _, err := EnsureSecrets(SecretsOptions{Dir: dir, Generate: true}); err != nil {
		t.Fatal(err)
	}
	before := readFile(t, dir, LocalFile)
	res, err := EnsureSecrets(SecretsOptions{Dir: dir})
	if err != nil {
		t.Fatal(err)
	}
	if readFile(t, dir, LocalFile) != before || len(res.Added)+len(res.Changed) != 0 || res.Created {
		t.Errorf("a second run changed the file or reported changes: %+v", res)
	}
}

func TestWarnsWhenTheProjectLayerShadowsAGlobalCredential(t *testing.T) {
	dir := project(t, tmpl, "")
	res, err := EnsureSecrets(SecretsOptions{Dir: dir, Global: map[string]string{"LITELLM_MASTER_KEY": "sk-from-global", "POSTGRES_PASSWORD": "pg_default"}})
	if err != nil {
		t.Fatal(err)
	}
	if len(res.ShadowsGlobal) != 1 || res.ShadowsGlobal[0] != "LITELLM_MASTER_KEY" {
		t.Errorf("shadows = %v, want only LITELLM_MASTER_KEY (the postgres value matches)", res.ShadowsGlobal)
	}
}

func TestSecretStateNeverReturnsValuesAndSeesTheLocalLayer(t *testing.T) {
	old := strings.ReplaceAll(tmpl, "LITELLM_SALT_KEY=sk-default-salt\n", "")
	dir := project(t, old, "POSTGRES_PASSWORD=mine\n")
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
