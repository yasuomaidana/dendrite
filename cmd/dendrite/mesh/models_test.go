package mesh

import (
	"Dendrite/pkg/modelcatalog"
	"bytes"
	"context"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

type listSource []modelcatalog.Model

func (listSource) Name() string { return "list" }
func (l listSource) List(context.Context) ([]modelcatalog.Model, error) {
	return l, nil
}

func testManager(t *testing.T) *modelcatalog.Manager {
	t.Helper()
	m := modelcatalog.New(t.TempDir(), listSource{
		{Name: "gemini-3.8-flash-medium", Provider: "gemini", Upstream: "gemini-3.8-flash-medium", InputCost: 7.5e-8, MaxInput: 1048576, Mode: "chat", Routed: true, Source: "gateway"},
		{Name: "gemini-5", Provider: "gemini", Upstream: "gemini-5", Source: "gemini"},
	})
	m.Getenv = func(string) string { return "" }
	m.Now = func() time.Time { return time.Date(2026, 11, 17, 9, 0, 0, 0, time.UTC) }
	return m
}

func TestModelsListExplainsAnEmptyCatalogThenShowsRolesAndNewModels(t *testing.T) {
	m := testManager(t)
	var out bytes.Buffer
	if err := runModelsList(&out, m); err != nil || !strings.Contains(out.String(), "cleft models refresh") {
		t.Fatalf("empty: %q, %v", out.String(), err)
	}
	out.Reset()
	if err := runModelsRefresh(context.Background(), &out, m); err != nil || !strings.Contains(out.String(), "cheap=gemini-3.8-flash-medium") {
		t.Fatalf("refresh: %q, %v", out.String(), err)
	}
	out.Reset()
	if err := runModelsList(&out, m); err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"fresh", "cheap", "gemini-3.8-flash-medium", "0.075", "1048k", "gemini-5"} {
		if !strings.Contains(out.String(), want) {
			t.Errorf("list lacks %q:\n%s", want, out.String())
		}
	}
}

func TestModelsScheduleWritesAndRemovesThePlistWithoutLoadingIt(t *testing.T) {
	dir := t.TempDir()
	var out bytes.Buffer
	if err := runModelsSchedule(&out, dir, false, false); err != nil {
		t.Fatal(err)
	}
	plist := filepath.Join(dir, modelcatalog.ScheduleLabel+".plist")
	data, err := os.ReadFile(plist)
	if err != nil || !strings.Contains(string(data), "StartCalendarInterval") {
		t.Fatalf("plist = %q, %v", data, err)
	}
	out.Reset()
	if err := runModelsSchedule(&out, dir, true, false); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(plist); !os.IsNotExist(err) {
		t.Errorf("plist still there after --remove: %v", err)
	}
}

func recordLaunchctl(t *testing.T, fail bool) *[]string {
	t.Helper()
	var calls []string
	oldCtl := launchctl
	launchctl = func(args ...string) ([]byte, error) {
		calls = append(calls, strings.Join(args[:1], ""))
		if fail && args[0] == "bootstrap" {
			return []byte("Bootstrap failed: 5"), os.ErrInvalid
		}
		return nil, nil
	}
	t.Cleanup(func() { launchctl = oldCtl })
	return &calls
}

func TestModelsScheduleLoadsAndUnloadsThroughLaunchctl(t *testing.T) {
	calls := recordLaunchctl(t, false)
	dir := t.TempDir()
	var out bytes.Buffer
	if err := runModelsSchedule(&out, dir, false, true); err != nil || !strings.Contains(out.String(), "Loaded into launchd") {
		t.Fatalf("install: %q, %v", out.String(), err)
	}
	if got := strings.Join(*calls, ","); got != "bootout,bootstrap" {
		t.Errorf("install launchctl calls = %s (a reload boots out first)", got)
	}
	*calls = nil
	if err := runModelsSchedule(&out, dir, true, true); err != nil {
		t.Fatal(err)
	}
	if got := strings.Join(*calls, ","); got != "bootout" {
		t.Errorf("remove launchctl calls = %s", got)
	}
}

