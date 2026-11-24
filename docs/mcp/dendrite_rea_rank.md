---
id: dendrite_rea_rank
title: "dendrite_rea_rank"
sidebar_label: "dendrite_rea_rank"
---

# dendrite_rea_rank

Say which documents of an analysis to read first: the overview, then the most connected ones.

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
    "name": "dendrite_rea_rank"
  }
}
```

### CLI

```bash
dendrite mcp call dendrite_rea_rank '{"id":"sample"}'
```
