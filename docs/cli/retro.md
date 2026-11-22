---
id: retro
title: "dendrite retro"
sidebar_label: "retro"
---

# dendrite retro

Reverse engineering cleanroom analysis and parity verification

## Category

Dendritic Arbor

## Aliases

- `rea`
- `retrograde-analysis`

## Subcommands

| Subcommand | Description |
| :--- | :--- |
| `adr` | Draft a proposed Architecture Decision Record from a finished analysis |
| `analyze` | Analyze any target (PE, ELF, Mach-O, package, bundle, source directory): inspect it, write its structure as documents and index them for search |
| `engines` | List the reverse-engineering engines and where each can run on this machine |
| `engines build` | Build the container image of an engine from its Dockerfile (never done implicitly) |
| `export` | Write the structure of a finished analysis as safe Rust structs under --output |
| `handoff` | Hand an REA ticket this session refuses to AGY (rule: the [rea] directive) |
| `list` | List the open-source showcase targets |
| `pause` | Pause an analysis at its next stage boundary; its checkpoint is kept |
| `purge` | Delete an analysis: its documents, its index and its checkpoint |
| `rank` | Say which documents of an analysis to read first (most connected first) |
| `resume` | Continue an analysis from its last checkpoint, optionally on another model tier or model |
| `reuse` | Find code in this project and its synapse that already does what the analysis found |
| `search` | Search the documents of a finished analysis; only the best passages are returned |
| `showcase` | Run cleanroom analysis and parity verification for one target or all (all, aegis, dxball, ctf, th04, mattermost) |

## Flags

None

## Subcommand Reference

### `dendrite retro adr`

Draft a proposed Architecture Decision Record from a finished analysis

```bash
dendrite retro adr <id> [flags]
```

| Flag | Description |
| :--- | :--- |
| `--output, -o` | Directory of the project's decision records (default: docs/architecture/decisions) |
| `--title` | Title of the record (default: Cleanroom recreation of <target>) |

```bash
dendrite retro adr pe-1a2b3c4d --title "Recreate the checksum routine"
```

### `dendrite retro analyze`

Analyze any target (PE, ELF, Mach-O, package, bundle, source directory): inspect it, write its structure as documents and index them for search

```bash
dendrite retro analyze <target> [flags]
```

| Flag | Description |
| :--- | :--- |
| `--engine` | Engine to run over the target: auto (the best usable open-source one), none, rizin, radare2 or objdump (default: the [rea] directive) |
| `--format` | Format the target must have (default: detect it): pe, elf, macho, fat-macho, asar, zip, js-bundle, source-tree or unknown |
| `--id` | Name of the analysis (default: generated from the binary) |
| `--model` | Model to use on that tier |
| `--sandbox` | Where the engine runs: container (default) or host |
| `--ticket` | Leave a note with the summary and the analysis id on this ticket |
| `--tier` | Model tier: local or paid (default: local) |
| `--trusted` | Say the target is safe to open on the host (needed for a binary with --sandbox host) |

```bash
dendrite retro analyze sample.exe --tier local
dendrite retro analyze ./app.asar
dendrite retro analyze ./src --format source-tree
dendrite retro search pe-1a2b3c4d "CreateFileW"
```

### `dendrite retro engines`

List the reverse-engineering engines and where each can run on this machine

```bash
dendrite retro engines
```

```bash
dendrite retro engines
dendrite retro engines build rizin
```

### `dendrite retro engines build`

Build the container image of an engine from its Dockerfile (never done implicitly)

```bash
dendrite retro engines build <engine> [flags]
```

| Flag | Description |
| :--- | :--- |
| `--sha256` | Ghidra only: the checksum the release zip must have |
| `--url` | Ghidra only: https link of the Ghidra release zip to install |

```bash
dendrite retro engines build rizin
dendrite retro engines build ghidra --url https://github.com/NationalSecurityAgency/ghidra/releases/download/<tag>/<zip> --sha256 <64 hex digits>
```

### `dendrite retro export`

Write the structure of a finished analysis as safe Rust structs under --output

```bash
dendrite retro export <id> [flags]
```

| Flag | Description |
| :--- | :--- |
| `--output` | Directory to write the generated file into |
| `--target-language` | Language of the generated code (rust) (default: rust) |

```bash
dendrite retro export pe-1a2b3c4d --target-language rust --output ./generated
```

### `dendrite retro handoff`

Hand an REA ticket this session refuses to AGY (rule: the [rea] directive)

```bash
dendrite retro handoff <ticket> --refused [flags]
```

| Flag | Description |
| :--- | :--- |
| `--reason` | Why, in a sentence; kept on the ticket |
| `--refused` | This session refuses the ticket (required) |
| `--worktree` | Working copy of this session (default: the current directory) |

```bash
dendrite retro handoff DEN-1062 --refused --reason "declined to analyse the binary"
```

### `dendrite retro list`

List the open-source showcase targets

```bash
dendrite retro list
```

Aliases: `targets`, `ls`

### `dendrite retro purge`

Delete an analysis: its documents, its index and its checkpoint

```bash
dendrite retro purge <id> [flags]
```

| Flag | Description |
| :--- | :--- |
| `--force` | Confirm the deletion |

### `dendrite retro resume`

Continue an analysis from its last checkpoint, optionally on another model tier or model

```bash
dendrite retro resume <id> [flags]
```

| Flag | Description |
| :--- | :--- |
| `--engine` | Engine to run over the target: auto (the best usable open-source one), none, rizin, radare2 or objdump (default: the [rea] directive) |
| `--model` | Switch to this model |
| `--sandbox` | Where the engine runs: container (default) or host |
| `--tier` | Switch to this model tier: local or paid |
| `--trusted` | Say the target is safe to open on the host (needed for a binary with --sandbox host) |

```bash
dendrite retro resume pe-1a2b3c4d --tier paid --model cloud-pro
```

### `dendrite retro reuse`

Find code in this project and its synapse that already does what the analysis found

```bash
dendrite retro reuse <id> [flags]
```

| Flag | Description |
| :--- | :--- |
| `--project` | Project directory to search (default: .) |
| `--synapse` | Synapse to search (default: the one the project belongs to) |
| `--top, -k` | How many passages to return (default: 10) |

```bash
dendrite retro reuse pe-1a2b3c4d
dendrite retro reuse pe-1a2b3c4d --synapse dendrite-ecosystem -k 5
```

### `dendrite retro search`

Search the documents of a finished analysis; only the best passages are returned

```bash
dendrite retro search <id> <query> [flags]
```

| Flag | Description |
| :--- | :--- |
| `--top, -k` | How many passages to return (at most 20) (default: 5) |

```bash
dendrite retro search pe-1a2b3c4d "CreateFileW" -k 3
```

### `dendrite retro showcase`

Run cleanroom analysis and parity verification for one target or all (all, aegis, dxball, ctf, th04, mattermost)

```bash
dendrite retro showcase [target] [flags]
```

| Flag | Description |
| :--- | :--- |
| `--format` | Report format: text or markdown (use --json for JSON) (default: text) |
| `--output, -o` | Write the report to this file instead of stdout |

## Examples

```bash
dendrite retro list
dendrite retro showcase
dendrite retro showcase aegis --format markdown --output aegis.md
dendrite retro showcase ctf --json
dendrite retro handoff DEN-1062 --refused
dendrite retro analyze sample.exe
dendrite retro search <id> "CreateFileW"
```
