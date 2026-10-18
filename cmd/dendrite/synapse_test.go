package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"strings"
	"testing"

	"Dendrite/pkg/projectid"
	"Dendrite/pkg/synapse"
)

func TestSynapseCommand_CreateAndInspect(t *testing.T) {
	home := t.TempDir()
	t.Setenv("DENDRITE_HOME", home)

	dir1 := t.TempDir()
	dir2 := t.TempDir()

	var createOut bytes.Buffer
	args := []string{
		"create",
		"web-api",
		"--members", fmt.Sprintf("%s=api-gateway:producer,%s=client-web:consumer", dir1, dir2),
		"--name", "Web and API Synapse",
		"--contract", "openapi:api-gateway:api/openapi.yaml>client-web:src/types.ts",
		"--contract", "proto:api-gateway:proto/service.proto>client-web:gen/service.ts",
		"--sync", "go.work",
		"--parity", "api-gateway=go test ./...",
		"--parity", "client-web=bun test",
	}

	err := runSynapseCommand(context.Background(), args, &createOut)
	if err != nil {
		t.Fatalf("runSynapseCommand create failed: %v", err)
	}

	createStr := createOut.String()
	if !strings.Contains(createStr, "[✓] Synapse web-api created with 2 members") {
		t.Errorf("create output missing success prefix, got: %s", createStr)
	}
	if !strings.Contains(createStr, "web-api.toml") {
		t.Errorf("create output missing manifest path, got: %s", createStr)
	}

	// Verify project.id exists in both dirs
	id1, err := projectid.Read(dir1)
	if err != nil {
		t.Fatalf("projectid.Read(dir1) failed: %v", err)
	}
	if !projectid.Valid(id1) {
		t.Errorf("project id1 invalid: %s", id1)
	}

	id2, err := projectid.Read(dir2)
	if err != nil {
		t.Fatalf("projectid.Read(dir2) failed: %v", err)
	}
	if !projectid.Valid(id2) {
		t.Errorf("project id2 invalid: %s", id2)
	}
	if id1 == id2 {
		t.Errorf("expected different project IDs, got %s and %s", id1, id2)
	}

	// Verify projects.json under DENDRITE_HOME contains both
	indexPath, err := projectid.DefaultIndexPath()
	if err != nil {
		t.Fatalf("DefaultIndexPath failed: %v", err)
	}
	idx, err := projectid.LoadIndex(indexPath)
	if err != nil {
		t.Fatalf("LoadIndex failed: %v", err)
	}

	e1, ok := idx.Lookup(id1)
	if !ok {
		t.Errorf("project id1 not found in index")
	}
	if e1.Path != dir1 {
		t.Errorf("entry 1 path = %q, want %q", e1.Path, dir1)
	}

	e2, ok := idx.Lookup(id2)
	if !ok {
		t.Errorf("project id2 not found in index")
	}
	if e2.Path != dir2 {
		t.Errorf("entry 2 path = %q, want %q", e2.Path, dir2)
	}

	// Inspect with --json
	var inspectJSONOut bytes.Buffer
	err = runSynapseCommand(context.Background(), []string{"inspect", "web-api", "--json"}, &inspectJSONOut)
	if err != nil {
		t.Fatalf("inspect --json failed: %v", err)
	}

	var inspected synapseInspectJSON
	if err := json.Unmarshal(inspectJSONOut.Bytes(), &inspected); err != nil {
		t.Fatalf("failed to parse inspect JSON output: %v\nOutput: %s", err, inspectJSONOut.String())
	}

	if inspected.ID != "web-api" {
		t.Errorf("inspected.ID = %q, want %q", inspected.ID, "web-api")
	}
	if inspected.Name != "Web and API Synapse" {
		t.Errorf("inspected.Name = %q, want %q", inspected.Name, "Web and API Synapse")
	}
	if inspected.State != "active" {
		t.Errorf("inspected.State = %q, want %q", inspected.State, "active")
	}
	if inspected.Sync.Strategy != "go.work" {
		t.Errorf("inspected.Sync.Strategy = %q, want %q", inspected.Sync.Strategy, "go.work")
	}
	if inspected.Parity.Commands["api-gateway"] != "go test ./..." {
		t.Errorf("inspected.Parity api-gateway = %q, want %q", inspected.Parity.Commands["api-gateway"], "go test ./...")
	}
	if inspected.Parity.Commands["client-web"] != "bun test" {
		t.Errorf("inspected.Parity client-web = %q, want %q", inspected.Parity.Commands["client-web"], "bun test")
	}
	if len(inspected.Members) != 2 {
		t.Fatalf("expected 2 members in inspect JSON, got %d", len(inspected.Members))
	}

	// Member 0
	if inspected.Members[0].Alias != "api-gateway" || inspected.Members[0].ProjectID != id1 ||
		inspected.Members[0].Role != "producer" || inspected.Members[0].PathHint != dir1 ||
		inspected.Members[0].Status != "OK" {
		t.Errorf("unexpected member 0: %+v", inspected.Members[0])
	}
	// Member 1
	if inspected.Members[1].Alias != "client-web" || inspected.Members[1].ProjectID != id2 ||
		inspected.Members[1].Role != "consumer" || inspected.Members[1].PathHint != dir2 ||
		inspected.Members[1].Status != "OK" {
		t.Errorf("unexpected member 1: %+v", inspected.Members[1])
	}

	// Contracts
	if len(inspected.Contracts) != 2 {
		t.Fatalf("expected 2 contracts in inspect JSON, got %d", len(inspected.Contracts))
	}
	c0 := inspected.Contracts[0]
	if c0.Kind != "openapi" || c0.Producer != "api-gateway" || c0.Path != "api/openapi.yaml" ||
		len(c0.Consumers) != 1 || c0.Consumers[0].Alias != "client-web" || c0.Consumers[0].Path != "src/types.ts" {
		t.Errorf("unexpected contract 0: %+v", c0)
	}
	c1 := inspected.Contracts[1]
	if c1.Kind != "proto" || c1.Producer != "api-gateway" || c1.Path != "proto/service.proto" ||
		len(c1.Consumers) != 1 || c1.Consumers[0].Alias != "client-web" || c1.Consumers[0].Path != "gen/service.ts" {
		t.Errorf("unexpected contract 1: %+v", c1)
	}

	// Inspect human output
	var humanOut bytes.Buffer
	err = runSynapseCommand(context.Background(), []string{"inspect", "web-api"}, &humanOut)
	if err != nil {
		t.Fatalf("inspect human output failed: %v", err)
	}
	humanStr := humanOut.String()
	for _, expected := range []string{
		"[+] Synapse web-api:",
		"Name:          Web and API Synapse",
		"State:         active",
		"Sync Strategy: go.work",
		"api-gateway",
		"client-web",
		"(OK)",
		"openapi",
		"proto",
		"api-gateway = go test ./...",
		"client-web = bun test",
	} {
		if !strings.Contains(humanStr, expected) {
			t.Errorf("inspect human output missing %q, got:\n%s", expected, humanStr)
		}
	}
}

