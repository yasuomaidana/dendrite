# Sub-Project Map & Responsibilities

Dendrite is split into sibling repositories that `dendrite pool` keeps in sync. This page is the control plane's index of who owns what. Each sub-project also has its own authoritative `RESPONSIBILITIES` document, plus the Dendrite docs that describe its scope, copied in on 2026-10-18.

The boundary comes from [2026-10 Quality Audit & Execution Plan §5](./2026-10-quality-audit-execution-plan.md). Tickets were relocated in Plane on 2026-10-18; see the relocation section below.

## Repositories

| Repository | Path | Language | Plane project | Extraction umbrella | Responsibilities doc |
|---|---|---|---|---|---|
| **dendrite-control** (this repo) | `~/GoLandProjects/Dendrite` | Go | `DEN` (Dendrite Core) | — | this page |
| **dendrite-mcp-hub** | `../dendrite-mcp-hub` | Go | `DMCP` | DEN-971 | `dendrite-mcp-hub/docs/RESPONSIBILITIES.md` |
| **dendrite-core-rs** | `../dendrite-core-rs` | Rust | `DCORE` | DEN-972 | `dendrite-core-rs/docs/RESPONSIBILITIES.md` |
| **dendrite-git-rs** | `../dendrite-git-rs` | Rust | `DGIT` | DEN-973 | `dendrite-git-rs/docs/RESPONSIBILITIES.md` |
| **dendrite-rag-rs** | `../dendrite-rag-rs` | Rust | `DRAG` | DEN-974 | `dendrite-rag-rs/docs/RESPONSIBILITIES.md` |
| **embedag** | `~/RustRoverProjects/embedag` | Rust | `EMBEDAG` (Milestone 9 imported from `DEV_STATUS.md`) | DEN-051 / 068 / 074 | `embedag/doc/dendrite-integration.md` |

## Projects and synapses rule

Every repository we can modify is a Plane project, and repositories that depend on each other share a synapse (rule 3 in `AGENTS.md`).

