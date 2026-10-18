package synapseide

import (
	"Dendrite/pkg/fsutil"
	"Dendrite/pkg/synapse"
	"bytes"
	"encoding/xml"
	"errors"
	"fmt"
	"github.com/yasuomaidana/dendrite-mcp-hub/pkg/jetbrains"
	"path/filepath"
	"sort"
	"strings"
)

// SynapseServerName returns "dendrite-synapse-<id>".
func SynapseServerName(id string) string {
	return "dendrite-synapse-" + id
}

// SynapseScopeName returns "Synapse_" + id with '-' replaced by '_'.
func SynapseScopeName(id string) string {
	return "Synapse_" + strings.ReplaceAll(id, "-", "_")
}

// SynapseScopeXML renders a JetBrains scope whose pattern joins, with "||", `file:<rel>` for every contract
// file that lives in member `alias` (contract.Path when contract.Producer==alias, consumer.Path for consumers with that alias).
// Output: <component name="DependencyValidationManager">\n  <scope name="NAME" pattern="PATTERN" />\n</component>\n  (XML-escape the pattern with encoding/xml EscapeText semantics). Empty pattern -> "file:*" is NOT allowed: return ("", false).
func SynapseScopeXML(s *synapse.Synapse, alias string) (string, bool) {
	if s == nil || alias == "" {
		return "", false
	}

	var paths []string
	seen := make(map[string]bool)

	for _, c := range s.Contracts {
		if c.Producer == alias && c.Path != "" {
			if !seen[c.Path] {
				seen[c.Path] = true
				paths = append(paths, c.Path)
			}
		}
		for _, cons := range c.Consumers {
			if cons.Alias == alias && cons.Path != "" {
				if !seen[cons.Path] {
					seen[cons.Path] = true
					paths = append(paths, cons.Path)
				}
			}
		}
	}

	if len(paths) == 0 {
		return "", false
	}

	parts := make([]string, len(paths))
	for i, p := range paths {
		parts[i] = "file:" + p
	}
	pattern := strings.Join(parts, "||")

	// xml.EscapeText only fails when the writer does; a bytes.Buffer never does.
	var patBuf, nameBuf bytes.Buffer
	_ = xml.EscapeText(&patBuf, []byte(pattern))
	_ = xml.EscapeText(&nameBuf, []byte(SynapseScopeName(s.ID)))

	out := fmt.Sprintf("<component name=\"DependencyValidationManager\">\n  <scope name=\"%s\" pattern=\"%s\" />\n</component>\n", nameBuf.String(), patBuf.String())
	return out, true
}

// SyncSynapseIDE, for every member that resolves (synapse.Resolve), writes
//
//	<member>/.idea/scopes/<SynapseScopeName>.xml (only when SynapseScopeXML ok; never touches other scope files)
//	and upserts mcpServers[SynapseServerName] = {Command: binary, Args: ["mcp","serve","--synapse",id], Env: {"DENDRITE_SYNAPSE": id}}
//	into <member>/.ai/mcp/mcp.json.
//
// Unresolved members are skipped and reported. Returns written file paths (sorted) and skipped aliases.
func SyncSynapseIDE(s *synapse.Synapse, binary string) (written []string, skipped []string, err error) {
	if s == nil {
		return nil, nil, errors.New("synapse is nil")
	}
	if s.ID == "" {
		return nil, nil, errors.New("synapse id is empty")
	}

	written = make([]string, 0)
	skipped = make([]string, 0)

	serverName := SynapseServerName(s.ID)
	scopeFileName := SynapseScopeName(s.ID) + ".xml"

	mcpEntry := jetbrains.MCPServerEntry{
		Command: binary,
		Args:    []string{"mcp", "serve", "--synapse", s.ID},
	}

	for _, m := range s.Members {
		memberDir, err := synapse.Resolve(m)
		if err != nil {
			skipped = append(skipped, m.Alias)
			continue
		}

		if xmlContent, ok := SynapseScopeXML(s, m.Alias); ok {
			scopePath := filepath.Join(memberDir, ".idea", "scopes", scopeFileName)
			if err := fsutil.WriteFileAtomic(scopePath, []byte(xmlContent)); err != nil {
				return nil, nil, fmt.Errorf("failed to write scope xml for member %s: %w", m.Alias, err)
			}
			written = append(written, scopePath)
		}

		mcpPath := filepath.Join(memberDir, ".ai", "mcp", "mcp.json")
		if _, err := jetbrains.UpsertMCPServer(mcpPath, serverName, mcpEntry); err != nil {
			return nil, nil, fmt.Errorf("failed to upsert mcp server for member %s: %w", m.Alias, err)
		}
		written = append(written, mcpPath)
	}

	sort.Strings(written)
	return written, skipped, nil
}
