---
id: dendrite_rea_search
title: "dendrite_rea_search"
sidebar_label: "dendrite_rea_search"
---

# dendrite_rea_search

Search the documents of a finished analysis; returns only the best passages, within the 4,000-token boundary.

## Input Schema

```json
{
  "properties": {
    "id": {
      "description": "Analysis id, as printed by dendrite_rea_analyze",
      "type": "string"
    },
    "k": {
      "description": "How many passages (default 5, at most 20)",
      "type": "number"
    },
    "query": {
      "description": "What to look for, e.g. an imported function",
      "type": "string"
    }
  },
  "required": [
    "id",
    "query"
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
      "k": 1,
      "query": "sample"
    },
    "name": "dendrite_rea_search"
  }
}
```

### CLI

```bash
dendrite mcp call dendrite_rea_search '{"id":"sample","k":1,"query":"sample"}'
```
