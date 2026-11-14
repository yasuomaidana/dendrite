package swap

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func TestSubsystemSwapper_ExtraBranches(t *testing.T) {
	sw := NewSubsystemSwapper()
	ctx := context.Background()

	// 1. TargetLanguage is empty -> defaults to "rust", OutputDir is empty
	res, err := sw.Swap(ctx, SwapRequest{
		SourcesPath:   "/path/to/legacy-sources",
		FromSubsystem: SubsystemOpenGL,
		ToSubsystem:   SubsystemVulkan,
	})
	if err != nil {
		t.Fatalf("Swap failed: %v", err)
	}
	if res.TargetLanguage != "rust" {
		t.Errorf("expected default target language 'rust', got: %s", res.TargetLanguage)
	}
	if len(res.EmittedFiles) != 0 {
		t.Errorf("expected 0 emitted files when OutputDir is empty, got: %d", len(res.EmittedFiles))
	}

	// 2. TargetLanguage is "ts" (shorthand for typescript)
	tempDir := t.TempDir()
	outDir := filepath.Join(tempDir, "ts_shorthand")
	resTS, err := sw.Swap(ctx, SwapRequest{
		SourcesPath:    "/path/to/sources",
		FromSubsystem:  SubsystemDirectX,
		ToSubsystem:    SubsystemThreeJS,
		TargetLanguage: "ts",
		OutputDir:      outDir,
	})
	if err != nil {
		t.Fatalf("Swap with 'ts' failed: %v", err)
	}
	if resTS.TargetLanguage != "ts" {
		t.Errorf("expected target language 'ts', got: %s", resTS.TargetLanguage)
	}
	if len(resTS.EmittedFiles) != 2 {
		t.Errorf("expected 2 emitted files for 'ts', got: %d", len(resTS.EmittedFiles))
	}

	// 3. Kinematic parity failure in Swap (using generateNodesFunc seam)
	origFunc := generateNodesFunc
	defer func() { generateNodesFunc = origFunc }()

	generateNodesFunc = func() []SceneNode {
		return []SceneNode{
			{
				ID:          "corrupt",
				Position:    [3]float64{10, 10, 10},
				Scale:       [3]float64{1, 1, 1},
				WorldMatrix: [16]float64{999}, // intentionally mismatched
			},
		}
	}

	_, err = sw.Swap(ctx, SwapRequest{
		SourcesPath:   "/path/to/sources",
		FromSubsystem: SubsystemOpenGL,
		ToSubsystem:   SubsystemWGPU,
	})
	if !errors.Is(err, ErrDomainSimulationCorrupted) {
		t.Fatalf("expected ErrDomainSimulationCorrupted, got: %v", err)
	}
}

func TestSubsystemSwapper_EmitArtifacts_Errors(t *testing.T) {
	sw := NewSubsystemSwapper()
	tempDir := t.TempDir()

	// 1. MkdirAll failure: outDir is inside a regular file
	parentFile := filepath.Join(tempDir, "file_as_dir")
	if err := os.WriteFile(parentFile, []byte("data"), 0644); err != nil {
		t.Fatal(err)
	}
	invalidOutDir := filepath.Join(parentFile, "sub")
	_, err := sw.emitArtifacts(invalidOutDir, "go", "trait", "adapter")
	if err == nil {
		t.Errorf("expected MkdirAll error in emitArtifacts")
	}

	// 2. WriteFile failure: destination file paths are existing directories
	outDir := filepath.Join(tempDir, "existing_dirs")
	if err := os.MkdirAll(filepath.Join(outDir, "backend.go"), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(outDir, "adapter.go"), 0755); err != nil {
		t.Fatal(err)
	}

	files, err := sw.emitArtifacts(outDir, "go", "trait", "adapter")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// Neither file should be appended since WriteFile failed for both directories
	if len(files) != 0 {
		t.Errorf("expected 0 files appended on WriteFile failures, got: %d", len(files))
	}
}
