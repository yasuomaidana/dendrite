---
id: dendrite_dist_sources
title: "dendrite_dist_sources"
sidebar_label: "dendrite_dist_sources"
---

# dendrite_dist_sources

List or filter registered codebase sources (own-repo, curated-oss, registry) and libraries with priority order

## Input Schema

```json
{
  "properties": {
    "language": {
      "description": "Filter by language (e.g. rust, go, typescript)",
      "type": "string"
    },
    "priority": {
      "description": "Filter by priority level (1 = own repo, 2 = curated OSS, 3 = registry)",
      "type": "integer"
    },
    "query": {
      "description": "Capability or library name query (e.g. wgpu, cte, turso, pixels)",
      "type": "string"
    }
  },
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
      "language": "sample",
      "priority": 1,
      "query": "sample"
    },
    "name": "dendrite_dist_sources"
  }
}
```

### CLI

```bash
dendrite mcp call dendrite_dist_sources '{"language":"sample","priority":1,"query":"sample"}'
```
