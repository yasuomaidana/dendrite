---
id: dendrite_rag_show
title: "dendrite_rag_show"
sidebar_label: "dendrite_rag_show"
---

# dendrite_rag_show

Full text of chunks by id: the bodies behind the outlines dendrite_rag_retrieve returns with detail "map"

## Input Schema

```json
{
  "properties": {
    "ids": {
      "description": "Chunk ids from dendrite_rag_retrieve",
      "items": {
        "type": "string"
      },
      "type": "array"
    },
    "project_path": {
      "description": "Target project directory (default: '.')",
      "type": "string"
    }
  },
  "required": [
    "ids"
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
      "ids": [
        "item1"
      ],
      "project_path": "'.'"
    },
    "name": "dendrite_rag_show"
  }
}
```

### CLI

```bash
dendrite mcp call dendrite_rag_show '{"ids":["item1"],"project_path":"'.'"}'
```
