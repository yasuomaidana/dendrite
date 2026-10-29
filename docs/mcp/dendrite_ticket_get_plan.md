---
id: dendrite_ticket_get_plan
title: "dendrite_ticket_get_plan"
sidebar_label: "dendrite_ticket_get_plan"
---

# dendrite_ticket_get_plan

Read the plan of a ticket: from Plane, or from the local plan store when Plane is down. Returns the markdown, its version and the next unchecked step.

## Input Schema

```json
{
  "properties": {
    "ticket": {
      "description": "Ticket reference: DEN-377, its Plane number or the issue UUID",
      "type": "string"
    },
    "worktree": {
      "description": "Working copy the claim unlocks (default: the server's current directory)",
      "type": "string"
    }
  },
  "required": [
    "ticket"
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
      "ticket": "DEN-985",
      "worktree": "the server's current directory"
    },
    "name": "dendrite_ticket_get_plan"
  }
}
```

### CLI

```bash
dendrite mcp call dendrite_ticket_get_plan '{"ticket":"DEN-985","worktree":"the server's current directory"}'
```