func TestSynapseCommand_CreateErrors(t *testing.T) {
	home := t.TempDir()
	t.Setenv("DENDRITE_HOME", home)

	dir1 := t.TempDir()
	dir2 := t.TempDir()

	// Initial create succeeds
	var buf bytes.Buffer
	err := runSynapseCommand(context.Background(), []string{
		"create", "test-syn",
		"--members", fmt.Sprintf("%s,%s", dir1, dir2),
	}, &buf)
	if err != nil {
		t.Fatalf("initial create failed: %v", err)
	}

	// 1. Duplicate ID -> error
	buf.Reset()
	err = runSynapseCommand(context.Background(), []string{
		"create", "test-syn",
		"--members", fmt.Sprintf("%s,%s", dir1, dir2),
	}, &buf)
	if err == nil {
		t.Fatal("expected error on duplicate synapse ID, got nil")
	}
	if !strings.Contains(err.Error(), `synapse "test-syn" already exists`) {
		t.Errorf("expected duplicate error mentioning 'synapse \"test-syn\" already exists', got: %v", err)
	}

	// 2. Create with 1 member -> validation error
	buf.Reset()
	err = runSynapseCommand(context.Background(), []string{
		"create", "single-member",
		"--members", dir1,
	}, &buf)
	if err == nil {
		t.Fatal("expected validation error on single member, got nil")
	}
	if !strings.Contains(err.Error(), "at least 2 members") {
		t.Errorf("expected validation error to mention 'at least 2 members', got: %v", err)
	}

	// 3. Bad --contract syntax -> error
	buf.Reset()
	err = runSynapseCommand(context.Background(), []string{
		"create", "bad-contract",
		"--members", fmt.Sprintf("%s,%s", dir1, dir2),
		"--contract", "invalid_contract_no_colons",
	}, &buf)
	if err == nil {
		t.Fatal("expected error on bad contract syntax, got nil")
	}
	if !strings.Contains(err.Error(), "invalid contract syntax") {
		t.Errorf("expected error to mention 'invalid contract syntax', got: %v", err)
	}

	// Bad contract consumer syntax
	buf.Reset()
	err = runSynapseCommand(context.Background(), []string{
		"create", "bad-consumer",
		"--members", fmt.Sprintf("%s,%s", dir1, dir2),
		"--contract", "openapi:prod:spec.yaml>bad_consumer_no_colon",
	}, &buf)
	if err == nil {
		t.Fatal("expected error on bad contract consumer syntax, got nil")
	}
	if !strings.Contains(err.Error(), "invalid contract consumer syntax") {
		t.Errorf("expected error to mention 'invalid contract consumer syntax', got: %v", err)
	}

	// 4. Unknown flag -> error
	buf.Reset()
	err = runSynapseCommand(context.Background(), []string{
		"create", "unknown-flag-syn",
		"--members", fmt.Sprintf("%s,%s", dir1, dir2),
		"--some-invalid-flag",
	}, &buf)
	if err == nil {
		t.Fatal("expected error on unknown flag, got nil")
	}
	if !strings.Contains(err.Error(), "unknown flag: --some-invalid-flag") {
		t.Errorf("expected error to mention 'unknown flag: --some-invalid-flag', got: %v", err)
	}

	// 5. Missing synapse ID -> error
	buf.Reset()
	err = runSynapseCommand(context.Background(), []string{
		"create",
		"--members", fmt.Sprintf("%s,%s", dir1, dir2),
	}, &buf)
	if err == nil {
		t.Fatal("expected error on missing synapse ID, got nil")
	}

	// 6. Missing --members -> error
	buf.Reset()
	err = runSynapseCommand(context.Background(), []string{
		"create", "no-members-syn",
	}, &buf)
	if err == nil {
		t.Fatal("expected error on missing --members, got nil")
	}
}

