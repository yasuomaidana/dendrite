---
id: dendrite_quality_audit
title: "dendrite_quality_audit"
sidebar_label: "dendrite_quality_audit"
---

# dendrite_quality_audit

Execute anti-mock static scan and 100% coverage profiler against a package.

## Input Schema

```json
{
  "properties": {
    "pkg": {
      "description": "Package path to audit (e.g. pkg/vcs)",
      "type": "string"
    }
  },
  "required": [
    "pkg"
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
      "pkg": "sample"
    },
    "name": "dendrite_quality_audit"
  }
}
```

### CLI

```bash
dendrite mcp call dendrite_quality_audit '{"pkg":"sample"}'
```
