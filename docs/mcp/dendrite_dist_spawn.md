---
id: dendrite_dist_spawn
title: "dendrite_dist_spawn"
sidebar_label: "dendrite_dist_spawn"
---

# dendrite_dist_spawn

Spawn parallel sub-projects or generative satellite projects with git, scaffolding, and Plane integration

## Input Schema

```json
{
  "properties": {
    "create_plane_project": {
      "description": "Whether to create a dedicated project in Plane v2",
      "type": "boolean"
    },
    "description": {
      "description": "Description of the sub-project",
      "type": "string"
    },
    "identifier": {
      "description": "Short code identifier (e.g. CSEN)",
      "type": "string"
    },
    "init_git": {
      "description": "Whether to run git init (default: true)",
      "type": "boolean"
    },
    "is_satellite": {
      "description": "True if spawned as a side-project (e.g. Carla Rust rewrite)",
      "type": "boolean"
    },
    "language": {
      "description": "Project language (rust, go, typescript, python)",
      "type": "string"
    },
    "project_name": {
      "description": "Name of the sub-project or satellite project",
      "type": "string"
    },
    "target_dir": {
      "description": "Target filesystem directory path",
      "type": "string"
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
      "create_plane_project": false,
      "description": "sample",
      "identifier": "sample",
      "init_git": false,
      "is_satellite": false,
      "language": "sample",
      "project_name": "sample",
      "target_dir": "sample"
    },
    "name": "dendrite_dist_spawn"
  }
}
```

### CLI

```bash
dendrite mcp call dendrite_dist_spawn '{"create_plane_project":false,"description":"sample","identifier":"sample","init_git":false,"is_satellite":false,"language":"sample","project_name":"sample","target_dir":"sample"}'
```
