---
id: afferent
title: "dendrite afferent"
sidebar_label: "afferent"
---

# dendrite afferent

Manage Tier 1 source references for library reuse and cleanroom synthesis

## Category

Dendritic Arbor

## Aliases

- `ref`
- `dendritic-afferent`
- `aff`

## Subcommands

| Subcommand | Description |
| :--- | :--- |
| `add` | Register a source reference |
| `list` | List registered source references |
| `remove` | Remove a source reference |

## Flags

None

## Subcommand Reference

### `dendrite afferent add`

Register a source reference

```bash
dendrite afferent add [flags]
```

| Flag | Description |
| :--- | :--- |
| `--caps` | Capabilities (comma-separated) |
| `--id` | Reference ID |
| `--lang` | Primary language |
| `--license` | SPDX license identifier |
| `--name` | Display name (default: the ID) |
| `--repo` | Repository URL |

### `dendrite afferent list`

List registered source references

```bash
dendrite afferent list
```

Aliases: `ls`

### `dendrite afferent remove`

Remove a source reference

```bash
dendrite afferent remove <id>
```

Aliases: `rm`

## Examples

```bash
dendrite afferent list --json
dendrite afferent add --id totp-go --repo https://github.com/pquerna/otp --lang go --caps totp,hotp
dendrite afferent remove totp-go
```
