package inference

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func newRegistry(t *testing.T) *ZeroCostRegistry {
	t.Helper()
	t.Setenv(EnvZeroCostModels, "")
	t.Setenv(EnvZeroCostPrefixes, "")
	return NewZeroCostRegistry(filepath.Join(t.TempDir(), "zero-cost.json"))
}

func TestZeroCostMatchingExplainsItsReason(t *testing.T) {
	r := newRegistry(t)
	r.AddModel("my-model")
	r.AddPrefix("team/")
	r.parseEnvConfig("env-model,env-prefix/", "")
	r.RemoveModel("llama3")

	cases := []struct {
		model  string
		want   bool
		reason string
	}{
		{"", false, "empty model name"},
		{"my-model", true, "custom addition"},
		{"env-model", true, "configured via .env"},
		{"team/anything", true, "prefix match 'team/' (custom addition)"},
		{"env-prefix/x", true, "prefix match 'env-prefix/' (configured via .env)"},
		{"ollama/anything", true, "built-in default"},
		{"llama3", false, "explicitly removed"},
		{"unknown-model", false, "does not match"},
	}
	for _, c := range cases {
		got, reason := r.Test(c.model)
		if got != c.want || !strings.Contains(reason, c.reason) {
			t.Errorf("Test(%q) = %v, %q; want %v containing %q", c.model, got, reason, c.want, c.reason)
		}
		if r.IsZeroCost(c.model) != c.want {
			t.Errorf("IsZeroCost(%q) disagrees with Test", c.model)
		}
	}
}

func TestZeroCostEditing(t *testing.T) {
	r := newRegistry(t)
	if r.AddPrefix("") || r.AddModel("  ") || r.RemovePrefix("") || r.RemoveModel("") {
		t.Error("blank entries are refused")
	}
	if ok, prefix := r.Add(""); ok || prefix {
		t.Error("Add of nothing")
	}
	if ok, prefix := r.Remove(" "); ok || prefix {
		t.Error("Remove of nothing")
	}
	if ok, prefix := r.Add("team/"); !ok || !prefix {
		t.Error("a trailing slash adds a prefix")
	}
	if ok, prefix := r.Remove("team"); !ok || !prefix {
		t.Error("removing a name that is a prefix removes the prefix")
	}
	r.parseEnvConfig("", "from-env")
	if _, prefix := r.Remove("from-env"); !prefix {
		t.Error("an env prefix is removable by its bare name")
	}
	if _, prefix := r.Remove("ollama"); !prefix {
		t.Error("a default prefix is removable by its bare name")
	}
	if ok, prefix := r.Remove("plain-model"); !ok || prefix {
		t.Error("anything else is removed as a model")
	}
}

func TestZeroCostEnvironmentParsing(t *testing.T) {
	r := newRegistry(t)
	r.parseEnvConfig(" , a-model, b/ ", " ,c , d/")
	for _, want := range []string{"b/", "c/", "d/"} {
		if !r.envPrefixes[want] {
			t.Errorf("missing env prefix %q in %v", want, r.envPrefixes)
		}
	}
	if !r.envModels["a-model"] {
		t.Errorf("missing env model: %v", r.envModels)
	}
}

func TestZeroCostAutoLoadReadsEnvironmentAndFile(t *testing.T) {
	t.Setenv(EnvZeroCostModels, "env-one")
	t.Setenv(EnvZeroCostPrefixes, "env-prefix")
	path := filepath.Join(t.TempDir(), "zc.json")
	saved := NewZeroCostRegistry(path)
	saved.AddModel("saved-model")
	if err := saved.SaveToFile(path); err != nil {
		t.Fatal(err)
	}

	r := NewZeroCostRegistry(path)
	r.AutoLoad()
	if !r.IsZeroCost("env-one") || !r.IsZeroCost("env-prefix/x") || !r.IsZeroCost("saved-model") {
		t.Error("AutoLoad must read both the environment and the saved file")
	}
	NewZeroCostRegistry("").AutoLoad() // no file configured
}

