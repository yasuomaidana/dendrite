---
id: dendrite_rea_resume
title: "dendrite_rea_resume"
sidebar_label: "dendrite_rea_resume"
---

# dendrite_rea_resume

Continue an analysis from its last checkpoint, optionally on another model tier or model.

## Input Schema

```json
{
  "properties": {
    "engine": {
      "description": "Engine to run over the target: auto (best usable open-source one), none, rizin, radare2 or objdump (default: the [rea] directive)",
      "type": "string"
    },
    "id": {
      "description": "Analysis id, as printed by dendrite_rea_analyze",
      "type": "string"
    },
    "model": {
      "description": "Model to use on that tier",
      "type": "string"
    },
    "sandbox": {
      "description": "Where the engine runs: container (default) or host",
      "type": "string"
    },
    "tier": {
      "description": "Model tier: local or paid",
      "type": "string"
    },
    "trusted": {
      "description": "The target is safe to open on the host (needed for a binary with sandbox=host)",
      "type": "boolean"
    }
  },
  "required": [
    "id"
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
      "engine": "the [rea] directive",
      "id": "sample",
      "model": "sample",
      "sandbox": "sample",
      "tier": "sample",
      "trusted": false
    },
    "name": "dendrite_rea_resume"
  }
}
```

### CLI

```bash
dendrite mcp call dendrite_rea_resume '{"engine":"the [rea] directive","id":"sample","model":"sample","sandbox":"sample","tier":"sample","trusted":false}'
```
