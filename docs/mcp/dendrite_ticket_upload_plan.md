---
id: dendrite_ticket_upload_plan
title: "dendrite_ticket_upload_plan"
sidebar_label: "dendrite_ticket_upload_plan"
---

# dendrite_ticket_upload_plan

Store the plan of a ticket (markdown with `- [ ]` / `- [x]` steps) in Plane, as one comment edited in place, and in the local plan store. Call it when the plan changes so a resumed session continues from the next unchecked step.

## Input Schema

```json
{
  "properties": {
    "markdown": {
      "description": "The plan, with `- [ ]` and `- [x]` steps",
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
    "markdown"
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
      "markdown": "sample",
      "ticket": "DEN-985",
      "worktree": "the server's current directory"
    },
    "name": "dendrite_ticket_upload_plan"
  }
}
```

### CLI

```bash
dendrite mcp call dendrite_ticket_upload_plan '{"markdown":"sample","ticket":"DEN-985","worktree":"the server's current directory"}'
```
