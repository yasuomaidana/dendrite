---
id: dendrite_ticket_transition
title: "dendrite_ticket_transition"
sidebar_label: "dendrite_ticket_transition"
---

# dendrite_ticket_transition

End or pause work on a ticket: verify (finished, awaiting a human), blocked (needs a handoff with completed, challenges and missing), cancelled or backlog. done is set only by `dendrite chronos finish`.

## Input Schema

```json
{
  "properties": {
    "handoff": {
      "description": "Required for blocked: what is done, what got in the way, what is missing against the acceptance criteria",
      "properties": {
        "challenges": {
          "type": "string"
        },
        "completed": {
          "type": "string"
        },
        "missing": {
          "type": "string"
        }
      },
      "type": "object"
    },
    "outcome": {
      "description": "verify, blocked, cancelled, backlog",
      "type": "string"
    },
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
    "ticket",
    "outcome"
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
      "handoff": {},
      "outcome": "sample",
      "ticket": "DEN-985",
      "worktree": "the server's current directory"
    },
    "name": "dendrite_ticket_transition"
  }
}
```

### CLI

```bash
dendrite mcp call dendrite_ticket_transition '{"handoff":{},"outcome":"sample","ticket":"DEN-985","worktree":"the server's current directory"}'
```
