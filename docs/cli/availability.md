---
id: availability
title: "Command availability"
sidebar_label: "Availability"
---

# Command availability

Which commands a user gets from `main` and which exist only on `develop` until release 0.1.0 is merged. The pages in this folder describe `develop`. Until the release PR is merged, a command listed as develop-only is not in a build made from `main`.

## On main today

These top-level commands are on `main` and on `develop`: `afferent`, `allostasis`, `arbor`, `ask`, `axon`, `bouton`, `bridge`, `chronos`, `cleft`, `codex`, `cognition`, `cortex`, `engram`, `ganglion`, `greeting`, `inhibitory-triage`, `install`, `inter`, `mesh`, `motor`, `myelin`, `plexus`, `prune`, `quantal`, `reflex`, `retro`, `saga`, `sensory`, `session`, `soma`, `spike`, `synapse`, `sync`, `translocate`, `update`, `velocity`, `version`, `vesicle`, `work`.

## Only on develop (new top-level commands)

| Command | What it does |
| :--- | :--- |
| [`directive`](directive.md) | Reads and changes the quality directives in `.github/dendrite/directives.toml` |
| [`hook`](hook.md) | Entry points agent runtimes call around their tools (ticket write lock, raw-search guard) |
| [`rag`](rag.md) | Retrieval, symbols, graph, history and sidecar management served by the `dendrite-rag` sidecar |

## Only on develop (new subcommands of commands on main)

| Command | New on develop |
| :--- | :--- |
| [`allostasis`](allostasis.md) | `gate` |
| [`bouton`](bouton.md) | `jobs show`, `jobs watch` |
| [`chronos`](chronos.md) | `claim`, `comment`, `comments`, `decision`, `finish`, `handoff`, `plan push`, `project refresh`, `project remove`, `request` |
| [`inter`](inter.md) | `probe` |
| [`sensory`](sensory.md) | `usage` |
| [`soma`](soma.md) | `migrate-state`, `secrets` (`ensure`, `status`), `stacks` (`migrate`) |
| [`spike`](spike.md) | `bench`, `hold`, `models`, `prune`, `status` |
| [`synapse`](synapse.md) | `add`, `delete`, `remove-member` |

## Changed on develop

Flags or help text changed in [`ask`](ask.md), [`cleft`](cleft.md), [`prune`](prune.md), [`quantal`](quantal.md), [`sync`](sync.md), [`vesicle`](vesicle.md) and [`work`](work.md). Behaviour that already worked on `main` keeps working.

See [`CHANGELOG.md`](https://github.com/yasuomaidana/dendrite/blob/develop/CHANGELOG.md) for the full list of changes in this release.
