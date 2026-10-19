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
| `gateway` | Stress-test the LiteLLM gateway's fallback cascade under fault injection |
| `inference` | Benchmark an inference engine's TTFT and TPS |
| `matrix` | Benchmark every registered engine and compare them in one table |
| `stress` | Concurrency stress test of the sliding-window budget limiter |

## Flags

None

## Subcommand Reference

### `dendrite velocity gateway`

Stress-test the LiteLLM gateway's fallback cascade under fault injection

```bash
dendrite velocity gateway [flags]
```

| Flag | Description |
| :--- | :--- |
| `--duration` | Test duration (default: 20s) |
| `--fault` | Injected fault: none, 429 (rate-limit-429) or 503 (node-glitch-503) (default: none) |
| `--output, -o` | Write the Markdown scorecard to this file instead of stdout |
| `--vus` | Concurrent virtual users (default: 10) |

### `dendrite velocity inference`

Benchmark an inference engine's TTFT and TPS

```bash
dendrite velocity inference [flags]
```

| Flag | Description |
| :--- | :--- |
| `--duration` | Test duration (default: 15s) |
| `--endpoint` | Target API endpoint (default: the engine's endpoint) |
| `--engine` | Inference engine ID (default: oxillama) |
| `--model` | LLM model name (default: qwen2.5-coder-7b) |
| `--output, -o` | Write the Markdown scorecard to this file instead of stdout |
| `--vus` | Concurrent virtual users (default: 5) |

### `dendrite velocity matrix`

Benchmark every registered engine and compare them in one table

```bash
dendrite velocity matrix [flags]
```

| Flag | Description |
| :--- | :--- |
| `--duration` | Test duration (default: 10s) |
| `--output, -o` | Write the Markdown scorecard to this file instead of stdout |
| `--vus` | Concurrent virtual users (default: 5) |

### `dendrite velocity stress`

Concurrency stress test of the sliding-window budget limiter

```bash
dendrite velocity stress [flags]
```

| Flag | Description |
| :--- | :--- |
| `--spends` | Total spend operations (default: 500) |
| `--vus` | Concurrent virtual users (default: 20) |

## Examples

```bash
dendrite velocity inference --engine oxillama --vus 5 --duration 15s
dendrite velocity gateway --fault 429 --output gateway.md
dendrite velocity matrix
dendrite velocity stress --vus 50 --spends 1000
```
