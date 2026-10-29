---
id: vesicle
title: "dendrite vesicle"
sidebar_label: "vesicle"
---

# dendrite vesicle

Inspect 24h & 7d token consumption windows and tier quotas

## Category

Terminal Boutons & Vesicles

## Aliases

- `tokens`
- `token`
- `vesicle-quota`
- `quota`
- `tok`

## Subcommands

| Subcommand | Description |
| :--- | :--- |
| `estimate` | Estimate a prompt's size in characters, words and tokens |
| `status` | Show day and week token windows against capacity, and AGY access |

## Flags

None

## Subcommand Reference

### `dendrite vesicle estimate`

Estimate a prompt's size in characters, words and tokens

```bash
dendrite vesicle estimate <prompt...>
```

Aliases: `audit-prompt`

### `dendrite vesicle status`

Show day and week token windows against capacity, and AGY access

```bash
dendrite vesicle status [flags]
```

| Flag | Description |
| :--- | :--- |
| `--day-tokens` | Day window capacity in tokens (default: 1000000) |
| `--model` | LLM model name (default: gemini-3.8-flash-high) |
| `--policy-file` | Scheduler policy file (default: /Users/yasuomaidana/GoLandProjects/dendrite-work/Dendrite-DEN-363/.dendrite/scheduler_policy.json) |
| `--probe` | Send a probe request to confirm AGY access |
| `--spend-file` | Spend ledger file (default: /Users/yasuomaidana/GoLandProjects/dendrite-work/Dendrite-DEN-363/.dendrite/spend.json) |
| `--week-tokens` | Week window capacity in tokens (default: 7000000) |

## Examples

```bash
dendrite vesicle status
dendrite vesicle status --probe --json
dendrite vesicle estimate "Refactor the planner to use the DAG solver"
```
