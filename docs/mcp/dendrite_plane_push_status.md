---
id: dendrite_plane_push_status
title: "dendrite_plane_push_status"
sidebar_label: "dendrite_plane_push_status"
---

# dendrite_plane_push_status

Move local DEV_STATUS.md content (tickets, milestones, descriptions) to Plane service

## Input Schema

```json
{
  "properties": {
    "file": {
      "description": "Source markdown file path (default: DEV_STATUS.md)",
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
      "file": "DEV_STATUS.md"
    },
    "name": "dendrite_plane_push_status"
  }
}
```

### CLI

```bash
dendrite mcp call dendrite_plane_push_status '{"file":"DEV_STATUS.md"}'
```
