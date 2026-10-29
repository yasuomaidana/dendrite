---
id: sensory
title: "dendrite sensory"
sidebar_label: "sensory"
---

# dendrite sensory

Hardware thermal, token spend, and Prometheus telemetry

## Category

Cortex & Sensory Integration

## Aliases

- `telemetry`
- `afferent-telemetry`
- `tel`

## Subcommands

| Subcommand | Description |
| :--- | :--- |
| `metrics` | Print the metrics in Prometheus exposition format |
| `snapshot` | Print one telemetry snapshot (temperature, spend, node health) |
| `usage` | How agent tool use splits between Dendrite retrieval and raw rg/grep searches |

## Flags

None

## Subcommand Reference

### `dendrite sensory metrics`

Print the metrics in Prometheus exposition format

```bash
dendrite sensory metrics
```

Aliases: `export`, `prometheus`

### `dendrite sensory usage`

How agent tool use splits between Dendrite retrieval and raw rg/grep searches

```bash
dendrite sensory usage [flags]
```

| Flag | Description |
| :--- | :--- |
| `--since` | Window to report (default: 168h0m0s) |

## Examples

```bash
dendrite sensory
dendrite sensory snapshot --json
dendrite sensory metrics
```
