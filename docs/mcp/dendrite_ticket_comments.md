---
id: dendrite_ticket_comments
title: "dendrite_ticket_comments"
sidebar_label: "dendrite_ticket_comments"
---

# dendrite_ticket_comments

Read the comments of a ticket: claims, progress, decisions, handoffs and plans. Filter by kind, or take only the last N. Use it to read a handoff or the decisions already taken before you start.

## Input Schema

```json
{
  "properties": {
    "kind": {
      "description": "claim, progress, milestone, note, handoff, decision or plan",
      "type": "string"
    },
    "last": {
      "description": "Only the last N comments",
      "type": "number"
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
      "kind": "sample",
      "last": 1,
      "ticket": "DEN-985",
      "worktree": "the server's current directory"
    },
    "name": "dendrite_ticket_comments"
  }
}
```

### CLI

```bash
dendrite mcp call dendrite_ticket_comments '{"kind":"sample","last":1,"ticket":"DEN-985","worktree":"the server's current directory"}'
```
