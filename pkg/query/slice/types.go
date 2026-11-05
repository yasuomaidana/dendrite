package slice

import (
	"errors"
	"time"
)

// SliceDirection defines reachability traversal direction.
type SliceDirection string

const (
	DirectionForward       SliceDirection = "forward"
	DirectionBackward      SliceDirection = "backward"
	DirectionBidirectional SliceDirection = "bidirectional"
)

// Standard typed errors.
var (
	ErrSourceNotFound     = errors.New("sources path does not exist")
	ErrEntryPointNotFound = errors.New("entrypoint symbol was not found in knowledge graph")
	ErrSliceEmpty         = errors.New("reachability slice returned zero symbols")
	ErrMissingParameter   = errors.New("required parameter missing")
)

// GoldenTestVector captures deterministic input/output parity assertions.
type GoldenTestVector struct {
	SymbolName        string `json:"symbol_name"`
	Input             string `json:"input"`
	ExpectedOutput    string `json:"expected_output"`
	DeterministicHash string `json:"deterministic_hash"`
}

// SliceRequest specifies criteria for intent-driven CFG/AST debloating.
type SliceRequest struct {
	SourcesPath      string         `json:"sources_path"`
	EntryPointSymbol string         `json:"entrypoint_symbol"`
	Intent           string         `json:"intent"`
	Direction        SliceDirection `json:"direction"`
	OutputDir        string         `json:"output_dir,omitempty"`
}

// SliceResult summarizes reachability traversal and dead-code reduction metrics.
type SliceResult struct {
	SourcesPath          string             `json:"sources_path"`
	EntryPoint           string             `json:"entrypoint"`
	Intent               string             `json:"intent"`
	OriginalSymbolsCount int                `json:"original_symbols_count"`
	RetainedSymbolsCount int                `json:"retained_symbols_count"`
	PruneRatio           float64            `json:"prune_ratio"` // e.g. 0.85 = 85% debloated
	RetainedSymbols      []string           `json:"retained_symbols"`
	PrunedSymbols        []string           `json:"pruned_symbols"`
	GoldenVectors        []GoldenTestVector `json:"golden_vectors"`
	EmittedProjectPath   string             `json:"emitted_project_path,omitempty"`
	Timestamp            time.Time          `json:"timestamp"`
}
