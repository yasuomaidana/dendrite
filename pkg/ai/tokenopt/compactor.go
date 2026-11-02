package tokenopt

import (
	"context"
	"math"
	"strings"

	"Dendrite/pkg/coreclient"
)

// EstimateTokens calculates an approximate token volume (~4 chars/token).
func EstimateTokens(text string) int {
	chars := len(strings.TrimSpace(text))
	tokens := chars / 4
	if chars > 0 && tokens == 0 {
		return 1
	}
	return tokens
}

// CompactGoSource prunes non-essential function bodies from Go source files,
// preserving imports, structs, interface signatures, and methods explicitly in keepSymbols.
// It leverages dendrite-core's ast.fold_signatures for syntax-aware AST pruning.
func CompactGoSource(source string, keepSymbols []string) (*CompactedContext, error) {
	trimmedSource := strings.TrimSpace(source)
	if trimmedSource == "" {
		return nil, ErrEmptySource
	}

	folded, err := coreclient.Default().FoldSignatures(context.Background(), "go", source, keepSymbols)
	if err != nil {
		return nil, err
	}
	compacted := folded.Source

	origChars := len(source)
	compactChars := len(compacted)

	origTokens := EstimateTokens(source)
	compactTokens := EstimateTokens(compacted)

	var ratio float64
	if origChars > 0 {
		ratio = math.Max(0, float64(origChars-compactChars)/float64(origChars))
	}

	return &CompactedContext{
		OriginalChars:            origChars,
		CompactedChars:           compactChars,
		EstimatedOriginalTokens:  origTokens,
		EstimatedCompactedTokens: compactTokens,
		CompactionRatio:          ratio,
		CompactedContent:         compacted,
	}, nil
}
