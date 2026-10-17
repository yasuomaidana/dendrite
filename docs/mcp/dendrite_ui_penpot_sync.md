---
id: dendrite_ui_penpot_sync
title: "dendrite_ui_penpot_sync"
sidebar_label: "dendrite_ui_penpot_sync"
---

# dendrite_ui_penpot_sync

Synchronize UI component development (.svelte, .tsx, .vue, .css) with Penpot design frames and post deep-links to Plane issues

## Input Schema

```json
{
  "properties": {
    "comment_plane": {
      "description": "If true, posts deep-link comment to Plane issue (default: true)",
      "type": "boolean"
    },
    "dry_run": {
      "description": "If true, simulates synchronization without modifying Plane or Penpot",
      "type": "boolean"
    },
    "file": {
      "description": "Path to UI component file (e.g. 'web/src/components/Button.svelte')",
      "type": "string"
    },
    "penpot_endpoint": {
      "description": "Penpot service endpoint (default: 'http://localhost:9000')",
      "type": "string"
    },
    "ticket_id": {
      "description": "Target Plane ticket ID (e.g. 'DEN-810')",
      "type": "string"
    }
  },
  "required": [
    "file"
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
      "comment_plane": false,
      "dry_run": false,
      "file": "DEV_STATUS.md",
      "penpot_endpoint": "'http://localhost:9000'",
      "ticket_id": "DEN-985"
    },
    "name": "dendrite_ui_penpot_sync"
  }
}
```

### CLI

```bash
dendrite mcp call dendrite_ui_penpot_sync '{"comment_plane":false,"dry_run":false,"file":"DEV_STATUS.md","penpot_endpoint":"'http://localhost:9000'","ticket_id":"DEN-985"}'
```
