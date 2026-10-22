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
			case "channel":
				s.Channel = Channel(val)
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
			case "source":
				m.Source = Source(val)
			case "remote":
				m.Remote = val
			case "ref":
				m.Ref = val
			case "module":
				m.Module = val
			case "locked_sha":
				m.LockedSHA = val
			case "locked_at":
				t, err := time.Parse(time.RFC3339, val)
				if err != nil {
					return nil, fmt.Errorf("line %d: invalid locked_at timestamp %q: %w", lineNum, val, err)
				}
				m.LockedAt = t
			case "plane_project":
				m.PlaneProject = val
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
