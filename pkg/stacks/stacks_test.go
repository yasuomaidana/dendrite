package stacks

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func write(t *testing.T, path, body string, mode os.FileMode) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(body), mode); err != nil {
		t.Fatal(err)
	}
}

func TestDirIsUnderTheHomeUnlessOverridden(t *testing.T) {
	home := t.TempDir()
	t.Setenv("DENDRITE_HOME", home)
	t.Setenv(EnvDir, "")
	if got, want := Dir(), filepath.Join(home, "stacks"); got != want {
		t.Errorf("Dir = %q, want %q", got, want)
	}
	if got, want := Path("compose.control.yaml"), filepath.Join(home, "stacks", "compose.control.yaml"); got != want {
		t.Errorf("Path = %q, want %q", got, want)
	}
	t.Setenv(EnvDir, "/elsewhere")
	if Dir() != "/elsewhere" {
		t.Errorf("override ignored: %q", Dir())
	}
}

func checkout(t *testing.T) string {
	t.Helper()
	src := filepath.Join(t.TempDir(), "stacks")
	write(t, filepath.Join(src, "compose.control.yaml"), "services: {}\n", 0o644)
	write(t, filepath.Join(src, "litellm_config.yaml"), "model_list: []\n", 0o644)
	write(t, filepath.Join(src, "entrypoint-plane.sh"), "#!/bin/sh\n", 0o755)
	write(t, filepath.Join(src, "grafana", "dash.json"), "{}\n", 0o644)
	write(t, filepath.Join(src, ".env.example"), "TEMPLATE=1\n", 0o644)
	write(t, filepath.Join(src, ".env"), "SECRET=1\n", 0o600)
	write(t, filepath.Join(src, ".DS_Store"), "x", 0o644)
	return src
}

func TestMigrateCopiesTheStackButNeverCredentialsOrNoise(t *testing.T) {
	src, dst := checkout(t), filepath.Join(t.TempDir(), "stacks")
	rep, err := Migrate(src, dst, false)
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"compose.control.yaml", "entrypoint-plane.sh", "grafana/dash.json", "litellm_config.yaml"}
	if len(rep.Copied) != len(want) {
		t.Fatalf("copied = %v, want %v", rep.Copied, want)
	}
	for i, w := range want {
		if rep.Copied[i] != w {
			t.Errorf("copied[%d] = %q, want %q", i, rep.Copied[i], w)
		}
	}
	for _, never := range []string{".env", ".env.example", ".DS_Store"} {
		if _, err := os.Stat(filepath.Join(dst, never)); err == nil {
			t.Errorf("%s was copied", never)
		}
	}
	if info, _ := os.Stat(filepath.Join(dst, "entrypoint-plane.sh")); info.Mode().Perm() != 0o755 {
		t.Errorf("script mode = %v, want 0755 preserved", info.Mode().Perm())
	}
}

func TestMigrateNeverOverwritesAndIsRepeatable(t *testing.T) {
	src, dst := checkout(t), filepath.Join(t.TempDir(), "stacks")
	write(t, filepath.Join(dst, "compose.control.yaml"), "services: {mine: true}\n", 0o644)
	rep, err := Migrate(src, dst, false)
	if err != nil {
		t.Fatal(err)
	}
	if got, _ := os.ReadFile(filepath.Join(dst, "compose.control.yaml")); string(got) != "services: {mine: true}\n" {
		t.Errorf("an existing destination file was overwritten: %q", got)
	}
	if len(rep.Kept) != 1 || rep.Kept[0] != "compose.control.yaml" {
		t.Errorf("kept = %v", rep.Kept)
	}
	again, err := Migrate(src, dst, false)
	if err != nil || len(again.Copied) != 0 {
		t.Errorf("a second run copied %v (err %v)", again.Copied, err)
	}
}

func TestMigrateDryRunWritesNothing(t *testing.T) {
	src, dst := checkout(t), filepath.Join(t.TempDir(), "stacks")
	rep, err := Migrate(src, dst, true)
	if err != nil || len(rep.Copied) == 0 {
		t.Fatalf("rep=%+v err=%v", rep, err)
	}
	if _, err := os.Stat(dst); err == nil {
		t.Error("dry run created the destination")
	}
}

func TestMigrateOntoItselfDoesNothingAndAMissingSourceIsAnError(t *testing.T) {
	src := checkout(t)
	if rep, err := Migrate(src, src, false); err != nil || len(rep.Copied) != 0 {
		t.Errorf("self-migration: %+v %v", rep, err)
	}
	if _, err := Migrate(filepath.Join(t.TempDir(), "nope"), t.TempDir(), false); err == nil {
		t.Error("a missing source was accepted")
	}
}

func TestCheckoutStacksNeedsAControlCompose(t *testing.T) {
	root := t.TempDir()
	if CheckoutStacks(root) != "" {
		t.Error("an empty checkout has stacks")
	}
	write(t, filepath.Join(root, "stacks", "compose.control.yaml"), "x", 0o644)
	if CheckoutStacks(root) != filepath.Join(root, "stacks") {
		t.Errorf("got %q", CheckoutStacks(root))
	}
}

func TestATestBinaryWithoutAHomeNeverResolvesToTheRealHome(t *testing.T) {
	t.Setenv("DENDRITE_HOME", "")
	t.Setenv(EnvDir, "")
	got := Dir()
	if !strings.HasPrefix(got, os.TempDir()) {
		t.Errorf("Dir = %q, want a throwaway directory under %s", got, os.TempDir())
	}
	if real, err := os.UserHomeDir(); err == nil && strings.HasPrefix(got, filepath.Join(real, ".dendrite")) {
		t.Errorf("Dir = %q points into the real home from a test", got)
	}
}
