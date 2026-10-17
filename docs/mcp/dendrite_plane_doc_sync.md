---
id: dendrite_plane_doc_sync
title: "dendrite_plane_doc_sync"
sidebar_label: "dendrite_plane_doc_sync"
---

# dendrite_plane_doc_sync

Synchronize documentation between Plane pages and local repository (direction: to-plane, from-plane, bidirectional)

## Input Schema

```json
{
  "properties": {
    "dir": {
      "description": "Documentation directory path (default: docs)",
      "type": "string"
    },
    "direction": {
      "description": "Sync direction: 'to-plane', 'from-plane', or 'bidirectional' (default: bidirectional)",
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
      "dir": "docs",
      "direction": "bidirectional"
    },
    "name": "dendrite_plane_doc_sync"
  }
}
```

### CLI

```bash
dendrite mcp call dendrite_plane_doc_sync '{"dir":"docs","direction":"bidirectional"}'
```
