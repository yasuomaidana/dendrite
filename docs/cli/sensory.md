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

## Flags

None

## Subcommand Reference

### `dendrite sensory metrics`

Print the metrics in Prometheus exposition format

```bash
dendrite sensory metrics
```

Aliases: `export`, `prometheus`

## Examples

```bash
dendrite sensory
dendrite sensory snapshot --json
dendrite sensory metrics
```
