---
id: dendrite_plane_replicate
title: "dendrite_plane_replicate"
sidebar_label: "dendrite_plane_replicate"
---

# dendrite_plane_replicate

Replicate DEV_STATUS.md directly from Plane service state (Plane owns status)

## Input Schema

```json
{
  "properties": {
    "dry_run": {
      "description": "If true, simulates replication without modifying disk",
      "type": "boolean"
    },
    "file": {
      "description": "Target markdown file path (default: DEV_STATUS.md)",
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
      "dry_run": false,
      "file": "DEV_STATUS.md"
    },
    "name": "dendrite_plane_replicate"
  }
}
```

### CLI

```bash
dendrite mcp call dendrite_plane_replicate '{"dry_run":false,"file":"DEV_STATUS.md"}'
```
