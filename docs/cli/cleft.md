---
id: cleft
title: "dendrite cleft"
sidebar_label: "cleft"
---

# dendrite cleft

LiteLLM token gateway rendering, dynamic routing updater & scheduler

## Category

Synaptic Cleft & Gating

## Aliases

- `gateway`
- `synaptic-cleft`
- `gate`

## Subcommands

| Subcommand | Description |
| :--- | :--- |
| `render` | Render the LiteLLM config for a topology scenario (default local-hybrid) |
| `schedule` | Keep the LiteLLM routing table updated on an interval (foreground; Ctrl+C stops it) |
| `update` | Discover live inference engines and rewrite the LiteLLM routing table |

## Flags

None

## Subcommand Reference

### `dendrite cleft schedule`

Keep the LiteLLM routing table updated on an interval (foreground; Ctrl+C stops it)

```bash
dendrite cleft schedule [flags]
```

Aliases: `daemon`, `watch`

| Flag | Description |
| :--- | :--- |
| `--docker` | Rewrite engine hosts for LiteLLM running in Docker (--docker=false for a host install) (default: true) |
| `--file, -f` | LiteLLM config file to rewrite (default: /Users/yasuomaidana/.dendrite/stacks/litellm_config.yaml) |
| `--interval, -i` | Time between updates (default: 24h0m0s) |

### `dendrite cleft update`

Discover live inference engines and rewrite the LiteLLM routing table

```bash
dendrite cleft update [flags]
```

Aliases: `sync`, `refresh`

| Flag | Description |
| :--- | :--- |
| `--docker` | Rewrite engine hosts for LiteLLM running in Docker (--docker=false for a host install) (default: true) |
| `--dry-run` | Show what would happen without changing anything |
| `--file, -f` | LiteLLM config file to rewrite (default: /Users/yasuomaidana/.dendrite/stacks/litellm_config.yaml) |

## Examples

```bash
dendrite cleft update --dry-run
dendrite cleft schedule --interval 12h
dendrite cleft render azure-burst
```
