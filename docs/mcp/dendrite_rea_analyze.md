---
id: dendrite_rea_analyze
title: "dendrite_rea_analyze"
sidebar_label: "dendrite_rea_analyze"
---

# dendrite_rea_analyze

Analyze any target (PE, ELF, Mach-O, ASAR/ZIP package, JS bundle, unknown file or a directory of source): inspect it, write its structure as documents and index them. Returns an evidence bundle under 4,000 tokens and an analysis id; read more with dendrite_rea_search instead of dumping the target again. A paused analysis is returned with paused=true.

## Input Schema

```json
{
  "properties": {
    "binary": {
      "description": "Path of the file or directory to analyze",
      "type": "string"
    },
    "engine": {
      "description": "Engine to run over the target: auto (best usable open-source one), none, rizin, radare2 or objdump (default: the [rea] directive)",
      "type": "string"
    },
    "format": {
      "description": "Format the target must have (default: detect it): pe, elf, macho, fat-macho, asar, zip, js-bundle, source-tree, unknown",
      "type": "string"
    },
    "id": {
      "description": "Name of the analysis (default: generated)",
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
    "ticket": {
      "description": "Leave a note with the summary and the analysis id on this ticket",
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
    "binary"
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
      "binary": "sample",
      "engine": "the [rea] directive",
      "format": "detect it): pe, elf, macho, fat-macho, asar, zip, js-bundle, source-tree, unknown",
      "id": "generated",
      "model": "sample",
      "sandbox": "sample",
      "ticket": "DEN-985",
      "tier": "sample",
      "trusted": false
    },
    "name": "dendrite_rea_analyze"
  }
}
```

### CLI

```bash
dendrite mcp call dendrite_rea_analyze '{"binary":"sample","engine":"the [rea] directive","format":"detect it): pe, elf, macho, fat-macho, asar, zip, js-bundle, source-tree, unknown","id":"generated","model":"sample","sandbox":"sample","ticket":"DEN-985","tier":"sample","trusted":false}'
```
