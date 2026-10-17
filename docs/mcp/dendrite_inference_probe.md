---
id: dendrite_inference_probe
title: "dendrite_inference_probe"
sidebar_label: "dendrite_inference_probe"
---

# dendrite_inference_probe

Probe multi-provider inference engines (< 2s) for health, reachability, latency, and loaded models

## Input Schema

```json
{
  "properties": {
    "engine_id": {
      "description": "Optional engine ID to probe (e.g. 'ollama-metal', 'oxillama-rust'). If omitted, probes all engines.",
      "type": "string"
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
      "engine_id": "sample"
    },
    "name": "dendrite_inference_probe"
  }
}
```

### CLI

```bash
dendrite mcp call dendrite_inference_probe '{"engine_id":"sample"}'
```
