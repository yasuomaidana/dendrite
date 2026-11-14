package importer_test

import (
	"Dendrite/pkg/coreclient/coretest"
	"errors"
	"strings"
	"testing"

	"Dendrite/pkg/ai/importer"
	"Dendrite/pkg/testkit"
)

func TestMain(m *testing.M) { coretest.Main(m) }

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
	testkit.Check(t, err)

	testkit.Equal(t, "symbol", cap.SymbolName, "StripSpaces")
	testkit.Equal(t, "vectors length", len(cap.TestVectors), 2)
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

	// 3. Valid synthesis for Go with custom package and name
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
	testkit.Check(t, err)

	testkit.Equal(t, "package name", mod.PackageName, "faststr")

	if !strings.Contains(mod.SourceCode, "func StripSpaces(input string) string") {
		t.Errorf("expected function declaration in source code")
	}
	if !strings.Contains(mod.SourceCode, "Zero allocations on empty string") {
		t.Errorf("expected invariant comments in source code")
	}
	if !strings.Contains(mod.TestCode, "TestStripSpaces_Vectors") || !strings.Contains(mod.TestCode, "want: \"abc\"") {
		t.Errorf("expected test vector assertions in test code")
	}

	if mod.CyclomaticComplexity > 10 {
		t.Errorf("expected CC <= 10, got %d", mod.CyclomaticComplexity)
	}
	if mod.TotalLOC > 60 {
		t.Errorf("expected total LOC <= 60, got %d", mod.TotalLOC)
	}

	// 4. Valid synthesis with defaults (empty PackageName, empty StructOrFuncName, empty TargetLanguage)
	specDefaults := importer.CleanroomSpec{
		Capability: &importer.ExternalCapability{
			SourceRepo: "github.com/simd/parser",
			SymbolName: "StripSpaces",
		},
	}
	modDefaults, err := importer.SynthesizeCleanroom(specDefaults)
	testkit.Check(t, err)
	testkit.Equal(t, "default package", modDefaults.PackageName, "imported")
}

func TestSynthesizeCleanroom_ComplexityLimits(t *testing.T) {
	// Exceeded complexity
	specExceeded := importer.CleanroomSpec{
		TargetLanguage:   "go",
		StructOrFuncName: "HighComplexity",
		Capability: &importer.ExternalCapability{
			SourceRepo: "github.com/simd/parser",
			SymbolName: "HighComplexity",
		},
	}
	_, err := importer.SynthesizeCleanroom(specExceeded)
	if !errors.Is(err, importer.ErrComplexityExceeded) {
		t.Fatalf("expected ErrComplexityExceeded, got %v", err)
	}

	// Analysis error
	specError := importer.CleanroomSpec{
		TargetLanguage:   "go",
		StructOrFuncName: "FORCE_ERROR",
		Capability: &importer.ExternalCapability{
			SourceRepo: "github.com/simd/parser",
			SymbolName: "FORCE_ERROR",
		},
	}
	_, err = importer.SynthesizeCleanroom(specError)
	if err == nil {
		t.Fatal("expected error on complexity analysis error")
	}
}
