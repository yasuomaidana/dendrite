---
id: inter
title: "dendrite inter"
sidebar_label: "inter"
---

# dendrite inter

Model Context Protocol (MCP) server over stdio

## Category

Synaptic Cleft & Gating

## Aliases

- `mcp`
- `interneuron-mcp`

## Subcommands

| Subcommand | Description |
| :--- | :--- |
| `call` | Call one MCP tool and print its result |
| `probe` | Call every read-only MCP tool once and print a pass/fail matrix |
| `serve` | Serve MCP over stdin/stdout |
| `tools` | List the registered MCP tools |

## Flags

None

## Subcommand Reference

### `dendrite inter probe`

Call every read-only MCP tool once and print a pass/fail matrix

```bash
dendrite inter probe [flags]
```

| Flag | Description |
| :--- | :--- |
| `--timeout` | Override every probe's timeout (default: per tool, 5s unless it needs more) |

### `dendrite inter serve`

Serve MCP over stdin/stdout

```bash
dendrite inter serve
```

Aliases: `stdio`

### `dendrite inter tools`

List the registered MCP tools

```bash
dendrite inter tools
```

Aliases: `list`, `ls`

## Examples

```bash
dendrite inter serve
dendrite inter serve --synapse web-api
dendrite inter tools
dendrite inter call dendrite_ticket_smart_select '{}'
```
