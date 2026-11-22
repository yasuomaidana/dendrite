package modelcatalog

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func writeScript(t *testing.T, body string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "agy")
	if err := os.WriteFile(path, []byte("#!/bin/sh\n"+body+"\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	return path
}

// configuredManager is Default() with every address and key coming from the environment.
func configuredManager(t *testing.T) *Manager {
	t.Helper()
	home := t.TempDir()
	t.Setenv("DENDRITE_HOME", home)
	t.Setenv("LITELLM_PROXY_URL", "http://gw:4000")
	t.Setenv("LITELLM_MASTER_KEY", "") // a runner that exports the gateway key must not leak into the expectation
	t.Setenv("DENDRITE_OLLAMA_ENDPOINT", "")
	t.Setenv("OLLAMA_HOST", "ollama-box:11434")
	t.Setenv("GEMINI_API_KEY", "g-key")
	t.Setenv("COPILOT_API_KEY", "c-key")
	if err := os.WriteFile(filepath.Join(home, configFile), []byte("[catalog]\ninclude_preview = true\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	m, err := Default()
	if err != nil {
		t.Fatal(err)
	}
	return m
}

func TestDefaultHasEverySourceAndSpawnsItself(t *testing.T) {
	m := configuredManager(t)
	var names []string
	for _, s := range m.Sources {
		names = append(names, s.Name())
	}
	if got := strings.Join(names, ","); got != "gateway,ollama,agy,gemini,copilot" || m.Spawn == nil {
		t.Errorf("sources = %s, spawn set = %v", got, m.Spawn != nil)
	}
}

func TestDefaultReadsAddressesAndKeysFromTheEnvironment(t *testing.T) {
	m := configuredManager(t)
	got := []any{m.Sources[0], m.Sources[1], m.Sources[3], m.Sources[4]}
	want := []any{
		GatewaySource{URL: "http://gw:4000"},
		OllamaSource{URL: "http://ollama-box:11434"}, // a bare host gets a scheme
		GeminiSource{Key: "g-key", Preview: true},    // include_preview comes from models.toml
		CopilotSource{Token: "c-key"},
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("source %d = %+v, want %+v", i, got[i], want[i])
		}
	}
}

func TestListAGYRunsTheCLI(t *testing.T) {
	ok := writeScript(t, `printf 'gemini-3.8-flash-low\tFlash\nclaude-opus-4.6\tOpus\n'`)
	got, err := ListAGY(context.Background(), ok)
	if err != nil || strings.Join(got, ",") != "claude-opus-4.6,gemini-3.8-flash-low" {
		t.Fatalf("list = %v, %v", got, err)
	}
	if _, err := ListAGY(context.Background(), writeScript(t, `echo "nothing here"`)); err == nil || !strings.Contains(err.Error(), "no models") {
		t.Errorf("empty listing: err = %v", err)
	}
	if _, err := ListAGY(context.Background(), writeScript(t, `exit 3`)); err == nil {
		t.Error("a failing agy was not an error")
	}
}

func TestSpawnSelfStartsTheRefreshWithoutWaiting(t *testing.T) {
	marker := filepath.Join(t.TempDir(), "ran")
	script := writeScript(t, `echo "$@" > `+marker)
	old := selfPath
	selfPath = func() (string, error) { return script, nil }
	t.Cleanup(func() { selfPath = old })
	if err := SpawnSelf(); err != nil {
		t.Fatal(err)
	}
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		if data, err := os.ReadFile(marker); err == nil {
			if got := strings.TrimSpace(string(data)); got != "cleft models refresh --quiet" {
				t.Errorf("args = %q", got)
			}
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Error("the spawned refresh never ran")
}

func TestSpawnSelfReportsAMissingBinary(t *testing.T) {
	old := selfPath
	selfPath = func() (string, error) { return filepath.Join(t.TempDir(), "absent"), nil }
	t.Cleanup(func() { selfPath = old })
	if err := SpawnSelf(); err == nil {
		t.Error("a missing binary was not an error")
	}
}

func TestDefaultUsesLocalAddressesWhenNothingIsConfigured(t *testing.T) {
	t.Setenv("DENDRITE_HOME", t.TempDir())
	for _, k := range []string{"LITELLM_PROXY_URL", "DENDRITE_OLLAMA_ENDPOINT", "OLLAMA_HOST"} {
		t.Setenv(k, "")
	}
	m, err := Default()
	if err != nil {
		t.Fatal(err)
	}
	if g := m.Sources[0].(GatewaySource); g.URL != "http://localhost:4000" {
		t.Errorf("gateway = %+v", g)
	}
	if o := m.Sources[1].(OllamaSource); o.URL != "http://localhost:11434" {
		t.Errorf("ollama = %+v", o)
	}
}

func TestDefaultNeedsAHomeDirectory(t *testing.T) {
	t.Setenv("DENDRITE_HOME", "")
	t.Setenv("HOME", "")
	if _, err := Default(); err == nil {
		t.Error("Default worked without any home directory")
	}
}

func TestTheAGYSourceRunsAgyFromThePath(t *testing.T) {
	t.Setenv("DENDRITE_HOME", t.TempDir())
	script := writeScript(t, `printf 'gemini-3.8-flash-low\tFlash\n'`)
	t.Setenv("PATH", filepath.Dir(script))
	m, err := Default()
	if err != nil {
		t.Fatal(err)
	}
	got, err := m.Sources[2].List(context.Background())
	if err != nil || len(got) != 1 || got[0].Name != "gemini-3.8-flash-low" {
		t.Errorf("agy source = %+v, %v", got, err)
	}
}

func TestSpawnSelfReportsAnUnknownExecutablePath(t *testing.T) {
	old := selfPath
	selfPath = func() (string, error) { return "", os.ErrNotExist }
	t.Cleanup(func() { selfPath = old })
	if err := SpawnSelf(); err == nil {
		t.Error("no executable path was not an error")
	}
}

func TestScheduleFunctionsReportAMissingHome(t *testing.T) {
	old := userHomeDir
	userHomeDir = func() (string, error) { return "", os.ErrNotExist }
	t.Cleanup(func() { userHomeDir = old })
	if _, err := PlistPath(""); err == nil {
		t.Error("PlistPath without a home")
	}
	if _, err := InstallSchedule("", "/bin/dendrite", t.TempDir()); err == nil {
		t.Error("InstallSchedule without a home")
	}
	if _, err := RemoveSchedule(""); err == nil {
		t.Error("RemoveSchedule without a home")
	}
}
