package synapse

import (
	"fmt"
	"strings"
	"time"
)

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
	p := &parser{s: &Synapse{}, seen: map[string]bool{}, singleTables: map[string]bool{}}
	for idx, raw := range strings.Split(string(data), "\n") {
		if err := p.line(raw); err != nil {
			return nil, fmt.Errorf("line %d: %w", idx+1, err)
		}
	}
	return p.s, nil
}

// parser reads a manifest one line at a time, remembering the table it is in and the keys that table has seen.
type parser struct {
	s            *Synapse
	ctx          tableContext
	seen         map[string]bool
	singleTables map[string]bool
}

// fieldSetters maps the keys of one table to the code that stores their value in a T.
type fieldSetters[T any] map[string]func(*T, string) error

func (f fieldSetters[T]) apply(t *T, table, key, val string) error {
	set, ok := f[key]
	if !ok {
		return fmt.Errorf("unknown key %q in %s", key, table)
	}
	return set(t, val)
}

func text[T any](field func(*T) *string) func(*T, string) error {
	return func(t *T, v string) error { *field(t) = v; return nil }
}

func stamp[T any](name string, field func(*T) *time.Time) func(*T, string) error {
	return func(t *T, v string) error {
		parsed, err := time.Parse(time.RFC3339, v)
		if err != nil {
			return fmt.Errorf("invalid %s timestamp %q: %w", name, v, err)
		}
		*field(t) = parsed
		return nil
	}
}

var rootFields = fieldSetters[Synapse]{
	"id":         text(func(s *Synapse) *string { return &s.ID }),
	"name":       text(func(s *Synapse) *string { return &s.Name }),
	"state":      func(s *Synapse, v string) error { s.State = State(v); return nil },
	"created_at": stamp("created_at", func(s *Synapse) *time.Time { return &s.CreatedAt }),
	"updated_at": stamp("updated_at", func(s *Synapse) *time.Time { return &s.UpdatedAt }),
	"channel":    func(s *Synapse, v string) error { s.Channel = Channel(v); return nil },
}

var syncFields = fieldSetters[Sync]{
	"strategy": text(func(s *Sync) *string { return &s.Strategy }),
}

var memberFields = fieldSetters[Member]{
	"project_id":    text(func(m *Member) *string { return &m.ProjectID }),
	"alias":         text(func(m *Member) *string { return &m.Alias }),
	"role":          func(m *Member, v string) error { m.Role = Role(v); return nil },
	"path_hint":     text(func(m *Member) *string { return &m.PathHint }),
	"source":        func(m *Member, v string) error { m.Source = Source(v); return nil },
	"remote":        text(func(m *Member) *string { return &m.Remote }),
	"ref":           text(func(m *Member) *string { return &m.Ref }),
	"module":        text(func(m *Member) *string { return &m.Module }),
	"locked_sha":    text(func(m *Member) *string { return &m.LockedSHA }),
	"locked_at":     stamp("locked_at", func(m *Member) *time.Time { return &m.LockedAt }),
	"plane_project": text(func(m *Member) *string { return &m.PlaneProject }),
	"branch":        text(func(m *Member) *string { return &m.Branch }),
	"push_remote":   text(func(m *Member) *string { return &m.PushRemote }),
	"target_remote": text(func(m *Member) *string { return &m.TargetRemote }),
}

var contractFields = fieldSetters[Contract]{
	"kind":     text(func(c *Contract) *string { return &c.Kind }),
	"producer": text(func(c *Contract) *string { return &c.Producer }),
	"path":     text(func(c *Contract) *string { return &c.Path }),
}

var consumerFields = fieldSetters[Consumer]{
	"alias": text(func(c *Consumer) *string { return &c.Alias }),
	"path":  text(func(c *Consumer) *string { return &c.Path }),
}

// tableLabels name each table in the errors about its keys.
var tableLabels = map[tableContext]string{
	contextRoot:               "root table",
	contextSync:               "[sync]",
	contextParityCommands:     "[parity.commands]",
	contextMembers:            "[[members]]",
	contextContracts:          "[[contracts]]",
	contextContractsConsumers: "[[contracts.consumers]]",
}

func (p *parser) line(raw string) error {
	stripped, err := stripComment(strings.TrimRight(raw, "\r"))
	if err != nil {
		return err
	}
	trimmed := strings.TrimSpace(stripped)
	switch {
	case trimmed == "":
		return nil
	case strings.HasPrefix(trimmed, "[["):
		return p.arrayHeader(trimmed)
	case strings.HasPrefix(trimmed, "["):
		return p.tableHeader(trimmed)
	}
	return p.assignment(trimmed)
}

func (p *parser) arrayHeader(trimmed string) error {
	if !strings.HasSuffix(trimmed, "]]") {
		return fmt.Errorf("malformed array of tables header: %s", trimmed)
	}
	name := strings.TrimSpace(trimmed[2 : len(trimmed)-2])
	s := p.s
	switch name {
	case "members":
		p.ctx = contextMembers
		s.Members = append(s.Members, Member{})
	case "contracts":
		p.ctx = contextContracts
		s.Contracts = append(s.Contracts, Contract{})
	case "contracts.consumers":
		if len(s.Contracts) == 0 {
			return fmt.Errorf("[[contracts.consumers]] declared before any [[contracts]]")
		}
		p.ctx = contextContractsConsumers
		latest := &s.Contracts[len(s.Contracts)-1]
		latest.Consumers = append(latest.Consumers, Consumer{})
	default:
		return fmt.Errorf("unknown array of tables %q", name)
	}
	p.seen = map[string]bool{}
	return nil
}

func (p *parser) tableHeader(trimmed string) error {
	if !strings.HasSuffix(trimmed, "]") {
		return fmt.Errorf("malformed table header: %s", trimmed)
	}
	name := strings.TrimSpace(trimmed[1 : len(trimmed)-1])
	if p.singleTables[name] {
		return fmt.Errorf("duplicate table %q", name)
	}
	p.singleTables[name] = true
	switch name {
	case "sync":
		p.ctx = contextSync
	case "parity.commands":
		p.ctx = contextParityCommands
		if p.s.Parity.Commands == nil {
			p.s.Parity.Commands = make(map[string]string)
		}
	default:
		return fmt.Errorf("unknown table %q", name)
	}
	p.seen = map[string]bool{}
	return nil
}

func (p *parser) assignment(trimmed string) error {
	key, rawVal, err := parseKeyAndValue(trimmed)
	if err != nil {
		return err
	}
	val, err := parseValue(rawVal, key)
	if err != nil {
		return err
	}
	label := tableLabels[p.ctx]
	if p.seen[key] {
		return fmt.Errorf("duplicate key %q in %s", key, label)
	}
	p.seen[key] = true
	s := p.s
	switch p.ctx {
	case contextRoot:
		return rootFields.apply(s, label, key, val)
	case contextSync:
		return syncFields.apply(&s.Sync, label, key, val)
	case contextParityCommands:
		s.Parity.Commands[key] = val
		return nil
	case contextMembers:
		return memberFields.apply(&s.Members[len(s.Members)-1], label, key, val)
	case contextContracts:
		return contractFields.apply(&s.Contracts[len(s.Contracts)-1], label, key, val)
	}
	latest := &s.Contracts[len(s.Contracts)-1]
	return consumerFields.apply(&latest.Consumers[len(latest.Consumers)-1], label, key, val)
}
