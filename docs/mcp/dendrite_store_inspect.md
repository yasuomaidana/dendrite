---
id: dendrite_store_inspect
title: "dendrite_store_inspect"
sidebar_label: "dendrite_store_inspect"
---

# dendrite_store_inspect

Detailed chunk-by-chunk and segment analysis breakdown for a specific stored project

## Input Schema

```json
{
  "properties": {
    "store_id": {
      "description": "Project / Store identifier",
      "type": "string"
    }
  },
  "required": [
    "store_id"
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
      "store_id": "sample"
    },
    "name": "dendrite_store_inspect"
  }
}
```

### CLI

```bash
dendrite mcp call dendrite_store_inspect '{"store_id":"sample"}'
```
