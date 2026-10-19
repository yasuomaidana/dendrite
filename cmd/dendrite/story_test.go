package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestSaga_ContractsAndUsage(t *testing.T) {
	t.Chdir(t.TempDir())
	steps := []struct {
		args []string
		code int
		want string
	}{
		{[]string{"saga", "start", "cache"}, ExitOK, "Story 'cache' initialized."},
		{[]string{"saga", "start", "cache", "--sync-contracts", "--schema-dir", "schemas", "--code-dir", "go=pkg"}, ExitOK, "Schema scan notice"},
		{[]string{"story", "check-contracts"}, ExitError, "no protobuf"},
		{[]string{"saga", "fork", "cache", "--variants", "only-one"}, ExitUsage, ""},
		{[]string{"saga", "fork", "cache"}, ExitUsage, ""},
		{[]string{"saga", "benchmark", "cache", "--iterations", "0"}, ExitUsage, ""},
		{[]string{"saga", "resolve", "cache"}, ExitUsage, ""},
		{[]string{"saga", "status", "cache"}, ExitError, "has no active fork"},
		{[]string{"saga", "next"}, ExitUsage, ""},
	}
	for _, s := range steps {
		out, code := cliOutput(t, s.args...)
		if code != s.code || !strings.Contains(out, s.want) {
			t.Errorf("%v: exit %d (want %d)\n%s", s.args, code, s.code, out)
		}
	}
}

func TestSaga_ForkStatusAbort(t *testing.T) {
	repo, _ := setupWorkRepo(t)
	t.Chdir(repo)
	for k, v := range map[string]string{"GIT_AUTHOR_NAME": "t", "GIT_AUTHOR_EMAIL": "t@example.com", "GIT_COMMITTER_NAME": "t", "GIT_COMMITTER_EMAIL": "t@example.com"} {
		t.Setenv(k, v)
	}
	if out, code := cliOutput(t, "saga", "fork", "cache", "--variants", "lru,arc"); code != ExitOK || !strings.Contains(out, "Variant 'arc'") {
		t.Fatalf("fork: exit %d\n%s", code, out)
	}
	if out, code := cliOutput(t, "saga", "status", "cache"); code != ExitOK || !strings.Contains(out, "Variants (2)") {
		t.Fatalf("status: exit %d\n%s", code, out)
	}
	if out, code := cliOutput(t, "saga", "status", "cache", "--json"); code != ExitOK || !strings.Contains(out, "{") {
		t.Fatalf("status --json: exit %d\n%s", code, out)
	}
	if out, code := cliOutput(t, "saga", "abort", "cache"); code != ExitOK || !strings.Contains(out, "successfully purged") {
		t.Fatalf("abort: exit %d\n%s", code, out)
	}
}

func TestSaga_CheckContractsWithSchema(t *testing.T) {
	dir := t.TempDir()
	t.Chdir(dir)
	if err := os.MkdirAll(filepath.Join(dir, "schemas"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "schemas", "user.proto"), []byte("syntax = \"proto3\";\nmessage User {\n  string id = 1;\n}\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	for _, args := range [][]string{{"saga", "check-contracts"}, {"saga", "check-contracts", "--json"}, {"saga", "start", "s", "--sync-contracts"}} {
		out, code := cliOutput(t, args...)
		if code == ExitUsage || !(strings.Contains(out, "Scanned Contract Specifications") || strings.Contains(out, "scanned_contracts")) {
			t.Errorf("%v: exit %d\n%s", args, code, out)
		}
	}
}
