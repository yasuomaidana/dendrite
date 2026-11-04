---
id: branch
title: "dendrite branch"
sidebar_label: "branch"
---

# dendrite branch

Where finished work merges: the integration branch, a parent ticket's feature branch, a fork

## Category

Soma & Axon Hillock

## Aliases

None

## Subcommands

| Subcommand | Description |
| :--- | :--- |
| `parent` | Make <parent> the parent of <ticket> in Plane, so their work shares the parent's feature branch |
| `show` | Explain the branch work merges into, and where that came from |

## Flags

None

## Subcommand Reference

### `dendrite branch parent`

Make <parent> the parent of <ticket> in Plane, so their work shares the parent's feature branch

```bash
dendrite branch parent <ticket> <parent>
```

```bash
dendrite branch parent DEN-841 DEN-840
```

### `dendrite branch show`

Explain the branch work merges into, and where that came from

```bash
dendrite branch show [flags]
```

| Flag | Description |
| :--- | :--- |
| `--ticket` | Explain the target for this ticket (asks Plane for its parent) |

```bash
dendrite branch show
dendrite branch show --ticket DEN-843
```

## Examples

```bash
dendrite branch
```