func TestSynapseCommand_ListAndHealth(t *testing.T) {
	home := t.TempDir()
	t.Setenv("DENDRITE_HOME", home)

	var buf bytes.Buffer

	// 1. List empty store (table)
	err := runSynapseCommand(context.Background(), []string{"list"}, &buf)
	if err != nil {
		t.Fatalf("list on empty store failed: %v", err)
	}
	if !strings.Contains(buf.String(), "[+] No synapses defined.") {
		t.Errorf("empty list output want '[+] No synapses defined.', got: %s", buf.String())
	}

	// 2. List empty store (--json)
	buf.Reset()
	err = runSynapseCommand(context.Background(), []string{"list", "--json"}, &buf)
	if err != nil {
		t.Fatalf("list --json on empty store failed: %v", err)
	}
	if strings.TrimSpace(buf.String()) != "[]" {
		t.Errorf("empty list --json want '[]', got: %q", buf.String())
	}

	// Create synapse with two members
	dirA := t.TempDir()
	dirB := t.TempDir()
	buf.Reset()
	err = runSynapseCommand(context.Background(), []string{
		"create", "active-syn",
		"--members", fmt.Sprintf("%s=node-a:producer,%s=node-b:consumer", dirA, dirB),
	}, &buf)
	if err != nil {
		t.Fatalf("create active-syn failed: %v", err)
	}

	// 3. List table with healthy synapse
	buf.Reset()
	err = runSynapseCommand(context.Background(), []string{"list"}, &buf)
	if err != nil {
		t.Fatalf("list table failed: %v", err)
	}
	tableOut := buf.String()
	if !strings.Contains(tableOut, "ID") || !strings.Contains(tableOut, "STATE") ||
		!strings.Contains(tableOut, "MEMBERS") || !strings.Contains(tableOut, "HEALTH") {
		t.Errorf("list table missing expected headers, got:\n%s", tableOut)
	}
	if !strings.Contains(tableOut, "active-syn") {
		t.Errorf("list table missing 'active-syn', got:\n%s", tableOut)
	}
	if !strings.Contains(tableOut, "node-a, node-b") {
		t.Errorf("list table missing member aliases 'node-a, node-b', got:\n%s", tableOut)
	}
	if !strings.Contains(tableOut, "ok") {
		t.Errorf("list table missing 'ok' health status, got:\n%s", tableOut)
	}

	// 4. List --json with healthy synapse
	buf.Reset()
	err = runSynapseCommand(context.Background(), []string{"list", "--json"}, &buf)
	if err != nil {
		t.Fatalf("list --json failed: %v", err)
	}
	var listData []synapseListJSON
	if err := json.Unmarshal(buf.Bytes(), &listData); err != nil {
		t.Fatalf("failed to unmarshal list JSON: %v\nOutput: %s", err, buf.String())
	}
	if len(listData) != 1 {
		t.Fatalf("expected 1 synapse in list, got %d", len(listData))
	}
	if listData[0].ID != "active-syn" {
		t.Errorf("listData[0].ID = %q, want %q", listData[0].ID, "active-syn")
	}
	if len(listData[0].Members) != 2 {
		t.Fatalf("expected 2 members, got %d", len(listData[0].Members))
	}
	if listData[0].Members[0].Status != "OK" || listData[0].Members[1].Status != "OK" {
		t.Errorf("expected all members to be 'ok', got: %+v", listData[0].Members)
	}

	// 5. Remove one member directory -> Health becomes "1 unresolved"
	if err := os.RemoveAll(dirA); err != nil {
		t.Fatalf("failed to remove dirA: %v", err)
	}

	buf.Reset()
	err = runSynapseCommand(context.Background(), []string{"list"}, &buf)
	if err != nil {
		t.Fatalf("list table after dirA removal failed: %v", err)
	}
	tableAfterRemove := buf.String()
	if !strings.Contains(tableAfterRemove, "1 unresolved") {
		t.Errorf("expected health '1 unresolved' in list table, got:\n%s", tableAfterRemove)
	}

	// List --json after removal
	buf.Reset()
	err = runSynapseCommand(context.Background(), []string{"list", "--json"}, &buf)
	if err != nil {
		t.Fatalf("list --json after dirA removal failed: %v", err)
	}
	listData = nil
	if err := json.Unmarshal(buf.Bytes(), &listData); err != nil {
		t.Fatalf("failed to unmarshal list JSON after removal: %v", err)
	}
	if len(listData) != 1 {
		t.Fatalf("expected 1 synapse, got %d", len(listData))
	}
	if listData[0].Members[0].Status != "MISSING" {
		t.Errorf("expected member 0 status 'MISSING', got %q", listData[0].Members[0].Status)
	}
	if listData[0].Members[1].Status != "OK" {
		t.Errorf("expected member 1 status 'ok', got %q", listData[0].Members[1].Status)
	}
}

