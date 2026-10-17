---
id: dendrite_env_prepare
title: "dendrite_env_prepare"
sidebar_label: "dendrite_env_prepare"
---

# dendrite_env_prepare

Prepares or synchronizes project environment, updates agent rules and skills, and enforces CI/CD tooling (default: GitHub Copilot)

## Input Schema

```json
{
  "properties": {
    "default_tool": {
      "description": "Default CI/CD assistant tool (default: 'copilot')",
      "type": "string"
    },
    "dry_run": {
      "description": "If true, simulates preparation without writing files",
      "type": "boolean"
    },
    "enforce_cicd": {
      "description": "Enforce CI/CD practices and GitHub Copilot configuration (default: true)",
      "type": "boolean"
    },
    "project_path": {
      "description": "Target workspace project root directory (default: current directory)",
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
      "default_tool": "'copilot'",
      "dry_run": false,
      "enforce_cicd": false,
      "project_path": "current directory"
    },
    "name": "dendrite_env_prepare"
  }
}
```

### CLI

```bash
dendrite mcp call dendrite_env_prepare '{"default_tool":"'copilot'","dry_run":false,"enforce_cicd":false,"project_path":"current directory"}'
```
