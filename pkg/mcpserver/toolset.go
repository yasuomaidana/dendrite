package mcpserver

import (
	"fmt"
	"os"
	"strings"
)

// Profile selects how much of the control-plane tool surface an agent sees. Every tool listed
// costs context in every request, so day-to-day agent sessions use the core profile.
type Profile string

const (
	// ProfileCore exposes retrieval, the code graph and Plane ticket tools only.
	ProfileCore Profile = "core"
	// ProfileFull exposes every registered tool.
	ProfileFull Profile = "full"
)

// ProfileEnv names the environment variable that selects the profile when no flag is given.
const ProfileEnv = "DENDRITE_MCP_PROFILE"

// The core set was chosen from `dendrite inter probe` (2026-10-28): every read-only RAG, graph and
// Plane tool answered. The slow ones (Plane smart-select 28s, hitl_sync 50s, replicate 8s on a
// 360-issue board) are tracked in DEN-338 rather than dropped, because Plane is essential.
// Writers (issue_create, relation_link, push_status, doc_sync, replicate, ticket_set) are not
// probed; they are covered by their own tests. Everything outside core is still available with
// the full profile.
//
// coreToolPrefixes and coreToolNames are the control-plane tools in the core profile.
// Synapse tools are served by the scoped synapse server (NewSynapseServer), which is always
// confined to the relationship and is not filtered.
var (
	coreToolPrefixes = []string{"dendrite_rag_", "dendrite_graph_", "dendrite_plane_"}
	coreToolNames    = map[string]bool{"dendrite_ticket_smart_select": true}
)

// ParseProfile validates a profile name; an empty name means defaultProfile.
func ParseProfile(name string, defaultProfile Profile) (Profile, error) {
	switch p := Profile(strings.ToLower(strings.TrimSpace(name))); p {
	case "":
		return defaultProfile, nil
	case ProfileCore, ProfileFull:
		return p, nil
	default:
		return "", fmt.Errorf("invalid MCP profile %q: use core or full", name)
	}
}

// ProfileFromEnv reads ProfileEnv, falling back to defaultProfile when unset.
func ProfileFromEnv(defaultProfile Profile) (Profile, error) {
	return ParseProfile(os.Getenv(ProfileEnv), defaultProfile)
}

// InProfile reports whether the named tool is exposed by the profile.
func (p Profile) InProfile(tool string) bool {
	if p != ProfileCore {
		return true
	}
	if coreToolNames[tool] {
		return true
	}
	for _, prefix := range coreToolPrefixes {
		if strings.HasPrefix(tool, prefix) {
			return true
		}
	}
	return false
}

// ApplyProfile returns a server holding only the tools of src that the profile exposes. The
// full profile returns src itself.
func ApplyProfile(src *Server, p Profile) *Server {
	if p != ProfileCore {
		return src
	}
	info := src.Info()
	dst := NewServer(ServerConfig{Name: info.Name, Version: info.Version})
	for _, t := range src.ListTools() {
		if !p.InProfile(t.Name) {
			continue
		}
		if h, ok := src.Handler(t.Name); ok {
			dst.RegisterTool(t, h)
		}
	}
	return dst
}
