---
id: dendrite_plane_relation_unlink
title: "dendrite_plane_relation_unlink"
sidebar_label: "dendrite_plane_relation_unlink"
---

# dendrite_plane_relation_unlink

Remove one dependency relationship between tickets in Plane (relation_type: 'blocking', 'blocked_by', 'relates_to', 'duplicate'); the reverse of dendrite_plane_relation_link

## Input Schema

```json
{
  "properties": {
    "issue_id": {
      "description": "Source ticket ID or UUID (e.g. 'DEN-036' or issue UUID)",
      "type": "string"
    },
    "relation_type": {
      "description": "Relation type to remove: 'blocking', 'blocked_by', 'relates_to', or 'duplicate'",
      "type": "string"
    },
    "target_issue_id": {
      "description": "Target ticket ID or UUID the relation points at",
      "type": "string"
    }
  },
  "required": [
    "issue_id",
    "relation_type",
    "target_issue_id"
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
      "issue_id": "sample",
      "relation_type": "sample",
      "target_issue_id": "sample"
    },
    "name": "dendrite_plane_relation_unlink"
  }
}
```

### CLI

```bash
dendrite mcp call dendrite_plane_relation_unlink '{"issue_id":"sample","relation_type":"sample","target_issue_id":"sample"}'
```
