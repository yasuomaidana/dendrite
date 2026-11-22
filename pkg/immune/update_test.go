package immune_test

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"Dendrite/pkg/immune"
	"Dendrite/pkg/testkit"
)

// uvWorld is an isolated home with a uv stand-in that "installs" SkillSpector by writing a
// scanner script of the requested version into ~/.local/bin (the latest being 3.0.0), and logs
// every call. It returns the log path.
func uvWorld(t *testing.T) string {
	t.Helper()
	home := t.TempDir()
	log := filepath.Join(home, "uv.log")
	uv := filepath.Join(home, "uv")
	testkit.WriteFile(t, uv, `#!/bin/sh
echo "$*" >> "`+log+`"
version=3.0.0
for a in "$@"; do case "$a" in skillspector==*) version="${a#skillspector==}";; esac; done
if [ "$version" = "0.0.0" ]; then echo "no such version" >&2; exit 2; fi
if [ "$version" = "6.6.6" ]; then version=6.6.7; fi
if [ "$version" = "7.7.7" ]; then rm -f "$DENDRITE_SKILLSPECTOR_BIN"; exit 0; fi
mkdir -p "$HOME/.local/bin"
printf '#!/bin/sh\necho "SkillSpector v%s"\n' "$version" > "$HOME/.local/bin/skillspector"
chmod +x "$HOME/.local/bin/skillspector"
`, 0o755)
	t.Setenv("HOME", home)
	t.Setenv("PATH", "/usr/bin:/bin")
	t.Setenv(immune.BinaryEnv, "")
	t.Setenv(immune.UVEnv, uv)
	return log
}

func uvCalls(t *testing.T, log string) []string {
	t.Helper()
	data, err := os.ReadFile(log)
	if os.IsNotExist(err) {
		return nil
	}
	testkit.Check(t, err)
	return strings.Split(strings.TrimSpace(string(data)), "\n")
}

func TestVersion(t *testing.T) {
	testkit.Table(t, map[string]string{
		"SkillSpector v2.12.0": "2.12.0", "skillspector-test 9.9": "9.9", "2.12.0\n": "2.12.0", "unknown": "unknown", "": "",
	}, immune.Version)
}

// Refresh installs the pin when the scanner is missing or another version, and does nothing,
// not even start uv, once the scanner is at the pin.
func TestRefreshToThePinIsIdempotent(t *testing.T) {
	log := uvWorld(t)
	ctx := context.Background()
	u, err := immune.Refresh(ctx, "2.12.0")
	testkit.Check(t, err)
	testkit.Equal(t, "first", u, immune.Update{Before: "", After: "2.12.0", Pin: "2.12.0", Changed: true})

	u, err = immune.Refresh(ctx, "2.12.0")
	testkit.Check(t, err)
	testkit.Equal(t, "again", u, immune.Update{Before: "2.12.0", After: "2.12.0", Pin: "2.12.0"})

	u, err = immune.Refresh(ctx, "2.13.1")
	testkit.Check(t, err)
	testkit.Equal(t, "new pin", u, immune.Update{Before: "2.12.0", After: "2.13.1", Pin: "2.13.1", Changed: true})

	calls := uvCalls(t, log)
	testkit.Equal(t, "uv calls", strings.Join(calls, " | "), "tool install --force skillspector==2.12.0 | tool install --force skillspector==2.13.1")
}

// Without a pin Refresh installs the scanner when there is none and upgrades it otherwise.
func TestRefreshWithoutAPinTakesTheLatest(t *testing.T) {
	log := uvWorld(t)
	ctx := context.Background()
	u, err := immune.Refresh(ctx, "")
	testkit.Check(t, err)
	testkit.Equal(t, "install", u, immune.Update{After: "3.0.0", Changed: true})
	u, err = immune.Refresh(ctx, "")
	testkit.Check(t, err)
	testkit.Equal(t, "upgrade", u, immune.Update{Before: "3.0.0", After: "3.0.0"})
	testkit.Equal(t, "uv calls", strings.Join(uvCalls(t, log), " | "), "tool install skillspector | tool upgrade skillspector")
}

func TestRefreshFailures(t *testing.T) {
	uvWorld(t)
	ctx := context.Background()
	if _, err := immune.Refresh(ctx, "0.0.0"); err == nil || !strings.Contains(err.Error(), "no such version") {
		t.Fatalf("a failing install must show what uv said: %v", err)
	}
	if _, err := immune.Refresh(ctx, "6.6.6"); !errors.Is(err, immune.ErrPinMismatch) {
		t.Fatalf("an install that lands on another version: %v", err)
	}

}

// What Refresh needs and cannot find is named: the scanner an install took away, uv, or a
// scanner override that does not work.
func TestRefreshNamesWhatIsMissing(t *testing.T) {
	uvWorld(t)
	ctx := context.Background()
	// An install that takes the scanner away is reported, not taken for success.
	gone := scannerScript(t, `echo "SkillSpector v1.0.0"`)
	t.Setenv(immune.BinaryEnv, gone)
	if _, err := immune.Refresh(ctx, "7.7.7"); err == nil || !strings.Contains(err.Error(), immune.BinaryEnv) {
		t.Fatalf("a scanner gone after the install: %v", err)
	}
	t.Setenv(immune.BinaryEnv, "")

	t.Setenv(immune.UVEnv, filepath.Join(t.TempDir(), "absent"))
	if _, err := immune.Refresh(ctx, "1.0.0"); err == nil || !strings.Contains(err.Error(), immune.UVEnv) {
		t.Fatalf("a bad %s must fail by name: %v", immune.UVEnv, err)
	}
	t.Setenv(immune.UVEnv, "")
	t.Setenv("PATH", t.TempDir())
	if _, err := immune.Refresh(ctx, "1.0.0"); !errors.Is(err, immune.ErrUVMissing) {
		t.Fatalf("without uv: %v", err)
	}
	t.Setenv(immune.BinaryEnv, filepath.Join(t.TempDir(), "absent"))
	if _, err := immune.Refresh(ctx, "1.0.0"); err == nil || !strings.Contains(err.Error(), immune.BinaryEnv) {
		t.Fatalf("a bad %s must fail by name: %v", immune.BinaryEnv, err)
	}
}

// uv found on PATH is used when no override is set.
func TestRefreshFindsUVOnPath(t *testing.T) {
	log := uvWorld(t)
	uv := os.Getenv(immune.UVEnv)
	t.Setenv(immune.UVEnv, "")
	t.Setenv("PATH", filepath.Dir(uv)+":/usr/bin:/bin")
	_, err := immune.Refresh(context.Background(), "2.12.0")
	testkit.Check(t, err)
	testkit.Equal(t, "uv calls", len(uvCalls(t, log)), 1)
}
