package jetbrains

import (
	"Dendrite/pkg/fsutil"
	"encoding/json"
	"fmt"
	"os"
	"strings"
)

// UpsertMCPServer writes entry under mcpServers[name] in the JSON file at path, preserving every
// other top-level key and every other server entry byte-for-byte semantically (use map[string]json.RawMessage
// at both levels). Creates parent dirs and the file if missing. Empty/whitespace file = empty object.
// Invalid JSON -> error (do NOT overwrite). Atomic write via Dendrite/pkg/fsutil.WriteFileAtomic. Output indented 2 spaces + trailing newline.
// Returns created=true when the file did not exist before.
func UpsertMCPServer(path, name string, entry MCPServerEntry) (created bool, err error) {
	if name == "" {
		return false, fmt.Errorf("server name cannot be empty")
	}

	created = false
	info, statErr := os.Stat(path)
	if statErr != nil {
		if os.IsNotExist(statErr) {
			created = true
		} else {
			return false, statErr
		}
	} else if info.IsDir() {
		return false, fmt.Errorf("%s is a directory", path)
	}

	topLevel := make(map[string]json.RawMessage)
	if !created {
		data, err := os.ReadFile(path)
		if err != nil {
			return false, err
		}
		trimmed := strings.TrimSpace(string(data))
		if trimmed != "" {
			if err := json.Unmarshal([]byte(trimmed), &topLevel); err != nil {
				return false, fmt.Errorf("invalid json in %s: %w", path, err)
			}
		}
	}
	if topLevel == nil {
		topLevel = make(map[string]json.RawMessage)
	}

	servers := make(map[string]json.RawMessage)
	if rawServers, exists := topLevel["mcpServers"]; exists {
		trimmedServers := strings.TrimSpace(string(rawServers))
		if trimmedServers != "" && trimmedServers != "null" {
			if err := json.Unmarshal(rawServers, &servers); err != nil {
				return false, fmt.Errorf("invalid mcpServers in %s: %w", path, err)
			}
		}
	}
	if servers == nil {
		servers = make(map[string]json.RawMessage)
	}

	entryBytes, err := json.Marshal(entry)
	if err != nil {
		return false, fmt.Errorf("failed to marshal server entry: %w", err)
	}
	servers[name] = json.RawMessage(entryBytes)

	serversBytes, err := json.Marshal(servers)
	if err != nil {
		return false, fmt.Errorf("failed to marshal mcpServers: %w", err)
	}
	topLevel["mcpServers"] = json.RawMessage(serversBytes)

	out, err := json.MarshalIndent(topLevel, "", "  ")
	if err != nil {
		return false, fmt.Errorf("failed to serialize json: %w", err)
	}
	out = append(out, '\n')

	if err := fsutil.WriteFileAtomic(path, out); err != nil {
		return false, err
	}

	return created, nil
}

// RemoveMCPServer deletes mcpServers[name]; missing file or name is not an error; returns whether something was removed.
func RemoveMCPServer(path, name string) (bool, error) {
	if name == "" {
		return false, nil
	}

	info, statErr := os.Stat(path)
	if statErr != nil {
		if os.IsNotExist(statErr) {
			return false, nil
		}
		return false, statErr
	}
	if info.IsDir() {
		return false, fmt.Errorf("%s is a directory", path)
	}

	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return false, nil
		}
		return false, err
	}

	trimmed := strings.TrimSpace(string(data))
	if trimmed == "" {
		return false, nil
	}

	topLevel := make(map[string]json.RawMessage)
	if err := json.Unmarshal([]byte(trimmed), &topLevel); err != nil {
		return false, fmt.Errorf("invalid json in %s: %w", path, err)
	}

	rawServers, exists := topLevel["mcpServers"]
	if !exists {
		return false, nil
	}

	trimmedServers := strings.TrimSpace(string(rawServers))
	if trimmedServers == "" || trimmedServers == "null" {
		return false, nil
	}

	servers := make(map[string]json.RawMessage)
	if err := json.Unmarshal(rawServers, &servers); err != nil {
		return false, fmt.Errorf("invalid mcpServers in %s: %w", path, err)
	}

	if _, ok := servers[name]; !ok {
		return false, nil
	}

	delete(servers, name)
	serversBytes, err := json.Marshal(servers)
	if err != nil {
		return false, fmt.Errorf("failed to marshal mcpServers: %w", err)
	}
	topLevel["mcpServers"] = json.RawMessage(serversBytes)

	out, err := json.MarshalIndent(topLevel, "", "  ")
	if err != nil {
		return false, fmt.Errorf("failed to serialize json: %w", err)
	}
	out = append(out, '\n')

	if err := fsutil.WriteFileAtomic(path, out); err != nil {
		return false, err
	}

	return true, nil
}
