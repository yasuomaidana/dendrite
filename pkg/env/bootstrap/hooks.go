package bootstrap

import (
	"bytes"
	"encoding/json"
	"fmt"
	"reflect"
	"strings"
)

const (
	// claudeSettingsPath is the project-level Claude Code settings file, shared with the team.
	claudeSettingsPath = ".claude/settings.json"
	// preToolMatcher selects the tool calls the hook sees: shell commands (rg, grep -r), the built-in
	// Grep tool, and Dendrite's own MCP tools (counted, never blocked).
	preToolMatcher = "Bash|Grep|mcp__dendrite.*"
	// preToolCommand is the hook entry point. It calls whichever dendrite is on PATH (a path inside the
	// project would break the day it moves) and fails open: a hook must never stop an agent from working.
	// With no dendrite, or one that predates `hook`, the call is allowed; only the guard's own message
	// ("dendrite-first:") with exit 2 blocks. The marker matters because an unknown subcommand also
	// exits 2, this project's usage-error code, which Claude Code would read as a block.
	preToolCommand = `command -v dendrite >/dev/null 2>&1 || exit 0; out=$(dendrite hook pre-tool 2>&1); c=$?; ` +
		`case "$out" in *dendrite-first:*) [ "$c" -eq 2 ] && { printf '%s\n' "$out" >&2; exit 2; };; esac; exit 0`
	// dendriteHookMarker identifies every hook command Dendrite owns, so a rewrite replaces them.
	dendriteHookMarker = "dendrite hook pre-tool"
)

// mergeClaudeHooks adds Dendrite's PreToolUse hook to a Claude Code settings document. Everything
// else in the document stays: other keys, other hooks, other matchers. A hook Dendrite wrote before
// is replaced rather than duplicated. changed is false when the document already had it.
func mergeClaudeHooks(existing []byte) (merged []byte, changed bool, err error) {
	doc := map[string]any{}
	if len(bytes.TrimSpace(existing)) > 0 {
		if err := json.Unmarshal(existing, &doc); err != nil {
			return nil, false, fmt.Errorf("%s is not valid JSON, leaving it alone: %w", claudeSettingsPath, err)
		}
	}
	hooks, _ := doc["hooks"].(map[string]any)
	if hooks == nil {
		if _, present := doc["hooks"]; present {
			return nil, false, fmt.Errorf("%s: \"hooks\" is not an object, leaving it alone", claudeSettingsPath)
		}
		hooks = map[string]any{}
	}
	pre, _ := hooks["PreToolUse"].([]any)

	var kept []any
	for _, entry := range pre {
		if !isDendriteHookEntry(entry) {
			kept = append(kept, entry)
		}
	}
	ours := map[string]any{
		"matcher": preToolMatcher,
		"hooks":   []any{map[string]any{"type": "command", "command": preToolCommand}},
	}
	hooks["PreToolUse"] = append(kept, ours)
	doc["hooks"] = hooks

	// No HTML escaping: the hook command contains > and &, which should stay readable in the file.
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetEscapeHTML(false)
	enc.SetIndent("", "  ")
	if err := enc.Encode(doc); err != nil {
		return nil, false, err
	}
	out := buf.Bytes() // Encode ends the document with a newline

	var before any
	if len(bytes.TrimSpace(existing)) > 0 && json.Unmarshal(existing, &before) == nil {
		var after any
		_ = json.Unmarshal(out, &after)
		if reflect.DeepEqual(before, after) {
			return existing, false, nil
		}
	}
	return out, true, nil
}

// isDendriteHookEntry reports whether a PreToolUse entry runs only Dendrite hook commands.
func isDendriteHookEntry(entry any) bool {
	m, ok := entry.(map[string]any)
	if !ok {
		return false
	}
	inner, _ := m["hooks"].([]any)
	if len(inner) == 0 {
		return false
	}
	for _, h := range inner {
		hm, _ := h.(map[string]any)
		cmd, _ := hm["command"].(string)
		if !strings.Contains(cmd, dendriteHookMarker) {
			return false
		}
	}
	return true
}
