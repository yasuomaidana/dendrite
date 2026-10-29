package main

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"Dendrite/pkg/projectid"
	"Dendrite/pkg/synapse"
	"Dendrite/pkg/tracker/plane"
)

func plainTestOutput(t *testing.T) {
	t.Helper()
	prevJSON := globals.JSON
	prevYes := globals.Yes
	globals.JSON = false
	globals.Yes = false
	t.Cleanup(func() {
		globals.JSON = prevJSON
		globals.Yes = prevYes
	})
}

func TestSynapseCommand_Add(t *testing.T) {
	plainTestOutput(t)
	home := t.TempDir()
	t.Setenv("DENDRITE_HOME", home)

	dir1 := t.TempDir()
	dir2 := t.TempDir()
	dir3 := t.TempDir()

	// Initial create with 2 members
	var buf bytes.Buffer
	err := runSynapseCommand(context.Background(), []string{
		"create", "add-syn",
		"--members", fmt.Sprintf("%s=api:producer,%s=web:consumer", dir1, dir2),
	}, &buf)
	if err != nil {
		t.Fatalf("create failed: %v", err)
	}

	// 1. Successful add of a third member
	buf.Reset()
	err = runSynapseCommand(context.Background(), []string{
		"add", "add-syn",
		"--members", fmt.Sprintf("%s=analytics:consumer", dir3),
	}, &buf)
	if err != nil {
		t.Fatalf("add failed: %v", err)
	}
	if !strings.Contains(buf.String(), "[+] Added analytics (consumer) to synapse add-syn") {
		t.Errorf("expected success message for add, got:\n%s", buf.String())
	}

	// Verify persistence in store
	st, err := synapse.DefaultStore()
	if err != nil {
		t.Fatalf("DefaultStore failed: %v", err)
	}
	syn, err := st.Load("add-syn")
	if err != nil {
		t.Fatalf("Load failed: %v", err)
	}
	if len(syn.Members) != 3 {
		t.Fatalf("expected 3 members, got %d", len(syn.Members))
	}
	mAnalytics, ok := syn.Member("analytics")
	if !ok {
		t.Fatalf("expected member analytics to exist")
	}
	if mAnalytics.Role != synapse.RoleConsumer || mAnalytics.PathHint != dir3 {
		t.Errorf("unexpected member analytics: %+v", mAnalytics)
	}

	// Verify registered in global index
	indexPath, err := projectid.DefaultIndexPath()
	if err != nil {
		t.Fatalf("DefaultIndexPath failed: %v", err)
	}
	idx, err := projectid.LoadIndex(indexPath)
	if err != nil {
		t.Fatalf("LoadIndex failed: %v", err)
	}
	if _, ok := idx.Lookup(mAnalytics.ProjectID); !ok {
		t.Errorf("expected analytics project ID %s to be in index", mAnalytics.ProjectID)
	}

	// 2. Refuse duplicate alias
	dirDupAlias := t.TempDir()
	buf.Reset()
	err = runSynapseCommand(context.Background(), []string{
		"add", "add-syn",
		"--members", fmt.Sprintf("%s=api:consumer", dirDupAlias),
	}, &buf)
	if err == nil {
		t.Fatal("expected error adding member with duplicate alias, got nil")
	}
	if !strings.Contains(err.Error(), "alias") || !strings.Contains(err.Error(), "api") {
		t.Errorf("expected error naming duplicate alias 'api', got: %v", err)
	}

	// 3. Refuse duplicate project
	buf.Reset()
	err = runSynapseCommand(context.Background(), []string{
		"add", "add-syn",
		"--members", fmt.Sprintf("%s=api2:consumer", dir1),
	}, &buf)
	if err == nil {
		t.Fatal("expected error adding member with duplicate project ID, got nil")
	}
	if !strings.Contains(err.Error(), "project ID") {
		t.Errorf("expected error naming duplicate project ID, got: %v", err)
	}

	// 4. @PLANE suffix sets PlaneProject
	dir4 := t.TempDir()
	buf.Reset()
	err = runSynapseCommand(context.Background(), []string{
		"add", "add-syn",
		"--members", fmt.Sprintf("%s=worker:consumer@WORKER", dir4),
	}, &buf)
	if err != nil {
		t.Fatalf("add with @PLANE failed: %v", err)
	}
	syn, err = st.Load("add-syn")
	if err != nil {
		t.Fatal(err)
	}
	mWorker, ok := syn.Member("worker")
	if !ok || mWorker.PlaneProject != "WORKER" {
		t.Errorf("expected worker to have PlaneProject WORKER, got: %+v", mWorker)
	}

	// 5. Plane-binding fallback sets PlaneProject when @PLANE is omitted
	dir5 := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir5, ".git"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := plane.WriteLink(dir5, plane.ProjectLink{Identifier: "FALLBACK", ProjectID: "uuid-fallback"}); err != nil {
		t.Fatal(err)
	}
	buf.Reset()
	err = runSynapseCommand(context.Background(), []string{
		"add", "add-syn",
		"--members", fmt.Sprintf("%s=billing:peer", dir5),
	}, &buf)
	if err != nil {
		t.Fatalf("add with plane-binding fallback failed: %v", err)
	}
	syn, err = st.Load("add-syn")
	if err != nil {
		t.Fatal(err)
	}
	mBilling, ok := syn.Member("billing")
	if !ok || mBilling.PlaneProject != "FALLBACK" {
		t.Errorf("expected billing to have PlaneProject FALLBACK, got: %+v", mBilling)
	}
}

