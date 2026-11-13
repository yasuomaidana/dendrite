---
id: repair
title: "dendrite repair"
sidebar_label: "repair"
---

# dendrite repair

Fix a dependency on a branch or fork of its own, tracked until the fix is merged and released

## Category

Soma & Axon Hillock

## Aliases

None

## Subcommands

| Subcommand | Description |
| :--- | :--- |
| `advance` | Move each repair on: pin the consumer to the merge commit once merged, restore it once released |
| `drop` | Forget a repair (e.g. DEN-890/embedag); manifests, branches and copies are left as they are |
| `pr` | Open the pull request of every repair that has none, the bottom of each chain first |
| `start` | Get a working copy of a dependency on a repair branch and point this project at it |
| `status` | Show every repair chain: who uses which repair branch, its pull request and its stage |

## Flags

None

## Subcommand Reference

### `dendrite repair start`

Get a working copy of a dependency on a repair branch and point this project at it

```bash
dendrite repair start <dependency> [flags]
```

| Flag | Description |
| :--- | :--- |
| `--checkout` | Local checkout of the dependency to branch from (default: the synapse peer's) |
| `--dep-ticket` | The dependency project's own ticket for the fix; names the repair branch |
| `--fork` | URL of a fork to push the repair branch to |
| `--repo` | URL of the dependency's repository (default: the synapse peer's) |
| `--ticket` | This project's ticket (default: the ticket of the current branch) |

### `dendrite repair status`

Show every repair chain: who uses which repair branch, its pull request and its stage

```bash
dendrite repair status
```

Aliases: `list`, `ls`

## Examples

```bash
dendrite repair start embedag --dep-ticket EMBEDAG-12
dendrite repair start serde --repo https://github.com/serde-rs/serde --fork https://github.com/me/serde
dendrite repair status
dendrite repair pr
dendrite repair advance
```
