---
id: dendrite_inference_migrate_model
title: "dendrite_inference_migrate_model"
sidebar_label: "dendrite_inference_migrate_model"
---

# dendrite_inference_migrate_model

Migrate models from Ollama to OxiLLaMa pure-Rust inference engine and evict source blobs to reclaim memory

## Input Schema

```json
{
  "properties": {
    "delete_source": {
      "description": "If true, evicts model from source engine upon successful verification (default: false; OxiLLaMa runs prompts on the CPU, so keep the Ollama copy for GPU inference)",
      "type": "boolean"
    },
    "dry_run": {
      "description": "If true, simulates migration without modifying disk (default: false)",
      "type": "boolean"
    },
    "model": {
      "description": "Model identifier to migrate (e.g. 'qwen3.5:9b', 'qwen3.8:27b')",
      "type": "string"
    },
    "source_engine": {
      "description": "Source engine identifier (default: 'ollama')",
      "type": "string"
    },
    "target_engine": {
      "description": "Target engine identifier (default: 'oxillama')",
      "type": "string"
    }
  },
  "required": [
    "model"
  ],
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
      "delete_source": false,
      "dry_run": false,
      "model": "sample",
      "source_engine": "'ollama'",
      "target_engine": "'oxillama'"
    },
    "name": "dendrite_inference_migrate_model"
  }
}
```

### CLI

```bash
dendrite mcp call dendrite_inference_migrate_model '{"delete_source":false,"dry_run":false,"model":"sample","source_engine":"'ollama'","target_engine":"'oxillama'"}'
```
