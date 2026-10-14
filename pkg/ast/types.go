package ast

import (
	"errors"
)

// Standard error definitions for AST and graph operations.
var (
	ErrNodeAlreadyExists = errors.New("node already exists in graph")
	ErrNodeNotFound      = errors.New("node not found in graph")
	ErrInvalidDepth      = errors.New("query depth must be greater than zero")
)

// Language represents the programming language of a source artifact.
type Language string

const (
	LangGo         Language = "go"
	LangRust       Language = "rust"
	LangTypeScript Language = "typescript"
	LangPython     Language = "python"
)

// SymbolKind denotes the syntactic role of a code symbol.
type SymbolKind string

const (
	SymbolFunction  SymbolKind = "function"
	SymbolMethod    SymbolKind = "method"
	SymbolStruct    SymbolKind = "struct"
	SymbolInterface SymbolKind = "interface"
	SymbolVariable  SymbolKind = "variable"
)

// EdgeType defines the dependency or invocation relationship between symbols.
type EdgeType string

const (
	EdgeCalls      EdgeType = "calls"
	EdgeImports    EdgeType = "imports"
	EdgeImplements EdgeType = "implements"
	EdgeDefines    EdgeType = "defines"
)

// SymbolNode represents an indexed symbol in the codebase knowledge graph.
type SymbolNode struct {
	ID                   string     `json:"id"`
	Name                 string     `json:"name"`
	Kind                 SymbolKind `json:"kind"`
	Language             Language   `json:"language"`
	FilePath             string     `json:"file_path"`
	StartLine            int        `json:"start_line"`
	EndLine              int        `json:"end_line"`
	CyclomaticComplexity int        `json:"cyclomatic_complexity"`
	CognitiveComplexity  int        `json:"cognitive_complexity"`
}

// SymbolEdge denotes a directed connection between two symbols.
type SymbolEdge struct {
	FromID string   `json:"from_id"`
	ToID   string   `json:"to_id"`
	Type   EdgeType `json:"type"`
}

// BlastRadiusReport summarizes the upstream blast radius impact when refactoring a symbol.
type BlastRadiusReport struct {
	RootSymbol      string       `json:"root_symbol"`
	MaxDepth        int          `json:"max_depth"`
	AffectedSymbols []SymbolNode `json:"affected_symbols"`
	TotalAffected   int          `json:"total_affected"`
	MaxComplexity   int          `json:"max_complexity"`
}
