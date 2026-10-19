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
| `list` | List the open-source showcase targets |
| `showcase` | Run cleanroom analysis and parity verification for one target or all (all, aegis, dxball, ctf, th04, mattermost) |

## Flags

None

## Subcommand Reference

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
```
