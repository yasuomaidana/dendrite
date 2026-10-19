package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestAfferent_LifecycleHonorsStateDir(t *testing.T) {
	t.Setenv("DENDRITE_STATE_DIR", "")
	state := t.TempDir()

	steps := []struct {
		args []string
		code int
		want string
	}{
		{[]string{"afferent", "list", "--json", "--state-dir", state}, ExitOK, "["},
		{[]string{"--state-dir", state, "afferent", "add", "--id", "otp", "--repo", "https://example.com/otp", "--lang", "go", "--caps", "totp,hotp"}, ExitOK, "Registered Tier 1 Source Reference: otp"},
		{[]string{"ref", "ls", "--state-dir", state}, ExitOK, "Capabilities: totp, hotp"},
		{[]string{"afferent", "add", "--id", "x"}, ExitUsage, ""},
		{[]string{"afferent", "rm", "otp", "--state-dir", state}, ExitOK, "Removed Tier 1 Source Reference: otp"},
		{[]string{"afferent", "remove"}, ExitUsage, ""},
	}
	for _, s := range steps {
		out, code := cliOutput(t, s.args...)
		if code != s.code || !strings.Contains(out, s.want) {
			t.Errorf("%v: exit %d (want %d)\n%s", s.args, code, s.code, out)
		}
	}
	if _, err := os.Stat(filepath.Join(state, "refs.json")); err != nil {
		t.Fatalf("--state-dir was not used for the reference registry: %v", err)
	}
}
