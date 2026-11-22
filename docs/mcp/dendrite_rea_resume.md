---
id: dendrite_rea_resume
title: "dendrite_rea_resume"
sidebar_label: "dendrite_rea_resume"
---

# dendrite_rea_resume

Continue an analysis from its last checkpoint, optionally on another model tier or model.

## Input Schema

```json
{
  "properties": {
    "id": {
      "description": "Analysis id, as printed by dendrite_rea_analyze",
      "type": "string"
    },
    "model": {
      "description": "Model to use on that tier",
      "type": "string"
    },
    "tier": {
      "description": "Model tier: local or paid",
      "type": "string"
    }
  },
  "required": [
    "id"
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
      "id": "sample",
      "model": "sample",
      "tier": "sample"
    },
    "name": "dendrite_rea_resume"
  }
}
```

### CLI

```bash
dendrite mcp call dendrite_rea_resume '{"id":"sample","model":"sample","tier":"sample"}'
```
