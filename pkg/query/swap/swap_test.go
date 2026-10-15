package swap

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func TestSubsystemSwapper_OpenGLToWGPU_Rust(t *testing.T) {
	sw := NewSubsystemSwapper()
	ctx := context.Background()
	tempDir := t.TempDir()
	outDir := filepath.Join(tempDir, "swapped_rust")

	res, err := sw.Swap(ctx, SwapRequest{
		SourcesPath:    "/path/to/legacy-opengl",
		FromSubsystem:  SubsystemOpenGL,
		ToSubsystem:    SubsystemWGPU,
		TargetLanguage: "rust",
		OutputDir:      outDir,
	})
	if err != nil {
		t.Fatalf("Swap failed: %v", err)
	}

	if !res.MatrixParityVerified {
		t.Error("expected matrix parity to be verified")
	}
	if res.TraitBoundaryGenerated != "RenderBackend" {
		t.Errorf("expected RenderBackend trait, got %s", res.TraitBoundaryGenerated)
	}
	if len(res.EmittedFiles) != 2 {
		t.Errorf("expected 2 emitted files, got %d", len(res.EmittedFiles))
	}
	if _, err := os.Stat(filepath.Join(outDir, "backend.rs")); err != nil {
		t.Errorf("backend.rs not found: %v", err)
	}
}

func TestSubsystemSwapper_DirectXToThreeJS_TypeScript(t *testing.T) {
	sw := NewSubsystemSwapper()
	ctx := context.Background()
	tempDir := t.TempDir()
	outDir := filepath.Join(tempDir, "swapped_ts")

	res, err := sw.Swap(ctx, SwapRequest{
		SourcesPath:    "/path/to/directx-app",
		FromSubsystem:  SubsystemDirectX,
		ToSubsystem:    SubsystemThreeJS,
		TargetLanguage: "typescript",
		OutputDir:      outDir,
	})
	if err != nil {
		t.Fatalf("Swap failed: %v", err)
	}

	if !res.MatrixParityVerified {
		t.Error("expected matrix parity to be verified")
	}
	if _, err := os.Stat(filepath.Join(outDir, "RenderBackend.ts")); err != nil {
		t.Errorf("RenderBackend.ts not found: %v", err)
	}
}

func TestSubsystemSwapper_VulkanToWGPU_Go(t *testing.T) {
	sw := NewSubsystemSwapper()
	ctx := context.Background()
	tempDir := t.TempDir()
	outDir := filepath.Join(tempDir, "swapped_go")

	res, err := sw.Swap(ctx, SwapRequest{
		SourcesPath:    "/path/to/vulkan-app",
		FromSubsystem:  SubsystemVulkan,
		ToSubsystem:    SubsystemWGPU,
		TargetLanguage: "go",
		OutputDir:      outDir,
	})
	if err != nil {
		t.Fatalf("Swap failed: %v", err)
	}

	if !res.MatrixParityVerified {
		t.Error("expected matrix parity to be verified")
	}
	if _, err := os.Stat(filepath.Join(outDir, "backend.go")); err != nil {
		t.Errorf("backend.go not found: %v", err)
	}
}

func TestSubsystemSwapper_ValidationErrors(t *testing.T) {
	sw := NewSubsystemSwapper()
	ctx := context.Background()

	// Empty sources path
	_, err := sw.Swap(ctx, SwapRequest{
		FromSubsystem: SubsystemOpenGL,
		ToSubsystem:   SubsystemWGPU,
	})
	if !errors.Is(err, ErrInvalidSourcesPath) {
		t.Errorf("expected ErrInvalidSourcesPath, got %v", err)
	}

	// Unsupported from subsystem
	_, err = sw.Swap(ctx, SwapRequest{
		SourcesPath:   "/path/to/app",
		FromSubsystem: "unsupported_subsystem",
		ToSubsystem:   SubsystemWGPU,
	})
	if !errors.Is(err, ErrUnsupportedSubsystem) {
		t.Errorf("expected ErrUnsupportedSubsystem, got %v", err)
	}

	// Unsupported to subsystem
	_, err = sw.Swap(ctx, SwapRequest{
		SourcesPath:   "/path/to/app",
		FromSubsystem: SubsystemOpenGL,
		ToSubsystem:   "unsupported_subsystem",
	})
	if !errors.Is(err, ErrUnsupportedSubsystem) {
		t.Errorf("expected ErrUnsupportedSubsystem, got %v", err)
	}

	// Corrupted kinematic node check
	corruptedNodes := []SceneNode{
		{
			ID:          "corrupt_node",
			Position:    [3]float64{10, 20, 30},
			Scale:       [3]float64{1, 1, 1},
			WorldMatrix: [16]float64{999, 999, 999}, // Corrupted matrix
		},
	}
	if sw.verifyKinematicsParity(corruptedNodes) {
		t.Error("expected verifyKinematicsParity to return false for corrupted matrix")
	}
}
