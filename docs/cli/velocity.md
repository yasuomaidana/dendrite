---
id: velocity
title: "dendrite velocity"
sidebar_label: "velocity"
---

# dendrite velocity

TTFT, TPS, and gateway resilience benchmarking suite

## Category

Axon & Myelin Sheath

## Aliases

- `bench`
- `benchmark`
- `conduction-velocity`
- `bnch`

## Subcommands

| Subcommand | Description |
| :--- | :--- |
| `inference` | Benchmark Time-to-First-Token and tokens/sec generation velocity |
| `gateway` | Stress-test LiteLLM gateway with cascade fault injection |
| `matrix` | Multi-engine comparative benchmark matrix |
| `stress` | Concurrency stress test against sliding window budget limiter |

## Flags

None

## Examples

```bash
dendrite bench inference
dendrite bench gateway
```
