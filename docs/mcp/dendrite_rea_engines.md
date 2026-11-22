---
id: dendrite_rea_engines
title: "dendrite_rea_engines"
sidebar_label: "dendrite_rea_engines"
---

# dendrite_rea_engines

List the reverse-engineering engines (open-source: rizin, radare2, objdump, Ghidra; proprietary and documented only: Hopper, IDA, Binary Ninja) with their license, capabilities and whether each is usable in the container or on the host here.

## Input Schema

```json
{
  "properties": {},
  "required": null,
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
    "arguments": {},
    "name": "dendrite_rea_engines"
  }
}
```

### CLI

```bash
dendrite mcp call dendrite_rea_engines
```
