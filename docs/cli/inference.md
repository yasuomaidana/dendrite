---
id: inference
title: "dendrite inference"
sidebar_label: "inference"
---

# dendrite inference

Unified multi-provider inference engine gateway (Ollama, OxiLLaMa, vLLM, TGI)

## Category

Synaptic Cleft & Gating

## Aliases

- `action-potential`
- `spike`
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
