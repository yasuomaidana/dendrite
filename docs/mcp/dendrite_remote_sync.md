---
id: dendrite_remote_sync
title: "dendrite_remote_sync"
sidebar_label: "dendrite_remote_sync"
---

# dendrite_remote_sync

Fetch and detect remote commits/hotfixes, pull with rebase, and reconcile tickets.

## Input Schema

```json
{
  "properties": {
    "branch": {
      "description": "Branch to sync (default: active branch)",
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
      "branch": "active branch"
    },
    "name": "dendrite_remote_sync"
  }
}
```

### CLI

```bash
dendrite mcp call dendrite_remote_sync '{"branch":"active branch"}'
```
