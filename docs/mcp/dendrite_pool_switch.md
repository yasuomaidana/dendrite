---
id: dendrite_pool_switch
title: "dendrite_pool_switch"
sidebar_label: "dendrite_pool_switch"
---

# dendrite_pool_switch

Atomically transition a dependency mode across Published, Branch, Local, or Fork with JetBrains scope sync

## Input Schema

```json
{
  "properties": {
    "branch_name": {
      "description": "Branch name (for branch mode)",
      "type": "string"
    },
    "commit_sha": {
      "description": "Commit SHA (optional pin for branch mode)",
      "type": "string"
    },
    "dependency_name": {
      "description": "Name or module path of target dependency",
      "type": "string"
    },
    "fork_branch": {
      "description": "Remote branch on fork (optional for fork mode)",
      "type": "string"
    },
    "fork_url": {
      "description": "Remote git fork URL (required for fork mode)",
      "type": "string"
    },
    "local_path": {
      "description": "Relative or absolute filesystem path (required for local mode)",
      "type": "string"
    },
    "mode": {
      "description": "Target dependency mode",
      "enum": [
        "published",
        "branch",
        "local",
        "fork"
      ],
      "type": "string"
    },
    "sync_ide": {
      "description": "Whether to synchronize JetBrains IDE scopes (.idea/scopes)",
      "type": "boolean"
    },
    "target_project_dir": {
      "description": "Target project directory containing manifests",
      "type": "string"
    },
    "version": {
      "description": "Version string (required for published mode)",
      "type": "string"
    }
  },
  "required": [
    "target_project_dir",
    "dependency_name",
    "mode"
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
      "branch_name": "sample",
      "commit_sha": "sample",
      "dependency_name": "sample",
      "fork_branch": "sample",
      "fork_url": "sample",
      "local_path": "DEV_STATUS.md",
      "mode": "sample",
      "sync_ide": false,
      "target_project_dir": "sample",
      "version": "sample"
    },
    "name": "dendrite_pool_switch"
  }
}
```

### CLI

```bash
dendrite mcp call dendrite_pool_switch '{"branch_name":"sample","commit_sha":"sample","dependency_name":"sample","fork_branch":"sample","fork_url":"sample","local_path":"DEV_STATUS.md","mode":"sample","sync_ide":false,"target_project_dir":"sample","version":"sample"}'
```