func TestSynapseCommand_InspectUnknown(t *testing.T) {
	home := t.TempDir()
	t.Setenv("DENDRITE_HOME", home)

	var buf bytes.Buffer
	err := runSynapseCommand(context.Background(), []string{"inspect", "nonexistent-synapse"}, &buf)
	if err == nil {
		t.Fatal("expected error inspecting nonexistent synapse, got nil")
	}
	if !errors.Is(err, synapse.ErrNotFound) {
		t.Errorf("expected errors.Is(err, synapse.ErrNotFound) to be true, got: %v", err)
	}

	// Also test inspect --json with unknown ID
	buf.Reset()
	err = runSynapseCommand(context.Background(), []string{"inspect", "nonexistent-synapse", "--json"}, &buf)
	if err == nil {
		t.Fatal("expected error inspecting nonexistent synapse with --json, got nil")
	}
	if !errors.Is(err, synapse.ErrNotFound) {
		t.Errorf("expected errors.Is(err, synapse.ErrNotFound) to be true with --json, got: %v", err)
	}
}

func TestSynapseCommand_UnknownActionAndUsage(t *testing.T) {
	var buf bytes.Buffer

	// Unknown action
	err := runSynapseCommand(context.Background(), []string{"unknown-action"}, &buf)
	if err == nil {
		t.Fatal("expected error for unknown action, got nil")
	}
	expectedMsg := `unknown synapse action "unknown-action"`
	if !strings.Contains(err.Error(), expectedMsg) {
		t.Errorf("expected error containing %q, got: %v", expectedMsg, err)
	}

	// Empty args
	buf.Reset()
	err = runSynapseCommand(context.Background(), []string{}, &buf)
	if err == nil {
		t.Fatal("expected error for empty args, got nil")
	}
	if !strings.Contains(err.Error(), `unknown synapse action ""`) {
		t.Errorf("expected error containing %q, got: %v", `unknown synapse action ""`, err)
	}

	// printSynapseUsage
	buf.Reset()
	printSynapseUsage(&buf)
	usage := buf.String()
	for _, sub := range []string{"create", "list", "inspect"} {
		if !strings.Contains(usage, sub) {
			t.Errorf("printSynapseUsage missing subcommand %q, got:\n%s", sub, usage)
		}
	}
}

