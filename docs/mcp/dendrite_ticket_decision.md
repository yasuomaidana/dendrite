---
id: dendrite_ticket_decision
title: "dendrite_ticket_decision"
sidebar_label: "dendrite_ticket_decision"
---

# dendrite_ticket_decision

Record a question you asked the user and the answer you got, or a decision you took, on a ticket. Do it for every decision that shapes the work; Claude Code's AskUserQuestion is recorded by a hook, other agents call this.

## Input Schema

```json
{
  "properties": {
    "answer": {
      "description": "The answer or decision",
      "type": "string"
    },
    "note": {
      "description": "Why, or what follows from it",
      "type": "string"
    },
    "options": {
      "description": "Options that were on the table",
      "items": {
        "type": "string"
      },
      "type": "array"
    },
    "question": {
      "description": "The question that was asked",
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
    "question",
    "answer"
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
      "answer": "sample",
      "note": "sample",
      "options": [
        "item1"
      ],
      "question": "sample",
      "ticket": "DEN-985",
      "worktree": "the server's current directory"
    },
    "name": "dendrite_ticket_decision"
  }
}
```

### CLI

```bash
dendrite mcp call dendrite_ticket_decision '{"answer":"sample","note":"sample","options":["item1"],"question":"sample","ticket":"DEN-985","worktree":"the server's current directory"}'
```