func TestSynapseCommand_RemoveMember(t *testing.T) {
	plainTestOutput(t)
	home := t.TempDir()
	t.Setenv("DENDRITE_HOME", home)

	dir1 := t.TempDir()
	dir2 := t.TempDir()
	dir3 := t.TempDir()

	// Initial create with 3 members
	var buf bytes.Buffer
	err := runSynapseCommand(context.Background(), []string{
		"create", "rm-syn",
		"--members", fmt.Sprintf("%s=m1:peer,%s=m2:peer,%s=m3:peer", dir1, dir2, dir3),
	}, &buf)
	if err != nil {
		t.Fatalf("create failed: %v", err)
	}

	st, err := synapse.DefaultStore()
	if err != nil {
		t.Fatalf("DefaultStore failed: %v", err)
	}

	// 1. Remove by alias
	buf.Reset()
	err = runSynapseCommand(context.Background(), []string{
		"remove-member", "rm-syn", "m3",
	}, &buf)
	if err != nil {
		t.Fatalf("remove-member by alias failed: %v", err)
	}
	if !strings.Contains(buf.String(), "[-] Removed m3 from synapse rm-syn") {
		t.Errorf("unexpected output removing by alias:\n%s", buf.String())
	}

	syn, err := st.Load("rm-syn")
	if err != nil {
		t.Fatal(err)
	}
	if len(syn.Members) != 2 {
		t.Fatalf("expected 2 members after removal, got %d", len(syn.Members))
	}
	if _, ok := syn.Member("m3"); ok {
		t.Errorf("m3 should have been removed")
	}

	// 2. Refuse when only 2 members remain
	buf.Reset()
	err = runSynapseCommand(context.Background(), []string{
		"remove-member", "rm-syn", "m2",
	}, &buf)
	if err == nil {
		t.Fatal("expected error when only 2 members remain, got nil")
	}
	if !strings.Contains(err.Error(), "at least 2 members") {
		t.Errorf("expected error mentioning 'at least 2 members', got: %v", err)
	}

	// 3. Add m3 back and remove by project ID
	buf.Reset()
	err = runSynapseCommand(context.Background(), []string{
		"add", "rm-syn",
		"--members", fmt.Sprintf("%s=m3:peer", dir3),
	}, &buf)
	if err != nil {
		t.Fatalf("add m3 back failed: %v", err)
	}
	syn, err = st.Load("rm-syn")
	if err != nil {
		t.Fatal(err)
	}
	m3, ok := syn.Member("m3")
	if !ok {
		t.Fatal("expected m3 to exist")
	}
	m3ProjectID := m3.ProjectID

	buf.Reset()
	err = runSynapseCommand(context.Background(), []string{
		"remove-member", "rm-syn", m3ProjectID,
	}, &buf)
	if err != nil {
		t.Fatalf("remove-member by project ID failed: %v", err)
	}
	if !strings.Contains(buf.String(), "[-] Removed m3 from synapse rm-syn") {
		t.Errorf("unexpected output removing by project ID:\n%s", buf.String())
	}

	// 4. Refuse when contract or parity command references the alias
	dirA := t.TempDir()
	dirB := t.TempDir()
	dirC := t.TempDir()
	buf.Reset()
	err = runSynapseCommand(context.Background(), []string{
		"create", "contract-syn",
		"--members", fmt.Sprintf("%s=prod:producer,%s=cons:consumer,%s=peer:peer", dirA, dirB, dirC),
		"--contract", "openapi:prod:spec.yaml>cons:client.ts",
		"--parity", "peer=go test ./...",
	}, &buf)
	if err != nil {
		t.Fatalf("create contract-syn failed: %v", err)
	}

	// Try removing producer
	buf.Reset()
	err = runSynapseCommand(context.Background(), []string{
		"remove-member", "contract-syn", "prod",
	}, &buf)
	if err == nil {
		t.Fatal("expected error removing producer referenced by contract, got nil")
	}
	if !strings.Contains(err.Error(), "contract") || !strings.Contains(err.Error(), "prod") {
		t.Errorf("expected error naming producer contract reference, got: %v", err)
	}

	// Try removing consumer
	buf.Reset()
	err = runSynapseCommand(context.Background(), []string{
		"remove-member", "contract-syn", "cons",
	}, &buf)
	if err == nil {
		t.Fatal("expected error removing consumer referenced by contract, got nil")
	}
	if !strings.Contains(err.Error(), "contract") || !strings.Contains(err.Error(), "cons") {
		t.Errorf("expected error naming consumer contract reference, got: %v", err)
	}

	// Try removing peer referenced by parity
	buf.Reset()
	err = runSynapseCommand(context.Background(), []string{
		"remove-member", "contract-syn", "peer",
	}, &buf)
	if err == nil {
		t.Fatal("expected error removing member referenced by parity command, got nil")
	}
	if !strings.Contains(err.Error(), "parity") || !strings.Contains(err.Error(), "peer") {
		t.Errorf("expected error naming parity reference, got: %v", err)
	}
}

