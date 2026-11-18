package modelcatalog

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"
)

var sample = []Model{
	{Name: "gemini-4-argon-medium", Provider: "gemini", Upstream: "gemini-4-argon-medium", InputCost: 1.25e-6, MaxInput: 1048576, Mode: "chat", Routed: true},
	{Name: "gemini-3.8-flash-medium", Provider: "gemini", Upstream: "gemini-3.8-flash-medium", InputCost: 7.5e-8, MaxInput: 1048576, Mode: "chat", Routed: true},
	{Name: "copilot-sonnet", Provider: "github_copilot", Upstream: "claude-sonnet-4.6", InputCost: 3e-6, MaxInput: 200000, Mode: "chat", Routed: true},
	{Name: "local-coder", Provider: "ollama", Upstream: "hermes3:8b", Mode: "chat", Routed: true, Local: true},
	{Name: "local-embed", Provider: "ollama", Upstream: "nomic-embed-text:latest", InputCost: 1e-9, Mode: "chat", Routed: true},
}

func TestComputeRolesPicksByPriceAndContext(t *testing.T) {
	want := map[string]string{
		RoleLocal:   "local-coder",
		RoleCheap:   "gemini-3.8-flash-medium",
		RoleStrong:  "gemini-4-argon-medium",
		RoleCopilot: "copilot-sonnet",
	}
	if got := ComputeRoles(sample, "local-coder"); !reflect.DeepEqual(got, want) {
		t.Errorf("roles = %v, want %v", got, want)
	}
}

func TestComputeRolesIgnoresWhatTheGatewayDoesNotRoute(t *testing.T) {
	models := []Model{{Name: "gemini-9", Provider: "gemini", Upstream: "gemini-9", InputCost: 1e-9, Mode: "chat"}}
	if got := ComputeRoles(models, ""); len(got) != 0 {
		t.Errorf("an unrouted model got a role: %v", got)
	}
}

func TestComputeRolesIsStableOnTies(t *testing.T) {
	a := Model{Name: "b", Provider: "x", Upstream: "b", InputCost: 1, Mode: "chat", Routed: true, MaxInput: 300000}
	b := Model{Name: "a", Provider: "x", Upstream: "a", InputCost: 1, Mode: "chat", Routed: true, MaxInput: 300000}
	for _, models := range [][]Model{{a, b}, {b, a}} {
		if got := ComputeRoles(models, "")[RoleCheap]; got != "a" {
			t.Errorf("cheap = %q, want a", got)
		}
	}
}

func TestMergeMarksRoutedModelsTheProviderConfirms(t *testing.T) {
	gateway := []Model{{Name: "gemini-4-argon-medium", Provider: "gemini", Upstream: "gemini-4-argon-medium", Routed: true}}
	provider := []Model{
		{Name: "gemini-4-argon-medium", Provider: "gemini", Upstream: "gemini-4-argon-medium", MaxInput: 1000},
		{Name: "gemini-5", Provider: "gemini", Upstream: "gemini-5"},
	}
	for name, order := range map[string][][]Model{"gateway first": {gateway, provider}, "provider first": {provider, gateway}} {
		t.Run(name, func(t *testing.T) {
			got := Merge(order...)
			if len(got) != 2 {
				t.Fatalf("merged = %+v, want the routed model and the new one", got)
			}
			known, fresh := got[0], got[1]
			if known.Name != "gemini-4-argon-medium" || !known.Routed || !known.Listed || known.MaxInput != 1000 {
				t.Errorf("routed model = %+v", known)
			}
			if fresh.Name != "gemini-5" || fresh.Routed || fresh.Listed {
				t.Errorf("new model = %+v", fresh)
			}
		})
	}
}

func TestComputeRolesWithTwoCopilotModelsAndOnlyShortContexts(t *testing.T) {
	models := []Model{
		{Name: "cp-small", Provider: providerCopilot, Upstream: "a", InputCost: 1e-6, Mode: "chat", Routed: true},
		{Name: "cp-big", Provider: providerCopilot, Upstream: "b", InputCost: 3e-6, Mode: "chat", Routed: true},
		{Name: "short", Provider: "gemini", Upstream: "c", InputCost: 2e-6, MaxInput: 32000, Mode: "chat", Routed: true},
	}
	got := ComputeRoles(models, "")
	if got[RoleCopilot] != "cp-big" || got[RoleStrong] != "short" || got[RoleCheap] != "cp-small" {
		t.Errorf("roles = %v", got)
	}
}

func TestSaveLoadRoundTripAndStale(t *testing.T) {
	dir := t.TempDir()
	if c, err := Load(dir); err != nil || !c.Stale(time.Now(), TTL) {
		t.Fatalf("missing catalog: %+v, %v (want an empty, stale one)", c, err)
	}
	now := time.Date(2026, 11, 17, 9, 0, 0, 0, time.UTC)
	in := Catalog{FetchedAt: now, Models: sample[:1], Roles: map[string]string{RoleCheap: "x"}, Notes: []string{"n"}}
	if err := in.Save(dir); err != nil {
		t.Fatal(err)
	}
	out, err := Load(dir)
	if err != nil || !reflect.DeepEqual(in, out) {
		t.Fatalf("round trip = %+v, %v", out, err)
	}
	if out.Stale(now.Add(TTL), TTL) || !out.Stale(now.Add(TTL+time.Second), TTL) {
		t.Error("stale boundary is not the TTL")
	}
}

func TestLoadAndSaveReportFilesystemProblems(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, FileName), []byte("{not json"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := Load(dir); err == nil || !strings.Contains(err.Error(), "corrupt") {
		t.Errorf("corrupt file: err = %v", err)
	}
	unreadable := t.TempDir()
	if err := os.Mkdir(filepath.Join(unreadable, FileName), 0o755); err != nil {
		t.Fatal(err)
	}
	if _, err := Load(unreadable); err == nil {
		t.Error("a catalog that is a directory was read")
	}
	if err := (Catalog{}).Save(unreadable); err == nil {
		t.Error("saving over a directory succeeded")
	}
	file := filepath.Join(t.TempDir(), "plain")
	if err := os.WriteFile(file, nil, 0o644); err != nil {
		t.Fatal(err)
	}
	if err := (Catalog{}).Save(filepath.Join(file, "sub")); err == nil {
		t.Error("saving below a file succeeded")
	}
	leftovers, _ := filepath.Glob(filepath.Join(unreadable, "*.tmp"))
	if len(leftovers) != 0 {
		t.Errorf("a failed save left %v", leftovers)
	}
}

func TestSplitUpstream(t *testing.T) {
	for in, want := range map[string][2]string{
		"gemini/gemini-4":         {"gemini", "gemini-4"},
		"ollama/hermes3:8b":       {"ollama", "hermes3:8b"},
		"github_copilot/claude-4": {"github_copilot", "claude-4"},
		"bare-model":              {"", "bare-model"},
	} {
		if p, id := splitUpstream(in); p != want[0] || id != want[1] {
			t.Errorf("splitUpstream(%q) = %q, %q", in, p, id)
		}
	}
}

func TestSaveFailsWhenTheDirectoryCannotBeWritten(t *testing.T) {
	dir := t.TempDir()
	if err := os.Chmod(dir, 0o500); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(dir, 0o755) })
	err := (Catalog{}).Save(dir)
	if os.Geteuid() == 0 {
		return // root writes anywhere, so there is nothing to refuse
	}
	if err == nil {
		t.Error("saving into a read-only directory succeeded")
	}
}
