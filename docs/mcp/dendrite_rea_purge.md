---
id: dendrite_rea_purge
title: "dendrite_rea_purge"
sidebar_label: "dendrite_rea_purge"
---

# dendrite_rea_purge

Delete an analysis: its documents, its index and its checkpoint. Needs force=true.

## Input Schema

```json
{
  "properties": {
    "force": {
      "description": "Confirm the deletion",
      "type": "boolean"
    },
    "id": {
      "description": "Analysis id, as printed by dendrite_rea_analyze",
      "type": "string"
    }
  },
  "required": [
    "id",
    "force"
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
      "force": false,
      "id": "sample"
    },
    "name": "dendrite_rea_purge"
  }
}
```

### CLI

```bash
dendrite mcp call dendrite_rea_purge '{"force":false,"id":"sample"}'
```
