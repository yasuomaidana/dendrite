---
id: dendrite_ticket_update
title: "dendrite_ticket_update"
sidebar_label: "dendrite_ticket_update"
---

# dendrite_ticket_update

Update the status of a project ticket in DEV_STATUS.md (e.g. DONE, IN_PROGRESS, TODO)

## Input Schema

```json
{
  "properties": {
    "file": {
      "description": "Path to DEV_STATUS.md (default: DEV_STATUS.md)",
      "type": "string"
    },
    "status": {
      "description": "New status, e.g. DONE, IN_PROGRESS, TODO",
      "type": "string"
    },
    "ticket_id": {
      "description": "The ticket identifier, e.g. DEN-034",
      "type": "string"
    }
  },
  "required": [
    "ticket_id",
    "status"
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
      "file": "DEV_STATUS.md",
      "status": "DONE",
      "ticket_id": "DEN-985"
    },
    "name": "dendrite_ticket_update"
  }
}
```

### CLI

```bash
dendrite mcp call dendrite_ticket_update '{"file":"DEV_STATUS.md","status":"DONE","ticket_id":"DEN-985"}'
```
