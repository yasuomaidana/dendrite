package rea

import (
	"errors"
)

// Standard errors for REA client operations.
var (
	ErrTokenLimitExceeded = errors.New("evidence bundle exceeds maximum token boundary (4,000 tokens)")
	ErrEmptyBinaryPath    = errors.New("target binary path cannot be empty")
	ErrSymbolNotFound     = errors.New("requested function symbol not found in binary")
	ErrUntrustedOnHost    = errors.New("untrusted ELF/PE binaries must run in remote Linux container")
)

// BinaryFormat denotes the binary executable format.
type BinaryFormat string

const (
	FormatMachO BinaryFormat = "Mach-O"
	FormatELF   BinaryFormat = "ELF"
	FormatPE    BinaryFormat = "PE"
	FormatASAR  BinaryFormat = "ASAR"
)

// WorkerTarget indicates where the analysis executes.
type WorkerTarget string

const (
	TargetLocalMac    WorkerTarget = "local-macos"
	TargetRemoteLinux WorkerTarget = "remote-linux-container"
)

// DisassemblerEngine identifies the disassembly and decompilation engine.
type DisassemblerEngine string

const (
	EngineHopper DisassemblerEngine = "hopper"
	EngineGhidra DisassemblerEngine = "ghidra"
	EngineRizin  DisassemblerEngine = "rizin"
)

// EvidenceBundle represents a bounded, high-precision reverse-engineering context bundle.
type EvidenceBundle struct {
	TargetBinary    string       `json:"target_binary"`
	Format          BinaryFormat `json:"format"`
	FunctionSymbol  string       `json:"function_symbol"`
	Pseudocode      string       `json:"pseudocode"`
	Disassembly     string       `json:"disassembly,omitempty"`
	CrossReferences []string     `json:"cross_references,omitempty"`
	StringConstants []string     `json:"string_constants,omitempty"`
	TokenEstimate   int          `json:"token_estimate"`
	Truncated       bool         `json:"truncated"`
}

// ADRDocument defines an Architecture Decision Record generated during the REA workflow.
type ADRDocument struct {
	ID           string   `json:"id"`
	Title        string   `json:"title"`
	Status       string   `json:"status"`
	Date         string   `json:"date"`
	PlaneEpic    string   `json:"plane_epic,omitempty"`
	Repository   string   `json:"repository"`
	Tags         []string `json:"tags"`
	Context      string   `json:"context"`
	Decision     string   `json:"decision"`
	Consequences string   `json:"consequences"`
}

// AnalysisRequest configures a disassembler target and symbol request.
type AnalysisRequest struct {
	BinaryPath string
	Format     BinaryFormat
	Symbol     string
	Engine     DisassemblerEngine
}
