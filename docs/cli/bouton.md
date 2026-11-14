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
| `jobs cancel` | Cancel a job and stop its process group |
| `jobs daemon` | Run the stall watcher (foreground), once, or install it as a LaunchAgent |
| `jobs list` | List jobs |
| `jobs models` | Refresh the cached `agy models` listing and show the AGY models Dendrite schedules |
| `jobs retry` | Re-dispatch a job |
| `jobs show` | Show a job's specification and event history |
| `jobs submit` | Submit a job to an agent (runs detached) |
| `jobs tail` | Print the last lines of a job's agent.log |
| `jobs wait` | Block until the matching jobs finish |
| `jobs watch` | Follow a job live: status, process, model requests, changed files and the stall countdown |
| `route` | Pick the best agent, tier and model for a task, and optionally run it |
| `run` | Launch a coding agent (aider, openhands, opencode, agy, copilot) through the gateway |
| `search` | Rank agents for a query through the dendrite-rag sidecar (BM25, embeddings, RRF) |
| `sessions` | List recorded agent sessions (duration, cost, status) |

## Flags

None

## Subcommand Reference

### `dendrite bouton jobs`

Submit, watch and manage delegated agent jobs

```bash
dendrite bouton jobs
```

```bash
dendrite bouton jobs submit --ticket DEN-123 --agent agy --scope pkg/foo --prompt-file prompt.md --isolate
dendrite bouton jobs list --ticket DEN-123
dendrite bouton jobs show --ticket DEN-123
dendrite bouton jobs wait --ticket DEN-123 --timeout 2h
```

### `dendrite bouton jobs daemon`

Run the stall watcher (foreground), once, or install it as a LaunchAgent

```bash
dendrite bouton jobs daemon [flags]
```

| Flag | Description |
| :--- | :--- |
| `--idle-exit` | Exit once no job has needed the watcher for this long (0: run until stopped) |
| `--install` | Install the watcher as a macOS LaunchAgent |
| `--interval` | Time between watcher passes (default: 30s) |
| `--once` | Run one watcher pass and exit |

### `dendrite bouton jobs list`

List jobs

```bash
dendrite bouton jobs list [flags]
```

Aliases: `ls`

| Flag | Description |
| :--- | :--- |
| `--ticket, -t` | Ticket identifier (e.g. DEN-123) |

### `dendrite bouton jobs show`

Show a job's specification and event history

```bash
dendrite bouton jobs show [<job-id>] [flags]
```

| Flag | Description |
| :--- | :--- |
| `--ticket, -t` | Ticket identifier (e.g. DEN-123) |

### `dendrite bouton jobs submit`

Submit a job to an agent (runs detached)

```bash
dendrite bouton jobs submit [flags]
```

| Flag | Description |
| :--- | :--- |
| `--agent` | Agent: agy, aider, copilot, ... (default: agy) |
| `--complexity` | Task complexity (1-10); picks the model tier (default: 1) |
| `--isolate` | Run in the ticket's dendrite-work/ copy |
| `--model` | LLM model name, or auto to pick by complexity |
| `--prompt` | Prompt text |
| `--prompt-file` | File with the prompt |
| `--scope` | Paths the agent may change (comma-separated) |
| `--ticket, -t` | Ticket identifier (e.g. DEN-123) |
| `--worktree` | Run in this checkout |

### `dendrite bouton jobs tail`

Print the last lines of a job's agent.log

```bash
dendrite bouton jobs tail <job-id> [flags]
```

| Flag | Description |
| :--- | :--- |
| `--lines, -n` | Number of lines (default: 50) |

### `dendrite bouton jobs wait`

Block until the matching jobs finish

```bash
dendrite bouton jobs wait [flags]
```

| Flag | Description |
| :--- | :--- |
| `--interval` | Polling interval (default: 2s) |
| `--ticket, -t` | Ticket identifier (e.g. DEN-123) |
| `--timeout` | Give up after this long (default: 30m0s) |

### `dendrite bouton jobs watch`

Follow a job live: status, process, model requests, changed files and the stall countdown

```bash
dendrite bouton jobs watch [<job-id>] [flags]
```

| Flag | Description |
| :--- | :--- |
| `--interval` | Time between snapshots (default: 10s) |
| `--once` | Print one snapshot and exit |
| `--ticket, -t` | Ticket identifier (e.g. DEN-123) |

```bash
dendrite bouton jobs watch --ticket DEN-123
dendrite bouton jobs watch DEN-123-20261029T053329Z-ab7522 --once --json
```

### `dendrite bouton route`

Pick the best agent, tier and model for a task, and optionally run it

```bash
dendrite bouton route [task...] [flags]
```

| Flag | Description |
| :--- | :--- |
| `--exec` | Launch the selected agent |
| `--local` | Require local inference (runs sandboxed with --exec) |
| `--pkg` | Target package |
| `--task` | Task description |

### `dendrite bouton run`

Launch a coding agent (aider, openhands, opencode, agy, copilot) through the gateway

```bash
dendrite bouton run <agent> [flags]
```

| Flag | Description |
| :--- | :--- |
| `--allow-install` | Install the agent CLI when missing (also --yes) |
| `--model` | LLM model name |
| `--sandbox` | Run in an ephemeral cap-dropped container |
| `--task` | Task description for the agent |
| `--ticket, -t` | Ticket identifier (e.g. DEN-123) |

### `dendrite bouton search`

Rank agents for a query through the dendrite-rag sidecar (BM25, embeddings, RRF)

```bash
dendrite bouton search <query...> [flags]
```

| Flag | Description |
| :--- | :--- |
| `--local` | Only agents that can run on local inference |
| `--max-cost` | Maximum estimated cost in USD (0: no limit) |
| `--pkg` | Target package |

### `dendrite bouton sessions`

List recorded agent sessions (duration, cost, status)

```bash
dendrite bouton sessions [flags]
```

Aliases: `session`

| Flag | Description |
| :--- | :--- |
| `--ticket, -t` | Ticket identifier (e.g. DEN-123) |

## Examples

```bash
dendrite bouton run aider --ticket DEN-030 --task "fix the parser"
dendrite bouton search "refactor AST parser" --local
dendrite bouton route --task "add a CLI flag" --exec
dendrite bouton jobs submit --ticket DEN-123 --agent agy --scope pkg/foo --prompt-file prompt.md --isolate
```
