package main

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"Dendrite/pkg/synapse"
)

// healWorkspace creates two member dirs, isolates HOME/DENDRITE_HOME and creates synapse "web-api".
func healWorkspace(t *testing.T) (root string) {
	t.Helper()
	root = t.TempDir()
	t.Setenv("HOME", filepath.Join(root, "home"))
	t.Setenv("DENDRITE_HOME", filepath.Join(root, "dendrite-home"))
	for _, d := range []string{"ws/api", "ws/web"} {
		if err := os.MkdirAll(filepath.Join(root, d), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	runOK(t, "create", "web-api", "--members",
		filepath.Join(root, "ws/api")+"=api:producer,"+filepath.Join(root, "ws/web")+"=web:consumer",
		"--contract", "openapi:api:openapi.yaml>web:src/types.ts")
	return root
}

func runOK(t *testing.T, args ...string) string {
	t.Helper()
	var out bytes.Buffer
	if err := runSynapseCommand(context.Background(), args, &out); err != nil {
		t.Fatalf("synapse %v: %v\n%s", args, err, out.String())
	}
	return out.String()
}

func runErr(t *testing.T, args ...string) error {
	t.Helper()
	var out bytes.Buffer
	err := runSynapseCommand(context.Background(), args, &out)
	if err == nil {
		t.Fatalf("synapse %v succeeded, want error\n%s", args, out.String())
	}
	return err
}

func TestSynapseHealCLI(t *testing.T) {
	root := healWorkspace(t)
	if out := runOK(t, "heal"); !strings.Contains(out, "OK") || strings.Contains(out, "[!]") {
		t.Fatalf("healthy heal output:\n%s", out)
	}

	newAPI := filepath.Join(root, "ws", "services", "api")
	if err := os.MkdirAll(filepath.Dir(newAPI), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Rename(filepath.Join(root, "ws/api"), newAPI); err != nil {
		t.Fatal(err)
	}

	out := runOK(t, "heal", "--roots", filepath.Join(root, "ws"), "--depth", "3")
	if !strings.Contains(out, "MOVED") || !strings.Contains(out, "--auto") {
		t.Fatalf("dry-run heal should propose the move:\n%s", out)
	}

	var rows []synapse.HealRow
	if err := json.Unmarshal([]byte(runOK(t, "heal", "--auto", "--json")), &rows); err != nil {
		t.Fatal(err)
	}
	applied := false
	for _, r := range rows {
		if r.Alias == "api" && r.Applied && r.NewPath == newAPI {
			applied = true
		}
	}
	if !applied {
		t.Fatalf("heal --auto did not apply the move: %+v", rows)
	}
	if out := runOK(t, "heal"); !strings.Contains(out, "OK") || strings.Contains(out, "MOVED") {
		t.Fatalf("after heal everything should be OK:\n%s", out)
	}

	// A member moved out of reach is reported unresolved.
	if err := os.Rename(filepath.Join(root, "ws/web"), filepath.Join(t.TempDir(), "web")); err != nil {
		t.Fatal(err)
	}
	if out := runOK(t, "heal"); !strings.Contains(out, "MISSING") || !strings.Contains(out, "unresolved") {
		t.Fatalf("missing member not reported:\n%s", out)
	}
}

func TestSynapseHealCLIEmptyAndFlags(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	t.Setenv("DENDRITE_HOME", t.TempDir())
	if out := runOK(t, "heal"); !strings.Contains(out, "No synapses") {
		t.Fatalf("empty heal output: %s", out)
	}
	if out := runOK(t, "heal", "--json"); strings.TrimSpace(out) != "[]" {
		t.Fatalf("empty heal --json = %q", out)
	}
	runErr(t, "heal", "--depth", "zero")
	runErr(t, "heal", "--roots")
	runErr(t, "heal", "extra")
}

func TestSynapseLocateCLI(t *testing.T) {
	root := healWorkspace(t)
	if out := runOK(t, "locate", "api"); !strings.Contains(out, "found at") {
		t.Fatalf("locate resolved member: %s", out)
	}
	hidden := t.TempDir()
	if err := os.Rename(filepath.Join(root, "ws/api"), filepath.Join(hidden, "api")); err != nil {
		t.Fatal(err)
	}
	if err := runErr(t, "locate", "api"); !strings.Contains(err.Error(), "not found") {
		t.Fatalf("locate without hint: %v", err)
	}
	var loc synapse.Location
	if err := json.Unmarshal([]byte(runOK(t, "locate", "api", hidden, "--json")), &loc); err != nil {
		t.Fatal(err)
	}
	if loc.Path != filepath.Join(hidden, "api") {
		t.Fatalf("locate with hint = %+v", loc)
	}
	if out := runOK(t, "locate", "api", hidden); !strings.Contains(out, "via id match") {
		t.Fatalf("locate human output: %s", out)
	}

	// A same-named directory without identity is only a candidate.
	if err := os.RemoveAll(filepath.Join(hidden, "api")); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(hidden, "api", ".git"), 0o755); err != nil {
		t.Fatal(err)
	}
	if out := runOK(t, "locate", "api", hidden); !strings.Contains(out, "candidate") {
		t.Fatalf("name match should be a candidate list: %s", out)
	}

	runErr(t, "locate")
	runErr(t, "locate", "--bogus")
	if err := runErr(t, "locate", "nobody"); !strings.Contains(err.Error(), "no synapse member") {
		t.Fatalf("unknown member: %v", err)
	}
}

func TestSynapseLocateAmbiguousAlias(t *testing.T) {
	root := healWorkspace(t)
	other := filepath.Join(root, "other", "api")
	peer := filepath.Join(root, "other", "peer")
	for _, d := range []string{other, peer} {
		if err := os.MkdirAll(d, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	runOK(t, "create", "other", "--members", other+"=api,"+peer+"=peer")
	if err := runErr(t, "locate", "api"); !strings.Contains(err.Error(), "ambiguous") {
		t.Fatalf("expected ambiguous alias error, got %v", err)
	}
}

func TestSynapseIDESyncCLI(t *testing.T) {
	root := healWorkspace(t)
	out := runOK(t, "ide-sync", "web-api", "--binary", "/usr/local/bin/dendrite")
	if !strings.Contains(out, "dendrite-synapse-web-api") {
		t.Fatalf("ide-sync output: %s", out)
	}
	data, err := os.ReadFile(filepath.Join(root, "ws/web/.ai/mcp/mcp.json"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(data), `"--synapse"`) || !strings.Contains(string(data), "/usr/local/bin/dendrite") {
		t.Fatalf("mcp.json not written correctly:\n%s", data)
	}
	if _, err := os.Stat(filepath.Join(root, "ws/api/.idea/scopes/Synapse_web_api.xml")); err != nil {
		t.Fatalf("scope file missing: %v", err)
	}

	// Default binary is the running executable; an unresolved member is skipped.
	if err := os.RemoveAll(filepath.Join(root, "ws/web")); err != nil {
		t.Fatal(err)
	}
	if out := runOK(t, "ide-sync", "web-api"); !strings.Contains(out, "skipped unresolved member web") {
		t.Fatalf("expected skip notice: %s", out)
	}

	runErr(t, "ide-sync")
	runErr(t, "ide-sync", "web-api", "--binary")
	runErr(t, "ide-sync", "web-api", "--x")
	runErr(t, "ide-sync", "web-api", "extra")
	runErr(t, "ide-sync", "missing")
}

func TestSynapseMoveCLI(t *testing.T) {
	root := healWorkspace(t)
	dst := filepath.Join(root, "relocated", "api")
	out := runOK(t, "move", "api", dst)
	if !strings.Contains(out, "Moved") || !strings.Contains(out, "web-api") {
		t.Fatalf("move output: %s", out)
	}
	if _, err := os.Stat(filepath.Join(dst, ".dendrite", "project.id")); err != nil {
		t.Fatalf("project not at destination: %v", err)
	}
	var inspected struct {
		Members []struct{ Alias, Status, PathHint string } `json:"members"`
	}
	if err := json.Unmarshal([]byte(runOK(t, "inspect", "web-api", "--json")), &inspected); err != nil {
		t.Fatal(err)
	}
	for _, m := range inspected.Members {
		if m.Status != "OK" {
			t.Fatalf("member %s status %s after move", m.Alias, m.Status)
		}
	}

	var res synapse.MoveResult
	back := filepath.Join(root, "ws", "api")
	if err := json.Unmarshal([]byte(runOK(t, "mv", "api", back, "--json")), &res); err != nil {
		t.Fatal(err)
	}
	if res.NewPath != back {
		t.Fatalf("move --json = %+v", res)
	}

	runErr(t, "move", "api")
	runErr(t, "move", "api", back, "--force")
	if err := runErr(t, "move", "api", filepath.Join(root, "ws", "web")); !strings.Contains(err.Error(), "exists") {
		t.Fatalf("moving onto an existing dir: %v", err)
	}
}
