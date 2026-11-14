package sidecar

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestRPCError_Kind_And_Error(t *testing.T) {
	// Empty Data
	errEmpty := &RPCError{Code: -32000, Message: "engine error"}
	if kind := errEmpty.Kind(); kind != "" {
		t.Errorf("expected empty kind, got: %q", kind)
	}
	if errEmpty.Error() != "sidecar error -32000: engine error" {
		t.Errorf("unexpected Error() string: %s", errEmpty.Error())
	}

	// Valid Data with kind
	errWithKind := &RPCError{
		Code:    -32000,
		Message: "engine error",
		Data:    json.RawMessage(`{"kind": "engine_oom"}`),
	}
	if kind := errWithKind.Kind(); kind != "engine_oom" {
		t.Errorf("expected 'engine_oom', got: %q", kind)
	}

	// Invalid Data json
	errBadJSON := &RPCError{
		Code:    -32000,
		Message: "bad json",
		Data:    json.RawMessage(`{invalid`),
	}
	if kind := errBadJSON.Kind(); kind != "" {
		t.Errorf("expected empty kind on invalid json, got: %q", kind)
	}
}

func TestStartWith_ExtraEnv(t *testing.T) {
	ctx := context.Background()
	c, err := StartWith(ctx, testBinary, 1, []string{"EXTRA_KEY=EXTRA_VAL"})
	if err != nil {
		t.Fatalf("StartWith failed: %v", err)
	}
	defer c.Close()

	if c.Health.Name != "testsidecar" {
		t.Errorf("expected testsidecar, got: %s", c.Health.Name)
	}
}

func TestCall_UnmarshalResultError(t *testing.T) {
	ctx := context.Background()
	c, err := Start(ctx, testBinary, 1)
	if err != nil {
		t.Fatalf("Start failed: %v", err)
	}
	defer c.Close()

	// echo returns an object {"count": 42}, try decoding into a string pointer
	var wrongTarget string
	err = c.Call(ctx, "echo", map[string]int{"count": 42}, &wrongTarget)
	if err == nil {
		t.Fatal("expected unmarshal error when decoding object into string, got nil")
	}
}

func TestLocate_NonExecutableCargoBin(t *testing.T) {
	userHome := t.TempDir()
	cargoBinDir := filepath.Join(userHome, ".cargo", "bin")
	if err := os.MkdirAll(cargoBinDir, 0755); err != nil {
		t.Fatal(err)
	}

	// 1. Cargo bin entry is a directory
	dirName := "testdir_bin"
	if err := os.MkdirAll(filepath.Join(cargoBinDir, dirName), 0755); err != nil {
		t.Fatal(err)
	}
	loc := Locator{UserHome: userHome, LookPath: noPath}
	_, err := loc.Locate(dirName)
	if err == nil {
		t.Errorf("expected error locating directory as executable")
	}

	// 2. Cargo bin entry exists but is mode 0644 (not executable)
	nonExecName := "nonexec_bin"
	filePath := filepath.Join(cargoBinDir, nonExecName)
	if err := os.WriteFile(filePath, []byte("#!/bin/sh\n"), 0644); err != nil {
		t.Fatal(err)
	}
	_, err = loc.Locate(nonExecName)
	if err == nil {
		t.Errorf("expected error locating non-executable file")
	}

	// 3. Candidates with empty Home
	emptyLoc := Locator{}
	cands := emptyLoc.Candidates("tool")
	if len(cands) != 0 {
		t.Errorf("expected 0 candidates for empty locator, got: %v", cands)
	}
}
