package importer_test

import (
	"errors"
	"strings"
	"testing"

	"Dendrite/pkg/ai/importer"
)

func TestDissectCapability(t *testing.T) {
	// 1. Error on empty repo
	if _, err := importer.DissectCapability("", "parse_tokens", "python", nil, nil); !errors.Is(err, importer.ErrEmptySourceRepo) {
		t.Errorf("expected ErrEmptySourceRepo, got: %v", err)
	}

	// 2. Error on empty symbol
	if _, err := importer.DissectCapability("github.com/fast/parser", "", "rust", nil, nil); !errors.Is(err, importer.ErrEmptySymbolName) {
		t.Errorf("expected ErrEmptySymbolName, got: %v", err)
	}

	// 3. Valid capability
	vectors := []importer.TestVector{
		{Name: "simple word", Input: "hello world", ExpectedOutput: "helloworld", IsEdgeCase: false},
		{Name: "empty input", Input: "", ExpectedOutput: "", IsEdgeCase: true},
	}
	invariants := []string{"O(N) runtime", "Preserves ASCII fidelity"}

	cap, err := importer.DissectCapability("github.com/simd/faststring", "StripSpaces", "c", invariants, vectors)
	if err != nil {
		t.Fatalf("unexpected error dissecting capability: %v", err)
	}

	if cap.SymbolName != "StripSpaces" || len(cap.TestVectors) != 2 {
		t.Errorf("unexpected capability fields: %v", cap)
	}
}

func TestSynthesizeCleanroom(t *testing.T) {
	// 1. Error on nil capability
	if _, err := importer.SynthesizeCleanroom(importer.CleanroomSpec{Capability: nil}); err == nil {
		t.Errorf("expected error when capability is nil")
	}

	// 2. Error on unsupported target language
	cap := &importer.ExternalCapability{
		SourceRepo: "github.com/legacy/c-algo",
		SymbolName: "ComputeHash",
	}
	specUnsupported := importer.CleanroomSpec{
		TargetLanguage: "fortran",
		Capability:     cap,
	}
	if _, err := importer.SynthesizeCleanroom(specUnsupported); !errors.Is(err, importer.ErrUnsupportedTarget) {
		t.Errorf("expected ErrUnsupportedTarget for fortran, got: %v", err)
	}

	// 3. Valid synthesis for Go
	specValid := importer.CleanroomSpec{
		TargetLanguage:   "go",
		PackageName:      "faststr",
		StructOrFuncName: "StripSpaces",
		Capability: &importer.ExternalCapability{
			SourceRepo:     "github.com/simd/parser",
			SymbolName:     "StripSpaces",
			SourceLanguage: "c",
			Invariants:     []string{"Zero allocations on empty string", "Preserves byte order"},
			TestVectors: []importer.TestVector{
				{Name: "basic", Input: "a b c", ExpectedOutput: "abc"},
				{Name: "empty", Input: "", ExpectedOutput: ""},
			},
		},
	}

	mod, err := importer.SynthesizeCleanroom(specValid)
	if err != nil {
		t.Fatalf("unexpected error synthesizing cleanroom module: %v", err)
	}

	// Verify package name
	if mod.PackageName != "faststr" {
		t.Errorf("expected package faststr, got %s", mod.PackageName)
	}

	// Verify source code
	if !strings.Contains(mod.SourceCode, "func StripSpaces(input string) string") {
		t.Errorf("expected function declaration in source code")
	}
	if !strings.Contains(mod.SourceCode, "Zero allocations on empty string") {
		t.Errorf("expected invariant comments in source code")
	}

	// Verify test code
	if !strings.Contains(mod.TestCode, "TestStripSpaces_Vectors") || !strings.Contains(mod.TestCode, "want: \"abc\"") {
		t.Errorf("expected test vector assertions in test code")
	}

	// Verify AC-4 Complexity limits: CC <= 10, LOC <= 60
	if mod.CyclomaticComplexity > 10 {
		t.Errorf("expected CC <= 10, got %d", mod.CyclomaticComplexity)
	}
	if mod.TotalLOC > 60 {
		t.Errorf("expected total LOC <= 60, got %d", mod.TotalLOC)
	}
}
