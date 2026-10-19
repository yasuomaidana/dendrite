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
| `build` | Build the project in the IDE |
| `open` | Open a file in the IDE editor |
| `probe` | Connect to the IDE and list its MCP tools |
| `problems` | Show the IDE's inspection results for a file |
| `run` | Run an IDE run configuration |
| `setup` | Write the IDE's MCP config, run configurations and external tools (no running IDE needed) |
| `symbols` | Search the IDE's symbol index |

## Flags

None

## Subcommand Reference

### `dendrite bridge open`

Open a file in the IDE editor

```bash
dendrite bridge open <file> [flags]
```

| Flag | Description |
| :--- | :--- |
| `--line, -l` | Line to place the caret on (default: 1) |

### `dendrite bridge setup`

Write the IDE's MCP config, run configurations and external tools (no running IDE needed)

```bash
dendrite bridge setup [flags]
```

Aliases: `setup-mcp`, `configure`

| Flag | Description |
| :--- | :--- |
| `--global-only` | Only write the global MCP configuration |
| `--project-only` | Only write this project's configuration |

## Examples

```bash
dendrite bridge setup
dendrite bridge probe
dendrite bridge open cmd/dendrite/root.go --line 40
dendrite bridge symbols newRootCmd
```
