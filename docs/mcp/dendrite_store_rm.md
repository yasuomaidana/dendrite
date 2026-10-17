---
id: dendrite_store_rm
title: "dendrite_store_rm"
sidebar_label: "dendrite_store_rm"
---

# dendrite_store_rm

Safely removes SQLite analysis records and unlinks compressed blobs with confirmation gate

## Input Schema

```json
{
  "properties": {
    "force": {
      "description": "Mandatory confirmation gate to perform permanent deletion",
      "type": "boolean"
    },
    "store_id": {
      "description": "Project / Store identifier to delete",
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
      "force": false,
      "store_id": "sample"
    },
    "name": "dendrite_store_rm"
  }
}
```

### CLI

```bash
dendrite mcp call dendrite_store_rm '{"force":false,"store_id":"sample"}'
```
