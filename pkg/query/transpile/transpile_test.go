package transpile

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestRustTranspiler_SafeTranspile(t *testing.T) {
	trans := NewRustTranspiler()
	ctx := context.Background()
	tempDir := t.TempDir()
	outDir := filepath.Join(tempDir, "my_transpiled_lib")

	res, err := trans.Transpile(ctx, TranspileRequest{
		SourcesArtifact:   "/path/to/legacy-sim.bin",
		TargetLanguage:    "rust",
		NoStd:             false,
		AuditLibraryReuse: true,
		Prompt:            "translate physics simulation to safe rust with newtypes",
		OutputDir:         outDir,
	})
	if err != nil {
		t.Fatalf("Transpile failed: %v", err)
	}

	if res.CrateName != "legacy_sim_bin" {
		t.Errorf("expected crate name legacy_sim_bin, got %s", res.CrateName)
	}
	if !res.ZeroUnsafeVerified {
		t.Error("expected zero unsafe verification to be true")
	}
	if !res.DifferentialTestsPassed {
		t.Error("expected differential tests to pass")
	}
	if res.MaxDelta > 1e-6 {
		t.Errorf("max delta %e exceeds 1e-6", res.MaxDelta)
	}
	if len(res.NewtypesEmitted) < 3 {
		t.Errorf("expected at least 3 newtypes, got %d", len(res.NewtypesEmitted))
	}

	// Verify emitted crate files
	libContent, err := os.ReadFile(filepath.Join(outDir, "src", "lib.rs"))
	if err != nil {
		t.Fatalf("reading emitted lib.rs: %v", err)
	}
	if !strings.Contains(string(libContent), "#![forbid(unsafe_code)]") {
		t.Error("missing #![forbid(unsafe_code)] in lib.rs")
	}
	if !strings.Contains(string(libContent), "pub struct EntityId(pub u64);") {
		t.Error("missing EntityId Newtype in lib.rs")
	}
}

func TestRustTranspiler_NoStd(t *testing.T) {
	trans := NewRustTranspiler()
	ctx := context.Background()
	tempDir := t.TempDir()
	outDir := filepath.Join(tempDir, "no_std_crate")

	res, err := trans.Transpile(ctx, TranspileRequest{
		SourcesArtifact: "embedded_sensor",
		NoStd:           true,
		OutputDir:       outDir,
	})
	if err != nil {
		t.Fatalf("no_std transpile failed: %v", err)
	}
	if !res.NoStd {
		t.Error("expected NoStd to be true")
	}

	libContent, _ := os.ReadFile(filepath.Join(outDir, "src", "lib.rs"))
	if !strings.Contains(string(libContent), "#![no_std]") {
		t.Error("missing #![no_std] directive in lib.rs")
	}
}

func TestRustTranspiler_ValidationAndUnsafeAudit(t *testing.T) {
	trans := NewRustTranspiler()
	ctx := context.Background()

	// Empty artifact path
	_, err := trans.Transpile(ctx, TranspileRequest{
		SourcesArtifact: "   ",
	})
	if !errors.Is(err, ErrEmptyArtifact) {
		t.Errorf("expected ErrEmptyArtifact, got %v", err)
	}

	// Sanitize crate name fallbacks
	if sanitizeCrateName("...") != "transpiled_crate" {
		t.Errorf("expected fallback transpiled_crate, got %s", sanitizeCrateName("..."))
	}
}
