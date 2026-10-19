---
id: dendrite
title: "dendrite"
sidebar_label: "dendrite"
sidebar_position: 1
---

# dendrite

Autonomous AI development & infrastructure control plane

Commands use Neuro-Lore names; the plain names listed as aliases work too.

## Global Flags

| Flag | Description |
| :--- | :--- |
| `--gateway-url` | LiteLLM gateway base URL (env LITELLM_PROXY_URL, default http://localhost:4000) |
| `--home` | Dendrite home holding the global project index (env DENDRITE_HOME) |
| `--json` | Emit machine-readable JSON output |
| `--plane-url` | Plane API base URL (env PLANE_BASE_URL) |
| `--state-dir` | Dendrite state directory (env DENDRITE_STATE_DIR) |
| `--yes, -y` | Assume yes for every interactive prompt |

## Commands

### Soma & Axon Hillock

| Command | Aliases | Description |
| :--- | :--- | :--- |
| [`dev`](dev.md) |  | Developer diagnostics, local Ollama probe, and ticket tools |
| [`engram`](engram.md) | `store`, `engram-store`, `gram` | Persistent segmented store, memory checkpointer, and chunk query |
| [`ganglion`](ganglion.md) | `project`, `ganglion-workspace`, `prj` | Manage multi-project isolated workspaces |
| [`myelin`](myelin.md) | `vcs`, `hillock-guard`, `myelin-guard`, `guard` | Git branch guardrails prohibiting main pushes and ensuring rebase integrity |
| [`plexus`](plexus.md) | `pool`, `connectome-pool`, `plexus-pool`, `connectome`, `plx` | Multi-directory dependency inference & topological DAG solver |
| [`soma`](soma.md) | `env`, `homeostasis`, `soma-homeostasis` | Manage and replicate development environments |
| [`work`](work.md) | `workcopy`, `wc` | Manage model-owned working copies in dendrite-work/ (create, list, remove, auto-clean merged) |

### Cortex & Sensory Integration

| Command | Aliases | Description |
| :--- | :--- | :--- |
| [`ask`](ask.md) | `query-project`, `rag-ask`, `q` | Ask questions to Dendrite about a project with local RAG and interactive paid fallback |
| [`codex`](codex.md) | `doc`, `docs`, `engram-codex` | Docs-as-Code reflection, Docusaurus synthesis and portal |
| [`cortex`](cortex.md) | `ui`, `cortex-cockpit` | Embedded Web Control Cockpit and telemetry server |
| [`sensory`](sensory.md) | `telemetry`, `afferent-telemetry`, `tel` | Hardware thermal, token spend, and Prometheus telemetry |
| [`version`](version.md) |  | Print Dendrite CLI version |

### Postsynaptic Effectors & Actuation

| Command | Aliases | Description |
| :--- | :--- | :--- |
| [`bouton`](bouton.md) | `agent`, `effector-dispatch`, `bouton-dispatch`, `dispatch`, `agt` | Autonomous coding agent harness and job dispatcher (Aider, OpenHands, OpenCode, AGY, Copilot) |
| [`inhibitory-triage`](inhibitory-triage.md) | `triage`, `trg` | Resolve local agent task triage and priority routing |
| [`motor`](motor.md) | `service`, `motor-unit`, `effector-service`, `svc` | Manage microservices (Plane, Penpot, Ollama, LiteLLM) |
| [`reflex`](reflex.md) | `test`, `myotatic-reflex`, `tst` | Table-driven test synthesis, Testcontainers & UI tests |
| [`saga`](saga.md) | `story`, `stories`, `synaptic-saga`, `sty` | Autonomous multi-turn story execution and DAG branching |

### Synaptic Cleft & Gating

| Command | Aliases | Description |
| :--- | :--- | :--- |
| [`bridge`](bridge.md) | `jetbrains`, `neuromuscular-bridge`, `jb` | JetBrains IDE MCP protocol integration and PSI symbol indexer |
| [`cleft`](cleft.md) | `gateway`, `synaptic-cleft`, `gate` | LiteLLM token gateway rendering, dynamic routing updater & scheduler |
| [`greeting`](greeting.md) | `greet`, `pcontext-greeting` | Generate dynamic 10-style greeting with cliché suppression |
| [`inter`](inter.md) | `mcp`, `interneuron-mcp` | Model Context Protocol (MCP) server over stdio |
| [`session`](session.md) | `handoff` | Session checkpoint and resume for multi-agent coordination |
| [`spike`](spike.md) | `inference`, `action-potential`, `inf` | Unified multi-provider inference engine gateway (Ollama, OxiLLaMa, vLLM, TGI) |
| [`synapse`](synapse.md) | `syn`, `synapses` | Typed cross-project links with scoped MCP servers and self-healing paths |

### Axon & Myelin Sheath

| Command | Aliases | Description |
| :--- | :--- | :--- |
| [`axon`](axon.md) | `topology`, `axon-pathway`, `top` | Multi-node placement models and hardware service router |
| [`github-sync`](github-sync.md) | `sync-github`, `saltatory-sync`, `salt-sync`, `gsync` | Synchronize prioritized tickets and DAG dependencies to GitHub Issues |
| [`mesh`](mesh.md) | `dist`, `distcodebase`, `distributed`, `neural-mesh`, `syn-mesh` | Distributed codebase spawner, modular REA, and repo coordinator |
| [`sync-remote`](sync-remote.md) |  | Detect, fetch, and rebase remote fixes with commit reconciliation |
| [`translocate`](translocate.md) | `move`, `axonal-translocation`, `mv` | Zero-credential interactive service migration across SSH nodes |
| [`velocity`](velocity.md) | `bench`, `benchmark`, `conduction-velocity`, `bnch` | TTFT, TPS, and gateway resilience benchmarking suite |

### Dendritic Arbor

| Command | Aliases | Description |
| :--- | :--- | :--- |
| [`afferent`](afferent.md) | `ref`, `dendritic-afferent`, `aff` | Manage Tier 1 source references for library reuse and cleanroom synthesis |
| [`arbor`](arbor.md) | `query`, `dendritic-arbor`, `dend` | Intent-driven CFG/AST reachability slicing, debloater, transpile & fuse |
| [`retro`](retro.md) | `rea`, `retrograde-analysis` | Reverse engineering cleanroom analysis and parity verification |

### Glial Regulation & Pruning

| Command | Aliases | Description |
| :--- | :--- | :--- |
| [`allostasis`](allostasis.md) | `quality`, `homeostatic-audit`, `allostasis-audit`, `qlty` | Anti-mock scan, 100% coverage profiler, and HITL verification poller |
| [`cognition`](cognition.md) | `analyze`, `cognitive-analysis`, `anlz` | Cognitive complexity analyzer, structural anti-pattern scanner, and policy router |
| [`prune`](prune.md) | `audit-changes`, `microglial-prune`, `synaptic-pruning`, `prn` | Topological vendor audit and rebase engine for local/* branches |

### Terminal Boutons & Vesicles

| Command | Aliases | Description |
| :--- | :--- | :--- |
| [`chronos`](chronos.md) | `plane`, `chronos-tracker`, `pln` | Sovereign issue tracking, Gantt alignment, and DAG scheduler |
| [`quantal`](quantal.md) | `commit`, `quantal-commit`, `cmt` | Autonomous Git commit with local model message generation |
| [`vesicle`](vesicle.md) | `tokens`, `token`, `vesicle-quota`, `quota`, `tok` | Inspect 24h & 7d token consumption windows and tier quotas |

