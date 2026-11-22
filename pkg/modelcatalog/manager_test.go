package modelcatalog

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

type scriptedSource struct {
	name   string
	models []Model
	err    error
	calls  int
}

func (f *scriptedSource) Name() string { return f.name }
func (f *scriptedSource) List(context.Context) ([]Model, error) {
	f.calls++
	return f.models, f.err
}

func newManager(t *testing.T, sources ...Source) (*Manager, *time.Time, *int) {
	t.Helper()
	now := time.Date(2026, 11, 17, 9, 0, 0, 0, time.UTC)
	spawns := 0
	m := New(t.TempDir(), sources...)
	m.Now = func() time.Time { return now }
	m.Getenv = func(string) string { return "" }
	m.Spawn = func() error { spawns++; return nil }
	return m, &now, &spawns
}

func TestRefreshStoresModelsRolesAndNotesFailedSources(t *testing.T) {
	m, now, _ := newManager(t,
		&scriptedSource{name: "gateway", models: sample},
		&scriptedSource{name: "gemini", err: errors.New("HTTP 403")},
	)
	m.LocalPicker = func(context.Context) (string, bool) { return "local-coder", true }
	c, err := m.Refresh(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if c.Roles[RoleCheap] != "gemini-3.8-flash-medium" || c.Roles[RoleLocal] != "local-coder" || !c.FetchedAt.Equal(*now) {
		t.Errorf("catalog = %+v", c)
	}
	if len(c.Notes) != 1 || !strings.Contains(c.Notes[0], "gemini: HTTP 403") {
		t.Errorf("notes = %v", c.Notes)
	}
	if stored, _ := Load(m.Dir); stored.Roles[RoleStrong] != c.Roles[RoleStrong] {
		t.Errorf("stored = %+v", stored)
	}
}

func TestRefreshKeepsTheStoredCatalogWhenEverySourceFails(t *testing.T) {
	m, _, _ := newManager(t, &scriptedSource{name: "gateway", models: sample})
	if _, err := m.Refresh(context.Background()); err != nil {
		t.Fatal(err)
	}
	m.Sources = []Source{&scriptedSource{name: "gateway", err: errors.New("unreachable")}}
	if _, err := m.Refresh(context.Background()); err == nil {
		t.Fatal("all sources failed without an error")
	}
	if c, _ := Load(m.Dir); len(c.Models) == 0 {
		t.Error("a failed refresh wiped the stored catalog")
	}
}

func TestRoleOrderIsOverrideThenCatalogThenFallback(t *testing.T) {
	m, _, _ := newManager(t, &scriptedSource{name: "gateway", models: sample})
	if got := m.Role(RoleCheap, "fallback-model"); got != "fallback-model" {
		t.Errorf("no catalog: %q", got)
	}
	if _, err := m.Refresh(context.Background()); err != nil {
		t.Fatal(err)
	}
	if got := m.Role(RoleCheap, "fallback-model"); got != "gemini-3.8-flash-medium" {
		t.Errorf("catalog: %q", got)
	}
	if got := m.Role("unknown-role", "fallback-model"); got != "fallback-model" {
		t.Errorf("unknown role: %q", got)
	}
	if err := os.WriteFile(filepath.Join(m.Dir, "models.toml"), []byte("[roles]\ncheap = \"from-toml\"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if got := m.Role(RoleCheap, "fallback-model"); got != "from-toml" {
		t.Errorf("models.toml: %q", got)
	}
	m.Getenv = func(k string) string {
		if k == "DENDRITE_MODEL_CHEAP" {
			return "from-env"
		}
		return ""
	}
	if got := m.Role(RoleCheap, "fallback-model"); got != "from-env" {
		t.Errorf("env: %q", got)
	}
}

func TestStaleReadStartsExactlyOneBackgroundRefresh(t *testing.T) {
	m, now, spawns := newManager(t, &scriptedSource{name: "gateway", models: sample})
	m.Role(RoleCheap, "f")
	m.Role(RoleCheap, "f")
	if *spawns != 1 {
		t.Fatalf("spawns after two reads of a missing catalog = %d, want 1", *spawns)
	}
	if _, err := m.Refresh(context.Background()); err != nil {
		t.Fatal(err)
	}
	m.Role(RoleCheap, "f")
	if *spawns != 1 {
		t.Errorf("a fresh catalog started a refresh (spawns = %d)", *spawns)
	}
	*now = now.Add(TTL + time.Hour)
	if got := m.Role(RoleCheap, "f"); got != "gemini-3.8-flash-medium" {
		t.Errorf("a stale catalog should still answer: %q", got)
	}
	if *spawns != 2 {
		t.Errorf("spawns after the catalog went stale = %d, want 2", *spawns)
	}
	m.Role(RoleCheap, "f")
	if *spawns != 2 {
		t.Errorf("a second stale read inside the gap started another refresh (spawns = %d)", *spawns)
	}
}

func TestRefreshIfStale(t *testing.T) {
	src := &scriptedSource{name: "gateway", models: sample}
	m, now, _ := newManager(t, src)
	for step, want := range []struct {
		advance   time.Duration
		refreshed bool
	}{{0, true}, {time.Hour, false}, {TTL, true}} {
		*now = now.Add(want.advance)
		got, err := m.RefreshIfStale(context.Background())
		if err != nil || got != want.refreshed {
			t.Errorf("step %d: refreshed = %v, %v; want %v", step, got, err, want.refreshed)
		}
	}
	if src.calls != 2 {
		t.Errorf("source asked %d times, want 2", src.calls)
	}
}

func TestLaunchdPlistRunsWeeklyOnMonday(t *testing.T) {
	p := LaunchdPlist("/usr/local/bin/dendrite", "/tmp/logs")
	for _, want := range []string{ScheduleLabel, "StartCalendarInterval", "<key>Weekday</key>\n        <integer>1</integer>",
		"<string>/usr/local/bin/dendrite</string>", "<string>models</string>", "<string>refresh</string>", "/tmp/logs/models-refresh.log"} {
		if !strings.Contains(p, want) {
			t.Errorf("plist lacks %q:\n%s", want, p)
		}
	}
	dir := t.TempDir()
	path, err := InstallSchedule(dir, "/bin/dendrite", dir)
	if err != nil || filepath.Base(path) != ScheduleLabel+".plist" {
		t.Fatalf("install = %q, %v", path, err)
	}
	if _, err := os.Stat(path); err != nil {
		t.Error(err)
	}
	if _, err := RemoveSchedule(dir); err != nil {
		t.Fatal(err)
	}
	if _, err := RemoveSchedule(dir); err != nil {
		t.Errorf("removing twice: %v", err)
	}
}

func TestAFailedRefreshIsNotRetriedEveryTick(t *testing.T) {
	src := &scriptedSource{name: "gateway", err: errors.New("unreachable")}
	m, now, _ := newManager(t, src)
	if refreshed, err := m.RefreshIfStale(context.Background()); !refreshed || err == nil {
		t.Fatalf("first attempt: refreshed = %v, err = %v", refreshed, err)
	}
	*now = now.Add(time.Minute)
	if refreshed, _ := m.RefreshIfStale(context.Background()); refreshed || src.calls != 1 {
		t.Errorf("a retry one minute later asked the providers again (calls = %d)", src.calls)
	}
	*now = now.Add(attemptGap)
	if refreshed, _ := m.RefreshIfStale(context.Background()); !refreshed || src.calls != 2 {
		t.Errorf("no retry after the gap (calls = %d)", src.calls)
	}
}

func TestModelsTomlOverridesRolesAndAsksForPreviews(t *testing.T) {
	m, _, _ := newManager(t, &scriptedSource{name: "gateway", models: sample})
	toml := "[roles]\ncheap = \"pinned-cheap\"\n[catalog]\ninclude_preview = true\n"
	if err := os.WriteFile(filepath.Join(m.Dir, "models.toml"), []byte(toml), 0o644); err != nil {
		t.Fatal(err)
	}
	if !m.IncludePreview() {
		t.Error("include_preview was not read")
	}
	c, err := m.Refresh(context.Background())
	if err != nil || c.Roles[RoleCheap] != "pinned-cheap" || c.Roles[RoleStrong] != "gemini-4-argon-medium" {
		t.Errorf("roles = %v, %v (the pin replaces cheap only)", c.Roles, err)
	}
	if err := os.WriteFile(filepath.Join(m.Dir, "models.toml"), []byte("[[[ not toml"), 0o644); err != nil {
		t.Fatal(err)
	}
	if m.IncludePreview() {
		t.Error("a broken models.toml should mean no configuration")
	}
	if got := m.Role("never-set", "fallback"); got != "fallback" {
		t.Errorf("role with a broken models.toml = %q", got)
	}
}

func TestSchedulePathsAndFailures(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	path, err := PlistPath("")
	if err != nil || path != filepath.Join(home, "Library", "LaunchAgents", ScheduleLabel+".plist") {
		t.Fatalf("default plist path = %q, %v", path, err)
	}
	file := filepath.Join(t.TempDir(), "plain")
	if err := os.WriteFile(file, nil, 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := InstallSchedule(filepath.Join(file, "sub"), "/bin/dendrite", t.TempDir()); err == nil {
		t.Error("installing below a file succeeded")
	}
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, ScheduleLabel+".plist", "keep"), 0o755); err != nil {
		t.Fatal(err)
	}
	if _, err := RemoveSchedule(dir); err == nil {
		t.Error("removing a plist that is a non-empty directory succeeded")
	}
}

func TestRefreshAttemptsAreNotRecordedWhenTheHomeCannotBeWritten(t *testing.T) {
	file := filepath.Join(t.TempDir(), "plain")
	if err := os.WriteFile(file, nil, 0o644); err != nil {
		t.Fatal(err)
	}
	m, _, spawns := newManager(t, &scriptedSource{name: "gateway", models: sample})
	m.Dir = filepath.Join(file, "home")
	m.Role(RoleCheap, "f")
	if *spawns != 0 {
		t.Errorf("a refresh was started although the attempt could not be recorded (spawns = %d)", *spawns)
	}
}
