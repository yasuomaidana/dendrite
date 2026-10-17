---
id: dendrite_canvas_penpot_bridge
title: "dendrite_canvas_penpot_bridge"
sidebar_label: "dendrite_canvas_penpot_bridge"
---

# dendrite_canvas_penpot_bridge

Relate Plane tickets and UI project functionalities to Penpot visual design frames and execute bidirectional project state synchronization

## Input Schema

```json
{
  "properties": {
    "comment": {
      "description": "Optional comment or note to attach to Plane issue",
      "type": "string"
    },
    "dry_run": {
      "description": "If true, simulates synchronization without modifying remote services",
      "type": "boolean"
    },
    "feature_name": {
      "description": "Project feature or functionality name (e.g. 'Canvas Workspace UI')",
      "type": "string"
    },
    "file_path": {
      "description": "Path to UI component file (e.g. 'web/src/Canvas.svelte')",
      "type": "string"
    },
    "penpot_frame_url": {
      "description": "Custom Penpot frame URL or deep-link",
      "type": "string"
    },
    "push_to_plane": {
      "description": "If true, synchronizes state mutation directly to Plane v2 (default: true)",
      "type": "boolean"
    },
    "subaction": {
      "description": "Operation to perform: 'relate', 'sync', 'update_state', or 'list'",
      "type": "string"
    },
    "target_state": {
      "description": "Target project state for transition (e.g. 'In Progress', 'To Verify', 'Done')",
      "type": "string"
    },
    "ticket_id": {
      "description": "Target Plane ticket ID (e.g. 'DEN-807')",
      "type": "string"
    }
  },
  "required": [
    "subaction"
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
      "comment": "sample",
      "dry_run": false,
      "feature_name": "sample",
      "file_path": "DEV_STATUS.md",
      "penpot_frame_url": "sample",
      "push_to_plane": false,
      "subaction": "sample",
      "target_state": "sample",
      "ticket_id": "DEN-985"
    },
    "name": "dendrite_canvas_penpot_bridge"
  }
}
```

### CLI

```bash
dendrite mcp call dendrite_canvas_penpot_bridge '{"comment":"sample","dry_run":false,"feature_name":"sample","file_path":"DEV_STATUS.md","penpot_frame_url":"sample","push_to_plane":false,"subaction":"sample","target_state":"sample","ticket_id":"DEN-985"}'
```
