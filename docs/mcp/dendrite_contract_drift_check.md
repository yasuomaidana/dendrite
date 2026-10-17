---
id: dendrite_contract_drift_check
title: "dendrite_contract_drift_check"
sidebar_label: "dendrite_contract_drift_check"
---

# dendrite_contract_drift_check

Analyze cross-language schema and IPC contract drift across Protobuf, OpenAPI, and JSON-Schema definitions against Go, Rust, TS, and Python models in Quad-IDE projects

## Input Schema

```json
{
  "properties": {
    "code_dirs": {
      "description": "Map of target languages ('go', 'rust', 'typescript', 'python') to repository source directories",
      "type": "object"
    },
    "schema_dirs": {
      "description": "List of directories containing .proto, openapi.yaml, or .json schema definitions",
      "items": {
        "type": "string"
      },
      "type": "array"
    }
  },
  "required": [
    "schema_dirs"
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
      "code_dirs": {},
      "schema_dirs": [
        "item1"
      ]
    },
    "name": "dendrite_contract_drift_check"
  }
}
```

### CLI

```bash
dendrite mcp call dendrite_contract_drift_check '{"code_dirs":{},"schema_dirs":["item1"]}'
```
