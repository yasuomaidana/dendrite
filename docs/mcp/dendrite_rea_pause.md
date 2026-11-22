---
id: dendrite_rea_pause
title: "dendrite_rea_pause"
sidebar_label: "dendrite_rea_pause"
---

# dendrite_rea_pause

Pause an analysis at its next stage boundary; its checkpoint is kept.

## Input Schema

```json
{
  "properties": {
    "id": {
      "description": "Analysis id, as printed by dendrite_rea_analyze",
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
      "id": "sample"
    },
    "name": "dendrite_rea_pause"
  }
}
```

### CLI

```bash
dendrite mcp call dendrite_rea_pause '{"id":"sample"}'
```
