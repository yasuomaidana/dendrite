package inference

import (
	"bufio"
	"bytes"
	"errors"
	"fmt"
	"strconv"
	"strings"
)

// ParseTOMLCatalog parses a TOML capability catalog without external dependencies.
func ParseTOMLCatalog(data []byte) (*Catalog, error) {
	scanner := bufio.NewScanner(bytes.NewReader(data))
	var models []ModelCapability
	var current *ModelCapability
	nonCommentLines := 0

	for scanner.Scan() {
		line := stripComment(scanner.Text())
		if line == "" {
			continue
		}
		nonCommentLines++
		if isTableHeader(line) {
			if current != nil && current.ID != "" {
				models = append(models, *current)
			}
			current = &ModelCapability{ContextWindow: 32768}
			continue
		}
		if current == nil {
			current = &ModelCapability{ContextWindow: 32768}
		}
		parseTOMLLine(line, current)
	}

	if current != nil && current.ID != "" {
		models = append(models, *current)
	}

	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("failed to scan TOML: %w", err)
	}
	if nonCommentLines > 0 && len(models) == 0 {
		return nil, errors.New("no valid [[model]] tables found in TOML data")
	}
	return &Catalog{Models: models}, nil
}

func isTableHeader(line string) bool {
	return line == "[[model]]" || line == "[[models]]"
}

func parseTOMLLine(line string, curr *ModelCapability) {
	pairs := strings.Split(line, ";")
	for _, pair := range pairs {
		parseKeyValue(pair, curr)
	}
}

func parseKeyValue(pair string, curr *ModelCapability) {
	parts := strings.SplitN(pair, "=", 2)
	if len(parts) != 2 {
		return
	}
	key := strings.ToLower(strings.TrimSpace(parts[0]))
	val := strings.Trim(strings.TrimSpace(parts[1]), "\"'")
	assignField(key, val, curr)
}

func assignField(key, val string, curr *ModelCapability) {
	switch key {
	case "id":
		curr.ID = val
	case "engine":
		curr.Engine = EngineType(val)
	case "reasoning_tier":
		if n, err := strconv.Atoi(val); err == nil {
			curr.ReasoningTier = n
		}
	case "coding_benchmark":
		if f, err := strconv.ParseFloat(val, 64); err == nil {
			curr.CodingBenchmark = f
		}
	case "context_window":
		if n, err := strconv.Atoi(val); err == nil {
			curr.ContextWindow = n
		}
	case "tokens_per_sec":
		if f, err := strconv.ParseFloat(val, 64); err == nil {
			curr.TokensPerSec = f
		}
	case "min_complexity":
		if n, err := strconv.Atoi(val); err == nil {
			curr.MinComplexity = n
		}
	case "max_complexity":
		if n, err := strconv.Atoi(val); err == nil {
			curr.MaxComplexity = n
		}
	}
}

func stripComment(line string) string {
	inQuote := false
	var quoteChar rune
	for i, r := range line {
		if inQuote {
			if r == quoteChar {
				inQuote = false
			}
		} else {
			if r == '"' || r == '\'' {
				inQuote = true
				quoteChar = r
			} else if r == '#' {
				return strings.TrimSpace(line[:i])
			}
		}
	}
	return strings.TrimSpace(line)
}
