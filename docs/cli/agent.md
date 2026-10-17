---
id: agent
title: "dendrite agent"
sidebar_label: "agent"
---

# dendrite agent

Autonomous coding agent harness (Aider, OpenHands, OpenCode)

## Category

Postsynaptic Effectors & Actuation

## Aliases

- `effector-dispatch`
- `bouton-dispatch`
- `dispatch`
- `bouton`
- `agt`

## Subcommands

| Subcommand | Description |
| :--- | :--- |
| `run` | Launch an autonomous coding agent (aider, openhands, opencode, agy, copilot) |
| `search` | Search and rank agents with hybrid vector & BM25 search |
| `route` | Auto-route task to optimal agent with LiteLLM virtual key |
| `session` | View agent execution history, token spend, and commits |

## Flags

| Flag | Description |
| :--- | :--- |
| `--ticket` | Target ticket identifier (e.g. DEN-030) |
| `--model` | Target LLM model name |
| `--spend-cap` | Maximum USD budget ceiling for session |
| `--sandbox` | Execute inside ephemeral container sandbox |

## Examples

```bash
dendrite agent run aider --ticket DEN-030
dendrite agent search "refactor AST parser"
dendrite agent session list
```
