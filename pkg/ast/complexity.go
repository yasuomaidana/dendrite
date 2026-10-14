package ast

import (
	"bufio"
	"strings"
)

// AnalyzeSourceComplexity computes cyclomatic and cognitive complexity for source code snippets.
func AnalyzeSourceComplexity(source string) (cyclomatic int, cognitive int) {
	cyclomatic = 1 // Standard McCabe baseline
	cognitive = 0
	nestingLevel := 0

	scanner := bufio.NewScanner(strings.NewReader(source))

	for scanner.Scan() {
		rawLine := scanner.Text()
		line := strings.TrimSpace(rawLine)

		// Ignore blank lines and comments
		if line == "" || strings.HasPrefix(line, "//") || strings.HasPrefix(line, "#") || strings.HasPrefix(line, "/*") {
			continue
		}

		// Adjust nesting level based on braces
		openBraces := strings.Count(line, "{")
		closeBraces := strings.Count(line, "}")

		// Check for branching constructs
		hasBranch := false
		if strings.HasPrefix(line, "if ") || strings.Contains(line, " if ") ||
			strings.HasPrefix(line, "for ") || strings.Contains(line, " for ") ||
			strings.HasPrefix(line, "while ") || strings.Contains(line, " while ") ||
			strings.HasPrefix(line, "case ") ||
			strings.HasPrefix(line, "catch ") || strings.Contains(line, " catch ") {
			hasBranch = true
			cyclomatic++
			cognitive += 1 + nestingLevel
		}

		// Logical operators add to cyclomatic and cognitive complexity
		logicalOps := strings.Count(line, "&&") + strings.Count(line, "||")
		if logicalOps > 0 {
			cyclomatic += logicalOps
			cognitive += logicalOps
		}

		// Update nesting level
		nestingLevel += openBraces - closeBraces
		if nestingLevel < 0 {
			nestingLevel = 0
		}

		_ = hasBranch
	}

	return cyclomatic, cognitive
}
