---
id: dendrite_rea_showcase
title: "dendrite_rea_showcase"
sidebar_label: "dendrite_rea_showcase"
---

# dendrite_rea_showcase

Execute automated open-source REA cleanroom analysis, evidence extraction, property test verification, and ADR generation across showcase targets (aegis, dxball, ctf, th04, mattermost, all)

## Input Schema

```json
{
  "properties": {
    "format": {
      "description": "Output representation: 'json', 'markdown', or 'summary' (default: 'summary')",
      "type": "string"
    },
    "target": {
      "description": "Showcase target: 'all', 'aegis', 'dxball', 'ctf', 'th04', 'mattermost' (default: 'all')",
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
      "format": "'summary'",
      "target": "'all'"
    },
    "name": "dendrite_rea_showcase"
  }
}
```

### CLI

```bash
dendrite mcp call dendrite_rea_showcase '{"format":"'summary'","target":"'all'"}'
```
