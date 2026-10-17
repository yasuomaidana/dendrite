---
id: dendrite_service_plan
title: "dendrite_service_plan"
sidebar_label: "dendrite_service_plan"
---

# dendrite_service_plan

Evaluates hardware capacity (RAM and Disk) and returns which services can live on a Debian server (e.g. 4 GB RAM with 1 TB Disk)

## Input Schema

```json
{
  "properties": {
    "disk_gb": {
      "description": "Server Disk in GB (default: 1000)",
      "type": "integer"
    },
    "ram_mb": {
      "description": "Server RAM in MB (default: 4096)",
      "type": "integer"
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
      "disk_gb": 1,
      "ram_mb": 1
    },
    "name": "dendrite_service_plan"
  }
}
```

### CLI

```bash
dendrite mcp call dendrite_service_plan '{"disk_gb":1,"ram_mb":1}'
```
