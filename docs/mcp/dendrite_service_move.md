---
id: dendrite_service_move
title: "dendrite_service_move"
sidebar_label: "dendrite_service_move"
---

# dendrite_service_move

Migrates containerized Dendrite services (Plane v2, LiteLLM, Database, Penpot, or 'debian' bundle) to a remote Debian SSH server with zero-credential authorization, ephemeral streaming, health verification, and atomic cutover

## Input Schema

```json
{
  "properties": {
    "dry_run": {
      "description": "If true, simulates validation without transferring data or modifying containers",
      "type": "boolean"
    },
    "prune_local": {
      "description": "If true, safely prunes local containers after successful remote health check (default: true)",
      "type": "boolean"
    },
    "services": {
      "description": "List of services to migrate (e.g. ['plane'], ['plane', 'litellm', 'database'], or ['debian'])",
      "items": {
        "type": "string"
      },
      "type": "array"
    },
    "ssh_port": {
      "description": "SSH port on target host (default: 22)",
      "type": "integer"
    },
    "ssh_user": {
      "description": "SSH username (default: derived from target_host or deployer)",
      "type": "string"
    },
    "target_disk_gb": {
      "description": "Target server Disk in GB (default: 1000 for 1 TB)",
      "type": "integer"
    },
    "target_host": {
      "description": "Target remote SSH host (e.g. 'user@debian-server' or '192.168.1.50')",
      "type": "string"
    },
    "target_ram_mb": {
      "description": "Target server RAM in MB (default: 4096 for 4 GB)",
      "type": "integer"
    },
    "timeout_seconds": {
      "description": "Remote health check timeout in seconds (default: 120)",
      "type": "integer"
    }
  },
  "required": [
    "target_host"
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
      "dry_run": false,
      "prune_local": false,
      "services": [
        "item1"
      ],
      "ssh_port": 1,
      "ssh_user": "derived from target_host or deployer",
      "target_disk_gb": 1,
      "target_host": "sample",
      "target_ram_mb": 1,
      "timeout_seconds": 1
    },
    "name": "dendrite_service_move"
  }
}
```

### CLI

```bash
dendrite mcp call dendrite_service_move '{"dry_run":false,"prune_local":false,"services":["item1"],"ssh_port":1,"ssh_user":"derived from target_host or deployer","target_disk_gb":1,"target_host":"sample","target_ram_mb":1,"timeout_seconds":1}'
```
