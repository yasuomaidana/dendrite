---
id: myelin
title: "dendrite myelin"
sidebar_label: "myelin"
---

# dendrite myelin

Git branch guardrails prohibiting main pushes and ensuring rebase integrity

## Category

Axon & Myelin Sheath

## Aliases

- `vcs`
- `hillock-guard`
- `myelin-guard`
- `guard`

## Subcommands

| Subcommand | Description |
| :--- | :--- |
| `check` | Check the branch policy; on main/master, move to develop and exit 1 |
| `evacuate` | Check out develop, creating it when missing |
| `github-sync` | Sync the P0 blocker tickets and their dependencies to GitHub Issues (--hitl polls for human verification) |
| `sync-remote` | Fetch and rebase remote fixes and reconcile their tickets |

## Flags

None

## Subcommand Reference

### `dendrite myelin check`

Check the branch policy; on main/master, move to develop and exit 1

```bash
dendrite myelin check
```

Aliases: `guard`, `status`

### `dendrite myelin github-sync`

Sync the P0 blocker tickets and their dependencies to GitHub Issues (--hitl polls for human verification)

```bash
dendrite myelin github-sync [flags]
```

Aliases: `sync-github`, `saltatory-sync`, `gsync`

| Flag | Description |
| :--- | :--- |
| `--hitl` | Then poll the issues for human-in-the-loop approval or feedback |
| `--repo, -r` | GitHub repository (owner/name) (default: yasuomaidana/dendrite) |

### `dendrite myelin sync-remote`

Fetch and rebase remote fixes and reconcile their tickets

```bash
dendrite myelin sync-remote
```

Aliases: `remote-sync`

## Examples

```bash
dendrite myelin check
dendrite myelin sync-remote
dendrite myelin github-sync --repo owner/repo --hitl
```
