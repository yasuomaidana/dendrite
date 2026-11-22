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
| `analyze` | Analyze any target (PE, ELF, Mach-O, package, bundle, source directory): inspect it, write its structure as documents and index them for search |
| `export` | Write the structure of a finished analysis as safe Rust structs under --output |
| `handoff` | Hand an REA ticket this session refuses to AGY (rule: the [rea] directive) |
| `list` | List the open-source showcase targets |
| `pause` | Pause an analysis at its next stage boundary; its checkpoint is kept |
| `purge` | Delete an analysis: its documents, its index and its checkpoint |
| `resume` | Continue an analysis from its last checkpoint, optionally on another model tier or model |
| `search` | Search the documents of a finished analysis; only the best passages are returned |
| `showcase` | Run cleanroom analysis and parity verification for one target or all (all, aegis, dxball, ctf, th04, mattermost) |

## Flags

None

## Subcommand Reference

### `dendrite retro analyze`

Analyze any target (PE, ELF, Mach-O, package, bundle, source directory): inspect it, write its structure as documents and index them for search

```bash
dendrite retro analyze <target> [flags]
```

| Flag | Description |
| :--- | :--- |
| `--format` | Format the target must have (default: detect it): pe, elf, macho, fat-macho, asar, zip, js-bundle, source-tree or unknown |
| `--id` | Name of the analysis (default: generated from the binary) |
| `--model` | Model to use on that tier |
| `--tier` | Model tier: local or paid (default: local) |

```bash
dendrite retro analyze sample.exe --tier local
dendrite retro analyze ./app.asar
dendrite retro analyze ./src --format source-tree
dendrite retro search pe-1a2b3c4d "CreateFileW"
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
| `--model` | Switch to this model |
| `--tier` | Switch to this model tier: local or paid |

```bash
dendrite retro resume pe-1a2b3c4d --tier paid --model cloud-pro
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
