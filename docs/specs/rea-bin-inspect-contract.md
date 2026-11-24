# REA: the `bin.inspect` method of dendrite-core

`dendrite retro analyze` reads any target through the `dendrite-core` Rust sidecar, so Dendrite keeps no format parser of its own. `bin.inspect` is the format-neutral successor of `pe.parse` (DCORE-31, delivered in dendrite-core 1.3.0): one method for PE, ELF, Mach-O, package containers, JS bundles, unknown files and source directories. The Go client is `pkg/coreclient/inspect.go`; the sidecar stand-in (`pkg/coreclient/coretest/standin/inspect.go`) shows the shape of every answer. Until a sidecar has it, the client falls back to `pe.parse` for PE images (`coreclient.FromPE`) and reports `ErrInspectUnsupported` for everything else.

## Request

```json
{"path": "/path/to/file-or-directory"}
```

## Result: an `Image`

```json
{
  "kind": "executable",
  "format": "elf",
  "arch": "x86_64",
  "bits": 64,
  "endian": "little",
  "entry": "0x401040",
  "base": "0x400000",
  "sections": [{"name": ".text", "address": "0x401040", "size": 2048, "raw_size": 2048, "flags": ["alloc", "execute"]}],
  "libraries": [{"name": "libc.so.6", "symbols": ["printf", "strcmp"]}],
  "exports": ["check_flag"],
  "files": [],
  "strings": ["usage: %s flag"],
  "notes": {"interpreter": "/lib64/ld-linux-x86-64.so.2", "rpath": "$ORIGIN"},
  "entropy": 5.1,
  "truncated": false
}
```

- `kind`: `executable`, `library`, `object`, `package`, `bundle`, `source` or `data`.
- `format`: `pe`, `elf`, `macho`, `fat-macho`, `asar`, `zip` (APK, JAR, IPA, ...), `js-bundle`, `source-tree`, `unknown`. New formats may be added; a client treats an unknown value as opaque.
- Addresses are hex strings (`0x...`) so 64-bit values survive JSON. `sections` holds sections or segments, whichever the format has.
- `libraries` are the DLLs, `DT_NEEDED` entries, dylibs or packages the target depends on, each with the symbols taken from it when the format records them. A symbol imported by ordinal is `#<ordinal>`.
- `files` lists the entries of a package, bundle or source directory (path, size, kind); for a directory only files directly below the first levels are listed, up to a cap.
- `notes` holds format specifics as strings: PE `subsystem`; ELF `interpreter`, `rpath`, `soname`; Mach-O `codesign`, `entitlements`, `architectures` (fat); ASAR `integrity`; ZIP `manifest`.
- `strings` are the printable strings worth reading (URLs, paths, format strings), capped. `entropy` is Shannon entropy of the file in bits per byte (0 to 8); a value near 8 suggests packing or encryption.
- The whole answer is capped so it stays under 4,000 tokens (about 3,000 at most): names, symbols, files and strings share one budget, spent in that order. When anything was cut, `truncated` is `true`.
- A damaged table does not fail the call: the answer holds what was read before the damage.
- A file of a format the sidecar does not know is **not** an error: it answers `kind: data`, `format: unknown`, `entropy` and `strings`.

## Errors

| Code | When | Go error |
|---|---|---|
| `-32601` | the sidecar predates `bin.inspect` | `coreclient.ErrInspectUnsupported` |
| `-32602` | the path does not exist, cannot be read, or is neither a regular file nor a directory; a file larger than 256 MiB | `coreclient.ErrNotRecognised` (with the message) |

A hostile file must return an answer or an error, never panic, hang or allocate without bound. Nothing is ever executed.

## Tests the sidecar owes

A PE32+ and a PE32 fixture; a 64-bit ELF with `DT_NEEDED` and an interpreter; a thin and a fat Mach-O; an ASAR archive and a ZIP; a minified JS bundle; a file of random bytes (entropy near 8); a directory with nested files; a file cut inside its header; a directory with a symlink loop.
