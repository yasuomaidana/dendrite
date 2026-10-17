---
id: bench
title: "dendrite bench"
sidebar_label: "bench"
---

# dendrite bench

TTFT, TPS, and gateway resilience benchmarking suite

## Category

Axon & Myelin Sheath

## Aliases

- `benchmark`
- `conduction-velocity`
- `velocity`
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
