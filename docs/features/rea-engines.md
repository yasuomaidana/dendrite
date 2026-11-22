---
id: FEAT-102
title: REA engines: open-source tools in a container, proprietary tools documented
status: approved
dependencies: [FEAT-003]
repository: dendrite
tags: [reverse-engineering, rea, rizin, radare2, objdump, ghidra, sandbox, hopper, ida, binaryninja]
---

# REA engines

`dendrite retro analyze` always reads the **structure** of a target (format, sections, libraries, exports, strings)
through the `dendrite-core` sidecar. An **engine** goes further: it lists the functions of a binary, reads their
disassembly and, with Ghidra, decompiles them to C. Engines are external tools; Dendrite runs them, parses their output
and stores the result in the analysis (`engine.json`, one document per large function under `functions/`).

## The engines

| Engine | License | Needs | Gives | Runs in |
|---|---|---|---|---|
| `rizin` | LGPL-3.0 | image `dendrite-rea/rizin:1` or `rizin` on PATH | functions, complexity, calls, disassembly | container (default), host |
| `radare2` | LGPL-3.0 | image `dendrite-rea/radare2:1` or `radare2`/`r2` | same as rizin | container, host |
| `objdump` | GPL-3.0-or-later | image `dendrite-rea/objdump:1` or `objdump`/`llvm-objdump` | functions (from symbols), disassembly, calls; no complexity | container, host |
| `ghidra` | Apache-2.0 | image `dendrite-rea/ghidra:1` or `analyzeHeadless` + JDK 17 | functions, complexity, calls, disassembly and **decompiled C** | container, host |
| `hopper`, `ida`, `binaryninja` | proprietary | the user's own licensed installation | (documented only, see below) | not run by Dendrite |

`dendrite retro engines` lists every engine with where it can run on the machine and what is missing.

## Choosing an engine

```bash
dendrite retro analyze sample.elf --engine auto          # default
dendrite retro analyze sample.elf --engine ghidra
dendrite retro analyze sample.elf --engine none          # structure only
dendrite retro analyze sample.elf --sandbox host --trusted
```

* `--engine auto` takes the best **usable open-source** engine: capability first (decompiler > call graph + functions >
  disassembly only), catalog order among equals. "Usable" means: in the container, Docker answers and the engine's image
  is already built; on the host, its binary is on PATH. Proprietary tools are never chosen by `auto`.
* `auto` with nothing usable is not an error: the analysis keeps its structure result and records why (`engine_note`).
* An engine named explicitly that is not usable **is** an error that names the fix. There is no silent fallback.
* The default comes from the flag, then the environment (`DENDRITE_REA_ENGINE`, `DENDRITE_REA_SANDBOX`), then the project's
  directive: `dendrite directive set rea.engine=ghidra rea.sandbox=container`.
* `dendrite retro resume <id> --engine objdump` re-runs the engine stage with another engine.

## Sandbox policy

Parsing a hostile binary with a disassembler has had memory-safety bugs, so:

* The **container is the default**. It runs with no network, no capabilities, `no-new-privileges`, a read-only root, a
  small `noexec` scratch directory, memory/CPU limits and a time limit. The target is mounted **read-only** at
  `/target/input`; nothing it contains is executed.
* A binary (PE, ELF, Mach-O, package or unknown file) is opened on the **host** only with `--sandbox host --trusted`:
  the second flag is your statement that the file is safe, and it is recorded in the analysis. Source directories and JS
  bundles may be read on the host without it.
* Nothing is pulled or built implicitly: images exist only after `dendrite retro engines build <engine>`.

## Building the images

```bash
dendrite retro engines build rizin
dendrite retro engines build radare2
dendrite retro engines build objdump
dendrite retro engines build ghidra --url https://github.com/NationalSecurityAgency/ghidra/releases/download/<tag>/<zip> --sha256 <64 hex digits>
```

The first three use distribution packages (Debian). Ghidra's release is large and Dendrite does not pin one for you: you
choose the version and give its checksum, and the build refuses a download that does not hash to it. Ghidra needs
4 GiB of memory and may take minutes on a large binary; its limits are set per engine (`MemoryMB`, `Timeout` in
`pkg/rea/engine/engine.go`). Ghidra's headless analyzer runs a small script (`dockerfiles/ghidra/ExportFunctions.java`)
that prints the functions as JSON between two marker lines, because Ghidra's own log shares the output.

## Proprietary tools (Hopper, IDA, Binary Ninja)

You may configure and use a proprietary tool on your own machine and license. Dendrite **does not run one yet**; this
section says what is in place and how it would be used.

* They are in the engine catalog (`dendrite retro engines` shows them as "documented only") and accepted by the
  `rea.engine` directive, so a project can already record its choice.
* They are **never** selected by `auto`. Naming one (`--engine ida`) answers a clear "documented only" error.
* Dendrite does not bundle, download, containerize or license-check them. A licensed GUI or commercial tool belongs to
  the user's machine, so the only mode that makes sense is the **host** with `--sandbox host --trusted`.
* To add one later it is one more engine family in `pkg/rea/engine`: a command that exports the functions as the same
  JSON the other engines produce (`name`, `address`, `size`, `cyclomatic`, `calls`, `disassembly`, `pseudocode`), a parser,
  and a catalog entry with `Implemented: true`. Everything downstream (documents, search, triage, export) already works
  from that `Function` shape.
* IDA: scripts run through `idat64 -A -S<script>`; Hopper: its command-line mode with a Python script; Binary Ninja: its
  headless Python API. All need the user's license and a script that prints the same JSON.

## Where the result goes

`engine.json` holds the functions; `functions/<address>-<name>.md` holds one document per large function (disassembly,
calls, decompiled C when available), indexed with the rest of the analysis, so `dendrite retro search`,
`retro reuse` and `retro rank` find them. When core-rs cannot list the functions of a target (anything but PE for now), the functions an engine found are routed by cyclomatic complexity like core-rs's (limit 15, the same pause gate; `triage.json` says `"source": "engine"`); functions with no complexity (objdump) are counted as skipped. `retro reuse` also asks about the names of the largest functions an engine found, leaving out names a disassembler generated (`sub_*`, `FUN_*`, `fcn.*`, `sym.imp.*`).
