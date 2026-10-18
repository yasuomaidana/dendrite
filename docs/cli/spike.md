---
id: spike
title: "dendrite spike"
sidebar_label: "spike"
---

# dendrite spike

Unified multi-provider inference engine gateway (Ollama, OxiLLaMa, vLLM, TGI)

## Category

Synaptic Cleft & Gating

## Aliases

- `inference`
- `action-potential`
- `inf`

## Subcommands

| Subcommand | Description |
| :--- | :--- |
| `list` | List registered engines, endpoints, and failover cascade order |
| `probe` | Probe inference engines for reachability, latency, and loaded models |
| `switch` | Set primary inference engine |
| `render` | Render engine configuration |

## Flags

None

## Examples

```bash
dendrite inference list
dendrite inference probe ollama
```
