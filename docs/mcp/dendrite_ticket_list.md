---
id: dendrite_ticket_list
title: "dendrite_ticket_list"
sidebar_label: "dendrite_ticket_list"
---

# dendrite_ticket_list

List all project tickets, their statuses, complexity scores, and milestones from DEV_STATUS.md

## Input Schema

```json
{
  "properties": {
    "file": {
      "description": "Path to DEV_STATUS.md (default: DEV_STATUS.md)",
      "type": "string"
    }
  },
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
      "file": "DEV_STATUS.md"
    },
    "name": "dendrite_ticket_list"
  }
}
```

### CLI

```bash
dendrite mcp call dendrite_ticket_list '{"file":"DEV_STATUS.md"}'
```
