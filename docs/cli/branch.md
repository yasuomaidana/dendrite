---
id: branch
title: "dendrite branch"
sidebar_label: "branch"
---

# dendrite branch

Which branch a ticket is worked on and where it merges: the integration branch, a parent ticket's trunk, a fork

## Category

Soma & Axon Hillock

## Aliases

None

## Subcommands

| Subcommand | Description |
| :--- | :--- |
| `parent` | Make <parent> the parent of <ticket> in Plane, so their work shares the parent's feature branch |
| `retire` | Open the last pull request of a branch (e.g. develop) into the integration branch and list what still targets it |
| `show` | Explain the branch a ticket is worked on, the branch it merges into, and where that came from |

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

### `dendrite branch retire`

Open the last pull request of a branch (e.g. develop) into the integration branch and list what still targets it

```bash
dendrite branch retire <branch>
```

```bash
dendrite branch retire develop
```

### `dendrite branch show`

Explain the branch a ticket is worked on, the branch it merges into, and where that came from

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
