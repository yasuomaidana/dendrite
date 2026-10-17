---
id: dendrite_dist_decompose
title: "dendrite_dist_decompose"
sidebar_label: "dendrite_dist_decompose"
---

# dendrite_dist_decompose

Forcibly decompose target project or REA evidence into reusable modules and map ecosystem libraries

## Input Schema

```json
{
  "properties": {
    "language": {
      "description": "Target language (default: rust)",
      "type": "string"
    },
    "modules": {
      "description": "Custom module names if overriding default modular split",
      "items": {
        "type": "string"
      },
      "type": "array"
    },
    "project_name": {
      "description": "Name of the project to decompose",
      "type": "string"
    },
    "target_binary_or_spec": {
      "description": "Target binary path or specification file",
      "type": "string"
    },
    "user_custom_overrides": {
      "description": "Modules where user explicitly forces in-house custom development",
      "items": {
        "type": "string"
      },
      "type": "array"
    }
  },
  "required": [
    "project_name"
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
      "language": "rust",
      "modules": [
        "item1"
      ],
      "project_name": "sample",
      "target_binary_or_spec": "sample",
      "user_custom_overrides": [
        "item1"
      ]
    },
    "name": "dendrite_dist_decompose"
  }
}
```

### CLI

```bash
dendrite mcp call dendrite_dist_decompose '{"language":"rust","modules":["item1"],"project_name":"sample","target_binary_or_spec":"sample","user_custom_overrides":["item1"]}'
```
