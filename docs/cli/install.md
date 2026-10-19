---
id: install
title: "dendrite install"
sidebar_label: "install"
---

# dendrite install

Install dendrite and its sub-projects from git: main (stable) or develop (--dev), per-member --ref overrides

## Category

Cortex & Sensory Integration

## Aliases

- `bootstrap`

## Subcommands

None

## Flags

| Flag | Description |
| :--- | :--- |
| `--dev` | Track develop (the channel is remembered for later installs and 'update --dev') |
| `--no-build` | Only clone and check out; build nothing |
| `--ref` | Track another branch or tag for one member as <alias>=<ref>; <alias>= clears it (repeatable) |
| `--root` | Where members are cloned (default: the directory holding this Dendrite checkout, else ~/dendrite) |
| `--stable` | Track main (default for a first install) |
| `--target` | Binary to replace (default: the running dendrite, or $GOBIN/dendrite under go run) |

## Examples

```bash
dendrite install
dendrite install --dev
dendrite install --dev --ref rag-rs=feat/ann
dendrite install --dev --ref rag-rs=
```
