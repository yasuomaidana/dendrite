package synapse

import (
	"bytes"
	"errors"
	"fmt"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"
)

var bareKeyRegex = regexp.MustCompile(`^[A-Za-z0-9_-]+$`)

// formatKey returns bare key if it matches [A-Za-z0-9_-]+, else quoted string.
func formatKey(k string) string {
	if bareKeyRegex.MatchString(k) {
		return k
	}
	return quoteString(k)
}

// quoteString formats a string into a basic double-quoted TOML string with escapes.
func quoteString(s string) string {
	var b strings.Builder
	b.WriteByte('"')
	for _, r := range s {
		switch r {
		case '"':
			b.WriteString(`\"`)
		case '\\':
			b.WriteString(`\\`)
		case '\n':
			b.WriteString(`\n`)
		case '\t':
			b.WriteString(`\t`)
		case '\r':
			b.WriteString(`\r`)
		default:
			if r < 0x20 {
				b.WriteString(fmt.Sprintf(`\u%04x`, r))
			} else {
				b.WriteRune(r)
			}
		}
	}
	b.WriteByte('"')
	return b.String()
}

// Marshal encodes a Synapse manifest into canonical on-disk TOML.
// The encoding is deterministic:
//   - Root properties (id, name, state, created_at, updated_at) emitted first
//   - Timestamps formatted as RFC3339 UTC
//   - Parity command keys sorted lexicographically; bare keys when matching [A-Za-z0-9_-]+ else quoted
//   - Empty sections omitted
func Marshal(s *Synapse) ([]byte, error) {
	if s == nil {
		return nil, errors.New("synapse is nil")
	}

	var buf bytes.Buffer

	// Root properties
	fmt.Fprintf(&buf, "id = %s\n", quoteString(s.ID))
	fmt.Fprintf(&buf, "name = %s\n", quoteString(s.Name))
	fmt.Fprintf(&buf, "state = %s\n", quoteString(string(s.State)))
	if !s.CreatedAt.IsZero() {
		fmt.Fprintf(&buf, "created_at = %s\n", quoteString(s.CreatedAt.UTC().Format(time.RFC3339)))
	}
	if !s.UpdatedAt.IsZero() {
		fmt.Fprintf(&buf, "updated_at = %s\n", quoteString(s.UpdatedAt.UTC().Format(time.RFC3339)))
	}

	// [sync]
	if s.Sync.Strategy != "" {
		buf.WriteString("\n[sync]\n")
		fmt.Fprintf(&buf, "strategy = %s\n", quoteString(s.Sync.Strategy))
	}

	// [parity.commands]
	if len(s.Parity.Commands) > 0 {
		buf.WriteString("\n[parity.commands]\n")
		keys := make([]string, 0, len(s.Parity.Commands))
		for k := range s.Parity.Commands {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		for _, k := range keys {
			fmt.Fprintf(&buf, "%s = %s\n", formatKey(k), quoteString(s.Parity.Commands[k]))
		}
	}

	// [[members]]
	for _, m := range s.Members {
		buf.WriteString("\n[[members]]\n")
		fmt.Fprintf(&buf, "project_id = %s\n", quoteString(m.ProjectID))
		fmt.Fprintf(&buf, "alias = %s\n", quoteString(m.Alias))
		fmt.Fprintf(&buf, "role = %s\n", quoteString(string(m.Role)))
		fmt.Fprintf(&buf, "path_hint = %s\n", quoteString(m.PathHint))
	}

	// [[contracts]] and [[contracts.consumers]]
	for _, c := range s.Contracts {
		buf.WriteString("\n[[contracts]]\n")
		fmt.Fprintf(&buf, "kind = %s\n", quoteString(c.Kind))
		fmt.Fprintf(&buf, "producer = %s\n", quoteString(c.Producer))
		fmt.Fprintf(&buf, "path = %s\n", quoteString(c.Path))

		for _, cons := range c.Consumers {
			buf.WriteString("\n[[contracts.consumers]]\n")
			fmt.Fprintf(&buf, "alias = %s\n", quoteString(cons.Alias))
			fmt.Fprintf(&buf, "path = %s\n", quoteString(cons.Path))
		}
	}

	return buf.Bytes(), nil
}

type tableContext int

const (
	contextRoot tableContext = iota
	contextSync
	contextParityCommands
	contextMembers
	contextContracts
	contextContractsConsumers
)

func stripComment(line string) (string, error) {
	inQuote := false
	runes := []rune(line)
	for i := 0; i < len(runes); i++ {
		r := runes[i]
		if inQuote {
			if r == '\\' {
				i++ // skip next escaped character
				continue
			}
			if r == '"' {
				inQuote = false
			}
			continue
		}
		if r == '"' {
			inQuote = true
			continue
		}
		if r == '#' {
			return string(runes[:i]), nil
		}
	}
	if inQuote {
		return "", errors.New("unclosed string literal")
	}
	return line, nil
}

func parseQuotedString(s string) (str string, rest string, err error) {
	runes := []rune(s)
	if len(runes) == 0 || runes[0] != '"' {
		return "", "", errors.New("expected string starting with '\"'")
	}
	var b strings.Builder
	i := 1
	for i < len(runes) {
		if runes[i] == '"' {
			return b.String(), string(runes[i+1:]), nil
		}
		if runes[i] == '\\' {
			i++
			if i >= len(runes) {
				return "", "", errors.New("unterminated escape sequence")
			}
			switch runes[i] {
			case '"':
				b.WriteRune('"')
			case '\\':
				b.WriteRune('\\')
			case 'n':
				b.WriteRune('\n')
			case 't':
				b.WriteRune('\t')
			case 'r':
				b.WriteRune('\r')
			case 'u':
				if i+4 >= len(runes) {
					return "", "", errors.New("short \\u escape sequence")
				}
				hexStr := string(runes[i+1 : i+5])
				val, err := strconv.ParseUint(hexStr, 16, 32)
				if err != nil {
					return "", "", fmt.Errorf("invalid \\u escape: %s", hexStr)
				}
				b.WriteRune(rune(val))
				i += 4
			case 'U':
				if i+8 >= len(runes) {
					return "", "", errors.New("short \\U escape sequence")
				}
				hexStr := string(runes[i+1 : i+9])
				val, err := strconv.ParseUint(hexStr, 16, 32)
				if err != nil {
					return "", "", fmt.Errorf("invalid \\U escape: %s", hexStr)
				}
				b.WriteRune(rune(val))
				i += 8
			default:
				return "", "", fmt.Errorf("unknown escape sequence \\%c", runes[i])
			}
			i++
			continue
		}
		b.WriteRune(runes[i])
		i++
	}
	return "", "", errors.New("unclosed string literal")
}

func parseKeyAndValue(line string) (key string, rawVal string, err error) {
	trimmed := strings.TrimSpace(line)
	if strings.HasPrefix(trimmed, "\"") {
		k, rest, err := parseQuotedString(trimmed)
		if err != nil {
			return "", "", err
		}
		rest = strings.TrimSpace(rest)
		if !strings.HasPrefix(rest, "=") {
			return "", "", errors.New("missing '=' after key")
		}
		return k, strings.TrimSpace(rest[1:]), nil
	}

	eqIdx := strings.Index(trimmed, "=")
	if eqIdx < 0 {
		return "", "", errors.New("missing '=' in key-value pair")
	}
	k := strings.TrimSpace(trimmed[:eqIdx])
	if !bareKeyRegex.MatchString(k) {
		return "", "", fmt.Errorf("invalid bare key %q", k)
	}
	return k, strings.TrimSpace(trimmed[eqIdx+1:]), nil
}

func parseValue(rawVal string, key string) (string, error) {
	if rawVal == "" {
		return "", errors.New("empty value")
	}
	if strings.HasPrefix(rawVal, "\"") {
		v, rest, err := parseQuotedString(rawVal)
		if err != nil {
			return "", err
		}
		if strings.TrimSpace(rest) != "" {
			return "", fmt.Errorf("unexpected characters after string: %s", strings.TrimSpace(rest))
		}
		return v, nil
	}
	// For timestamps, allow bare RFC3339 timestamps if valid
	if key == "created_at" || key == "updated_at" {
		if _, err := time.Parse(time.RFC3339, rawVal); err == nil {
			return rawVal, nil
		}
	}
	return "", fmt.Errorf("expected double-quoted string value, got %s", rawVal)
}

// Unmarshal parses a Synapse manifest from its on-disk TOML representation.
// It supports:
//   - '#' comments outside strings
//   - Blank lines and leading/trailing whitespace
//   - Double-quoted strings with \" \\ \n \t \r \uXXXX \UXXXXXXXX escapes
//   - Tables [sync] and [parity.commands]
//   - Array of tables [[members]], [[contracts]], and [[contracts.consumers]] (attaching to the latest contract)
//
// Any unknown keys, unknown tables, or malformed syntax produce an error containing the 1-based line number.
func Unmarshal(data []byte) (*Synapse, error) {
	s := &Synapse{}
	lines := strings.Split(string(data), "\n")

	ctx := contextRoot
	seenRootKeys := make(map[string]bool)
	seenSyncKeys := make(map[string]bool)
	seenParityKeys := make(map[string]bool)
	seenMemberKeys := make(map[string]bool)
	seenContractKeys := make(map[string]bool)
	seenConsumerKeys := make(map[string]bool)
	seenSingleTables := make(map[string]bool)

	for idx, rawLine := range lines {
		lineNum := idx + 1
		line := strings.TrimRight(rawLine, "\r")

		stripped, err := stripComment(line)
		if err != nil {
			return nil, fmt.Errorf("line %d: %w", lineNum, err)
		}
		trimmed := strings.TrimSpace(stripped)
		if trimmed == "" {
			continue
		}

		// Array of tables [[...]]
		if strings.HasPrefix(trimmed, "[[") {
			if !strings.HasSuffix(trimmed, "]]") {
				return nil, fmt.Errorf("line %d: malformed array of tables header: %s", lineNum, trimmed)
			}
			tableName := strings.TrimSpace(trimmed[2 : len(trimmed)-2])
			switch tableName {
			case "members":
				ctx = contextMembers
				s.Members = append(s.Members, Member{})
				seenMemberKeys = make(map[string]bool)
			case "contracts":
				ctx = contextContracts
				s.Contracts = append(s.Contracts, Contract{})
				seenContractKeys = make(map[string]bool)
			case "contracts.consumers":
				if len(s.Contracts) == 0 {
					return nil, fmt.Errorf("line %d: [[contracts.consumers]] declared before any [[contracts]]", lineNum)
				}
				ctx = contextContractsConsumers
				latestContract := &s.Contracts[len(s.Contracts)-1]
				latestContract.Consumers = append(latestContract.Consumers, Consumer{})
				seenConsumerKeys = make(map[string]bool)
			default:
				return nil, fmt.Errorf("line %d: unknown array of tables %q", lineNum, tableName)
			}
			continue
		}

		// Single table [...]
		if strings.HasPrefix(trimmed, "[") {
			if !strings.HasSuffix(trimmed, "]") {
				return nil, fmt.Errorf("line %d: malformed table header: %s", lineNum, trimmed)
			}
			tableName := strings.TrimSpace(trimmed[1 : len(trimmed)-1])
			if seenSingleTables[tableName] {
				return nil, fmt.Errorf("line %d: duplicate table %q", lineNum, tableName)
			}
			seenSingleTables[tableName] = true

			switch tableName {
			case "sync":
				ctx = contextSync
				seenSyncKeys = make(map[string]bool)
			case "parity.commands":
				ctx = contextParityCommands
				if s.Parity.Commands == nil {
					s.Parity.Commands = make(map[string]string)
				}
				seenParityKeys = make(map[string]bool)
			default:
				return nil, fmt.Errorf("line %d: unknown table %q", lineNum, tableName)
			}
			continue
		}

		// Key-value assignment
		key, rawVal, err := parseKeyAndValue(trimmed)
		if err != nil {
			return nil, fmt.Errorf("line %d: %w", lineNum, err)
		}
		val, err := parseValue(rawVal, key)
		if err != nil {
			return nil, fmt.Errorf("line %d: %w", lineNum, err)
		}

		switch ctx {
		case contextRoot:
			if seenRootKeys[key] {
				return nil, fmt.Errorf("line %d: duplicate key %q in root table", lineNum, key)
			}
			seenRootKeys[key] = true

			switch key {
			case "id":
				s.ID = val
			case "name":
				s.Name = val
			case "state":
				s.State = State(val)
			case "created_at":
				t, err := time.Parse(time.RFC3339, val)
				if err != nil {
					return nil, fmt.Errorf("line %d: invalid created_at timestamp %q: %w", lineNum, val, err)
				}
				s.CreatedAt = t
			case "updated_at":
				t, err := time.Parse(time.RFC3339, val)
				if err != nil {
					return nil, fmt.Errorf("line %d: invalid updated_at timestamp %q: %w", lineNum, val, err)
				}
				s.UpdatedAt = t
			default:
				return nil, fmt.Errorf("line %d: unknown key %q in root table", lineNum, key)
			}

		case contextSync:
			if seenSyncKeys[key] {
				return nil, fmt.Errorf("line %d: duplicate key %q in [sync]", lineNum, key)
			}
			seenSyncKeys[key] = true

			switch key {
			case "strategy":
				s.Sync.Strategy = val
			default:
				return nil, fmt.Errorf("line %d: unknown key %q in [sync]", lineNum, key)
			}

		case contextParityCommands:
			if seenParityKeys[key] {
				return nil, fmt.Errorf("line %d: duplicate key %q in [parity.commands]", lineNum, key)
			}
			seenParityKeys[key] = true
			s.Parity.Commands[key] = val

		case contextMembers:
			if seenMemberKeys[key] {
				return nil, fmt.Errorf("line %d: duplicate key %q in [[members]]", lineNum, key)
			}
			seenMemberKeys[key] = true
			m := &s.Members[len(s.Members)-1]
			switch key {
			case "project_id":
				m.ProjectID = val
			case "alias":
				m.Alias = val
			case "role":
				m.Role = Role(val)
			case "path_hint":
				m.PathHint = val
			default:
				return nil, fmt.Errorf("line %d: unknown key %q in [[members]]", lineNum, key)
			}

		case contextContracts:
			if seenContractKeys[key] {
				return nil, fmt.Errorf("line %d: duplicate key %q in [[contracts]]", lineNum, key)
			}
			seenContractKeys[key] = true
			c := &s.Contracts[len(s.Contracts)-1]
			switch key {
			case "kind":
				c.Kind = val
			case "producer":
				c.Producer = val
			case "path":
				c.Path = val
			default:
				return nil, fmt.Errorf("line %d: unknown key %q in [[contracts]]", lineNum, key)
			}

		case contextContractsConsumers:
			if seenConsumerKeys[key] {
				return nil, fmt.Errorf("line %d: duplicate key %q in [[contracts.consumers]]", lineNum, key)
			}
			seenConsumerKeys[key] = true
			latestContract := &s.Contracts[len(s.Contracts)-1]
			latestConsumer := &latestContract.Consumers[len(latestContract.Consumers)-1]
			switch key {
			case "alias":
				latestConsumer.Alias = val
			case "path":
				latestConsumer.Path = val
			default:
				return nil, fmt.Errorf("line %d: unknown key %q in [[contracts.consumers]]", lineNum, key)
			}
		}
	}

	return s, nil
}