func TestSynapseCommand_Delete(t *testing.T) {
	plainTestOutput(t)
	home := t.TempDir()
	t.Setenv("DENDRITE_HOME", home)

	dir1 := t.TempDir()
	dir2 := t.TempDir()

	// Create synapse to delete
	var buf bytes.Buffer
	err := runSynapseCommand(context.Background(), []string{
		"create", "del-syn",
		"--members", fmt.Sprintf("%s=m1:peer,%s=m2:peer", dir1, dir2),
	}, &buf)
	if err != nil {
		t.Fatalf("create failed: %v", err)
	}

	st, err := synapse.DefaultStore()
	if err != nil {
		t.Fatalf("DefaultStore failed: %v", err)
	}

	// 1. Refuse without --yes in non-interactive mode
	buf.Reset()
	err = runSynapseCommand(context.Background(), []string{
		"delete", "del-syn",
	}, &buf)
	if err == nil {
		t.Fatal("expected error deleting without --yes in non-interactive mode, got nil")
	}
	if !strings.Contains(err.Error(), "--yes") {
		t.Errorf("expected error mentioning '--yes', got: %v", err)
	}

	// Ensure synapse still exists
	if _, err := st.Load("del-syn"); err != nil {
		t.Fatalf("synapse should still exist after refusal, got error: %v", err)
	}

	// 2. Delete with --yes
	buf.Reset()
	err = runSynapseCommand(context.Background(), []string{
		"delete", "del-syn", "--yes",
	}, &buf)
	if err != nil {
		t.Fatalf("delete with --yes failed: %v", err)
	}
	if !strings.Contains(buf.String(), "[-] Deleted synapse del-syn") {
		t.Errorf("unexpected output deleting with --yes:\n%s", buf.String())
	}

	// Ensure synapse is gone
	if _, err := st.Load("del-syn"); !errors.Is(err, synapse.ErrNotFound) {
		t.Errorf("expected ErrNotFound after deletion, got: %v", err)
	}

	// 3. Unknown ID gives friendly error
	buf.Reset()
	err = runSynapseCommand(context.Background(), []string{
		"delete", "nonexistent-syn", "--yes",
	}, &buf)
	if err == nil {
		t.Fatal("expected error deleting nonexistent synapse, got nil")
	}
	if !strings.Contains(err.Error(), "no such synapse") || !strings.Contains(err.Error(), "nonexistent-syn") {
		t.Errorf("expected friendly error mentioning 'no such synapse', got: %v", err)
	}

	// 4. Interactive confirmation test
	prevIsTerminal := isTerminal
	isTerminal = func(io.Reader) bool { return true }
	t.Cleanup(func() { isTerminal = prevIsTerminal })

	// Create another synapse for prompt test
	buf.Reset()
	err = runSynapseCommand(context.Background(), []string{
		"create", "prompt-syn",
		"--members", fmt.Sprintf("%s=m1:peer,%s=m2:peer", dir1, dir2),
	}, &buf)
	if err != nil {
		t.Fatalf("create failed: %v", err)
	}

	// Prompt: decline with "n\n"
	var promptOut bytes.Buffer
	err = runSynapseDelete(context.Background(), "prompt-syn", strings.NewReader("n\n"), &promptOut)
	if err == nil {
		t.Fatal("expected cancellation error, got nil")
	}
	if !strings.Contains(err.Error(), "cancelled") {
		t.Errorf("expected cancellation error, got: %v", err)
	}
	if _, err := st.Load("prompt-syn"); err != nil {
		t.Fatalf("synapse should still exist after declined prompt, got: %v", err)
	}

	// Prompt: accept with "y\n"
	promptOut.Reset()
	err = runSynapseDelete(context.Background(), "prompt-syn", strings.NewReader("y\n"), &promptOut)
	if err != nil {
		t.Fatalf("delete with prompt acceptance failed: %v", err)
	}
	if !strings.Contains(promptOut.String(), "[-] Deleted synapse prompt-syn") {
		t.Errorf("expected deleted output, got: %s", promptOut.String())
	}
	if _, err := st.Load("prompt-syn"); !errors.Is(err, synapse.ErrNotFound) {
		t.Errorf("expected ErrNotFound after accepted deletion, got: %v", err)
	}
}
