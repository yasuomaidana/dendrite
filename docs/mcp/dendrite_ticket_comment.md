---
id: dendrite_ticket_comment
title: "dendrite_ticket_comment"
sidebar_label: "dendrite_ticket_comment"
---

# dendrite_ticket_comment

Post a structured progress, milestone, note or handoff comment on a ticket (marker, text, and a footer naming session, agent, tier, branch and commit).

## Input Schema

```json
{
  "properties": {
    "body": {
      "description": "The comment text",
      "type": "string"
    },
    "kind": {
      "description": "progress, milestone, note, handoff",
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
    "kind",
    "body"
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
      "body": "sample",
      "kind": "sample",
      "ticket": "DEN-985",
      "worktree": "the server's current directory"
    },
    "name": "dendrite_ticket_comment"
  }
}
```

### CLI

```bash
dendrite mcp call dendrite_ticket_comment '{"body":"sample","kind":"sample","ticket":"DEN-985","worktree":"the server's current directory"}'
```
