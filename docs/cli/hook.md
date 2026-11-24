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
| `decision` | Post-AskUserQuestion hook: record the questions and the user's answers on the claimed ticket (kept locally until a claim) |
| `plan-saved` | Post-ExitPlanMode hook: store the plan on the claimed ticket (kept locally when there is none) |
| `pre-commit` | Git pre-commit hook: the same guard for every staged path, so agents cannot bypass it through git |
| `pre-edit` | Pre-edit hook: refuse a file change unless this working copy has a claimed, In Progress ticket |
| `pre-tool` | Pre-tool hook: stop a raw recursive search once, point at the Dendrite equivalent, and log tool use |
| `session-end` | Session-end hook: a refused REA ticket goes to AGY; a ticket still In Progress gets an automatic handoff and is blocked (needs-input) |
| `stop` | Stop hook: an REA ticket the Claude session refused goes to AGY (the [rea] directive) |

## Flags

None

## Examples

```bash
# .claude/settings.json (written by `dendrite soma prepare`)
#   "PreToolUse": [{"matcher": "Bash|Grep|mcp__dendrite.*", "hooks": [{"type": "command", "command": "dendrite hook pre-tool"}]}]
```
