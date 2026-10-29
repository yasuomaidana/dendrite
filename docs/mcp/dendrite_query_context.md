---
id: dendrite_query_context
title: "dendrite_query_context"
sidebar_label: "dendrite_query_context"
---

# dendrite_query_context

Everything needed to start on a ticket: description, acceptance criteria, plan, state and the files it names. Call it right after claiming.

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
    "name": "dendrite_query_context"
  }
}
```

### CLI

```bash
dendrite mcp call dendrite_query_context '{"ticket":"DEN-985","worktree":"the server's current directory"}'
```
