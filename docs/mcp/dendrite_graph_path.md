---
id: dendrite_graph_path
title: "dendrite_graph_path"
sidebar_label: "dendrite_graph_path"
---

# dendrite_graph_path

Find shortest path between two symbols or files in the code graph

## Input Schema

```json
{
  "properties": {
    "from": {
      "description": "Starting symbol name or project-relative file path",
      "type": "string"
    },
    "kinds": {
      "description": "Edge kinds to follow (call, import, type; default: all)",
      "items": {
        "type": "string"
      },
      "type": "array"
    },
    "max_hops": {
      "description": "Maximum number of hops (default: 6)",
      "type": "integer"
    },
    "project_path": {
      "description": "Target project directory to index and analyze (default: '.')",
      "type": "string"
    },
    "to": {
      "description": "Destination symbol name or project-relative file path",
      "type": "string"
    }
  },
  "required": [
    "from",
    "to"
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
      "from": "sample",
      "kinds": [
        "item1"
      ],
      "max_hops": 1,
      "project_path": "'.'",
      "to": "sample"
    },
    "name": "dendrite_graph_path"
  }
}
```

### CLI

```bash
dendrite mcp call dendrite_graph_path '{"from":"sample","kinds":["item1"],"max_hops":1,"project_path":"'.'","to":"sample"}'
```