func TestModelsScheduleReportsALaunchctlFailure(t *testing.T) {
	recordLaunchctl(t, true)
	var out bytes.Buffer
	if err := runModelsSchedule(&out, t.TempDir(), false, true); err == nil || !strings.Contains(err.Error(), "Bootstrap failed") {
		t.Errorf("err = %v, want the launchctl message", err)
	}
}

func TestModelsScheduleOffMacPrintsTheCronLine(t *testing.T) {
	old := goos
	goos = "linux"
	t.Cleanup(func() { goos = old })
	var out bytes.Buffer
	if err := runModelsSchedule(&out, "", false, true); err != nil || !strings.Contains(out.String(), "0 9 * * 1") {
		t.Errorf("out = %q, err = %v", out.String(), err)
	}
}

func TestRefreshIfStaleReportsOnlyWhenItRefreshed(t *testing.T) {
	m := testManager(t)
	var out bytes.Buffer
	refreshIfStale(context.Background(), &out, m)
	if !strings.Contains(out.String(), "model catalog refreshed") {
		t.Fatalf("first: %q", out.String())
	}
	out.Reset()
	refreshIfStale(context.Background(), &out, m)
	if out.Len() != 0 {
		t.Errorf("a fresh catalog printed %q", out.String())
	}
}

func TestModelsListAsJSONAndStaleCatalog(t *testing.T) {
	m := testManager(t)
	if err := runModelsRefresh(context.Background(), io.Discard, m); err != nil {
		t.Fatal(err)
	}
	m.Now = func() time.Time { return time.Date(2026, 12, 1, 9, 0, 0, 0, time.UTC) }
	var out bytes.Buffer
	if err := runModelsList(&out, m); err != nil || !strings.Contains(out.String(), "STALE") {
		t.Errorf("stale list: %q, %v", out.String(), err)
	}
}

func useManager(t *testing.T, m *modelcatalog.Manager) {
	t.Helper()
	old := catalogManager
	catalogManager = func() (*modelcatalog.Manager, error) { return m, nil }
	t.Cleanup(func() { catalogManager = old })
}

func TestModelsCommandsRunThroughTheCLI(t *testing.T) {
	useManager(t, testManager(t))
	schedDir := t.TempDir()
	for _, c := range []struct {
		args []string
		want string // substring of the output; "" means no output at all
	}{
		{[]string{"cleft", "models"}, "No model catalog yet"},
		{[]string{"cleft", "models", "refresh"}, "Catalog refreshed: 2 models"},
		{[]string{"cleft", "models", "refresh", "--quiet"}, ""},
		{[]string{"cleft", "models", "--json"}, `"roles"`},
		{[]string{"cleft", "models", "schedule", "--agents-dir", schedDir, "--no-load"}, "Weekly model refresh written"},
	} {
		out, code := cliOutput(t, c.args...)
		if code != 0 || (c.want == "") != (out == "") || !strings.Contains(out, c.want) {
			t.Errorf("%v: %q (exit %d), want %q", c.args, out, code, c.want)
		}
	}
}

func TestTheMachineManagerIsFedByEverySourceAndPicksTheLocalModel(t *testing.T) {
	m, err := newCatalogManager()
	if err != nil {
		t.Fatal(err)
	}
	if len(m.Sources) != 5 || m.LocalPicker == nil || m.Spawn == nil {
		t.Errorf("manager = %d sources, picker %v, spawn %v", len(m.Sources), m.LocalPicker != nil, m.Spawn != nil)
	}
}

func TestPriceShowsDollarsPerMillionTokens(t *testing.T) {
	for _, c := range []struct {
		m    modelcatalog.Model
		want string
	}{
		{modelcatalog.Model{Routed: true, InputCost: 1.25e-6}, "1.25"},
		{modelcatalog.Model{Routed: true}, "free"},
		{modelcatalog.Model{InputCost: 1e-6}, "-"},
	} {
		if got := price(c.m); got != c.want {
			t.Errorf("price(%+v) = %q, want %q", c.m, got, c.want)
		}
	}
}
