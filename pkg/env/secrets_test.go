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

// fixture is a checkout plus a separate global file, like a project and $DENDRITE_HOME.
type fixture struct{ dir, global string }

func project(t *testing.T, committed, global, local string) fixture {
	t.Helper()
	f := fixture{dir: t.TempDir()}
	f.global = filepath.Join(t.TempDir(), ".env")
	if err := os.MkdirAll(filepath.Join(f.dir, "stacks"), 0o755); err != nil {
		t.Fatal(err)
	}
	for path, body := range map[string]string{
		filepath.Join(f.dir, TemplatePath): tmpl,
		filepath.Join(f.dir, ".env"):       committed,
		f.global:                           global,
		filepath.Join(f.dir, LocalFile):    local,
	} {
		if body != "" {
			if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
				t.Fatal(err)
			}
		}
	}
	return f
}

func (f fixture) opts(o SecretsOptions) SecretsOptions {
	o.Dir, o.GlobalPath = f.dir, f.global
	return o
}

func readFile(t *testing.T, path string) string {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

func kvOf(t *testing.T, path string) map[string]string {
	t.Helper()
	return parseKV(strings.Split(readFile(t, path), "\n"))
}

func exists(path string) bool { _, err := os.Stat(path); return err == nil }

func TestEnsureWithTheCommittedDefaultsWritesNothing(t *testing.T) {
	f := project(t, tmpl, "", "")
	res, err := EnsureSecrets(f.opts(SecretsOptions{}))
	if err != nil {
		t.Fatal(err)
	}
	if exists(f.global) || exists(filepath.Join(f.dir, LocalFile)) || res.Created {
		t.Error("a file was written although nothing needed changing")
	}
	if len(res.StillDefault) != len(ManagedSecrets) {
		t.Errorf("still default = %v, want all %d", res.StillDefault, len(ManagedSecrets))
	}
}

func TestEnsureCreatesTheCommittedEnvOnlyWhenMissing(t *testing.T) {
	f := project(t, "", "", "")
	if _, err := EnsureSecrets(f.opts(SecretsOptions{})); err != nil {
		t.Fatal(err)
	}
	if readFile(t, filepath.Join(f.dir, ".env")) != strings.TrimRight(tmpl, "\n")+"\n" {
		t.Error(".env was not created from the template")
	}
}

func TestSetGoesToTheGlobalFileAndLeavesItsOtherKeysAndTheCheckoutAlone(t *testing.T) {
	existing := "# my config\nPLANE_API_KEY=plane-token\nGEMINI_API_KEY=gem\n"
	f := project(t, tmpl, existing, "")
	before := readFile(t, filepath.Join(f.dir, ".env"))
	res, err := EnsureSecrets(f.opts(SecretsOptions{Set: map[string]string{"POSTGRES_PASSWORD": "chosen"}}))
	if err != nil {
		t.Fatal(err)
	}
	got := kvOf(t, f.global)
	if got["POSTGRES_PASSWORD"] != "chosen" || got["PLANE_API_KEY"] != "plane-token" || got["GEMINI_API_KEY"] != "gem" || len(got) != 3 {
		t.Errorf("global = %v, want the three keys and nothing else", got)
	}
	if !strings.HasPrefix(readFile(t, f.global), "# my config\n") {
		t.Error("the global file's comments were rewritten")
	}
	if readFile(t, filepath.Join(f.dir, ".env")) != before || exists(filepath.Join(f.dir, LocalFile)) {
		t.Error("the checkout was touched; user values belong in the global file")
	}
	if res.Path != f.global || contains(res.StillDefault, "POSTGRES_PASSWORD") {
		t.Errorf("result = %+v", res)
	}
	info, _ := os.Stat(f.global)
	if info.Mode().Perm() != 0o600 {
		t.Errorf("global mode = %v, want 0600", info.Mode().Perm())
	}
	if _, err := EnsureSecrets(f.opts(SecretsOptions{Set: map[string]string{"NOT_A_SETTING": "x"}})); err == nil {
		t.Error("an unknown key was accepted")
	}
}

func TestLocalWritesOnlyThisCheckoutsOverrideFile(t *testing.T) {
	f := project(t, tmpl, "", "")
	res, err := EnsureSecrets(f.opts(SecretsOptions{Local: true, Set: map[string]string{"POSTGRES_PASSWORD": "here-only", "LITELLM_PORT": "4100"}}))
	if err != nil {
		t.Fatal(err)
	}
	local := filepath.Join(f.dir, LocalFile)
	if got := kvOf(t, local); got["POSTGRES_PASSWORD"] != "here-only" || got["LITELLM_PORT"] != "4100" || len(got) != 2 {
		t.Errorf(".env.local = %v", got)
	}
	if exists(f.global) || res.Path != local {
		t.Errorf("the global file was written or the path is wrong: %+v", res)
	}
	if info, _ := os.Stat(local); info.Mode().Perm() != 0o600 {
		t.Errorf(".env.local mode = %v", info.Mode().Perm())
	}
}

func TestAnExistingGlobalValueCountsAsCustomAndSurvives(t *testing.T) {
	f := project(t, tmpl, "POSTGRES_PASSWORD=mine\n", "")
	res, err := EnsureSecrets(f.opts(SecretsOptions{}))
	if err != nil {
		t.Fatal(err)
	}
	if contains(res.StillDefault, "POSTGRES_PASSWORD") || kvOf(t, f.global)["POSTGRES_PASSWORD"] != "mine" {
		t.Errorf("a configured global value was treated as default or changed: %+v", res)
	}
}

func TestGenerateReplacesDefaultsButNotCustomValuesUnlessRotating(t *testing.T) {
	f := project(t, tmpl, "POSTGRES_PASSWORD=mine\nPLANE_API_KEY=keep\n", "")
	res, err := EnsureSecrets(f.opts(SecretsOptions{Generate: true}))
	if err != nil {
		t.Fatal(err)
	}
	got := kvOf(t, f.global)
	if got["POSTGRES_PASSWORD"] != "mine" || got["PLANE_API_KEY"] != "keep" {
		t.Error("--generate overwrote a custom value or an unrelated key")
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
	if _, err := EnsureSecrets(f.opts(SecretsOptions{Generate: true, Rotate: true})); err != nil {
		t.Fatal(err)
	}
	if kvOf(t, f.global)["POSTGRES_PASSWORD"] == "mine" {
		t.Error("--rotate left a custom value in place")
	}
}

func TestACredentialNoLayerDefinesGetsItsPublishedDefault(t *testing.T) {
	old := strings.ReplaceAll(tmpl, "LITELLM_SALT_KEY=sk-default-salt\n", "") // a checkout from before the salt key existed
	f := project(t, old, "", "")
	res, err := EnsureSecrets(f.opts(SecretsOptions{}))
	if err != nil {
		t.Fatal(err)
	}
	if got := kvOf(t, f.global); got["LITELLM_SALT_KEY"] != "sk-default-salt" || len(got) != 1 {
		t.Errorf("global = %v, want only the missing credential", got)
	}
	if !contains(res.Added, "LITELLM_SALT_KEY") {
		t.Errorf("added = %v", res.Added)
	}
}

func TestPromptIsAskedOnlyForKeysNotSetExplicitlyAndEmptyKeepsTheCurrentValue(t *testing.T) {
	f := project(t, tmpl, "", "")
	var asked []string
	_, err := EnsureSecrets(f.opts(SecretsOptions{
		Set: map[string]string{"POSTGRES_PASSWORD": "explicit"},
		Prompt: func(key string, isDefault bool) (string, error) {
			asked = append(asked, key)
			if key == "GRAFANA_ADMIN_PASSWORD" {
				return "typed", nil
			}
			return "", nil
		},
	}))
	if err != nil {
		t.Fatal(err)
	}
	if contains(asked, "POSTGRES_PASSWORD") {
		t.Error("prompted for a key that --set already decided")
	}
	if got := kvOf(t, f.global); got["GRAFANA_ADMIN_PASSWORD"] != "typed" || hasKey(got, "PLANE_DB_PASSWORD") {
		t.Errorf("global = %v", got)
	}
}

func TestReRunningEnsureChangesNothing(t *testing.T) {
	f := project(t, tmpl, "", "")
	if _, err := EnsureSecrets(f.opts(SecretsOptions{Generate: true})); err != nil {
		t.Fatal(err)
	}
	before := readFile(t, f.global)
	res, err := EnsureSecrets(f.opts(SecretsOptions{}))
	if err != nil {
		t.Fatal(err)
	}
	if readFile(t, f.global) != before || len(res.Added)+len(res.Changed) != 0 || res.Created {
		t.Errorf("a second run changed the file or reported changes: %+v", res)
	}
}

func TestWarnsWhenThisCheckoutsLocalFileOverridesWhatWasJustSet(t *testing.T) {
	f := project(t, tmpl, "", "LITELLM_MASTER_KEY=sk-local\n")
	res, err := EnsureSecrets(f.opts(SecretsOptions{Set: map[string]string{"LITELLM_MASTER_KEY": "sk-new", "POSTGRES_PASSWORD": "pw"}}))
	if err != nil {
		t.Fatal(err)
	}
	if len(res.ShadowedByLocal) != 1 || res.ShadowedByLocal[0] != "LITELLM_MASTER_KEY" {
		t.Errorf("shadowed = %v, want only LITELLM_MASTER_KEY", res.ShadowedByLocal)
	}
}

func TestSecretStateNeverReturnsValuesAndSeesEveryLayer(t *testing.T) {
	old := strings.ReplaceAll(tmpl, "LITELLM_SALT_KEY=sk-default-salt\n", "")
	f := project(t, old, "POSTGRES_PASSWORD=mine\n", "GRAFANA_ADMIN_PASSWORD=local\n")
	st, err := SecretState(f.dir, f.global)
	if err != nil {
		t.Fatal(err)
	}
	if st["POSTGRES_PASSWORD"] != "custom" || st["GRAFANA_ADMIN_PASSWORD"] != "custom" || st["LITELLM_MASTER_KEY"] != "default" || st["LITELLM_SALT_KEY"] != "missing" {
		t.Errorf("state = %v", st)
	}
	for _, v := range st {
		if v != "custom" && v != "default" && v != "missing" {
			t.Errorf("state leaks %q", v)
		}
	}
}

func TestEmbeddedDefaultsMatchTheCommittedTemplate(t *testing.T) {
	committed, err := os.ReadFile(filepath.Join("..", "..", TemplatePath))
	if err != nil {
		t.Skipf("not running inside the repository: %v", err)
	}
	if string(committed) != embeddedDefaults {
		t.Fatal("pkg/env/defaults.env drifted from stacks/.env.example: copy the template over it")
	}
}

func TestOutsideACheckoutTheEmbeddedDefaultsApplyAndNoCommittedEnvIsCreated(t *testing.T) {
	dir := t.TempDir() // an arbitrary directory: no stacks/.env.example, no .env
	global := filepath.Join(t.TempDir(), ".env")
	res, err := EnsureSecrets(SecretsOptions{Dir: dir, GlobalPath: global})
	if err != nil {
		t.Fatal(err)
	}
	if exists(filepath.Join(dir, ".env")) {
		t.Error("a .env was created in an arbitrary directory")
	}
	if !contains(res.Added, "LITELLM_SALT_KEY") || kvOf(t, global)["LITELLM_MASTER_KEY"] == "" {
		t.Errorf("the published defaults were not applied to the global file: %+v", res)
	}
}
