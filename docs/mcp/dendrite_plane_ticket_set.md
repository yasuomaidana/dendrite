---
id: dendrite_plane_ticket_set
title: "dendrite_plane_ticket_set"
sidebar_label: "dendrite_plane_ticket_set"
---

# dendrite_plane_ticket_set

Set a ticket's workflow state in Plane (backlog, todo, in progress, to verify, done). Plane is the owner of ticket state; completed work goes to 'to verify' for human sign-off, not 'done'.

## Input Schema

```json
{
  "properties": {
    "status": {
      "description": "backlog, todo, in progress, to verify or done",
      "type": "string"
    },
    "ticket": {
      "description": "Ticket reference: DEN-324, its Plane number (324) or the issue UUID",
      "type": "string"
    }
  },
  "required": [
    "ticket",
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
      "status": "DONE",
      "ticket": "DEN-985"
    },
    "name": "dendrite_plane_ticket_set"
  }
}
```

### CLI

```bash
dendrite mcp call dendrite_plane_ticket_set '{"status":"DONE","ticket":"DEN-985"}'
```
