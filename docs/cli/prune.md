---
id: prune
title: "dendrite prune"
sidebar_label: "prune"
---

# dendrite prune

Topological vendor audit and rebase engine for local/* branches

## Category

Glial Regulation & Pruning

## Aliases

- `audit-changes`
- `microglial-prune`
- `synaptic-pruning`
- `prn`

## Subcommands

None

## Flags

| Flag | Description |
| :--- | :--- |
| `--branch, -b` | Audit only this branch (default: every pending local/* branch) |
| `--dry-run` | Show what would happen without changing anything |
| `--max-tokens` | Token budget for the audit payload (default: 4096) |
| `--model` | LLM model name |

## Examples

```bash
dendrite prune --dry-run
dendrite prune --branch local/module-a --max-tokens 8192
```
