---
id: dendrite_synapse_peers
title: "dendrite_synapse_peers"
sidebar_label: "dendrite_synapse_peers"
---

# dendrite_synapse_peers

Lists, per synapse that contains the repository, its other members: alias, role, Plane project identifier, where the checkout resolves and its health. An empty list means the repository is in no synapse. To ask a peer for a feature, file a ticket in its Plane project with `dendrite chronos request <peer> --name ... --desc ...`

## Input Schema

```json
{
  "properties": {
    "dir": {
      "description": "Directory inside the repository; default the server's working directory",
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
      "dir": "sample"
    },
    "name": "dendrite_synapse_peers"
  }
}
```

### CLI

```bash
dendrite mcp call dendrite_synapse_peers '{"dir":"sample"}'
```
