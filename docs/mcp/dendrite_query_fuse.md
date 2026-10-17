---
id: dendrite_query_fuse
title: "dendrite_query_fuse"
sidebar_label: "dendrite_query_fuse"
---

# dendrite_query_fuse

Combines multiple binaries/projects, deduplicating libraries and synthesizing zero-copy in-process channels

## Input Schema

```json
{
  "properties": {
    "fused_member_name": {
      "description": "Identifier for the combined member",
      "type": "string"
    },
    "register_in_pool": {
      "description": "Register unified member in .dendrite/pool.json",
      "type": "boolean"
    },
    "sources": {
      "description": "List of source project or artifact paths",
      "items": {
        "type": "string"
      },
      "type": "array"
    },
    "target_directory": {
      "description": "Destination directory for unified project",
      "type": "string"
    },
    "target_language": {
      "description": "Target unified language",
      "enum": [
        "rust",
        "go"
      ],
      "type": "string"
    }
  },
  "required": [
    "sources",
    "fused_member_name",
    "target_directory"
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
      "fused_member_name": "sample",
      "register_in_pool": false,
      "sources": [
        "item1"
      ],
      "target_directory": "sample",
      "target_language": "sample"
    },
    "name": "dendrite_query_fuse"
  }
}
```

### CLI

```bash
dendrite mcp call dendrite_query_fuse '{"fused_member_name":"sample","register_in_pool":false,"sources":["item1"],"target_directory":"sample","target_language":"sample"}'
```
