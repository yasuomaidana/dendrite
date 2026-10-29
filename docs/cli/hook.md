---
id: hook
title: "dendrite hook"
sidebar_label: "hook"
---

# dendrite hook

Entry points agent runtimes call around their tools (not for typing by hand)

## Category

Terminal Boutons & Vesicles

## Aliases

None

## Subcommands

| Subcommand | Description |
| :--- | :--- |
| `pre-tool` | Pre-tool hook: stop a raw recursive search once, point at the Dendrite equivalent, and log tool use |

## Flags

None

## Examples

```bash
# .claude/settings.json (written by `dendrite soma prepare`)
#   "PreToolUse": [{"matcher": "Bash|Grep|mcp__dendrite.*", "hooks": [{"type": "command", "command": "dendrite hook pre-tool"}]}]
```
