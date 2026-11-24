---
id: dendrite_ticket_parent
title: "dendrite_ticket_parent"
sidebar_label: "dendrite_ticket_parent"
---

# dendrite_ticket_parent

Make a ticket a sub-ticket of another in Plane (parent), or remove its parent (clear). Give exactly one of them. A parent that would make a loop is refused; clearing needs dendrite-plane protocol 1.5 or newer.

## Input Schema

```json
{
  "properties": {
    "clear": {
      "description": "Remove the parent of the ticket instead",
      "type": "boolean"
    },
    "parent": {
      "description": "The ticket that becomes the parent",
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
      "clear": false,
      "parent": "sample",
      "ticket": "DEN-985",
      "worktree": "the server's current directory"
    },
    "name": "dendrite_ticket_parent"
  }
}
```

### CLI

```bash
dendrite mcp call dendrite_ticket_parent '{"clear":false,"parent":"sample","ticket":"DEN-985","worktree":"the server's current directory"}'
```
