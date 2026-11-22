---
id: dendrite_rea_handoff
title: "dendrite_rea_handoff"
sidebar_label: "dendrite_rea_handoff"
---

# dendrite_rea_handoff

Hand a reverse-engineering (REA) ticket you refuse to do to AGY instead of leaving it half done: records the refusal on the ticket, starts an AGY job in an isolated working copy, labels the ticket owner:agy and releases this session's claim. Repeating it for a ticket AGY already owns changes nothing; a ticket that is not REA work is refused.

## Input Schema

```json
{
  "properties": {
    "reason": {
      "description": "Why you refuse, in a sentence; kept on the ticket",
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
      "reason": "sample",
      "ticket": "DEN-985",
      "worktree": "the server's current directory"
    },
    "name": "dendrite_rea_handoff"
  }
}
```

### CLI

```bash
dendrite mcp call dendrite_rea_handoff '{"reason":"sample","ticket":"DEN-985","worktree":"the server's current directory"}'
```
