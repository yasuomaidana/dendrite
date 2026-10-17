---
id: mcp
title: "dendrite mcp"
sidebar_label: "mcp"
---

# dendrite mcp

Model Context Protocol (MCP) server over stdio

## Category

Synaptic Cleft & Gating

## Aliases

- `interneuron-mcp`
- `inter`

## Subcommands

| Subcommand | Description |
| :--- | :--- |
| `serve` | Launch Dendrite MCP server listening on stdio |
| `tools` | List all registered Dendrite MCP tools |
| `call` | Invoke an MCP tool directly with JSON arguments |

## Flags

None

## Examples

```bash
dendrite mcp serve
dendrite mcp tools
dendrite mcp call dendrite_env_status
```
