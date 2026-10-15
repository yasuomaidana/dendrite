package fuse

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"Dendrite/pkg/ui/modal"
)

func TestArchitectureFuser_RustFusionSuccess(t *testing.T) {
	tempDir := t.TempDir()
	p1 := filepath.Join(tempDir, "proj_a")
	p2 := filepath.Join(tempDir, "proj_b")
	_ = os.MkdirAll(p1, 0755)
	_ = os.MkdirAll(p2, 0755)

	_ = os.WriteFile(filepath.Join(p1, "Cargo.toml"), []byte("[package]\nname = \"proj_a\"\n[dependencies]\ntokio = \"1.0\"\nserde = \"1.0\"\n"), 0644)
	_ = os.WriteFile(filepath.Join(p2, "Cargo.toml"), []byte("[package]\nname = \"proj_b\"\n[dependencies]\ntokio = \"1.0\"\nprotobuf = \"3.0\"\n"), 0644)

	targetDir := filepath.Join(tempDir, "fused_core")

	fuser := NewArchitectureFuser()
	ctx := context.Background()

	promptCalled := false
	res, err := fuser.Fuse(ctx, FuseRequest{
		Sources:         []string{p1, p2},
		FusedMemberName: "fused_core",
		TargetLanguage:  "rust",
		Prompt: func(m *modal.DirectoryPickerModal) (string, error) {
			promptCalled = true
			return targetDir, nil
		},
	})
	if err != nil {
		t.Fatalf("Fuse failed: %v", err)
	}

	if !promptCalled {
		t.Error("expected interactive prompt to be called")
	}

	if res.ChannelMechanism != "crossbeam-channel (zero-copy ring buffer)" {
		t.Errorf("expected crossbeam-channel, got %s", res.ChannelMechanism)
	}

	// Verify emitted Rust files
	libContent, err := os.ReadFile(filepath.Join(targetDir, "src", "lib.rs"))
	if err != nil {
		t.Fatalf("failed reading lib.rs: %v", err)
	}
	if !strings.Contains(string(libContent), "pub mod channel;") {
		t.Error("missing channel mod in lib.rs")
	}

	// Verify deduplicated libraries
	foundTokio := false
	for _, lib := range res.DeduplicatedLibraries {
		if lib == "tokio" {
			foundTokio = true
			break
		}
	}
	if !foundTokio {
		t.Error("expected tokio to be in deduplicated libraries")
	}
}

func TestArchitectureFuser_GoFusionAndPoolRegister(t *testing.T) {
	tempDir := t.TempDir()
	p1 := filepath.Join(tempDir, "go_svc1")
	p2 := filepath.Join(tempDir, "go_svc2")
	_ = os.MkdirAll(p1, 0755)
	_ = os.MkdirAll(p2, 0755)

	targetDir := filepath.Join(tempDir, "fused_go")
	poolCfg := filepath.Join(tempDir, "pool.json")

	fuser := NewArchitectureFuser()
	ctx := context.Background()

	res, err := fuser.Fuse(ctx, FuseRequest{
		Sources:         []string{p1, p2},
		FusedMemberName: "fused_go_module",
		TargetLanguage:  "go",
		TargetDirectory: targetDir,
		RegisterInPool:  true,
		PoolConfigFile:  poolCfg,
	})
	if err != nil {
		t.Fatalf("Go fusion failed: %v", err)
	}

	if !res.PoolRegistered {
		t.Error("expected pool registration to be true")
	}
	if !strings.Contains(res.ChannelMechanism, "go-channel") {
		t.Errorf("expected go-channel mechanism, got %s", res.ChannelMechanism)
	}

	chanContent, err := os.ReadFile(filepath.Join(targetDir, "channel.go"))
	if err != nil {
		t.Fatalf("failed reading channel.go: %v", err)
	}
	if !strings.Contains(string(chanContent), "type RingBufferChannel struct") {
		t.Error("missing RingBufferChannel in channel.go")
	}
}

func TestArchitectureFuser_ValidationAndCancellation(t *testing.T) {
	fuser := NewArchitectureFuser()
	ctx := context.Background()

	// Insufficient sources
	_, err := fuser.Fuse(ctx, FuseRequest{
		Sources: []string{"only_one"},
	})
	if !errors.Is(err, ErrInsufficientSources) {
		t.Errorf("expected ErrInsufficientSources, got %v", err)
	}

	// Missing target directory and missing prompt
	_, err = fuser.Fuse(ctx, FuseRequest{
		Sources: []string{"s1", "s2"},
	})
	if !errors.Is(err, ErrFusionTargetRequired) {
		t.Errorf("expected ErrFusionTargetRequired, got %v", err)
	}

	// Prompt cancelled
	_, err = fuser.Fuse(ctx, FuseRequest{
		Sources: []string{"s1", "s2"},
		Prompt: func(m *modal.DirectoryPickerModal) (string, error) {
			return "", modal.ErrPromptCancelled
		},
	})
	if !errors.Is(err, ErrFusionPromptCancelled) {
		t.Errorf("expected ErrFusionPromptCancelled, got %v", err)
	}

	// Prompt returns empty string
	_, err = fuser.Fuse(ctx, FuseRequest{
		Sources: []string{"s1", "s2"},
		Prompt: func(m *modal.DirectoryPickerModal) (string, error) {
			return "   ", nil
		},
	})
	if !errors.Is(err, ErrFusionTargetRequired) {
		t.Errorf("expected ErrFusionTargetRequired on empty string, got %v", err)
	}
}
