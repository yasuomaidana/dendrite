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
| `github-sync` | Open a GitHub verification issue for every Plane To Verify ticket and close those of Done tickets (--hitl then polls them) |
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

Open a GitHub verification issue for every Plane To Verify ticket and close those of Done tickets (--hitl then polls them)

```bash
dendrite myelin github-sync [flags]
```

Aliases: `sync-github`, `saltatory-sync`, `gsync`

| Flag | Description |
| :--- | :--- |
| `--dry-run` | Show the planned issue changes without modifying GitHub |
| `--hitl` | Then poll the issues for human-in-the-loop approval or feedback |
| `--repo, -r` | GitHub repository owner/name (default: the origin remote) |
| `--ticket` | Limit the sync to these ticket IDs (comma-separated) |

```bash
dendrite myelin github-sync --dry-run
dendrite myelin github-sync --ticket DEN-123
dendrite myelin github-sync --hitl
```

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
