---
id: dendrite_rea_reuse
title: "dendrite_rea_reuse"
sidebar_label: "dendrite_rea_reuse"
---

# dendrite_rea_reuse

Find code in the project and its synapse that already does what an analysis found (exports, strings, non-system dependencies); ranked passages under 4,000 tokens. Use it before writing cleanroom code.

## Input Schema

```json
{
  "properties": {
    "id": {
      "description": "Analysis id, as printed by dendrite_rea_analyze",
      "type": "string"
    },
    "k": {
      "description": "How many passages (default 10)",
      "type": "number"
    },
    "project": {
      "description": "Project directory to search (default: the server's directory)",
      "type": "string"
    },
    "synapse": {
      "description": "Synapse to search (default: the one the project belongs to)",
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
      "k": 1,
      "project": "the server's directory",
      "synapse": "the one the project belongs to"
    },
    "name": "dendrite_rea_reuse"
  }
}
```

### CLI

```bash
dendrite mcp call dendrite_rea_reuse '{"id":"sample","k":1,"project":"the server's directory","synapse":"the one the project belongs to"}'
```