- **Link file:** each repository commits `.dendrite/plane.json` (identifier, project UUID, workspace) next to `.dendrite/project.id`. `dendrite plane …`, `dendrite sync` and agents use that board when run inside the repository; `PLANE_PROJECT_ID` still overrides it, and Dendrite Core is the fallback.
- **Commands:** `dendrite plane project ensure [dir] [--identifier ID] [--import DEV_STATUS.md]` adopts or creates the project, converges its states onto the canonical set and writes the link. `dendrite plane project audit` checks the rule across the IDE project folders. Editable means the origin belongs to the owner of this repository's origin, or the repository has no remote.
- **State on 2026-10-18:** 13 editable repositories, 13 Plane projects (DEN, DMCP, DCORE, DGIT, DRAG, EMBEDAG, DOCULA, OXI, K8SLAB, DIESELJP, EMBRUST, CAPACITANCE, INTROML). Two synapses: `dendrite-ecosystem` (this repo, the four sub-projects and embedag) and `docula-embedag` (docula's `docula-rag` depends on `embedag-rag`). `rqlite` is a third-party clone and is not a project.

## Who owns what

- **dendrite-control** owns:
  - the CLI (`cmd/dendrite`)
  - environment provisioning (`pkg/env`, `pkg/installer`)
  - the LiteLLM gateway (`pkg/gateway`)
  - the Plane tracker (`pkg/tracker`)
  - agent job orchestration (`pkg/agentjobs`)
  - multi-project sync (`pkg/pool`)
  - the server and UI (`pkg/server`, `pkg/ui`)
  - CI and quality gates

  It also keeps an in-process Go fallback for every sidecar.
- **dendrite-mcp-hub** owns:
  - the MCP JSON-RPC server and tool registry (`pkg/mcpserver`)
  - the JetBrains bridge (`pkg/jetbrains`)
  - cross-language contract drift monitoring (`pkg/polyglot/contract`)
  - upstream MCP bridges: Penpot, Plane AI, Engram, REA
  - IDE plugin adapters
- **dendrite-core-rs** owns:
  - tree-sitter signature folding and last-complete-node detection
  - complexity metrics and CFG slices
  - BLAKE3 hashing and token counting

  It replaces the heuristics in `pkg/ast` and `pkg/ai/tokenopt`.
- **dendrite-git-rs** owns:
  - the gitoxide hunk dependency graph
  - the semantic commit clusterer
  - the `--local-review` stacked-branch engine

  It sits behind the `pkg/vcs` / `pkg/devtool` commit flow.
- **dendrite-rag-rs** owns:
  - BM25 + vector hybrid retrieval
  - the content-addressed store
  - the memory sync modes (local-only, edge/WAL, push)

  It replaces `pkg/rag`, the search primitives in `pkg/agentsearch`, and `pkg/store`.
- **embedag** owns every graph and DAG algorithm: toposort, levels, reverse dependencies, critical path, Merkle delta trees, AST similarity graphs, and Graph-RAG primitives (RRF, community detection).
  - It is consumed by `dendrite-core-rs` and `dendrite-rag-rs` with `default-features = false, features = ["core", "matcher"]`.
  - New DAG code is not written in Go.

## Interface contract

- Each Rust crate ships a `dendrite-<x>` binary that speaks JSON-RPC 2.0 over stdio. Its schema lives in `proto/*.json`, versioned with semver.
- The Go side uses a `sidecar.Client` that does a health handshake and falls back to the in-process Go implementation when the binary is missing.
- `dendrite-mcp-hub` is a Go module (`dendrite.dev/mcphub`).

## Ticket relocation (2026-10-18)

- 39 tickets were copied from `DEN` into `DCORE`, `DRAG`, `DGIT` and `DMCP`. Each copy keeps the description, acceptance criteria, estimate, priority, dates and state (mapped by name). Relations were re-linked, and each copy carries `external_source=dendrite-relocate`.
- **`move`**: the 19 originals got a backlink comment, the `relocated` label and the `Relocated` (cancelled) state, and were then archived.
- **`split`**: the originals stay in `DEN` as umbrellas for their Go side, with a `relates_to` link to the child.
- 4 graph/DAG tickets went to embedag's `DEV_STATUS.md` (Milestone 9).
- The manifest and verification report are in `.dendrite/session/` (`relocation-manifest.csv`, `relocation-report.json`).

### Follow-up reconciliation (2026-10-18)

An audit of the live Plane data found gaps, which `reconcile_relocation.py` closed (report: `.dendrite/session/relocation-report-2026-10-18b.json`). A re-run plans zero writes.

- **DEND-202 (DEN #107)** was a `move` to embedag 9.4.1 but was still open in DEN. It now has a backlink, the `relocated` label and the `Relocated` state, and is archived. That makes **20** archived moves: 19 to Plane projects, plus this one to embedag.
- All **23** `split` originals carry the `relocated-split` label, a backlink comment and a "Relocated part" section naming the child. Their state is unchanged. DEN-051/068/074 (embedag 9.2.1/9.3.1/9.2.2) got their first backlink.
- The umbrellas DEN-971/972/973/974 now have `relates_to` links to DMCP-1/DCORE-1/DGIT-1/DRAG-1. Stale `blocked_by` links to the archived originals DEN #170 and #189 were removed; the copies DGIT-3 and DRAG-2 already carry the dependency. The same was done for 15 other live DEN tickets: each `blocked_by` an archived original now points at its sub-project copy, and no live DEN ticket depends on an archived one.
- Copy titles said "(was DEN-&lt;sequence&gt;)", which reads like a ticket ID. They now say "(was DEN #&lt;sequence&gt;)", or drop the suffix where it repeated the ticket's own number.
- DEN-997 to DEN-1016 (Synapse & mesh, created after the relocation) stay in DEN until the DEN-971 extraction. DEN-1004 to 1008, DEN-1012 and DEN-1016 have `relates_to` links to DMCP-1.
- `dendrite plane replicate` now lists sub-project tickets under their own keys (`DCORE-6`, not the copy's `DEN-018`), with their own project's states.

### Where every relocated ticket lives

| DEN original | Action | New home |
|---|---|---|
| DEN-009, DEN-028, DEN-040, DEN-063, DEN-091, DEN-093, DEND-201, DEN-808, DEN-812, DEN-930, DEN-960 | move | DMCP-22, -21, -20, -17, -14, -12, -10, -5, -4, -3, -2 |
| DEN-046, DEN-050, DEN-075, DEN-076, DEN-092, DEN-096, DEND-302, DEND-303, DEND-304, DEND-305 | split | DMCP-19, -18, -16, -15, -13, -11, -9, -8, -7, -6 |
| DEN-010, DEN-020, DEN-943 | move | DCORE-7, -5, -2 |
| DEN-018, DEN-056, DEN-942 | split | DCORE-6, -4, -3 |
| DEN-932, DEN-933 | move | DGIT-4, -3 |
| DEN-064, DEN-065, DEN-098, DEN-934 | split | DGIT-7, -6, -5, -2 |
| DEND-203, DEN-803, DEN-964 | move | DRAG-4, -3, -2 |
| DEN-037, DEN-059, DEN-060 | split | DRAG-7, -6, -5 |
| DEND-202 | move | EMBEDAG [9.4.1] |
| DEN-051, DEN-074, DEN-068 | split | EMBEDAG [9.2.1], [9.2.2], [9.3.1] (`relates_to` links since embedag got its Plane project) |

Each sub-project's `RESPONSIBILITIES` Ticket Map has the same mapping, with live states.

## Documentation that moved with the code

These Dendrite docs remain here, because generated references and the docs audit gate depend on them. Copies with a provenance header live under each sub-project's `docs/imported/` (embedag: `doc/imported/`), listed in that repo's `imports.json`.

Until a sub-project's extraction umbrella lands, edit the Dendrite copy and re-copy. After extraction, the sub-project copy becomes the source of truth and the Dendrite page should link to it.

`scripts/sync_subproject_docs.py` does the copying. It also regenerates each sub-project's docs index (`docs/README.md`; embedag: `doc/dendrite-integration.md` §7) with working `imported/...` links, and the table below. `--check` reports missing, stale or orphaned copies and broken index links, and exits 1 if it finds any.

```bash
uv run scripts/sync_subproject_docs.py            # copy, re-index, refresh this table
uv run scripts/sync_subproject_docs.py --check    # verify only
```

Which sub-project imports what was reviewed per repository against its `RESPONSIBILITIES` document on 2026-10-19 (DEN-1018).

<!-- sync_subproject_docs:begin -->

71 Dendrite docs are imported. After editing one, run `uv run scripts/sync_subproject_docs.py`.

| Dendrite doc | Imported by |
|---|---|
| [`docs/architecture/03-context-intelligence-re.md`](03-context-intelligence-re.md) | dendrite-core-rs |
| [`docs/architecture/2026-10-quality-audit-execution-plan.md`](2026-10-quality-audit-execution-plan.md) | dendrite-core-rs, dendrite-git-rs, dendrite-mcp-hub, dendrite-rag-rs, embedag |
| [`docs/cli/analyze.md`](../cli/analyze.md) | dendrite-core-rs |
| [`docs/cli/ask.md`](../cli/ask.md) | dendrite-rag-rs |
| [`docs/cli/audit-changes.md`](../cli/audit-changes.md) | dendrite-git-rs |
| [`docs/cli/canvas.md`](../cli/canvas.md) | dendrite-mcp-hub |
| [`docs/cli/commit.md`](../cli/commit.md) | dendrite-git-rs |
| [`docs/cli/jetbrains.md`](../cli/jetbrains.md) | dendrite-mcp-hub |
| [`docs/cli/mcp.md`](../cli/mcp.md) | dendrite-mcp-hub |
| [`docs/cli/pool.md`](../cli/pool.md) | embedag |
| [`docs/cli/query.md`](../cli/query.md) | dendrite-core-rs |
| [`docs/cli/store.md`](../cli/store.md) | dendrite-rag-rs |
| [`docs/cli/story.md`](../cli/story.md) | dendrite-git-rs |
| [`docs/cli/sync-remote.md`](../cli/sync-remote.md) | dendrite-git-rs |
| [`docs/cli/vcs.md`](../cli/vcs.md) | dendrite-git-rs |
| [`docs/features/FEAT-003-reverse-engineering-rea.md`](../features/FEAT-003-reverse-engineering-rea.md) | dendrite-mcp-hub |
| [`docs/features/FEAT-004-repo-intel-ast.md`](../features/FEAT-004-repo-intel-ast.md) | dendrite-core-rs, embedag |
| [`docs/features/FEAT-010-ai-assisted-token-optimization.md`](../features/FEAT-010-ai-assisted-token-optimization.md) | dendrite-core-rs |
| [`docs/features/FEAT-011-polyglot-translation-synthesis.md`](../features/FEAT-011-polyglot-translation-synthesis.md) | dendrite-core-rs |
| [`docs/features/FEAT-014-jetbrains-ide-mcp-integration.md`](../features/FEAT-014-jetbrains-ide-mcp-integration.md) | dendrite-mcp-hub |
| [`docs/features/FEAT-022-agent-search-and-capability-discovery.md`](../features/FEAT-022-agent-search-and-capability-discovery.md) | dendrite-rag-rs, embedag |
| [`docs/features/FEAT-026-distributed-codebase-ecosystem.md`](../features/FEAT-026-distributed-codebase-ecosystem.md) | embedag |
| [`docs/features/FEAT-028-local-agent-triage-gateway-token-efficiency.md`](../features/FEAT-028-local-agent-triage-gateway-token-efficiency.md) | dendrite-mcp-hub |
| [`docs/features/FEAT-029-autonomous-pool-dag-synchronization.md`](../features/FEAT-029-autonomous-pool-dag-synchronization.md) | embedag |
| [`docs/features/FEAT-030-universal-query-engine-3tier-library-reuse.md`](../features/FEAT-030-universal-query-engine-3tier-library-reuse.md) | dendrite-core-rs |
| [`docs/features/FEAT-031-persistent-store-hybrid-token-scheduler.md`](../features/FEAT-031-persistent-store-hybrid-token-scheduler.md) | dendrite-core-rs, dendrite-rag-rs |
| [`docs/features/FEAT-032-polyglot-speculative-drift-workflows.md`](../features/FEAT-032-polyglot-speculative-drift-workflows.md) | dendrite-git-rs |
| [`docs/features/FEAT-033-infrastructure-mobility-smart-backup.md`](../features/FEAT-033-infrastructure-mobility-smart-backup.md) | embedag |
| [`docs/features/FEAT-035-multiprovider-agent-governance.md`](../features/FEAT-035-multiprovider-agent-governance.md) | dendrite-git-rs |
| [`docs/features/FEAT-036-ticket-milestone-dag-story-prioritization.md`](../features/FEAT-036-ticket-milestone-dag-story-prioritization.md) | embedag |
| [`docs/features/FEAT-037-penpot-ui-documentation-mcp-integration.md`](../features/FEAT-037-penpot-ui-documentation-mcp-integration.md) | dendrite-mcp-hub |
| [`docs/features/FEAT-040-turso-offline-cache-outbox-sync.md`](../features/FEAT-040-turso-offline-cache-outbox-sync.md) | embedag |
| [`docs/features/FEAT-041-agent-security-quality-resilience-guardrails.md`](../features/FEAT-041-agent-security-quality-resilience-guardrails.md) | dendrite-git-rs |
| [`docs/features/FEAT-042-artifact-package-publisher-mcp.md`](../features/FEAT-042-artifact-package-publisher-mcp.md) | dendrite-mcp-hub |
| [`docs/features/FEAT-045-code-knowledge-mcp-graphify.md`](../features/FEAT-045-code-knowledge-mcp-graphify.md) | dendrite-mcp-hub, dendrite-rag-rs |
| [`docs/features/FEAT-046-multi-ide-plugins.md`](../features/FEAT-046-multi-ide-plugins.md) | dendrite-mcp-hub |
| [`docs/features/FEAT-070-speculative-stacked-branches-and-audit-pipeline.md`](../features/FEAT-070-speculative-stacked-branches-and-audit-pipeline.md) | dendrite-git-rs |
| [`docs/features/FEAT-098-autonomous-git-commit-engine.md`](../features/FEAT-098-autonomous-git-commit-engine.md) | dendrite-git-rs |
| [`docs/features/FEAT-100-multi-ide-ecosystem-support.md`](../features/FEAT-100-multi-ide-ecosystem-support.md) | dendrite-mcp-hub |
| [`docs/features/FEAT-101-personal-context-topic-memory-installer.md`](../features/FEAT-101-personal-context-topic-memory-installer.md) | dendrite-mcp-hub |
| [`docs/guides/asking-questions-to-dendrite.md`](../guides/asking-questions-to-dendrite.md) | dendrite-rag-rs |
| [`docs/guides/jetbrains-mcp-and-agents-setup.md`](../guides/jetbrains-mcp-and-agents-setup.md) | dendrite-mcp-hub |
| [`docs/guides/using-dendrite-tools-with-jetbrains.md`](../guides/using-dendrite-tools-with-jetbrains.md) | dendrite-mcp-hub |
| [`docs/mcp/dendrite_audit_changes.md`](../mcp/dendrite_audit_changes.md) | dendrite-git-rs |
| [`docs/mcp/dendrite_canvas_action_dispatch.md`](../mcp/dendrite_canvas_action_dispatch.md) | dendrite-mcp-hub |
| [`docs/mcp/dendrite_canvas_penpot_bridge.md`](../mcp/dendrite_canvas_penpot_bridge.md) | dendrite-mcp-hub |
| [`docs/mcp/dendrite_commit.md`](../mcp/dendrite_commit.md) | dendrite-git-rs |
| [`docs/mcp/dendrite_contract_drift_check.md`](../mcp/dendrite_contract_drift_check.md) | dendrite-mcp-hub |
| [`docs/mcp/dendrite_jetbrains_setup.md`](../mcp/dendrite_jetbrains_setup.md) | dendrite-mcp-hub |
| [`docs/mcp/dendrite_local_agent_triage.md`](../mcp/dendrite_local_agent_triage.md) | dendrite-mcp-hub |
| [`docs/mcp/dendrite_pcontext_greeting_generate.md`](../mcp/dendrite_pcontext_greeting_generate.md) | dendrite-mcp-hub |
| [`docs/mcp/dendrite_pool_resolve.md`](../mcp/dendrite_pool_resolve.md) | embedag |
| [`docs/mcp/dendrite_query_slice.md`](../mcp/dendrite_query_slice.md) | dendrite-core-rs |
| [`docs/mcp/dendrite_query_transpile.md`](../mcp/dendrite_query_transpile.md) | dendrite-core-rs |
| [`docs/mcp/dendrite_rag_ask.md`](../mcp/dendrite_rag_ask.md) | dendrite-rag-rs |
| [`docs/mcp/dendrite_rag_retrieve.md`](../mcp/dendrite_rag_retrieve.md) | dendrite-rag-rs |
| [`docs/mcp/dendrite_rea_showcase.md`](../mcp/dendrite_rea_showcase.md) | dendrite-mcp-hub |
| [`docs/mcp/dendrite_remote_sync.md`](../mcp/dendrite_remote_sync.md) | dendrite-git-rs |
| [`docs/mcp/dendrite_store_commit_chunk.md`](../mcp/dendrite_store_commit_chunk.md) | dendrite-rag-rs |
| [`docs/mcp/dendrite_store_inspect.md`](../mcp/dendrite_store_inspect.md) | dendrite-rag-rs |
| [`docs/mcp/dendrite_store_query_partial.md`](../mcp/dendrite_store_query_partial.md) | dendrite-rag-rs |
| [`docs/mcp/dendrite_store_rm.md`](../mcp/dendrite_store_rm.md) | dendrite-rag-rs |
| [`docs/mcp/dendrite_store_status.md`](../mcp/dendrite_store_status.md) | dendrite-rag-rs |
| [`docs/mcp/dendrite_story_abort.md`](../mcp/dendrite_story_abort.md) | dendrite-git-rs |
| [`docs/mcp/dendrite_story_fork.md`](../mcp/dendrite_story_fork.md) | dendrite-git-rs |
| [`docs/mcp/dendrite_story_resolve.md`](../mcp/dendrite_story_resolve.md) | dendrite-git-rs |
| [`docs/mcp/dendrite_ticket_smart_select.md`](../mcp/dendrite_ticket_smart_select.md) | embedag |
| [`docs/mcp/dendrite_ui_penpot_sync.md`](../mcp/dendrite_ui_penpot_sync.md) | dendrite-mcp-hub |
| [`docs/mcp/dendrite_vcs_guard.md`](../mcp/dendrite_vcs_guard.md) | dendrite-git-rs |
| [`docs/quality/scrum-and-complexity-scorecard.md`](../quality/scrum-and-complexity-scorecard.md) | dendrite-core-rs |
| [`docs/reports/2026-10-17-done-ticket-audit.md`](../reports/2026-10-17-done-ticket-audit.md) | dendrite-core-rs |

<!-- sync_subproject_docs:end -->

## Open boundary questions

Collected from the sub-project responsibility documents. Each is to be resolved in the `proto/*.json` contracts or by a ticket.

- **Crate and binary naming:** §5 says `dendrite-core` / `dendrite-rag` / `dendrite-git`, but the scaffolds are named `*-rs` and have no `[[bin]]` yet.
- **AST chunking and BLAKE3 for retrieval:** `pkg/rag/indexer.go` chunks Go ASTs itself. Decide whether `dendrite-rag-rs` links `dendrite-core` directly or receives pre-sliced chunks over RPC.
- **Split packages:**
  - `pkg/agentsearch`: the search primitives go to `dendrite-rag-rs`; agent routing stays here.
  - `pkg/store`: the engine goes to `dendrite-rag-rs`; the CLI and UI stay here.
  - `pkg/rag/ask.go`: retrieval goes to `dendrite-rag-rs`; LLM routing stays here.
- **MCP wire types:** they are defined twice, in `pkg/mcpserver/types.go` and `pkg/jetbrains/types.go`; unify them in the hub.
- **Sidecar invocation from the hub:** decide whether the hub spawns its own sidecars or delegates through dendrite-control.
- **Cyclic hunk graphs:** `dendrite-git-rs` needs SCC condensation over possibly-cyclic hunk graphs. That algorithm belongs in embedag (ticket 9.5.1), not in a local Tarjan pass.
