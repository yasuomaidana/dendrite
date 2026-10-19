---
id: translocate
title: "dendrite translocate"
sidebar_label: "translocate"
---

# dendrite translocate

Zero-credential interactive service migration across SSH nodes

## Category

Axon & Myelin Sheath

## Aliases

- `move`
- `axonal-translocation`
- `mv`

## Subcommands

| Subcommand | Description |
| :--- | :--- |
| `plan` | Show which services fit on a target server of the given size |
| `run` | Move services (or 'debian' for the full eligible set) to a host over SSH |

## Flags

None

## Subcommand Reference

### `dendrite translocate plan`

Show which services fit on a target server of the given size

```bash
dendrite translocate plan [flags]
```

| Flag | Description |
| :--- | :--- |
| `--disk` | Target disk (e.g. 1TB, 1000GB) (default: 1TB) |
| `--ram` | Target RAM (e.g. 4GB, 4096MB) (default: 4GB) |

### `dendrite translocate run`

Move services (or 'debian' for the full eligible set) to a host over SSH

```bash
dendrite translocate run <service>... [flags]
```

| Flag | Description |
| :--- | :--- |
| `--disk` | Target disk (e.g. 1TB, 1000GB) (default: 1TB) |
| `--dry-run` | Show what would happen without changing anything |
| `--key, -i` | SSH identity file (default: ssh-agent) |
| `--no-prune` | Keep the local containers running after the move |
| `--port, -p` | SSH port (default: 22) |
| `--ram` | Target RAM (e.g. 4GB, 4096MB) (default: 4GB) |
| `--ssh, -s` | Target SSH host (user@host or host) |
| `--timeout` | Remote health check timeout (default: 2m0s) |
| `--user, -u` | SSH user (default from the host or the deployer) |

## Examples

```bash
dendrite translocate plan --ram 4GB --disk 1TB
dendrite translocate run plane litellm --ssh user@debian-server --dry-run
dendrite translocate run debian --ssh user@debian-server --yes
```
