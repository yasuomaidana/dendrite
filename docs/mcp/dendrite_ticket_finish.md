---
id: dendrite_ticket_finish
title: "dendrite_ticket_finish"
sidebar_label: "dendrite_ticket_finish"
---

# dendrite_ticket_finish

Finish a ticket: run the quality gate and check the branch's diff against each [AC-n]. Sets Done when the project allows it and the ticket has no needs-human-review label, To Verify otherwise, and Blocked with a handoff when a check fails.

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
    "name": "dendrite_ticket_finish"
  }
}
```

### CLI

```bash
dendrite mcp call dendrite_ticket_finish '{"ticket":"DEN-985","worktree":"the server's current directory"}'
```
