---
id: bridge
title: "dendrite bridge"
sidebar_label: "bridge"
---

# dendrite bridge

JetBrains IDE MCP protocol integration and PSI symbol indexer

## Category

Synaptic Cleft & Gating

## Aliases

- `jetbrains`
- `neuromuscular-bridge`
- `jb`

## Subcommands

| Subcommand | Description |
| :--- | :--- |
| `probe` | Discover IDE MCP capabilities and tools |
| `open` | Open file at specified line in active JetBrains editor |
| `problems` | Query IDE inspection errors and diagnostics for file |
| `symbols` | Search symbol index in JetBrains PSI tree |
| `build` | Trigger IDE background build |
| `run` | Execute an IDE run configuration |
| `setup` | Configure JetBrains IDE toolset (.ai/mcp/mcp.json, agents, run configs) |

## Flags

| Flag | Description |
| :--- | :--- |
| `--line` | Line number to navigate to (default: 1) |

## Examples

```bash
dendrite jetbrains probe
dendrite jetbrains open main.go --line 42
dendrite jetbrains symbols "Registry"
```
