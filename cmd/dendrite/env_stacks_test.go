package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func stacksCheckout(t *testing.T) (repo, dest string) {
	t.Helper()
	repo, dest = t.TempDir(), filepath.Join(t.TempDir(), "home-stacks")
	if err := os.Mkdir(filepath.Join(repo, ".git"), 0o755); err != nil {
		t.Fatal(err)
	}
	for name, body := range map[string]string{
		"stacks/compose.control.yaml": "services: {}\n",
		"stacks/litellm_config.yaml":  "model_list: []\n",
		"stacks/.env.example":         "X=1\n",
	} {
		p := filepath.Join(repo, name)
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	t.Chdir(repo)
	t.Setenv("DENDRITE_STACKS_DIR", dest)
	return repo, dest
}

func TestSomaStacks_PathMigrateAndUp(t *testing.T) {
	_, dest := stacksCheckout(t)
	calls := useCompose(t, nil)

	if out, code := cliOutput(t, "soma", "stacks", "path"); code != ExitOK || strings.TrimSpace(out) != dest {
		t.Fatalf("path: exit %d, got %q want %q", code, out, dest)
	}
	if out, code := cliOutput(t, "soma", "stacks", "migrate", "--dry-run"); code != ExitOK || !strings.Contains(out, "Would copy 2 file(s)") {
		t.Fatalf("dry-run: exit %d\n%s", code, out)
	}
	if _, err := os.Stat(dest); err == nil {
		t.Fatal("dry run created the destination")
	}

	out, code := cliOutput(t, "soma", "stacks", "migrate")
	if code != ExitOK || !strings.Contains(out, "Copied 2 file(s)") || !strings.Contains(out, "migrate --up") {
		t.Fatalf("migrate: exit %d\n%s", code, out)
	}
	if _, err := os.Stat(filepath.Join(dest, ".env.example")); err == nil {
		t.Error("the template was copied")
	}
	if len(*calls) != 0 {
		t.Errorf("compose ran without --up: %v", *calls)
	}

	if out, code = cliOutput(t, "soma", "stacks", "migrate", "--up"); code != ExitOK || !strings.Contains(out, "Copied 0 file(s)") {
		t.Fatalf("--up: exit %d\n%s", code, out)
	}
	want := filepath.Join(dest, "compose.control.yaml") + " up -d"
	if len(*calls) != 1 || (*calls)[0] != want {
		t.Errorf("compose calls = %v, want [%s]", *calls, want)
	}
}

func TestSomaStacks_MigrateWithoutACheckoutAsksForFrom(t *testing.T) {
	t.Chdir(t.TempDir())
	t.Setenv("DENDRITE_STACKS_DIR", filepath.Join(t.TempDir(), "s"))
	if _, code := cliOutput(t, "soma", "stacks", "migrate"); code != ExitUsage {
		t.Errorf("exit %d, want %d", code, ExitUsage)
	}
}

func TestAdoptCheckoutStacksCopiesOnceAndOnlyWhenTheHomeHasNone(t *testing.T) {
	_, dest := stacksCheckout(t)
	var out strings.Builder
	adoptCheckoutStacks(&out)
	if !strings.Contains(out.String(), "Copied 2 stack file(s)") {
		t.Fatalf("first adoption said %q", out.String())
	}
	if err := os.WriteFile(filepath.Join(dest, "compose.control.yaml"), []byte("mine\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	out.Reset()
	adoptCheckoutStacks(&out)
	if out.Len() != 0 {
		t.Errorf("a second adoption ran: %q", out.String())
	}
	if got, _ := os.ReadFile(filepath.Join(dest, "compose.control.yaml")); string(got) != "mine\n" {
		t.Errorf("a maintained file was overwritten: %q", got)
	}
}
