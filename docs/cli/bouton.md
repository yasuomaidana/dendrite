---
id: bouton
title: "dendrite bouton"
sidebar_label: "bouton"
---

# dendrite bouton

Autonomous coding agent harness and job dispatcher (Aider, OpenHands, OpenCode, AGY, Copilot)

## Category

Postsynaptic Effectors & Actuation

## Aliases

- `agent`
- `effector-dispatch`
- `bouton-dispatch`
- `dispatch`
- `agt`

## Subcommands

| Subcommand | Description |
| :--- | :--- |
| `jobs` | Submit, watch and manage delegated agent jobs |
| `jobs cancel` | Cancel a job: <id> |
| `jobs daemon` | Run the stall watcher [--once] [--interval] [--install] |
| `jobs list` | List jobs [--json] |
| `jobs retry` | Retry a job: <id> |
| `jobs run` | Run a job in the foreground (internal): <id> |
| `jobs show` | Show one job: <id> |
| `jobs submit` | Submit a job: --ticket --agent --model --scope --prompt-file [--worktree|--isolate] [--complexity] |
| `jobs tail` | Print the last lines of a job log: <id> |
| `jobs wait` | Wait for jobs to finish [--ticket] [--timeout] [--interval] |
| `route` | Auto-route a task to the optimal agent with a LiteLLM virtual key |
| `run` | Launch an autonomous coding agent (aider, openhands, opencode, agy, copilot) |
| `search` | Search and rank agents with hybrid vector & BM25 search |
| `session` | View agent execution history, token spend, and commits |

## Flags

| Flag | Description |
| :--- | :--- |
| `--ticket` | Target ticket identifier (e.g. DEN-030) |
| `--model` | Target LLM model name |
| `--spend-cap` | Maximum USD budget ceiling for session |
| `--sandbox` | Execute inside ephemeral container sandbox |

## Subcommand Reference

### `dendrite bouton jobs`

Submit, watch and manage delegated agent jobs

```bash
dendrite bouton jobs
```

```bash
dendrite bouton jobs submit --ticket DEN-123 --agent agy --scope pkg/foo --prompt-file prompt.md --isolate
dendrite bouton jobs list --json
dendrite bouton jobs wait --ticket DEN-123
```

## Examples

```bash
dendrite bouton run aider --ticket DEN-030
dendrite bouton jobs submit --ticket DEN-123 --agent agy --scope pkg/foo --prompt-file prompt.md --isolate
```
