---
id: dendrite_plane_issues
title: "dendrite_plane_issues"
sidebar_label: "dendrite_plane_issues"
---

# dendrite_plane_issues

Query issues from self-hosted Plane v2 project management container

## Input Schema

```json
{
  "properties": {
    "project": {
      "description": "Project ID or Identifier (default: DEN)",
      "type": "string"
    },
    "workspace": {
      "description": "Workspace slug (default: dendrite-ws)",
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
      "project": "DEN",
      "workspace": "dendrite-ws"
    },
    "name": "dendrite_plane_issues"
  }
}
```

### CLI

```bash
dendrite mcp call dendrite_plane_issues '{"project":"DEN","workspace":"dendrite-ws"}'
```
