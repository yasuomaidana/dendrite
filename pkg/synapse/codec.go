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
//   - Empty sections and empty optional member keys (source, remote, ref, module, locked_*) omitted
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
	if s.Channel != "" {
		fmt.Fprintf(&buf, "channel = %s\n", quoteString(string(s.Channel)))
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
		for _, kv := range [][2]string{
			{"source", string(m.Source)},
			{"remote", m.Remote},
			{"ref", m.Ref},
			{"module", m.Module},
			{"locked_sha", m.LockedSHA},
		} {
			if kv[1] != "" {
				fmt.Fprintf(&buf, "%s = %s\n", kv[0], quoteString(kv[1]))
			}
		}
		if !m.LockedAt.IsZero() {
			fmt.Fprintf(&buf, "locked_at = %s\n", quoteString(m.LockedAt.UTC().Format(time.RFC3339)))
		}
		if m.PlaneProject != "" {
			fmt.Fprintf(&buf, "plane_project = %s\n", quoteString(m.PlaneProject))
		}
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
	if key == "created_at" || key == "updated_at" || key == "locked_at" {
		if _, err := time.Parse(time.RFC3339, rawVal); err == nil {
			return rawVal, nil
		}
	}
	return "", fmt.Errorf("expected double-quoted string value, got %s", rawVal)
}
