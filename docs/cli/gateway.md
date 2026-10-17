---
id: gateway
title: "dendrite gateway"
sidebar_label: "gateway"
---

# dendrite gateway

LiteLLM token gateway rendering, dynamic routing updater & scheduler

## Category

Synaptic Cleft & Gating

## Aliases

- `synaptic-cleft`
- `cleft`
- `gate`

## Subcommands

| Subcommand | Description |
| :--- | :--- |
| `update` | Dynamically discover live inference engines and update LiteLLM routing table |
| `schedule` | Run dynamic routing table updater on daily background schedule |

## Flags

| Flag | Description |
| :--- | :--- |
| `--file` | Target litellm_config.yaml path (default: stacks/litellm_config.yaml) |
| `--dry-run` | Simulate update without modifying disk |

## Examples

```bash
dendrite gateway update
dendrite gateway schedule
```
