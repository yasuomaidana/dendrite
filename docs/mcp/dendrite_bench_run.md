---
id: dendrite_bench_run
title: "dendrite_bench_run"
sidebar_label: "dendrite_bench_run"
---

# dendrite_bench_run

Execute an automated k6 or synthetic performance benchmark measuring TTFT, TPS, gateway cascade failover latency, or sliding window budget concurrency

## Input Schema

```json
{
  "properties": {
    "duration_seconds": {
      "description": "Benchmark duration in seconds (default: 10)",
      "type": "integer"
    },
    "endpoint": {
      "description": "Target endpoint URL (default: auto-resolved from registered engine)",
      "type": "string"
    },
    "engine_id": {
      "description": "Inference engine identifier (e.g. 'oxillama', 'ollama-metal', 'vllm-cluster')",
      "type": "string"
    },
    "failover_endpoint": {
      "description": "Secondary endpoint URL for failover testing",
      "type": "string"
    },
    "fault_injection": {
      "description": "Simulated fault injection: 'none', 'rate-limit-429', 'node-glitch-503'",
      "type": "string"
    },
    "mode": {
      "description": "Benchmark mode: 'inference', 'gateway', 'stress', 'matrix' (default: 'inference')",
      "type": "string"
    },
    "model": {
      "description": "Model identifier (default: 'qwen2.5-coder-7b')",
      "type": "string"
    },
    "virtual_users": {
      "description": "Number of concurrent virtual users (default: 5)",
      "type": "integer"
    }
  },
  "type": "object"
}
```

## Example Call

### JSON-RPC

```json
{
  "id": 1,
  "jsonrpc": "2.0",
  "method": "tools/call",
  "params": {
    "arguments": {
      "duration_seconds": 1,
      "endpoint": "auto-resolved from registered engine",
      "engine_id": "sample",
      "failover_endpoint": "sample",
      "fault_injection": "sample",
      "mode": "'inference'",
      "model": "'qwen2.5-coder-7b'",
      "virtual_users": 1
    },
    "name": "dendrite_bench_run"
  }
}
```

### CLI

```bash
dendrite mcp call dendrite_bench_run '{"duration_seconds":1,"endpoint":"auto-resolved from registered engine","engine_id":"sample","failover_endpoint":"sample","fault_injection":"sample","mode":"'inference'","model":"'qwen2.5-coder-7b'","virtual_users":1}'
```
