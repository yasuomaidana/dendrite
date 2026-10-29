---
id: dendrite_ticket_claim
title: "dendrite_ticket_claim"
sidebar_label: "dendrite_ticket_claim"
---

# dendrite_ticket_claim

Claim a ticket for this session before changing any file: moves it to In Progress, assigns it, labels it agent:/tier:, records the claim that unlocks edits in this working copy and returns the ticket's plan, if any. Refused when the ticket is finished or held by another live session.

## Input Schema

```json
{
  "properties": {
    "agent": {
      "description": "claude, agy, copilot, ... (default: detected)",
      "type": "string"
    },
    "model_tier": {
      "description": "Model tier of this session (default: DENDRITE_MODEL_TIER)",
      "type": "string"
    },
    "steal": {
      "description": "Take the ticket over from another live session",
      "type": "boolean"
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
      "agent": "detected",
      "model_tier": "DENDRITE_MODEL_TIER",
      "steal": false,
      "ticket": "DEN-985",
      "worktree": "the server's current directory"
    },
    "name": "dendrite_ticket_claim"
  }
}
```

### CLI

```bash
dendrite mcp call dendrite_ticket_claim '{"agent":"detected","model_tier":"DENDRITE_MODEL_TIER","steal":false,"ticket":"DEN-985","worktree":"the server's current directory"}'
```
