---
id: immune
title: "dendrite immune"
sidebar_label: "immune"
---

# dendrite immune

Scan agent skills with SkillSpector before they are installed

## Category

Glial Regulation & Pruning

## Aliases

None

## Subcommands

| Subcommand | Description |
| :--- | :--- |
| `allow` | Record a person's decision to accept a held scan of a skill |
| `scan` | Scan a skill and print approve / hold / quarantine |
| `update` | Install the pinned SkillSpector version, or the latest when none is pinned |

## Flags

None

## Subcommand Reference

### `dendrite immune allow`

Record a person's decision to accept a held scan of a skill

```bash
dendrite immune allow <skill-path> [flags]
```

| Flag | Description |
| :--- | :--- |
| `--reason` | why the held skill is safe to use (required) |

```bash
dendrite immune allow .claude/skills/my-skill --reason "reviewed: the URL is our own mirror"
```

### `dendrite immune scan`

Scan a skill and print approve / hold / quarantine

```bash
dendrite immune scan <skill-path> [flags]
```

| Flag | Description |
| :--- | :--- |
| `--approve-max` | highest score that is approved (default: immune.approve_max of the directives) (default: 20) |
| `--hold-max` | highest score that is held; above it is quarantined (default: immune.hold_max of the directives) (default: 50) |
| `--llm` | also run the scanner's model pass |

### `dendrite immune update`

Install the pinned SkillSpector version, or the latest when none is pinned

```bash
dendrite immune update
```

```bash
dendrite immune update
dendrite immune update --json
```

## Examples

```bash
dendrite immune scan .claude/skills/my-skill
dendrite immune scan ./skills --json
dendrite immune update
dendrite immune allow .claude/skills/my-skill --reason "reviewed: the URL is our own mirror"
```
