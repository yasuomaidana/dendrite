package importer

import (
	"errors"
)

// Standard error definitions for cleanroom import operations.
var (
	ErrEmptySymbolName    = errors.New("symbol name cannot be empty")
	ErrEmptySourceRepo    = errors.New("source repository cannot be empty")
	ErrUnsupportedTarget  = errors.New("unsupported target language")
	ErrComplexityExceeded = errors.New("synthesized module exceeds allowed complexity ceiling (CC <= 10)")
)

// TestVector captures input-output pairs observed from external capabilities.
type TestVector struct {
	Name           string `json:"name"`
	Input          string `json:"input"`
	ExpectedOutput string `json:"expected_output"`
	IsEdgeCase     bool   `json:"is_edge_case"`
}

// ExternalCapability encapsulates algorithmic behaviors and contracts extracted from external repositories.
type ExternalCapability struct {
	SourceRepo     string       `json:"source_repo"`
	SymbolName     string       `json:"symbol_name"`
	SourceLanguage string       `json:"source_language"`
	Invariants     []string     `json:"invariants"`
	TestVectors    []TestVector `json:"test_vectors"`
}

// CleanroomSpec configures the target synthesis parameters.
type CleanroomSpec struct {
	TargetLanguage   string              `json:"target_language"` // "go"
	PackageName      string              `json:"package_name"`
	StructOrFuncName string              `json:"struct_or_func_name"`
	Capability       *ExternalCapability `json:"capability"`
}

// SynthesizedModule contains the generated source implementation and unit test harness.
type SynthesizedModule struct {
	PackageName          string `json:"package_name"`
	SourceCode           string `json:"source_code"`
	TestCode             string `json:"test_code"`
	CyclomaticComplexity int    `json:"cyclomatic_complexity"`
	CognitiveComplexity  int    `json:"cognitive_complexity"`
	TotalLOC             int    `json:"total_loc"`
}