func TestSynapseCommand_RegistryRegistration(t *testing.T) {
	reg := buildRegistry()
	cmd, ok := reg.Lookup("synapse")
	if !ok || cmd == nil {
		t.Fatal("command 'synapse' not registered in buildRegistry()")
	}

	if cmd.Name != "synapse" {
		t.Errorf("cmd.Name = %q, want 'synapse'", cmd.Name)
	}
	if cmd.Category != "Synaptic Cleft & Gating" {
		t.Errorf("cmd.Category = %q, want 'Synaptic Cleft & Gating'", cmd.Category)
	}
	if cmd.Description != "Typed cross-project links with scoped MCP servers and self-healing paths" {
		t.Errorf("unexpected description: %q", cmd.Description)
	}

	// Check aliases
	foundSyn := false
	foundSynapses := false
	for _, a := range cmd.Aliases {
		if a == "syn" {
			foundSyn = true
		}
		if a == "synapses" {
			foundSynapses = true
		}
	}
	if !foundSyn || !foundSynapses {
		t.Errorf("expected aliases ['syn', 'synapses'], got %v", cmd.Aliases)
	}

	// Run with empty args should print usage without error
	var out bytes.Buffer
	oldStdout := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w
	err := cmd.Run(context.Background(), []string{})
	w.Close()
	os.Stdout = oldStdout
	if err != nil {
		t.Errorf("cmd.Run with no args returned error: %v", err)
	}
	_, _ = out.ReadFrom(r)
	if !strings.Contains(out.String(), "Usage: dendrite synapse") {
		t.Errorf("expected usage output when running with no args, got:\n%s", out.String())
	}
}
