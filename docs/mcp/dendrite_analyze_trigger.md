---
id: dendrite_analyze_trigger
title: "dendrite_analyze_trigger"
sidebar_label: "dendrite_analyze_trigger"
---

# dendrite_analyze_trigger

Trigger or simulate execution of an analysis chunk under current or simulated token scheduling constraints

## Input Schema

```json
{
  "properties": {
    "chunk_id": {
      "description": "Identifier of the code or AST chunk to analyze",
      "type": "string"
    },
    "simulate_night": {
      "description": "If true, simulates execution inside the scheduled night window",
      "type": "boolean"
    },
    "simulate_sprint": {
      "description": "If true, simulates execution inside the 48h pre-reset cloud sprint",
      "type": "boolean"
    }
  },
  "required": [
    "chunk_id"
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
      "chunk_id": "sample",
      "simulate_night": false,
      "simulate_sprint": false
    },
    "name": "dendrite_analyze_trigger"
  }
}
```

### CLI

```bash
dendrite mcp call dendrite_analyze_trigger '{"chunk_id":"sample","simulate_night":false,"simulate_sprint":false}'
```
