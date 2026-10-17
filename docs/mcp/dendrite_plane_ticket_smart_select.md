---
id: dendrite_plane_ticket_smart_select
title: "dendrite_plane_ticket_smart_select"
sidebar_label: "dendrite_plane_ticket_smart_select"
---

# dendrite_plane_ticket_smart_select

Smart selects the optimal unblocked ticket from Plane v2 project management container respecting priority and blocker relationships.

## Input Schema

```json
{
  "properties": {
    "milestone": {
      "description": "Optional milestone filter",
      "type": "string"
    },
    "ticket_id": {
      "description": "Optional specific ticket ID requested by user (e.g. 'DEN-036')",
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
      "milestone": "sample",
      "ticket_id": "DEN-985"
    },
    "name": "dendrite_plane_ticket_smart_select"
  }
}
```

### CLI

```bash
dendrite mcp call dendrite_plane_ticket_smart_select '{"milestone":"sample","ticket_id":"DEN-985"}'
```