func TestZeroCostFilePersistence(t *testing.T) {
	dir := t.TempDir()
	r := newRegistry(t)
	r.AddModel("kept")
	r.RemovePrefix("gone")
	path := filepath.Join(dir, "nested", "zc.json")
	if err := r.SaveToFile(path); err != nil {
		t.Fatal(err)
	}
	if err := r.SaveToFile(filepath.Join(dir, "plain-file", "x", "zc.json")); err != nil {
		t.Fatalf("a missing directory is created: %v", err)
	}

	blocker := filepath.Join(dir, "blocker")
	_ = os.WriteFile(blocker, nil, 0o644)
	if err := r.SaveToFile(filepath.Join(blocker, "zc.json")); err == nil {
		t.Error("a directory that cannot be created is an error")
	}

	fresh := newRegistry(t)
	if err := fresh.LoadFromFile(path); err != nil || !fresh.IsZeroCost("kept") {
		t.Fatalf("LoadFromFile: %v", err)
	}
	if err := fresh.LoadFromFile(filepath.Join(dir, "missing.json")); err == nil {
		t.Error("a missing file is an error")
	}
	_ = os.WriteFile(filepath.Join(dir, "bad.json"), []byte("{"), 0o644)
	if err := fresh.LoadFromFile(filepath.Join(dir, "bad.json")); err == nil {
		t.Error("a broken file is an error")
	}
	_ = os.WriteFile(filepath.Join(dir, "raw.json"), []byte(`{"added_prefixes":["no-slash"],"removed_prefixes":["also-none"],"removed_models":["Zed"]}`), 0o644)
	if err := fresh.LoadFromFile(filepath.Join(dir, "raw.json")); err != nil || !fresh.customPrefixes["no-slash/"] || !fresh.removedPrefixes["also-none/"] || !fresh.removedModels["zed"] {
		t.Errorf("prefixes gain their slash: %v %v %v (%v)", fresh.customPrefixes, fresh.removedPrefixes, fresh.removedModels, err)
	}
}

func TestZeroCostEnvFiles(t *testing.T) {
	dir := t.TempDir()
	envFile := filepath.Join(dir, ".env")
	_ = os.WriteFile(envFile, []byte("# comment\n\nnot a pair\nOTHER=1\n"+EnvZeroCostModels+"=\"quoted-model\"\n"+EnvZeroCostPrefixes+"='team'\n"), 0o644)

	r := newRegistry(t)
	if err := r.LoadFromEnvFile(envFile); err != nil || !r.IsZeroCost("quoted-model") || !r.IsZeroCost("team/x") {
		t.Fatalf("LoadFromEnvFile: %v", err)
	}
	if err := r.LoadFromEnvFile(filepath.Join(dir, "missing")); err == nil {
		t.Error("a missing env file is an error")
	}

	r.AddModel("custom-model")
	r.AddPrefix("mine/")
	if err := r.SaveToEnvFile(envFile); err != nil {
		t.Fatal(err)
	}
	data, _ := os.ReadFile(envFile)
	if !strings.Contains(string(data), "custom-model") || !strings.Contains(string(data), "mine/") || !strings.Contains(string(data), "OTHER=1") {
		t.Errorf("env file = %s", data)
	}

	fresh := filepath.Join(dir, "fresh.env")
	if err := r.SaveToEnvFile(fresh); err != nil {
		t.Fatal(err)
	}
	onlyModels := filepath.Join(dir, "models-only.env")
	_ = os.WriteFile(onlyModels, []byte(EnvZeroCostModels+"=x\n"), 0o644)
	if err := r.SaveToEnvFile(onlyModels); err != nil {
		t.Fatal(err)
	}
	if data, _ := os.ReadFile(onlyModels); !strings.Contains(string(data), EnvZeroCostPrefixes+"=") {
		t.Errorf("the missing prefixes line is appended: %s", data)
	}
	if err := r.SaveToEnvFile(dir); err == nil {
		t.Error("a directory cannot be written as an env file")
	}
}

func TestZeroCostDetailsListCustomEntries(t *testing.T) {
	r := newRegistry(t)
	r.AddPrefix("mine/")
	r.AddModel("mine-model")
	details := r.GetDetails()
	foundPrefix, foundModel := false, false
	for _, p := range details.Prefixes {
		foundPrefix = foundPrefix || (p.Pattern == "mine/" && p.Source == SourceCustom && p.Active)
	}
	for _, m := range details.Models {
		foundModel = foundModel || (m.Pattern == "mine-model" && m.Source == SourceCustom)
	}
	if !foundPrefix || !foundModel {
		t.Errorf("details = %+v", details)
	}
}
