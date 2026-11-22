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
| `handoff` | Hand an REA ticket this session refuses to AGY (rule: the [rea] directive) |
| `list` | List the open-source showcase targets |
| `showcase` | Run cleanroom analysis and parity verification for one target or all (all, aegis, dxball, ctf, th04, mattermost) |

## Flags

None

## Subcommand Reference

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
```
