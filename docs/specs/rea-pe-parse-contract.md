# REA: the `pe.parse` method of dendrite-core

`dendrite retro analyze` (DEN-1062, DEND-501) reads a Windows PE image through the `dendrite-core` Rust sidecar, so Dendrite keeps no PE parser of its own. The request to the core-rs project is DCORE-31. This is the contract the Go client (`pkg/coreclient/pe.go`) and the sidecar stand-in (`pkg/coreclient/coretest/standin`) already follow.

## Request

JSON-RPC 2.0 over the sidecar's stdio, method `pe.parse`:

```json
{"path": "/absolute/or/relative/path/to/sample.exe"}
```

## Result

```json
{
  "machine": "x86_64",
  "subsystem": "console",
  "pe32_plus": true,
  "entry_point": "0x140001000",
  "image_base": "0x140000000",
  "sections": [
    {"name": ".text", "virtual_address": "0x1000", "virtual_size": 4096, "raw_size": 4096, "flags": ["code", "execute", "read"]}
  ],
  "imports": [{"dll": "KERNEL32.dll", "functions": ["CreateFileW", "ReadFile"]}],
  "exports": ["ComputeChecksum"],
  "truncated": false
}
```

- Addresses are hex strings (`0x...`), so 64-bit values survive JSON.
- `machine` is `x86`, `x86_64`, `arm`, `arm64` or the numeric code in hex when unknown. `subsystem` is `console`, `gui`, `native`, ... or the number.
- `imports` and `exports` are capped (for example 2,000 names each in total); when anything was cut, `truncated` is `true`. A typical image answers in well under 4,000 tokens.
- Imports by ordinal appear as `#<ordinal>`.

## Errors

| Code | When | Go error |
|---|---|---|
| `-32601` | the sidecar predates `pe.parse` | `coreclient.ErrPEUnsupported` |
| `-32602` | the file is unreadable, shorter than a DOS header, has no `MZ` or `PE\0\0` signature, or its headers point outside the file | `coreclient.ErrNotPE` (with the message) |

A malformed or hostile file must return an error, never panic, hang or allocate without bound.

## Tests the sidecar owes

A PE32+ fixture and a PE32 fixture with the expected fields above, a file cut inside the optional header, a file with no import directory, and a file whose section table points past the end.
