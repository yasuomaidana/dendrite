package tokenopt

import (
	"bufio"
	"strings"

	"Dendrite/pkg/ast"
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
// It leverages ASTProvider.FoldSignatures for syntax-aware AST pruning, falling back
// to line-based heuristics for partial Go code snippets.
func CompactGoSource(source string, keepSymbols []string) (*CompactedContext, error) {
	trimmedSource := strings.TrimSpace(source)
	if trimmedSource == "" {
		return nil, ErrEmptySource
	}

	provider := ast.NewGoASTProvider()
	folded, err := provider.FoldSignatures([]byte(source), keepSymbols)
	var compacted string
	if err == nil {
		compacted = strings.ReplaceAll(string(folded), ast.PrunedBodyPlaceholder, "{\n\t// ... implementation pruned for token economy\n}")
	} else {
		// Fallback for non-parsable partial Go code snippets
		compacted = compactGoSourceLineBased(source, keepSymbols)
	}

	origChars := len(source)
	compactChars := len(compacted)

	origTokens := EstimateTokens(source)
	compactTokens := EstimateTokens(compacted)

	var ratio float64
	if origChars > 0 {
		ratio = float64(origChars-compactChars) / float64(origChars)
		if ratio < 0 {
			ratio = 0
		}
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

func compactGoSourceLineBased(source string, keepSymbols []string) string {
	keepMap := make(map[string]bool, len(keepSymbols))
	for _, sym := range keepSymbols {
		keepMap[sym] = true
	}

	scanner := bufio.NewScanner(strings.NewReader(source))
	var sb strings.Builder
	inFunctionBody := false
	braceDepth := 0
	skipCurrentFunction := false

	for scanner.Scan() {
		rawLine := scanner.Text()
		line := strings.TrimSpace(rawLine)

		if strings.HasPrefix(line, "func ") {
			skipCurrentFunction, inFunctionBody, braceDepth = handleFuncStart(&sb, rawLine, line, keepMap)
			if skipCurrentFunction {
				continue
			}
		}

		if inFunctionBody && skipCurrentFunction {
			braceDepth += strings.Count(line, "{") - strings.Count(line, "}")
			if braceDepth <= 0 {
				inFunctionBody = false
				skipCurrentFunction = false
			}
			continue
		}

		sb.WriteString(rawLine + "\n")
	}
	return sb.String()
}

func handleFuncStart(sb *strings.Builder, rawLine, line string, keepMap map[string]bool) (bool, bool, int) {
	funcName := extractFuncName(line)
	if keepMap[funcName] {
		return false, false, 0
	}
	if idx := strings.Index(line, "{"); idx != -1 {
		sb.WriteString(rawLine[:strings.Index(rawLine, "{")+1] + "\n")
		sb.WriteString("\t// ... [implementation pruned for token economy] ...\n}\n")
		depth := strings.Count(line, "{") - strings.Count(line, "}")
		if depth <= 0 {
			return false, false, 0
		}
		return true, true, depth
	}
	sb.WriteString(rawLine + "\n")
	return true, true, 0
}

func extractFuncName(line string) string {
	afterFunc := strings.TrimPrefix(line, "func ")
	afterFunc = stripReceiver(afterFunc)
	if parenIdx := strings.Index(afterFunc, "("); parenIdx != -1 {
		return strings.TrimSpace(afterFunc[:parenIdx])
	}
	fields := strings.Fields(afterFunc)
	if len(fields) > 0 {
		return fields[0]
	}
	return afterFunc
}

func stripReceiver(s string) string {
	if strings.HasPrefix(s, "(") {
		closeParen := strings.Index(s, ")")
		if closeParen != -1 && closeParen+1 < len(s) {
			return strings.TrimSpace(s[closeParen+1:])
		}
	}
	return s
}
