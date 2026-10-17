---
id: dendrite_plane_issue_create
title: "dendrite_plane_issue_create"
sidebar_label: "dendrite_plane_issue_create"
---

# dendrite_plane_issue_create

Create a new ticket in Plane, assign it to a milestone/module, set priority, schedule start and target dates for the Gantt timeline, and establish blocker relationships to place it correctly in the development plan

## Input Schema

```json
{
  "properties": {
    "blocked_by": {
      "description": "List of ticket IDs or UUIDs that block this ticket",
      "items": {
        "type": "string"
      },
      "type": "array"
    },
    "context": {
      "description": "Alias for description",
      "type": "string"
    },
    "description": {
      "description": "Mandatory ticket description (HTML or markdown detailing goals, acceptance criteria, and DoD)",
      "type": "string"
    },
    "estimate": {
      "description": "Story points estimate (e.g. 3, 5, 8, 13)",
      "type": "integer"
    },
    "milestone": {
      "description": "Milestone / Module name or UUID to assign this ticket to",
      "type": "string"
    },
    "name": {
      "description": "Ticket title / name (e.g. '[DEN-101] Feature Name')",
      "type": "string"
    },
    "priority": {
      "description": "Priority: 'urgent', 'high', 'medium', 'low', or 'none' (default: 'medium')",
      "type": "string"
    },
    "start_date": {
      "description": "Optional start date in YYYY-MM-DD format (auto-calculated if omitted)",
      "type": "string"
    },
    "target_date": {
      "description": "Optional target date in YYYY-MM-DD format (auto-calculated if omitted)",
      "type": "string"
    }
  },
  "required": [
    "name",
    "description"
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
      "blocked_by": [
        "item1"
      ],
      "context": "sample",
      "description": "sample",
      "estimate": 1,
      "milestone": "sample",
      "name": "sample",
      "priority": "'medium'",
      "start_date": "sample",
      "target_date": "sample"
    },
    "name": "dendrite_plane_issue_create"
  }
}
```

### CLI

```bash
dendrite mcp call dendrite_plane_issue_create '{"blocked_by":["item1"],"context":"sample","description":"sample","estimate":1,"milestone":"sample","name":"sample","priority":"'medium'","start_date":"sample","target_date":"sample"}'
```
