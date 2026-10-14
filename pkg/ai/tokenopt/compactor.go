package tokenopt

import (
	"bufio"
	"strings"
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
func CompactGoSource(source string, keepSymbols []string) (*CompactedContext, error) {
	trimmedSource := strings.TrimSpace(source)
	if trimmedSource == "" {
		return nil, ErrEmptySource
	}

	keepMap := make(map[string]bool)
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

		// Check if entering a function or method definition
		if strings.HasPrefix(line, "func ") {
			// Extract function/method name
			funcName := extractFuncName(line)
			if !keepMap[funcName] {
				skipCurrentFunction = true
				inFunctionBody = true
				braceDepth = 0

				// Retain signature line up to opening brace
				if idx := strings.Index(line, "{"); idx != -1 {
					sb.WriteString(rawLine[:strings.Index(rawLine, "{")+1] + "\n")
					sb.WriteString("\t// ... [implementation pruned for token economy] ...\n}\n")
					braceDepth += strings.Count(line, "{") - strings.Count(line, "}")
					if braceDepth <= 0 {
						inFunctionBody = false
						skipCurrentFunction = false
					}
					continue
				} else {
					sb.WriteString(rawLine + "\n")
					continue
				}
			} else {
				skipCurrentFunction = false
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

		// Keep all imports, type definitions, comments, and kept functions
		sb.WriteString(rawLine + "\n")
	}

	compacted := sb.String()
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

func extractFuncName(line string) string {
	afterFunc := strings.TrimPrefix(line, "func ")
	// Handle receiver: func (r *Receiver) MethodName(...)
	if strings.HasPrefix(afterFunc, "(") {
		closeParen := strings.Index(afterFunc, ")")
		if closeParen != -1 && closeParen+1 < len(afterFunc) {
			afterFunc = strings.TrimSpace(afterFunc[closeParen+1:])
		}
	}

	if parenIdx := strings.Index(afterFunc, "("); parenIdx != -1 {
		return strings.TrimSpace(afterFunc[:parenIdx])
	}
	return strings.Fields(afterFunc)[0]
}
