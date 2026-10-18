package ast

import (
	"errors"
)

// Standard error definitions for AST provider operations.
var (
	ErrEmptySource    = errors.New("source code payload cannot be empty")
	ErrNoCompleteNode = errors.New("no complete top-level declaration found")
	ErrInvalidSyntax  = errors.New("invalid syntax in source code")
)

// SymbolSlice maps a modified git diff hunk to its enclosing code declaration.
type SymbolSlice struct {
	File      string `json:"file"`
	Symbol    string `json:"symbol"`
	Kind      string `json:"kind"` // "func", "method", "type", "var", "const", "package", "file"
	Receiver  string `json:"receiver,omitempty"`
	StartLine int    `json:"start_line"`
	EndLine   int    `json:"end_line"`
	HunkRange string `json:"hunk_range,omitempty"`
	HunkStart int    `json:"hunk_start,omitempty"`
	HunkEnd   int    `json:"hunk_end,omitempty"`
	Signature string `json:"signature,omitempty"`
	Content   string `json:"content,omitempty"`
}

// ASTProvider abstracts syntax-aware AST parsing, symbol mapping, signature folding,
// and boundary detection across supported language runtimes.
type ASTProvider interface {
	// ChangedSymbols maps each git hunk between baseRef and the current working tree/HEAD to its enclosing symbol.
	ChangedSymbols(repoDir string, baseRef string) ([]SymbolSlice, error)

	// FoldSignatures prunes function bodies for non-kept functions, preserving only signatures and doc comments.
	FoldSignatures(src []byte, keepSymbols []string) ([]byte, error)

	// LastCompleteNode returns the byte offset of the end of the last complete top-level declaration.
	LastCompleteNode(src []byte) (int, error)
}
