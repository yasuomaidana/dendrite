---
id: dendrite_query_reuse_audit
title: "dendrite_query_reuse_audit"
sidebar_label: "dendrite_query_reuse_audit"
---

# dendrite_query_reuse_audit

Evaluates strict 3-tier library reuse hierarchy (Source Refs > Pool Members > Ecosystem Registries) and emits an Architecture Decision Record (ADR)

## Input Schema

```json
{
  "properties": {
    "capability": {
      "description": "Capability or domain required (e.g. linear_algebra, serialization, kinematics)",
      "type": "string"
    },
    "intent": {
      "description": "Contextual intent or user prompt description",
      "type": "string"
    },
    "language": {
      "description": "Target programming language (e.g. rust, go, typescript, python)",
      "type": "string"
    },
    "ticket_id": {
      "description": "Associated Plane ticket identifier (e.g. DEN-058)",
      "type": "string"
    }
  },
  "required": [
    "capability"
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
      "capability": "sample",
      "intent": "sample",
      "language": "sample",
      "ticket_id": "DEN-985"
    },
    "name": "dendrite_query_reuse_audit"
  }
}
```

### CLI

```bash
dendrite mcp call dendrite_query_reuse_audit '{"capability":"sample","intent":"sample","language":"sample","ticket_id":"DEN-985"}'
```
