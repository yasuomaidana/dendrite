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

	// editLockMatcher selects the tools that change files.
	editLockMatcher = "Edit|Write|MultiEdit|NotebookEdit"
	// editLockCommand is the ticket lock and, unlike the search guard, fails CLOSED: with no dendrite on PATH,
	// or one that crashes or predates `hook pre-edit`, the write is refused (exit 2) and the reason is shown.
	// An agent that cannot be checked against a ticket claim does not get to change files.
	editLockCommand = `command -v dendrite >/dev/null 2>&1 || { echo "blocked: dendrite is not on PATH, so this edit cannot be checked against a ticket claim - install it and run: dendrite session resume" >&2; exit 2; }; ` +
		`out=$(dendrite hook pre-edit 2>&1); c=$?; if [ "$c" -ne 0 ]; then printf '%s\n' "$out" >&2; exit 2; fi; exit 0`

	// planSavedMatcher and planSavedCommand store a plan on the claimed ticket after ExitPlanMode. Like the
	// session-end hook it fails open and never fails the agent.
	planSavedMatcher = "ExitPlanMode"
	planSavedCommand = `command -v dendrite >/dev/null 2>&1 || exit 0; dendrite hook plan-saved; exit 0`
	// decisionMatcher and decisionCommand record the user's answers to an AskUserQuestion on the claimed ticket.
	// Like the plan hook it fails open and never fails the agent.
	decisionMatcher = "AskUserQuestion"
	decisionCommand = `command -v dendrite >/dev/null 2>&1 || exit 0; dendrite hook decision; exit 0`
	// sessionEndCommand hands a ticket that is still In Progress over when the session ends.
	sessionEndCommand = `command -v dendrite >/dev/null 2>&1 || exit 0; dendrite hook session-end; exit 0`

	// dendriteHookMarker identifies every hook command Dendrite owns, so a rewrite replaces them.
	dendriteHookMarker = "dendrite hook "
)

// hookSpec is one hook Dendrite owns in the Claude Code settings.
type hookSpec struct {
	event, matcher, command string
}

// dendriteHooks are the hooks `soma prepare` installs, in the order they are added to their event.
var dendriteHooks = []hookSpec{
	{"PreToolUse", preToolMatcher, preToolCommand},
	{"PreToolUse", editLockMatcher, editLockCommand},
	{"PostToolUse", planSavedMatcher, planSavedCommand},
	{"PostToolUse", decisionMatcher, decisionCommand},
	{"SessionEnd", "", sessionEndCommand},
}

// mergeClaudeHooks adds Dendrite's hooks to a Claude Code settings document. Everything else in the
// document stays: other keys, other hooks, other matchers. A hook Dendrite wrote before is replaced rather
// than duplicated. changed is false when the document already had them.
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

	var events []string
	for _, h := range dendriteHooks {
		if len(events) == 0 || !contains(events, h.event) {
			events = append(events, h.event)
		}
	}
	for _, event := range events {
		list, present := hooks[event]
		entries, _ := list.([]any)
		if present && entries == nil && list != nil {
			return nil, false, fmt.Errorf("%s: \"hooks.%s\" is not a list, leaving it alone", claudeSettingsPath, event)
		}
		var kept []any
		for _, entry := range entries {
			if !isDendriteHookEntry(entry) {
				kept = append(kept, entry)
			}
		}
		for _, h := range dendriteHooks {
			if h.event != event {
				continue
			}
			ours := map[string]any{"hooks": []any{map[string]any{"type": "command", "command": h.command}}}
			if h.matcher != "" {
				ours["matcher"] = h.matcher
			}
			kept = append(kept, ours)
		}
		hooks[event] = kept
	}
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

func contains(list []string, s string) bool {
	for _, v := range list {
		if v == s {
			return true
		}
	}
	return false
}

// isDendriteHookEntry reports whether a hook entry runs only Dendrite hook commands.
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
