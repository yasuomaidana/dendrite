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
	writeRoot(&buf, s)
	writeParity(&buf, s.Parity)
	for _, m := range s.Members {
		writeMember(&buf, m)
	}
	for _, c := range s.Contracts {
		writeContract(&buf, c)
	}

	return buf.Bytes(), nil
}

// writeRoot emits the root keys and the [sync] table, leaving out what is empty.
func writeRoot(buf *bytes.Buffer, s *Synapse) {
	fmt.Fprintf(buf, "id = %s\n", quoteString(s.ID))
	fmt.Fprintf(buf, "name = %s\n", quoteString(s.Name))
	fmt.Fprintf(buf, "state = %s\n", quoteString(string(s.State)))
	stamps := [][2]string{{"created_at", ""}, {"updated_at", ""}}
	if !s.CreatedAt.IsZero() {
		stamps[0][1] = s.CreatedAt.UTC().Format(time.RFC3339)
	}
	if !s.UpdatedAt.IsZero() {
		stamps[1][1] = s.UpdatedAt.UTC().Format(time.RFC3339)
	}
	optional := append(stamps, [2]string{"channel", string(s.Channel)})
	for _, kv := range optional {
		if kv[1] != "" {
			fmt.Fprintf(buf, "%s = %s\n", kv[0], quoteString(kv[1]))
		}
	}
	if s.Sync.Strategy != "" {
		buf.WriteString("\n[sync]\n")
		fmt.Fprintf(buf, "strategy = %s\n", quoteString(s.Sync.Strategy))
	}
}

// writeParity emits the [parity.commands] table with its keys sorted.
func writeParity(buf *bytes.Buffer, p Parity) {
	if len(p.Commands) == 0 {
		return
	}
	buf.WriteString("\n[parity.commands]\n")
	keys := make([]string, 0, len(p.Commands))
	for k := range p.Commands {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		fmt.Fprintf(buf, "%s = %s\n", formatKey(k), quoteString(p.Commands[k]))
	}
}

// writeMember emits one [[members]] table, leaving out the optional keys that are empty.
func writeMember(buf *bytes.Buffer, m Member) {
	buf.WriteString("\n[[members]]\n")
	fmt.Fprintf(buf, "project_id = %s\n", quoteString(m.ProjectID))
	fmt.Fprintf(buf, "alias = %s\n", quoteString(m.Alias))
	fmt.Fprintf(buf, "role = %s\n", quoteString(string(m.Role)))
	fmt.Fprintf(buf, "path_hint = %s\n", quoteString(m.PathHint))
	locked := ""
	if !m.LockedAt.IsZero() {
		locked = m.LockedAt.UTC().Format(time.RFC3339)
	}
	for _, kv := range [][2]string{
		{"source", string(m.Source)},
		{"remote", m.Remote},
		{"ref", m.Ref},
		{"module", m.Module},
		{"locked_sha", m.LockedSHA},
		{"branch", m.Branch},
		{"push_remote", m.PushRemote},
		{"target_remote", m.TargetRemote},
		{"locked_at", locked},
		{"plane_project", m.PlaneProject},
	} {
		if kv[1] != "" {
			fmt.Fprintf(buf, "%s = %s\n", kv[0], quoteString(kv[1]))
		}
	}
}

// writeContract emits one [[contracts]] table and its [[contracts.consumers]].
func writeContract(buf *bytes.Buffer, c Contract) {
	buf.WriteString("\n[[contracts]]\n")
	fmt.Fprintf(buf, "kind = %s\n", quoteString(c.Kind))
	fmt.Fprintf(buf, "producer = %s\n", quoteString(c.Producer))
	fmt.Fprintf(buf, "path = %s\n", quoteString(c.Path))
	for _, cons := range c.Consumers {
		buf.WriteString("\n[[contracts.consumers]]\n")
		fmt.Fprintf(buf, "alias = %s\n", quoteString(cons.Alias))
		fmt.Fprintf(buf, "path = %s\n", quoteString(cons.Path))
	}
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
			case 'u', 'U':
				width := 4
				if runes[i] == 'U' {
					width = 8
				}
				if i+width >= len(runes) {
					return "", "", fmt.Errorf("short \\%c escape sequence", runes[i])
				}
				hexStr := string(runes[i+1 : i+1+width])
				val, err := strconv.ParseUint(hexStr, 16, 32)
				if err != nil {
					return "", "", fmt.Errorf("invalid \\%c escape: %s", runes[i], hexStr)
				}
				b.WriteRune(rune(val))
				i += width
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
