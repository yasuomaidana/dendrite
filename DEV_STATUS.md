# Dendrite Project Development Status & Roadmap

This document maintains the real-time development status, milestones, and active ticket execution pipeline for
**Dendrite**, managed following the autonomous ticket lifecycle protocol.

---

## Current Status Overview

- **Active Milestone**: Milestone 17 — Performance Testing, Inference Benchmarking & Gateway Load Harness (FEAT-021)
- **Current Focus**: `DEN-414: Code debt: remove the repeated code (124 clone groups)`
- **Last Updated**: 2026-11-01
- **Summary**: Backlog: 157 | To Do: 114 | In Progress: 51 | To Verify: 219 | Done: 287 (Total: 828)
- **Overall Progress**: 287 / 828 Tickets Completed (34.7%)

---

## Missing Ticket Complexity Analysis & Execution Ordering

All missing / incomplete tickets have been analyzed and ordered from lowest complexity (least dependencies,
self-contained) to highest complexity (multi-component, distributed persistence, AI AST feedback):

| Complexity Rank | Ticket ID   | Milestone    | Feature ID | Story Points | Complexity Score | Target Components                                            |        Status         |
|:---------------:|:------------|:-------------|:-----------|:------------:|:----------------:|:-------------------------------------------------------------|:---------------------:|
|      **1**      | **DEN-033** | Milestone 14 | FEAT-018   |     3 SP     |     2.0 / 10     | `pkg/telemetry/exporter`, `stacks/compose.grafana.yaml`      |   **COMPLETED [x]**   |
|      **2**      | **DEN-030** | Milestone 11 | FEAT-015   |     5 SP     |     4.0 / 10     | `pkg/topology/network`, `pkg/orchestrator`                   |   **COMPLETED [x]**   |
|      **3**      | **DEN-032** | Milestone 13 | FEAT-017   |     5 SP     |     5.0 / 10     | `pkg/topology/advisor`, `cmd/dendrite`                       |   **COMPLETED [x]**   |
|      **4**      | **DEN-034** | Milestone 15 | FEAT-019   |     8 SP     |     6.0 / 10     | `infra/modules/azure-burst`, `pkg/orchestrator/azure`        |   **COMPLETED [x]**   |
|      **5**      | **DEN-035** | Milestone 16 | FEAT-020   |     8 SP     |     7.0 / 10     | `pkg/inference`, `pkg/gateway`                               |   **COMPLETED [x]**   |
|      **6**      | **DEN-036** | Milestone 17 | FEAT-021   |     8 SP     |     7.5 / 10     | `pkg/perf`, `cmd/dendrite`, `tests/perf`                     | **CURRENT FOCUS [ ]** |
|      **8**      | **DEN-037** | Milestone 18 | FEAT-022   |    13 SP     |     8.5 / 10     | `pkg/agentsearch`, `cmd/dendrite`                            |    **QUEUED [ ]**     |
|      **9**      | **DEN-038** | Milestone 19 | FEAT-023   |    13 SP     |     9.0 / 10     | `pkg/testgen`, `pkg/uitest`, `cmd/dendrite`                  |    **QUEUED [ ]**     |
|      **7**      | **DEN-039** | Milestone 20 | FEAT-024   |     8 SP     |     8.0 / 10     | `pkg/rea`, `pkg/ai/import`, `tests/showcase`, `cmd/dendrite` |    **QUEUED [ ]**     |

---

## Milestone Execution Pipeline

### QA-M0: Safety & Hygiene

- [ ] **DEN-900**: Fix cmd/dendrite exit codes [State: To Verify, Priority: Urgent]
- [ ] **DEN-901**: Untrack runtime artifacts and committed secret [State: To Verify, Priority: Urgent]
- [ ] **DEN-902**: EnsureLiteLLMMasterKey + compose wiring [State: In Progress, Priority: Urgent]
- [ ] **DEN-976**: Spawned projects get lifecycle states (To Verify) and exact ticket-ID matching [State: To Verify, Priority: High]
- [ ] **DEN-977**: plane create/update accept Markdown spec files [State: To Verify, Priority: High]
- [ ] **DEN-978**: Hermetic agent tests: no live daemon probing or real agent execution [State: To Verify, Priority: Urgent]
- [ ] **DEN-979**: De-mock pkg/story/fork tests with real git repos [State: To Verify, Priority: High]
- [ ] **DEN-980**: dendrite commit pushes after committing when a remote is configured [State: To Verify, Priority: Urgent]
- [ ] **DEN-981**: GitHub Copilot access via LiteLLM: persisted device login and configurable model [State: In Progress, Priority: Urgent]
- [ ] **DEN-983**: dev ticket set accepts human-readable state names [State: To Verify, Priority: Urgent]
- [ ] **DEN-984**: plane replicate reflects every ticket and the To Verify state in DEV_STATUS.md [State: To Verify, Priority: High]
- [ ] **DEN-985**: Enforced documentation: generated CLI/MCP reference, README and docs audit gate [State: To Verify, Priority: High]
- [ ] **DEN-986**: Per-package coverage ratchet towards the 100% goal [State: To Verify, Priority: High]
- [ ] **DEN-987**: Stop env replicate and root tests from reverting the LiteLLM control stack [State: To Verify, Priority: Urgent]
- [ ] **DEN-994**: Raise pkg/mcpserver statement coverage to 100% [State: In Progress, Priority: High]
- [ ] **DEN-995**: Raise pkg/server statement coverage to 100% [State: In Progress, Priority: High]
- [ ] **DEN-996**: Raise pkg/devtool statement coverage to 100% [State: In Progress, Priority: High]

### Milestone 1: Core Foundation, Developer Tools & Ollama Engine

- [x] **DEN-001**: Initialize Development Status Ledger & Execution Roadmap [State: Done, Priority: Medium]
- [x] **DEN-002**: Implement Idiomatic Ollama Client & Metal Engine Support [State: Done, Priority: Medium]
- [x] **DEN-003**: Developer Productivity Tools on the Fly & CLI Scaffold [State: Done, Priority: Medium]
- [x] **DEN-004**: Ticket Module & Local Status Synchronization Engine [State: Done, Priority: Medium]
- [ ] **DEN-701**: Git Branch Guard: main lock, develop default & atomic CommitAndPush [State: To Verify, Priority: Urgent]
- [ ] **DEN-702**: Ticket Domain Model, Triage & Low-Budget DAG Prioritizer [State: To Verify, Priority: Urgent]
- [ ] **DEN-703**: GitHub Issues Two-Way Synchronizer & Label Provisioner [State: To Verify, Priority: Urgent]
- [ ] **DEN-704**: Speculative Stacked Branch Orchestrator & Degraded Budget Mode [State: To Verify, Priority: Urgent]
- [ ] **DEN-705**: dendrite audit-changes CLI Command & Vendor LLM Rebase/Merge Engine [State: To Verify, Priority: Urgent]
- [ ] **DEN-801**: Topic Zero State Prompt Engine & Multi-Turn State Machine [State: In Progress, Priority: High]
- [ ] **DEN-802**: Welcome Prompt Suite & Cross-Source Synthesis Dispatcher [State: To Verify, Priority: High]
- [ ] **DEN-804**: System Installer Helpers: Pre-Flight Audit & Port/Hardware Prober [State: To Verify, Priority: High]
- [ ] **DEN-805**: Local LLM Model Provisioner & LiteLLM Gateway Routing Bridge [State: To Verify, Priority: Medium]
- [ ] **DEN-806**: Dynamic 10-Style Greeting Matrix & Cliché Suppression Engine [State: To Verify, Priority: Medium]
- [ ] **DEN-807**: Canvas Interactive Writing & Direct Creative Action Handlers [State: To Verify, Priority: Medium]
- [ ] **DEN-809**: Local LLM Inference Concurrency Limiter & FIFO Priority Queue [State: To Verify, Priority: High]
- [ ] **DEN-817**: Decouple Git Commit Synthesis Engine: OxiLLaMa HTTP Adapter & Configurable Fallbacks [State: To Verify, Priority: Medium]
- [ ] **DEN-822**: Modernize Root CLI with Cobra-style Command Architecture and Local Model Delegation [State: To Verify, Priority: High]
- [ ] **DEN-910**: ASTProvider iface + Go go/ast impl [State: In Progress, Priority: High]
- [ ] **DEN-911**: AST mock-reachability analyzer [State: Backlog, Priority: High]
- [ ] **DEN-912**: Stage B toolchain adapter (go/cargo/bun/uv) + changed-pkg coverage & cyclomatic [State: To Verify, Priority: High]
- [ ] **DEN-913**: LLM assertion query + strict JSON verdict [State: Backlog, Priority: High]
- [ ] **DEN-914**: Delete stub analyzers; rewire AuditFinishedTicket to pkg/assert; 85% gate [State: Backlog, Priority: High]
- [ ] **DEN-915**: TODO extractor + Plane child-ticket spawner [State: Backlog, Priority: High]
- [ ] **DEN-916**: GatedTracker [State: Backlog, Priority: High]
- [ ] **DEN-917**: Backfill re-audit: only the tickets NOT machine-confirmed by the DEN-437 sweep [State: To Verify, Priority: High]
- [ ] **DEN-982**: Replace production mock types with real implementations [State: To Verify, Priority: Urgent]

### Milestone 2: Multi-Node Topology & Cloud-Burst Infrastructure

- [x] **DEN-005**: Topology Configuration Model & Scenario Placement Types [State: Done, Priority: Medium]
- [x] **DEN-006**: Terraform Cloud-Init & Compose Profile Runner [State: Done, Priority: Medium]
- [x] **DEN-016**: Terraform AWS Spot EC2 Module & Cloud-Init Bootstrap [State: Done, Priority: Medium]
- [x] **DEN-017**: Persistent EBS Volume Lifecycler & Zero-Ingress Security Group Validator [State: Done, Priority: Medium]
- [ ] **DEN-029**: Development Environment Replication, Host Installer & Containerized Tooling [State: To Verify, Priority: Medium]
- [ ] **DEN-920**: Capability catalog (models.toml) + bench ingestion [State: In Progress, Priority: High]
- [ ] **DEN-921**: Capability scorer + OxiLLaMa tie-break + C1-3 small-model rule [State: Backlog, Priority: High]
- [ ] **DEN-922**: Lifecycle manager: leases, derived 30m countdown, unload [State: Backlog, Priority: High]
- [ ] **DEN-923**: Probe + auto-start + VRAM check [State: Backlog, Priority: High]
- [ ] **DEN-924**: Escalation ladder + HandoverCapsule + retry budgets [State: Backlog, Priority: High]
- [ ] **DEN-925**: Route all inference through router/lease [State: Backlog, Priority: High]

### Milestone 3: Token Budget Governance & Fallback Gateway

- [x] **DEN-007**: LiteLLM Configuration Template & Dynamic Renderer [State: Done, Priority: Medium]
- [x] **DEN-008**: Sliding Window Budget Limiter & Ollama Failover Engine [State: Done, Priority: Medium]
- [ ] **DEN-934**: --local-review stacked branches + git notes telemetry + review audit/land [State: Backlog, Priority: High]

### Milestone 4: Autonomous Coding Agent Harness & Tool Orchestration

- [ ] **DEN-024**: Agent runner harness: bouton/agentjobs runner shape for current executors (claude, agy, local; Hermes via DEN-432) [State: To Verify, Priority: Medium]
- [x] **DEN-025**: Agent Sandbox Isolation & LiteLLM Gateway Budget Binding [State: Done, Priority: Medium]
- [ ] **DEN-026**: Agent Session Tracker, Git Commit Provenance & Issue Tracker Sync [State: In Progress, Priority: Medium]
- [ ] **DEN-819**: Externalize Agent Runner Gateway Credentials & Virtual Key Governance [State: Backlog, Priority: Medium]

### Milestone 4: Autonomous Coding Agent Harness &amp; Tool Orchestration

- [ ] **DEN-103**: FEAT-013 & Completed Agent Modules: Exhaustive Test Suite & 100% Statement Coverage [State: In Progress, Priority: Medium]

### QA-M4: Token-Limit Recovery (Pillar 13)

- [ ] **DEN-940**: TokenCounter impls + Budget.Fit [State: Backlog, Priority: High]
- [ ] **DEN-941**: <StateCheckpoint> schema + Engram flush [State: Backlog, Priority: High]
- [ ] **DEN-942**: Truncation splicer [State: Backlog, Priority: High]
- [ ] **DEN-944**: 429 backoff + quota-floor downshift / PAUSED_QUOTA_FLOOR [State: Backlog, Priority: High]
- [ ] **DEN-945**: LiteLLM enable_pre_call_checks + per-local-alias context_window_fallbacks [State: In Progress, Priority: High]
- [ ] **DEN-988**: Agent job ledger (WAL) and outcome classifier [State: To Verify, Priority: Urgent]
- [ ] **DEN-989**: dendrite agent jobs dispatcher CLI with scope guard [State: To Verify, Priority: Urgent]
- [ ] **DEN-990**: Quota fallback chain with governor integration and park/resume [State: To Verify, Priority: Urgent]
- [ ] **DEN-991**: Verify-only watcher daemon for agent jobs [State: In Progress, Priority: Urgent]
- [ ] **DEN-992**: dendrite session checkpoint and resume for orchestrator handoff [State: To Verify, Priority: Urgent]
- [ ] **DEN-993**: Spike: capture agent quota errors and AGY conversation resume [State: To Verify, Priority: Urgent]

### Milestone 5: Reverse Engineering & Sandboxed Intelligence

- [x] **DEN-011**: Ephemeral Hardened Container Sandbox with RAII Drop [State: Done, Priority: Medium]
- [ ] **DEN-950**: Cobra noun-verb tree; port all 28 commands; hidden deprecated aliases [State: Backlog, Priority: Medium]
- [ ] **DEN-951**: config edit transactional editor flow [State: Backlog, Priority: Medium]
- [ ] **DEN-952**: 4-source token aggregator + tokens live TUI/SSE [State: Backlog, Priority: Medium]
- [ ] **DEN-953**: Tool registry (tools.toml + SQLite) + installer enforcement [State: Backlog, Priority: Medium]
- [ ] **DEN-954**: Supervisor pre/stream/post hooks [State: Backlog, Priority: Medium]

### QA-M5: CLI, Tokens & Tool Registry (Pillars 10-12)

- [ ] **DEN-1023**: refactor(cli): cobra root, global flags, docs generator on the command tree [State: To Verify, Priority: High]
- [ ] **DEN-1024**: refactor(cli): native cobra soma, ganglion and work subcommands [State: To Verify, Priority: Medium]
- [ ] **DEN-1025**: refactor(cli): native cobra chronos (plane) subcommand tree [State: To Verify, Priority: Medium]
- [ ] **DEN-1026**: refactor(cli): native cobra cleft, spike, velocity and inter subcommands [State: To Verify, Priority: Medium]
- [ ] **DEN-1027**: refactor(cli): native cobra arbor, afferent, plexus, retro and mesh subcommands [State: To Verify, Priority: Medium]
- [ ] **DEN-1028**: refactor(cli): native cobra engram, cognition, vesicle and quantal subcommands [State: To Verify, Priority: Medium]
- [ ] **DEN-1029**: refactor(cli): native cobra cortex, sensory, codex and bridge subcommands [State: To Verify, Priority: Medium]
- [ ] **DEN-1030**: refactor(cli): native cobra bouton, reflex, triage, ask and greeting subcommands [State: To Verify, Priority: Medium]
- [ ] **DEN-1031**: refactor(cli): native cobra myelin, prune, allostasis, motor, translocate and saga subcommands [State: To Verify, Priority: Medium]
- [ ] **DEN-1032**: refactor(cli): rewrite callers, skills and docs to the lore command tree [State: To Verify, Priority: Medium]

### Milestone 6: AI-Assisted Token Optimization & Cleanroom Import

- [x] **DEN-018**: AST Context Compaction & LiteLLM Prefix Cache Harness [State: Done, Priority: Medium]
- [ ] **DEN-019**: Cross-Repository Capability Dissection & Cleanroom Synthesizer [State: To Do, Priority: Medium]
- [ ] **DEN-931**: Spike: Plane AI env/edition verification [State: Backlog, Priority: Medium]
- [ ] **DEN-961**: Plane AI → LiteLLM virtual key + local-coder alias [State: Backlog, Priority: Medium]
- [ ] **DEN-962**: TicketSpec schema + vagueness linter + /plan auto-trigger [State: Backlog, Priority: Medium]
- [ ] **DEN-963**: Single-source skill generation for .agents/.gemini lifecycle skills [State: Backlog, Priority: Medium]

### Milestone 7: Polyglot Translation & Documentation Reflection

- [ ] **DEN-021**: Idiomatic Target Code Synthesizer & Dual Property Test Parity Generator [State: In Progress, Priority: Medium]
- [x] **DEN-022**: AST Docs-as-Code Synthesizer for FEAT/ADR Specs & OpenAPI 3.1 [State: Done, Priority: Medium]
- [x] **DEN-023**: Cross-Project Documentation Reflection Engine & Docusaurus Sync [State: Done, Priority: Medium]
- [ ] **DEN-970**: Sidecar JSON-RPC contract + Go client with in-process fallback [State: Backlog, Priority: Low]
- [ ] **DEN-971**: Extract dendrite-mcp-hub [State: Backlog, Priority: Low]
- [ ] **DEN-972**: dendrite-core-rs: tree-sitter fold / last-node / complexity / blake3 / tokenize [State: Backlog, Priority: Low]
- [ ] **DEN-973**: dendrite-git-rs: gix hunk graph + clusterer [State: Backlog, Priority: Low]
- [ ] **DEN-974**: dendrite-rag-rs: BM25+vector hybrid + sync engine [State: Backlog, Priority: Low]
- [ ] **DEN-975**: Pool dogfood E2E across 5 sibling dirs [State: Backlog, Priority: Low]

### Milestone 8: Multi-Project Workspaces & Pluggable Trackers

- [x] **DEN-012**: Multi-Project Workspace Registry & Context Switcher [State: Done, Priority: Medium]
- [x] **DEN-013**: Pluggable Issue Tracker Interface & Atlassian Jira Adapter [State: Done, Priority: Medium]
- [ ] **DEN-027**: Plane v2 & Linear Strategy Adapters [State: Backlog, Priority: Medium]

### Milestone 9: Web UI & Dashboard Engine

- [x] **DEN-014**: Embedded HTTP Control Server & WebSocket Telemetry [State: Done, Priority: Medium]
- [ ] **DEN-015**: Real-Time Telemetry Gauges & Thermal Monitoring Daemon [State: In Progress, Priority: Medium]

### Milestone 10: IDE Intelligence & JetBrains MCP Bridge

- [ ] **DEND-301**: [JetBrains Validation]: Audit live IDE socket bridge & state detection without mocks [State: In Progress, Priority: High]
- [ ] **DEND-302**: [Plugin - JetBrains]: JetBrains Plugin (dendrite-jetbrains) with local AI chat & Graphify markers [State: To Do, Priority: Urgent]
- [ ] **DEND-303**: [Plugin - VS Code]: VS Code Extension (dendrite-vscode) built with bun & MCP context [State: To Do, Priority: High]
- [ ] **DEND-304**: [Plugin - Neovim]: Headless Lua RPC client [State: To Do, Priority: Medium]
- [ ] **DEND-305**: [Plugin - Zed]: Rust WASM extension [State: To Do, Priority: Medium]

### Milestone 11: Pluggable Mesh Networking & Zero-Trust Connectivity

- [ ] **DEN-030**: Multi-Mesh Networking Abstraction: Cloudflare Tunnels, SSH Port Forwarding, WireGuard, and Direct LAN [State: To Verify, Priority: Medium]

### Milestone 12: Open-Source Project Management & Design Tooling

- [ ] **DEN-031**: Self-Hosted Plane v2 & Penpot Design Token Stacks & Replicator [State: To Verify, Priority: Medium]
- [ ] **DEN-097**: Plane Doc Sync Taxonomy Guard & Markdown-to-HTML Robust Formatter [State: To Verify, Priority: Medium]
- [ ] **DEND-402**: [Infra - Start]: Mock-free service lifecycle manager with Penpot compose/MCP provisioning [State: To Verify, Priority: High]

### Milestone 13: Remote SSH Hardware Discovery & Automated Service Placement Router

- [ ] **DEN-032**: Remote SSH Node Topology Provisioner & Dynamic Service Placement Router [State: To Verify, Priority: Medium]
- [x] **DEND-401**: [Infra - Migration]: Zero-credential SSH service migration [State: Done, Priority: Urgent]
- [ ] **DEND-403**: [Infra - Sync]: Cross-node endpoint & schema update propagation [State: To Do, Priority: High]

### Milestone 14: Hybrid Telemetry Architecture & Prometheus/Grafana Observability Suite

- [ ] **DEN-033**: Prometheus `/metrics` Exporter & Grafana Observability Dashboard Integration Pack [State: To Verify, Priority: Low]

### Milestone 15: Azure Cloud-Burst Infrastructure & Managed Storage

- [ ] **DEN-034**: Terraform Azure Spot VM Module & Persistent Managed Disk Lifecycler [State: To Verify, Priority: Medium]

### Milestone 16: Pluggable Multi-Provider Inference Engines

- [ ] **DEN-035**: Unified Inference Engine Gateway: OxiLLaMa, Hugging Face TGI/TEI, vLLM, llama.cpp & Ollama Metal [State: To Verify, Priority: High]
- [ ] **DEN-809**: Model Migration Engine: Migrate Models from Ollama to OxiLLaMa & Evict Local Blobs [State: Backlog, Priority: High]
- [ ] **DEN-815**: Decouple Hardcoded Inference Engine Endpoints, Cascade Sequence & Model Catalog [State: To Verify, Priority: High]

### Milestone 17: Performance Testing, Inference Benchmarking & Gateway Load Harness

- [ ] **DEN-036**: k6 Performance Testing, TTFT/TPS Benchmarking & Cascade Failover Load Harness [State: To Verify, Priority: High]

### Milestone 18: Agent Search, Capability Discovery & Semantic Routing Engine

- [ ] **DEN-037**: Hybrid Agent Search, TEI Embeddings & Semantic Routing Engine [State: In Progress, Priority: High]

### Milestone 19: Developer Test Automation, Self-Healing Harness & UI Integration Suite

- [ ] **DEN-038**: Automated Test Synthesizer, Self-Healing Convergence Loop & Web UI Test Harness [State: In Progress, Priority: Urgent]
- [ ] **DEND-101**: [Anti-Mock Guard]: Tree-sitter AST mock detection & pre-commit hook [State: In Progress, Priority: Urgent]
- [ ] **DEND-102**: [Verification Routine]: Plane To Verify intermediate state & dendrite verify skill [State: In Progress, Priority: High]
- [ ] **DEND-103**: [Toolchain Migration]: Standardize build & test tooling on uv and bun [State: To Verify, Priority: Medium]
- [ ] **DEN-821**: Standardize Toolchain Migration on uv (Python) and bun (Node/TS) across Website, Scripts & CI/CD [State: Backlog, Priority: High]

### Milestone 20: Open-Source REA Showcase Pipeline & Cleanroom Parity Verification Suite

- [ ] **DEN-039**: Open-Source Reverse Engineering Showcases & Cleanroom Parity Verification Suite [State: To Verify, Priority: High]
- [ ] **DEND-501**: [REA]: macOS PE .EXE analysis, pause/resume checkpointing, multi-tier LLM triage & purge [State: In Progress, Priority: High]

### Milestone 22: Distributed Codebase Ecosystem, Modular REA & Cross-Project Interoperability Coordinator

- [ ] **DEN-041**: Distributed Codebase Multi-Source Ecosystem & Priority Resolver [State: To Verify, Priority: Medium]
- [ ] **DEN-042**: Scenario 1: REA Modular Decomposition & Ecosystem Reuse Discovery Engine [State: To Verify, Priority: Medium]
- [ ] **DEN-043**: Scenario 2: Parallel Multi-Project Workspace & Repository Spawner [State: To Verify, Priority: Medium]
- [ ] **DEN-044**: Scenario 3: Local LLM Cross-Project Interoperability Coordinator [State: To Verify, Priority: Medium]
- [ ] **DEN-045**: Scenario 4: Generative Satellite Project Synthesis & Autonomous Plane Project Provisioning [State: To Verify, Priority: Medium]
- [ ] **DEN-046**: JetBrains IDE Toolset Integration, 19 MCP Tools & Agentic Developer Workflow Guide [State: To Verify, Priority: Medium]

### Milestone 23: Hardening, Benchmark Scenarios & Cleanroom Invariant Governance

- [ ] **DEN-047**: Fix Goroutine Deadlock & Dynamic Buffer Starvation in Synthetic k6 Benchmark Runner [State: To Verify, Priority: Medium]
- [ ] **DEN-048**: Automatic Zero-Latency Fallback Cascade for Unreachable Inference Upstreams in Benchmark Scenarios [State: To Verify, Priority: Medium]
- [ ] **DEN-049**: Multi-Digest Test Vector Errata & 63-Byte Alignment Invariant Suite for REA Showcases [State: To Verify, Priority: Medium]

### Milestone 23: Hardening, Benchmark Scenarios, Cleanroom Invariants & Local Agent Triage

- [ ] **DEN-050**: Local Agent Triage Gateway, Token-Efficient Prompt Router & JetBrains AI Chat Multi-MCP Orchestrator [State: In Progress, Priority: Medium]
- [ ] **DEN-816**: Dynamic Triage Engine Hardware Probing & Model Precedence Resolution [State: In Progress, Priority: High]

### Milestone 24: Autonomous Pool DAG & Multi-IDE Project Synchronization

- [ ] **DEN-051**: Autonomous Dependency Inference & Multi-Directory DAG Solver [State: To Verify, Priority: Medium]
- [ ] **DEN-052**: Multi-Mode Dependency Switcher: Published, Branch, Local Lockstep & Remote Fork [State: To Verify, Priority: Medium]
- [ ] **DEN-053**: Mid-Flight Project Splitter & Mandatory Interactive Target Directory Gate [State: To Verify, Priority: Medium]
- [ ] **DEN-813**: Project Environment Preparation, Agent/Skill Sync & CI/CD Enforcement with GitHub Copilot [State: To Verify, Priority: High]

### Milestone 25: Universal Query Engine & 3-Tier Library Reuse Auditor

- [ ] **DEN-054**: Universal Intent Slicing & Binary/Repo Debloating Engine [State: To Verify, Priority: Medium]
- [ ] **DEN-055**: Subsystem Trait Boundary Adapter & Graphics Engine Swapper [State: To Verify, Priority: Medium]
- [ ] **DEN-056**: Idiomatic Binary/Repo-to-Rust Transpiler & Differential Test Harness [State: To Verify, Priority: Medium]
- [ ] **DEN-057**: Multi-Artifact Architecture Fusion & In-Process Channel Synthesizer [State: To Verify, Priority: Medium]
- [ ] **DEN-058**: Strict 3-Tier Library Reuse Hierarchy Lookup & ADR Provenance Recorder [State: To Verify, Priority: Medium]

### Milestone 26: Persistent Segmented Store & Token-Aware Hybrid Scheduler

- [ ] **DEN-059**: BLAKE3 Content-Addressed Persistent Segmented Store & Chunk Checkpointer [State: To Verify, Priority: Medium]
- [ ] **DEN-060**: Store Inspection, Chunk Status Telemetry & Explicit User Purge Lifecycler [State: To Verify, Priority: Medium]
- [ ] **DEN-061**: Token-Aware Hybrid Scheduler: Night Local Window & 48h Pre-Reset Cloud Sprint [State: To Verify, Priority: Medium]
- [ ] **DEN-062**: Quota Floor Safety Guard (20%/0% Drain Override) & Continuous Analysis Pause Engine [State: To Verify, Priority: Medium]

### Milestone 27: Advanced Polyglot Contract Sync, Speculative Branching & Upstream Drift

- [ ] **DEN-064**: Speculative Git Worktree Branching & Differential Architecture Benchmark Harness [State: To Verify, Priority: Medium]
- [ ] **DEN-065**: Sliced Upstream Commit Drift Analyzer & Bug Fix Porting Generator [State: To Do, Priority: Medium]
- [ ] **DEN-066**: Multi-Repo Context Re-Hydration & IDE Scope Restorer [State: To Do, Priority: Medium]

### Milestone 28: Zero-Data-Loss Infrastructure Mobility & Smart Repo-Aware Backup Engine

- [ ] **DEN-067**: 5-Phase Atomic Service Migration Engine: Local <-> Debian <-> Azure [State: To Verify, Priority: Medium]
- [ ] **DEN-068**: Write-Ahead Log (WAL) State Journaling & Merkle Digest Verification Protocol [State: To Do, Priority: Medium]
- [ ] **DEN-069**: Smart Repo-Aware Backup Engine with Git-Persisted Blob Eviction Priority [State: To Do, Priority: Medium]

### Milestone 29: Dual-Mode Configuration Parity & Multi-Channel Go CI/CD Automation

- [ ] **DEN-070**: Unified Configuration Subsystem with nvim/vim Transactional Staging & Taplo Schema [State: To Do, Priority: Medium]
- [ ] **DEN-071**: Multi-Channel CI/CD Distribution Pipeline: Brew Tap, Debian .deb, GHCR & GoReleaser [State: To Do, Priority: Medium]
- [ ] **DEN-818**: Externalize Hardcoded Tailscale IPs & Topology Mesh Endpoints [State: Backlog, Priority: Medium]

### Milestone 30: Multi-Provider AI Runtime & Semantic Hierarchical Git Governance

- [ ] **DEN-072**: Multi-Provider AI Inference Adapter: GitHub Copilot CLI & Native/Bridged Ollama [State: To Do, Priority: High]
- [ ] **DEN-073**: 5-Tier Version Strategy Interceptor & Semantic Hierarchical Branch/Merge Engine [State: To Do, Priority: Urgent]
- [ ] **DEN-098**: Autonomous Git Commit Engine with Local Model Message Delegation & Zero-Cost Synthesis [State: To Verify, Priority: High]

### Milestone 31: Ticket-Milestone DAG Linker & Story Prioritization Engine

- [ ] **DEN-074**: Ticket-Milestone Relational Ingestion & Dependency DAG Solver [State: To Verify, Priority: High]
- [ ] **DEN-075**: Context-Aware `dendrite stories next` Scheduler & Active Story Context Injector [State: In Progress, Priority: High]
- [ ] **DEND-601**: [Tickets Skill]: Ticket Connection & Priority Updater [State: To Verify, Priority: Urgent]

### Milestone 32: Bidirectional Penpot UI Documentation & MCP Integration

- [ ] **DEN-076**: UI File Mutation Watcher & Penpot Documentation Token Diff Synthesizer [State: To Verify, Priority: Medium]
- [ ] **DEN-077**: Bidirectional Traceability: Ticket Comment Frame Deep-Linking & Design Drift Sync [State: To Verify, Priority: Medium]
- [ ] **DEN-810**: Penpot Service Lifecycle & Web UI Development Synchronized with Plane [State: To Verify, Priority: High]

### Milestone 33: Self-Update Lifecycle & Developer Mode Dual-Source Engine

- [ ] **DEN-078**: Self-Update Lifecycle Manager with Checksum Verification & Binary Rollback [State: To Do, Priority: High]
- [ ] **DEN-079**: Developer Mode (`--dev`) Dual-Source Engine: Local-First Build & Remote Override [State: To Do, Priority: High]

### Milestone 34: Polyglot Environment Bootstrapper & GoF Pattern Analyzer

- [ ] **DEN-080**: Polyglot Stack Fingerprinter & Agent Environment Initializer [State: To Verify, Priority: Medium]
- [ ] **DEN-081**: GoF Design-Pattern Analyzer Skill Generator [State: To Do, Priority: Medium]
- [ ] **DEN-082**: SOTA Best-Practices Web/Advisory Diff Inspector & Automated Updater [State: To Do, Priority: Medium]

### Milestone 35: Offline-First Local Cache & Outbox Sync Engine

- [ ] **DEN-083**: Embedded Turso/libSQL Local Cache with WAL Journaling & Testcontainers Integration [State: To Do, Priority: Medium]
- [ ] **DEN-084**: Transactional Outbox Mutation Queue & Exponential Backoff Replay Worker [State: To Do, Priority: Medium]
- [ ] **DEN-085**: Automated Gitignore Binary & Cache Artifact Exclusion Guardian [State: To Do, Priority: Medium]

### Milestone 36: Pre-Commit Secret Scanner & Pre-PR/Merge Quality Gate

- [ ] **DEN-086**: Deterministic Pre-Commit Entropy & Credential Scanner Interceptor [State: To Do, Priority: Urgent]
- [ ] **DEN-087**: Automated Pre-PR & Pre-Merge Build, Lint, and Test Verification Gate [State: In Progress, Priority: Urgent]

### Milestone 37: Quota Failover, Worktree Isolation & Immutable Agent Audit Logging

- [ ] **DEN-088**: Hybrid Quota Scheduler & Mid-Story Copilot-to-Ollama Graceful Failover [State: To Do, Priority: Medium]
- [ ] **DEN-089**: Ephemeral Git Worktree Isolation Engine & RAII Rollback Controller [State: In Progress, Priority: Medium]
- [ ] **DEN-090**: Immutable libSQL Agent Audit Log & `dendrite audit` Trace CLI [State: To Do, Priority: Medium]

### Milestone 38: Artifact & Package Publisher MCP Service & Registry Governance

- [ ] **DEN-092**: Pre-Flight Validation, Dry-Run SemVer Guard & Secret-Isolated Execution Gate [State: To Do, Priority: Medium]
- [ ] **DEN-374**: Governance 0.1: clean stale ganglion registry entries [State: To Verify, Priority: Medium]
- [ ] **DEN-375**: Governance 1.1: ticketlock package (session token, claim, guard) [State: To Verify, Priority: High]
- [ ] **DEN-376**: Governance 1.2: Plane assignee and labels via sidecar [State: To Verify, Priority: High]
- [ ] **DEN-377**: Governance 1.3: MCP and CLI ticket lifecycle tools [State: To Verify, Priority: High]
- [ ] **DEN-378**: Governance 1.4: pre-edit hook, plan-saved hook, git pre-commit, MCP config writers [State: To Verify, Priority: High]
- [ ] **DEN-379**: Governance 1.5: finish command, two-stage assertion, auto-Done [State: To Verify, Priority: High]
- [x] **DEN-380**: Governance 2.3: dendrite directive command [State: Done, Priority: High]
- [x] **DEN-381**: Governance 2.4: allostasis gate and 100% coverage floor [State: Done, Priority: High]
- [ ] **DEN-382**: Governance 3.1: pkg/lang adapter and 5 profiles [State: Backlog, Priority: High]
- [ ] **DEN-383**: Governance 3.2: templated CI/Makefile/config rendering (Rust set) [State: Backlog, Priority: High]
- [ ] **DEN-384**: Governance 3.3: Python/TS/Go/Kotlin template sets [State: Backlog, Priority: Medium]
- [ ] **DEN-385**: Governance 3.4: unknown-language adapter generation [State: Backlog, Priority: Medium]
- [ ] **DEN-386**: Governance 4.1: afferent check three-tier reuse [State: Backlog, Priority: Medium]
- [ ] **DEN-387**: Governance 4.3: dendrite meiosis [State: Backlog, Priority: Medium]
- [ ] **DEN-388**: Governance 5.1: immune update (Macrophage) [State: Backlog, Priority: Medium]
- [ ] **DEN-389**: Governance 5.2: immune scan (T-cell) [State: Backlog, Priority: High]
- [ ] **DEN-390**: Governance 5.3: immune enforce (B-cell) [State: Backlog, Priority: Medium]
- [ ] **DEN-393**: Governance 1.6: ticketlock review fixes (bypass, needs-input, progress comments, adopt unclaimed plan) [State: To Verify, Priority: High]
- [ ] **DEN-394**: Chore: gofmt the ~20 unformatted Go files [State: To Verify, Priority: Medium]
- [ ] **DEN-395**: Governance 1.7: dogfood - soma prepare installs the ticket lock in the Dendrite repo [State: To Verify, Priority: Medium]
- [x] **DEN-396**: Bug: soma prepare overwrites a hand-maintained .github/workflows/ci.yml with the generic template [State: Done, Priority: High]
- [ ] **DEN-397**: Bug: AGENTS.md Synapses section says Plane project unknown for peers that have one [State: To Verify, Priority: Medium]
- [ ] **DEN-398**: Governance 1.8: record agent questions and decisions on the ticket [State: To Verify, Priority: High]
- [ ] **DEN-399**: Governance: decisions and plan (epic) [State: To Verify, Priority: Medium]
- [ ] **DEN-400**: Fix: data race in pkg/perf TestPreFlightCascadeToFallbackWithZeroPromptLoss [State: To Verify, Priority: High]
- [ ] **DEN-401**: Fix: finish gate report must show the failures, not only the last lines [State: To Verify, Priority: High]
- [ ] **DEN-402**: Fix: the finish gate runs the project's tests in a clean environment [State: To Verify, Priority: High]
- [ ] **DEN-403**: Fix: the finish review must see the ticket's whole work, merged or not [State: To Verify, Priority: High]
- [ ] **DEN-404**: Fix: the finish acceptance review must survive a chatty local model [State: To Verify, Priority: Medium]
- [ ] **DEN-405**: Fix: the finish review input must fit a local model [State: To Verify, Priority: High]
- [ ] **DEN-406**: Fix: the finish review proves its verdicts - evidence checked in the diff, one criterion at a time [State: To Verify, Priority: High]
- [ ] **DEN-407**: Fix: Dendrite's CI cannot run the tests - the runner has no dendrite-plane or dendrite-rag sidecar [State: To Verify, Priority: Urgent]
- [ ] **DEN-408**: Test: an isolated Plane stack (docker compose -p dendrite-test) for end-to-end checks [State: To Verify, Priority: High]

### Milestone 39: Local & Custom-Server Ephemeral CI/CD Routine & Mesh Runner

- [ ] **DEN-094**: Ephemeral Container Sandbox Runner & Automated Dynamic On-Demand Activation [State: To Do, Priority: Medium]
- [ ] **DEN-095**: Remote Tailscale Mesh SSH/Docker Execution Target & One-Time GitHub Ephemeral Runner [State: To Do, Priority: Medium]
- [ ] **DEN-096**: Token-Efficient Structured Log Tailing & MCP Routine Control Tools [State: To Do, Priority: Medium]
- [ ] **DEN-814**: Dynamic LiteLLM Routing Table Updater & Daily Scheduler with Manual Trigger [State: To Verify, Priority: High]
- [ ] **DEN-820**: Dynamic Gateway Renderer Model Routing by Live Engine Intelligence [State: To Verify, Priority: High]

### Milestone 40: Synapse Cross-Project Links & Multi-Agent Arbitration Mesh

- [ ] **DCORE-1**: dendrite-core-rs: tree-sitter fold / last-node / complexity / blake3 / tokenize sidecar [State: Backlog, Priority: Medium]
- [ ] **DGIT-1**: dendrite-git-rs: gix hunk graph + semantic clusterer sidecar [State: Backlog, Priority: Medium]
- [ ] **DMCP-1**: dendrite-mcp-hub: extract pkg/mcpserver into a standalone Go module [State: Backlog, Priority: Medium]
- [ ] **DRAG-1**: dendrite-rag-rs: BM25+vector hybrid retrieval and memory sync engine [State: Backlog, Priority: Medium]
- [ ] **DCORE-2**: [DEN-943] BLAKE3 chunk ledger + WAL + resume [State: Backlog, Priority: Medium]
- [ ] **DGIT-2**: [DEN-934] --local-review stacked branches + git notes telemetry + review audit/land [State: Backlog, Priority: Medium]
- [ ] **DMCP-2**: [DEN-960] Penpot MCP service + hub registration [State: Backlog, Priority: Medium]
- [ ] **DRAG-2**: [DEN-964] memory modes: local-only / edge (WAL) / push (+compaction) [State: Backlog, Priority: Medium]
- [ ] **DCORE-3**: [DEN-942] Truncation splicer (Go via go/ast; other languages after DEN-972) [State: Backlog, Priority: Medium]
- [ ] **DGIT-3**: [DEN-933] Clusterer + LLM plan + validators (exactly-once, topo order, per-commit build) [State: Backlog, Priority: Medium]
- [ ] **DMCP-3**: [DEN-930] Spike: Penpot official MCP server deployment shape [State: Backlog, Priority: Medium]
- [ ] **DRAG-3**: [DEN-803] Structured Topic Memory Parser & Zero-Extrapolation Fidelity Guard [State: To Verify, Priority: Medium]
- [ ] **DCORE-4**: [DEN-056] Idiomatic Binary/Repo-to-Rust Transpiler & Differential Test Harness [State: To Verify, Priority: Medium]
- [ ] **DGIT-4**: [DEN-932] Hunk extractor + symbol-dependency graph [State: Backlog, Priority: Medium]
- [ ] **DMCP-4**: [DEN-812] Bidirectional Multi-Vendor Migration & Real-Time Sync Engine (Plane <-> Jira, Penpot <-> Figma) [State: Backlog, Priority: Medium]
- [ ] **DRAG-4**: [DEND-203] [Local-First RAG & Shared Library Deduplication]: Zero-cost nomic embeddings & CAS library store [State: In Progress, Priority: Medium]
- [ ] **DCORE-5**: [DEN-020] Intermediate Semantic Representation ISR Tree-sitter Parser [State: To Do, Priority: Medium]
- [ ] **DGIT-5**: [DEN-098] Autonomous Git Commit Engine with Local Model Message Delegation & Zero-Cost Synthesis [State: To Verify, Priority: Medium]
- [ ] **DMCP-5**: [DEN-808] Personal Context MCP Connectors: Workspace & Enterprise Tool Adapters [State: Backlog, Priority: Medium]
- [ ] **DRAG-5**: [DEN-060] Store Inspection, Chunk Status Telemetry & Explicit User Purge Lifecycler [State: To Verify, Priority: Medium]
- [x] **DCORE-6**: [DEN-018] AST Context Compaction & LiteLLM Prefix Cache Harness [State: Done, Priority: Medium]
- [ ] **DGIT-6**: [DEN-065] Sliced Upstream Commit Drift Analyzer & Bug Fix Porting Generator [State: To Do, Priority: Medium]
- [ ] **DMCP-6**: [DEND-305] [Plugin - Zed]: Rust WASM extension (dendrite-zed) [State: To Do, Priority: Medium]
- [ ] **DRAG-6**: [DEN-059] BLAKE3 Content-Addressed Persistent Segmented Store & Chunk Checkpointer [State: To Verify, Priority: Medium]
- [x] **DCORE-7**: [DEN-010] Tree-sitter AST & Complexity Analyzer [State: Done, Priority: Medium]
- [ ] **DGIT-7**: [DEN-064] Speculative Git Worktree Branching & Differential Architecture Benchmark Harness [State: To Verify, Priority: Medium]
- [ ] **DMCP-7**: [DEND-304] [Plugin - Neovim]: Headless Lua RPC client (dendrite.nvim) [State: To Do, Priority: Medium]
- [ ] **DRAG-7**: [DEN-037] Hybrid Agent Search, TEI Embeddings & Semantic Routing Engine [State: In Progress, Priority: Medium]
- [ ] **DMCP-8**: [DEND-303] [Plugin - VS Code]: VS Code Extension (dendrite-vscode) built with bun & MCP context [State: To Do, Priority: Medium]
- [ ] **DMCP-9**: [DEND-302] [Plugin - JetBrains]: JetBrains Plugin (dendrite-jetbrains) with local AI chat & Graphify markers [State: To Do, Priority: Medium]
- [ ] **DMCP-10**: [DEND-201] [Code Knowledge MCP]: Local AST query MCP server [State: To Do, Priority: Medium]
- [ ] **DMCP-11**: [DEN-096] Token-Efficient Structured Log Tailing & MCP Routine Control Tools [State: To Do, Priority: Medium]
- [ ] **DMCP-12**: [DEN-093] Post-Publish Git Tag Lifecycle & Stdio JSON-RPC MCP Tool Registry [State: To Do, Priority: Medium]
- [ ] **DMCP-13**: [DEN-092] Pre-Flight Validation, Dry-Run SemVer Guard & Secret-Isolated Execution Gate [State: To Do, Priority: Medium]
- [ ] **DMCP-14**: [DEN-091] Pluggable Registry Adapter Interface & Cargo Workspace Topological Publisher [State: To Do, Priority: Medium]
- [ ] **DMCP-15**: [DEN-076] UI File Mutation Watcher & Penpot Documentation Token Diff Synthesizer [State: To Verify, Priority: Medium]
- [ ] **DMCP-16**: [DEN-075] Context-Aware `dendrite stories next` Scheduler & Active Story Context Injector [State: In Progress, Priority: Medium]
- [ ] **DMCP-17**: [DEN-063] Quad-IDE Cross-Language Schema & IPC Contract Drift Monitor [State: To Verify, Priority: Medium]
- [ ] **DMCP-18**: [DEN-050] Local Agent Triage Gateway, Token-Efficient Prompt Router & JetBrains AI Chat Multi-MCP Orchestrator [State: In Progress, Priority: Medium]
- [ ] **DMCP-19**: [DEN-046] JetBrains IDE Toolset Integration, 19 MCP Tools & Agentic Developer Workflow Guide [State: To Verify, Priority: Medium]
- [ ] **DMCP-20**: [DEN-040] Dendrite Native MCP Server & Tool Registry Bridge [State: To Verify, Priority: Medium]
- [ ] **DMCP-21**: [DEN-028] JetBrains IDE MCP Integration & Communication Bridge [State: To Verify, Priority: Medium]
- [ ] **DMCP-22**: [DEN-009] REA MCP Bridge & Disassembler Protocol Client [State: To Do, Priority: Medium]
- [ ] **DEN-101**: Multi-Vendor Agent Architecture Review: Evaluate Anthropic Claude Code, OpenAI Operator, and Cursor Integration [State: To Do, Priority: Medium]
- [ ] **DEN-102**: Autonomous Agent Harness: OpenCode Real Execution Validation & Sandboxing [State: In Progress, Priority: Medium]
- [ ] **DEN-701-COV-1**: Coverage Gap: Reach 100% coverage in pkg/vcs/guard.go [State: In Progress, Priority: Urgent]
- [ ] **DEN-702-COV-1**: Coverage Gap: Reach 100% coverage in pkg/tracker/routing.go [State: In Progress, Priority: Urgent]
- [ ] **DEN-703-COV-1**: Coverage Gap: Reach 100% coverage in pkg/tracker/github/github.go [State: To Verify, Priority: Urgent]
- [ ] **DEN-704-COV-1**: Coverage Gap: Reach 100% coverage in pkg/vcs/orchestrator.go & remote_sync.go [State: In Progress, Priority: Urgent]
- [ ] **DEN-705-COV-1**: Coverage Gap: Reach 100% coverage in pkg/audit/auditor.go [State: To Verify, Priority: Urgent]
- [ ] **DEN-811**: Modular CLI Command Registry & Dispatcher Architecture [State: To Verify, Priority: High]
- [ ] **DEN-997**: docs: Synapse & multi-agent mesh architecture spec [State: To Verify, Priority: Medium]
- [ ] **DEN-998**: feat(projectid): persistent project identity + global project index [State: To Verify, Priority: Medium]
- [ ] **DEN-999**: feat(pool): mint project IDs on pool add and project add [State: To Verify, Priority: Medium]
- [ ] **DEN-1000**: feat(synapse): manifest schema, store and state machine [State: To Verify, Priority: Medium]
- [ ] **DEN-1001**: feat(cli): dendrite synapse create/list/inspect [State: To Verify, Priority: Medium]
- [ ] **DEN-1002**: feat(synapse): locate and heal with health matrix [State: To Verify, Priority: High]
- [ ] **DEN-1003**: feat(synapse): atomic synapse move with rollback [State: To Verify, Priority: Medium]
- [ ] **DEN-1004**: feat(jetbrains): merge-aware scopes and per-synapse MCP entries [State: To Verify, Priority: Medium]
- [ ] **DEN-1005**: feat(mcp): scoped server mode mcp serve --synapse [State: To Verify, Priority: Medium]
- [ ] **DEN-1006**: feat(mcp): synapse_diff_contract [State: Backlog, Priority: Medium]
- [ ] **DEN-1007**: feat(mcp): synapse_query_peer with ast ExportedSymbols [State: Backlog, Priority: Medium]
- [ ] **DEN-1008**: feat(mcp): synapse_run_parity_tests and synapse_propose_sync [State: Backlog, Priority: Medium]
- [ ] **DEN-1009**: feat(mesh): cross-process store, sessions and liveness reaper [State: Backlog, Priority: High]
- [ ] **DEN-1010**: feat(mesh): semantic symbol leases with conflict notice and queue [State: Backlog, Priority: High]
- [ ] **DEN-1011**: feat(mesh): blackboard event log and SSE endpoint [State: Backlog, Priority: High]
- [ ] **DEN-1012**: feat(mesh): dendrite exec shim and mesh MCP tools [State: Backlog, Priority: High]
- [ ] **DEN-1013**: feat(cli): dendrite agent bus listen/sessions/leases/evict [State: Backlog, Priority: Medium]
- [ ] **DEN-1014**: feat(agentjobs): per-session isolated worktrees [State: Backlog, Priority: Medium]
- [ ] **DEN-1015**: feat(mesh): reconcile with symbol-disjoint merge [State: Backlog, Priority: High]
- [ ] **DEN-1016**: feat(jetbrains): surface active leases in the IDE [State: Backlog, Priority: Medium]
- [ ] **DEN-1017**: feat(plane): canonical workflow states and reconcile [State: Backlog, Priority: High]
- [ ] **DEN-1018**: feat(synapse): bind members to Plane projects [State: Backlog, Priority: Medium]
- [ ] **DEN-1019**: refactor(devtool): shared guarded push and repo inspection [State: Backlog, Priority: Medium]
- [ ] **DEN-1020**: feat(sync): dendrite sync fans out across synapse peers [State: Backlog, Priority: High]
- [ ] **DEN-1021**: feat(commit): dendrite commit --synapse commits and checks peers [State: Backlog, Priority: High]
- [ ] **DEN-1022**: feat(sync): continuous synapse sync via --watch [State: Backlog, Priority: Medium]

### General

- [x] **DEN-323**: refactor(agentsearch): rank agents through the dendrite-rag sidecar, no Go BM25/embeddings/RRF [State: Done, Priority: Medium]
- [ ] **DEN-324**: Reproduce and fix Plane create/update failures [State: To Verify, Priority: High]
- [ ] **DEN-325**: MCP tool profiles (core|full) with allowlist flag/env [State: To Verify, Priority: High]
- [ ] **DEN-326**: Read-only MCP probe harness [State: To Verify, Priority: Medium]
- [ ] **DEN-327**: Default-enable only passing MCP tools [State: To Verify, Priority: Medium]
- [ ] **DEN-328**: PreToolUse hook generator (warn-then-allow rg) [State: To Verify, Priority: High]
- [ ] **DEN-329**: AGY rule parity and rg/dendrite telemetry [State: To Verify, Priority: Medium]
- [ ] **DEN-330**: Render Plane tickets to statedir markdown [State: To Verify, Priority: High]
- [ ] **DEN-331**: tickets corpus in dendrite-rag sidecar [State: To Verify, Priority: High]
- [ ] **DEN-332**: dendrite ask --corpus tickets and MCP tool [State: To Verify, Priority: Medium]
- [ ] **DEN-333**: Synapse sync: RAG index step per member [State: To Verify, Priority: High]
- [ ] **DEN-334**: Synapse federated search [State: To Verify, Priority: Medium]
- [ ] **DEN-335**: Ticket lifecycle guard [State: To Verify, Priority: High]
- [ ] **DEN-336**: Delegation playbook for local/AGY models [State: To Verify, Priority: Low]
- [ ] **DEN-337**: Idempotent Plane upsert keyed on external id [State: Backlog, Priority: Medium]
- [ ] **DEN-338**: Plane smart-select and hitl-sync take 28s/50s (N+1 Plane calls) [State: To Verify, Priority: High]
- [ ] **DEN-339**: MCP tool to set a Plane ticket status (dendrite_ticket_update edits DEV_STATUS.md) [State: To Verify, Priority: High]
- [ ] **DEN-340**: Blocked and Cancelled ticket states in the tooling [State: To Verify, Priority: High]
- [ ] **DEN-341**: Lifecycle round-trip scratch (safe to ignore) [State: Cancelled, Priority: Low]
- [ ] **DEN-342**: dendrite-plane-rs: Rust Plane sidecar with local ticket mirror [State: In Progress, Priority: High]
- [ ] **DEN-343**: Plane API token must come from configuration, never a hardcoded default [State: To Verify, Priority: Urgent]
- [ ] **DEN-344**: Plane relations are fetched serially: 368 calls at 35ms (13s) per ticket selection [State: To Verify, Priority: High]
- [ ] **DEN-345**: Sidecar pool: bounded request queue, coalescing, lazy start, restart, idle shutdown [State: To Verify, Priority: High]
- [ ] **DEN-346**: Go adapter for dendrite-plane: Snapshot with Go fallback, relation-cache invalidation, switch next/smart-select [State: To Verify, Priority: High]
- [ ] **DEN-347**: RAG retrieval: use the pooled sidecar and never wait on embeddings in the query path [State: To Verify, Priority: High]
- [ ] **DEN-348**: dendrite-rag: lazy embedder and no-op index shortcut [State: In Progress, Priority: High]
- [ ] **DEN-349**: dendrite ask: faster local generation (num_ctx, keep_alive, max_tokens) and one local-model probe per process [State: To Verify, Priority: Medium]
- [ ] **DEN-350**: soma prepare overwrites a project's hand-written CI workflow [State: Backlog, Priority: Medium]
- [ ] **DEN-351**: Claude hook must fail open: an old or missing dendrite must never block the agent [State: To Verify, Priority: Urgent]
- [x] **DEN-352**: Right-size local models: benchmark candidates on this 24 GB M3, stop defaulting to qwen3.8:27b, stream ask, prune unused models [State: Done, Priority: High]
- [ ] **DEN-353**: Install dendrite-plane through the ecosystem; a missing sidecar is an error with its fix [State: To Verify, Priority: High]
- [ ] **DEN-354**: dendrite-plane-rs: issue get/create/update/comment methods [State: To Verify, Priority: High]
- [ ] **DEN-355**: dendrite-plane-rs: projects, states, modules and pages methods [State: Backlog, Priority: Medium]
- [ ] **DEN-356**: dendrite-plane-rs: relations add/remove and PR link [State: Backlog, Priority: Medium]
- [ ] **DEN-357**: Go: Plane only through planesvc; delete the Go HTTP client [State: Backlog, Priority: High]
- [ ] **DEN-358**: dendrite-rag-rs: ast.fold, ast.last_complete_node, ast.complexity [State: Backlog, Priority: Medium]
- [ ] **DEN-359**: Go: pkg/ast callers use the rag sidecar; delete pkg/ast [State: Backlog, Priority: Medium]
- [ ] **DEN-360**: Guard test: no Go duplicate of a Rust service [State: Backlog, Priority: Low]
- [ ] **DEN-361**: pkg/ast fold test measures a frozen fixture, not a live source file [State: To Verify, Priority: Medium]
- [ ] **DEN-362**: rag-rs: choose which code languages are parsed (rag.toml, cargo features, install flag) [State: Backlog, Priority: Medium]
- [ ] **DEN-363**: Smart local-model gateway: discover installed models, fit them to memory, expose them, role aliases [State: In Progress, Priority: High]
- [ ] **DEN-364**: OxiLLaMa lifecycle: session leases, idle shutdown, best available model [State: To Verify, Priority: High]
- [ ] **DEN-365**: Untrack .dendrite: local tool state, identity via index + root commit, worktrees share main state [State: To Verify, Priority: Medium]
- [ ] **DEN-366**: env secrets ensure: committed default credentials, changeable at install [State: To Verify, Priority: High]
- [ ] **DEN-367**: chronos project refresh: relocate or prune every stale entry of the global project index [State: To Verify, Priority: Medium]
- [ ] **DEN-368**: Decouple pkg/sidecar from pkg/synapse so synapsesync can fold into pkg/synapse [State: Backlog, Priority: Medium]
- [ ] **DEN-369**: synapse add / remove-member / delete: change synapse membership without hand-editing the manifest [State: To Verify, Priority: Medium]
- [ ] **DEN-370**: chronos project remove: archive (default) or hard-delete Plane projects through the sidecar [State: To Verify, Priority: Medium]
- [ ] **DEN-371**: Stack files in ~/.dendrite/stacks: replicate and cleft update work from any directory [State: To Verify, Priority: High]
- [ ] **DEN-372**: Untrack the Dendrite repo's own .dendrite files; coverage baseline moves to a visible path [State: To Verify, Priority: Medium]
- [ ] **DEN-373**: Synapse peers: agents know their peers, and a project can file a request in a peer's Plane project [State: To Verify, Priority: Medium]
- [ ] **DEN-391**: Agent jobs: visible progress and an always-on stall watcher [State: To Verify, Priority: Medium]
- [ ] **DEN-392**: query_context: linked tickets and a RAG map of the touched files [State: Backlog, Priority: Medium]
- [ ] **DEN-409**: directive: duplicated code is mitigated with macros and helper functions [State: To Verify, Priority: High]
- [ ] **DEN-410**: Rust quality pass 2: rstest crate and zero duplication across the Rust repos [State: Backlog, Priority: High]
- [ ] **DEN-411**: Right-size local models, part 2: stream ask and prune unused models [State: In Progress, Priority: Medium]
- [ ] **DEN-412**: Code debt: cmd/dendrite/agent.go over 500 production lines [State: To Verify, Priority: High]
- [ ] **DEN-413**: Code debt: Go packages below 100% coverage [State: Backlog, Priority: High]
- [ ] **DEN-414**: Code debt: remove the repeated code (124 clone groups) [State: In Progress, Priority: High]
- [ ] **DEN-415**: Coverage batch 1: telemetry/exporter synapse sandbox story/fork tracker [State: To Verify, Priority: Medium]
- [ ] **DEN-416**: Coverage batch 2: orchestrator/aws ui/store agent scheduler/governor statedir [State: To Verify, Priority: Medium]
- [ ] **DEN-417**: Coverage batch 3: rea/showcase ast orchestrator/azure synapsesync tracker/dag [State: To Verify, Priority: Medium]
- [ ] **DEN-418**: Coverage batch 4: docgen/reflect sidecar query/swap scheduler/token vcs [State: To Verify, Priority: Medium]
- [ ] **DEN-419**: Clone batch 1: pkg/topology and pkg/tracker/plane [State: To Verify, Priority: Medium]
- [ ] **DEN-420**: Clone batch 2: pkg/inference [State: To Verify, Priority: Medium]
- [ ] **DEN-421**: Clone batch 3: pkg/agent, pkg/agentjobs, pkg/devtool [State: To Verify, Priority: Medium]
- [ ] **DEN-440**: agy zero-trust: opt out of telemetry and find the settings.json key [State: Backlog, Priority: Medium]
- [ ] **DEN-1017**: fix(agentjobs): AGY model catalog, per-family cooldown, load spreading and safe transient retry [State: Backlog, Priority: High]
- [ ] **DEN-1018**: docs(subprojects): complete sub-project doc imports and fix their indexes [State: Backlog, Priority: Medium]
- [ ] **DEN-1033**: fix(github): derive github-sync and hitl-sync from Plane and git, isolate tests, fix DEV_STATUS sync [State: To Verify, Priority: High]
- [ ] **DEN-1034**: feat(plane): every editable repository is a Plane project; related repositories share a synapse [State: To Verify, Priority: High]
- [ ] **DEN-1035**: feat(install): branch-driven install and enforced synapse branches (main stable, develop dev, per-member override) [State: To Verify, Priority: High]
- [ ] **DEN-1036**: fix(vcs): branch guard must keep agents on their own branches and treat develop as the merge target [State: In Progress, Priority: High]
- [ ] **DEN-1037**: fix(perf): data race in TestPreFlightCascadeToFallbackWithZeroPromptLoss breaks go test -race [State: Backlog, Priority: High]
- [ ] **DEN-1038**: test(agentjobs): add a coverage baseline or reach 100% so the CI ratchet passes [State: Backlog, Priority: Medium]
- [ ] **DEN-1039**: docs(workflow): agents merge their own branch into develop when a ticket is finished [State: To Verify, Priority: High]
- [ ] **DEN-1040**: fix(commit): --files commits only the listed files [State: To Verify, Priority: Urgent]
- [ ] **DEN-1041**: fix(chronos): assign ticket numbers in chronos create and refuse ambiguous ticket lookups [State: Backlog, Priority: High]
- [x] **DEN-1042**: docs: sub-project specs live in their repositories; Dendrite docs link to them and sync_subproject_docs.py is retired [State: Done, Priority: High]
- [x] **DEN-1043**: feat(rag): pkg/rag retrieves and indexes through the dendrite-rag sidecar with Go fallback [State: Done, Priority: High]
- [x] **DEN-1044**: refactor(rag): retrieval is served only by dendrite-rag; remove the Go indexer and fallback, Go coordinates and serves [State: Done, Priority: High]
- [x] **DEN-1045**: fix(inference): GPU-capable engines take precedence over OxiLLaMa (CPU-only prefill): engine order, catalog ties, triage, gateway routing and the agent-jobs local tier [State: Done, Priority: High]
- [x] **DEN-1048**: fix(bouton): jobs wait/show --ticket find the job in the ticket's project and fail when none exists [State: Done, Priority: Medium]
- [x] **DEN-1056**: dendrite ask finds the LiteLLM key outside the Dendrite checkout [State: Done, Priority: High]
- [x] **DEN-1057**: API keys and vendor configuration are configured once in $DENDRITE_HOME/.env [State: Done, Priority: Urgent]

### RAG graph + Postgres

- [x] **DEN-1046**: feat(rag): graph MCP tools and HTTP endpoints over the dendrite-rag graph.* methods (FEAT-045) [State: Done, Priority: High]
- [x] **DEN-1047**: feat(stacks): pgvector Postgres and dendrite_rag database for the shared RAG tier; DENDRITE_RAG_PG_URL in prepare [State: Done, Priority: Medium]

### Rust RAG only + quality gate

- [ ] **DEN-1049**: refactor(rag): ask delegates generation to the dendrite-rag ask RPC; Go keeps the client and the escalation prompt [State: Backlog, Priority: High]
- [x] **DEN-1050**: refactor(agentsearch): rank agents through the dendrite-rag rank RPC; remove the Go BM25, embeddings and RRF [State: Done, Priority: High]
- [ ] **DEN-1051**: ci(quality): max 500 lines per code file and jscpd duplicate gate; fix the 19 clones [State: Backlog, Priority: High]
- [x] **DEN-1052**: refactor: split Go files over 500 lines in cmd/dendrite, tracker/plane, mcpserver and server [State: Done, Priority: Medium]
- [x] **DEN-1053**: refactor: split Go files over 500 lines in agentjobs, agent, devtool, quality, vcs and workcopy [State: Done, Priority: Medium]
- [ ] **DEN-1054**: refactor: split Go files over 500 lines in inference, env, gateway, synapse, node and pool [State: In Progress, Priority: Medium]
- [x] **DEN-1055**: refactor: split Go files over 500 lines in ast, docgen, docusaurus, personalcontext and story [State: Done, Priority: Medium]

### Technology Integration Roadmap (2026-11)

- [ ] **DEN-422**: Technology integration roadmap (EGM, Engram, Hermes executor, SkillSpector, dual-protocol): epic [State: Backlog, Priority: High]
- [x] **DEN-423**: Roadmap S0: tickets-RAG health + Plane board reconciliation [State: Done, Priority: Medium]
- [ ] **DEN-424**: Roadmap S1: bootstrap - adopt EGM, speq-skill, superpowers; SkillSpector pre-scan; agy zero-trust [State: To Verify, Priority: Medium]
- [ ] **DEN-425**: Roadmap S2: dendrite immune - SkillSpector gate wrapper (DEN-388/389/390) [State: Backlog, Priority: Medium]
- [ ] **DEN-426**: Roadmap S3: real local inference - kill the spike serve mock; finish capability catalog (DEN-920) [State: Backlog, Priority: Medium]
- [ ] **DEN-427**: Roadmap S4: Engram adoption + frozen session-start snapshot (DEN-941) [State: Backlog, Priority: Medium]
- [ ] **DEN-428**: Roadmap S5: consolidation and curation - Engram-native dream job, HRR gap evaluation, minimal skill ledger [State: Backlog, Priority: Medium]
- [ ] **DEN-429**: Roadmap S6: Harness2-lite - tools.toml 8-component schema (DEN-953) + Level-2 rule extraction; k=2 deferred [State: Backlog, Priority: Medium]
- [ ] **DEN-430**: Roadmap S7: EGM pipeline wiring - 7-lens config, goldfish routing to 3.8-flash, drift-report finish gate [State: Backlog, Priority: Medium]
- [ ] **DEN-431**: Roadmap S8: routing completion - scorer/ladder/router (DEN-921/924/925), tier tags, vesicle attribution [State: Backlog, Priority: Medium]
- [ ] **DEN-432**: Roadmap S9: Hermes Agent as fourth executor - sandboxed install, bouton runner, memory isolation, bench [State: Backlog, Priority: Medium]
- [ ] **DEN-433**: Roadmap S10: scoped MCP over HTTP - adopt official go-sdk transport + .well-known lazy schemas [State: Backlog, Priority: Medium]
- [ ] **DEN-434**: Roadmap S11: ACP blackboard + semantic leases on rag-rs symbol hashes (DEN-1009/1010/1011/1013) [State: Backlog, Priority: Medium]
- [ ] **DEN-435**: Roadmap S12: saga fork --mode parallel + deterministic resolve composer (Go interim) [State: Backlog, Priority: Medium]
- [ ] **DEN-436**: Roadmap S13: closeout - meiosis (DEN-387), cadence scheduler, backlog reconciliation + retro [State: Backlog, Priority: Medium]
- [x] **DEN-437**: CLI validation sweep: verify every documented command works, fix plan for failures, redesign GH issues as human test scripts [State: Done, Priority: High]
- [ ] **DEN-438**: Validation sweep follow-ups: re-scope DEN-024, fold coverage-gap issues into DEN-413, cloud-env tags for #41/#37, park Linear, narrow DEN-917, refresh DEND-402/601 [State: To Verify, Priority: High]
- [ ] **DEN-439**: chronos finish: detect [AC-n] criteria written inline in one paragraph, not only one-per-line [State: To Verify, Priority: High]
- [ ] **DEN-441**: Secrets rotation UX: 'ensure --rotate' alone silently no-ops, and 'motor restart' keeps a container's stale env [State: Backlog, Priority: Medium]

---

## Other Plane Projects

### CARLATEST

*(No active tickets)*

### DCORE

- [ ] **DCORE-1**: dendrite-core-rs: tree-sitter fold / last-node / complexity / blake3 / tokenize sidecar [State: Backlog, Priority: Medium]
- [ ] **DCORE-2**: [DEN-943] BLAKE3 chunk ledger + WAL + resume (was DEN #175) [State: Backlog, Priority: High]
- [x] **DCORE-3**: ci(quality): max 500 lines per code file and jscpd duplicate gate [State: Done, Priority: Low]
- [ ] **DCORE-3**: [DEN-942] Truncation splicer (Go via go/ast; other languages after DEN-972) (was DEN #174) [State: Backlog, Priority: High]
- [ ] **DCORE-4**: [DEN-056] Idiomatic Binary/Repo-to-Rust Transpiler & Differential Test Harness (`pkg/query/transpile`, `tests/differential`) [State: To Verify, Priority: Medium]
- [ ] **DCORE-5**: [DEN-020] Intermediate Semantic Representation ISR Tree-sitter Parser [State: To Do, Priority: Medium]
- [x] **DCORE-6**: [DEN-018] AST Context Compaction & LiteLLM Prefix Cache Harness [State: Done, Priority: Medium]
- [x] **DCORE-7**: [DEN-010] Tree-sitter AST & Complexity Analyzer [State: Done, Priority: Medium]
- [x] **DCORE-8**: docs: own technical specification (ARCHITECTURE.md), drop imported copies [State: Done, Priority: High]

### DGIT

- [ ] **DGIT-1**: dendrite-git-rs: gix hunk graph + semantic clusterer sidecar [State: Backlog, Priority: Medium]
- [ ] **DGIT-2**: [DEN-934] --local-review stacked branches + git notes telemetry + review audit/land (was DEN #171) [State: Backlog, Priority: High]
- [x] **DGIT-3**: ci(quality): max 500 lines per code file and jscpd duplicate gate [State: Done, Priority: Low]
- [ ] **DGIT-3**: [DEN-933] Clusterer + LLM plan + validators (exactly-once, topo order, per-commit build) (was DEN #170) [State: Backlog, Priority: High]
- [ ] **DGIT-4**: [DEN-932] Hunk extractor + symbol-dependency graph (was DEN #169) [State: Backlog, Priority: High]
- [ ] **DGIT-5**: [DEN-098] Autonomous Git Commit Engine with Local Model Message Delegation & Zero-Cost Synthesis [State: To Verify, Priority: High]
- [ ] **DGIT-6**: [DEN-065] Sliced Upstream Commit Drift Analyzer & Bug Fix Porting Generator (`pkg/story/drift`, `pkg/query`) [State: To Do, Priority: Medium]
- [ ] **DGIT-7**: [DEN-064] Speculative Git Worktree Branching & Differential Architecture Benchmark Harness (`pkg/story/fork`, `pkg/perf`) [State: To Verify, Priority: Medium]
- [x] **DGIT-8**: docs: own technical specification (ARCHITECTURE.md), drop imported copies [State: Done, Priority: High]

### DMCP

- [ ] **DMCP-1**: dendrite-mcp-hub: extract pkg/mcpserver into a standalone Go module [State: Backlog, Priority: Medium]
- [ ] **DMCP-2**: [DEN-960] Penpot MCP service + hub registration (was DEN #185) [State: Backlog, Priority: Medium]
- [ ] **DMCP-3**: [DEN-930] Spike: Penpot official MCP server deployment shape (was DEN #183) [State: Backlog, Priority: Medium]
- [ ] **DMCP-4**: [DEN-812] Bidirectional Multi-Vendor Migration & Real-Time Sync Engine (Plane <-> Jira, Penpot <-> Figma) (was DEN #141) [State: Backlog, Priority: High]
- [ ] **DMCP-5**: [DEN-808] Personal Context MCP Connectors: Workspace & Enterprise Tool Adapters (was DEN #136) [State: Backlog, Priority: Medium]
- [ ] **DMCP-6**: [DEND-305] [Plugin - Zed]: Rust WASM extension (dendrite-zed) (was DEN #113) [State: To Do, Priority: Medium]
- [ ] **DMCP-7**: [DEND-304] [Plugin - Neovim]: Headless Lua RPC client (dendrite.nvim) (was DEN #112) [State: To Do, Priority: Medium]
- [ ] **DMCP-8**: [DEND-303] [Plugin - VS Code]: VS Code Extension (dendrite-vscode) built with bun & MCP context (was DEN #111) [State: To Do, Priority: High]
- [ ] **DMCP-9**: [DEND-302] [Plugin - JetBrains]: JetBrains Plugin (dendrite-jetbrains) with local AI chat & Graphify markers (was DEN #110) [State: To Do, Priority: Urgent]
- [ ] **DMCP-10**: [DEND-201] [Code Knowledge MCP]: Local AST query MCP server (was DEN #106) [State: To Do, Priority: High]
- [x] **DMCP-11**: ci(quality): max 500 lines per code file and jscpd duplicate gate; split the 4 long Go files [State: Done, Priority: Medium]
- [ ] **DMCP-11**: [DEN-096] Token-Efficient Structured Log Tailing & MCP Routine Control Tools (`pkg/routines/cicd`, `pkg/mcpserver`, `cmd/dendrite`) [State: To Do, Priority: Medium]
- [ ] **DMCP-12**: [DEN-093] Post-Publish Git Tag Lifecycle & Stdio JSON-RPC MCP Tool Registry (`pkg/mcp/publisher`, `pkg/mcpserver`) [State: To Do, Priority: Medium]
- [ ] **DMCP-13**: [DEN-092] Pre-Flight Validation, Dry-Run SemVer Guard & Secret-Isolated Execution Gate (`pkg/mcp/publisher`, `cmd/dendrite`) [State: To Do, Priority: Medium]
- [ ] **DMCP-14**: [DEN-091] Pluggable Registry Adapter Interface & Cargo Workspace Topological Publisher (`pkg/mcp/publisher`, `internal/mcp/publisher`) [State: To Do, Priority: Medium]
- [ ] **DMCP-15**: [DEN-076] UI File Mutation Watcher & Penpot Documentation Token Diff Synthesizer (`pkg/ui/penpot`, `pkg/mcpserver`) [State: To Verify, Priority: Medium]
- [ ] **DMCP-16**: [DEN-075] Context-Aware `dendrite stories next` Scheduler & Active Story Context Injector (`pkg/stories/next`, `pkg/jetbrains`) [State: In Progress, Priority: High]
- [ ] **DMCP-17**: [DEN-063] Quad-IDE Cross-Language Schema & IPC Contract Drift Monitor (`pkg/polyglot/contract`, `pkg/jetbrains`) [State: To Verify, Priority: Medium]
- [ ] **DMCP-18**: [DEN-050] Local Agent Triage Gateway, Token-Efficient Prompt Router & JetBrains AI Chat Multi-MCP Orchestrator (`pkg/agent/triage`, `pkg/jetbrains`, `pkg/mcpserver`, `pkg/gateway`) [State: In Progress, Priority: Medium]
- [ ] **DMCP-19**: [DEN-046] JetBrains IDE Toolset Integration, 19 MCP Tools & Agentic Developer Workflow Guide (pkg/jetbrains, pkg/mcpserver, cmd/dendrite) [State: To Verify, Priority: Medium]
- [ ] **DMCP-20**: [DEN-040] Dendrite Native MCP Server & Tool Registry Bridge (`pkg/mcpserver`, `cmd/dendrite`) [State: To Verify, Priority: Medium]
- [ ] **DMCP-21**: [DEN-028] JetBrains IDE MCP Integration & Communication Bridge [State: To Verify, Priority: Medium]
- [ ] **DMCP-22**: [DEN-009] REA MCP Bridge & Disassembler Protocol Client [State: To Do, Priority: Medium]
- [x] **DMCP-23**: docs: own technical specification (ARCHITECTURE.md), drop imported copies [State: Done, Priority: High]

### DOCULA

- [x] **DOCULA-1**: [DOC-101] Spatial Geometry & DocumentSource (`docula-core`) [State: Done, Priority: Medium]
- [x] **DOCULA-2**: [DOC-102] DoculaConfig & Layered Loader (`docula-core`) [State: Done, Priority: Medium]
- [x] **DOCULA-3**: [DOC-103] Zero-Storage RAII NonStored<T> (`docula-core`) [State: Done, Priority: Medium]
- [x] **DOCULA-4**: [DOC-201] Tiny-Skia Pure-Rust Rasterizer (`docula-preprocess`) [State: Done, Priority: Medium]
- [x] **DOCULA-5**: [DOC-202] `oximedia` Image Filter Chain: Deskew & Binarize (`docula-preprocess`) [State: Done, Priority: Medium]
- [x] **DOCULA-6**: [DOC-203] PDF Inspector & Routing Heuristics (`docula-ingest`) [State: Done, Priority: Medium]
- [x] **DOCULA-7**: [DOC-301] PP-OCRv6 DBNet & CRNN Multi-Lingual (`docula-ocr`) [State: Done, Priority: Medium]
- [x] **DOCULA-8**: [DOC-302] `scirs2` Spatial Reading-Order Clustering (`docula-ocr`) [State: Done, Priority: Medium]
- [x] **DOCULA-9**: [DOC-303] Compliance Action Tags & Redactor (`docula-compliance`) [State: Done, Priority: Medium]
- [x] **DOCULA-10**: [DOC-304] Multi-Lingual Mmap Asset Manager (`docula-ocr`) [State: Done, Priority: Medium]
- [x] **DOCULA-11**: [DOC-401] JSON Schema to DFA Compiler (`docula-schema`) [State: Done, Priority: Medium]
- [x] **DOCULA-12**: [DOC-402] Autoregressive Logits Masking (`docula-schema`) [State: Done, Priority: Medium]
- [x] **DOCULA-13**: [DOC-403] Sovereign `oxillama` & `oxibonsai` Backends (`docula-llm`) [State: Done, Priority: Medium]
- [x] **DOCULA-14**: [DOC-501] `oxirag` 4-Layer RAG & `oxirs` DocKGs (`docula-rag`) [State: Done, Priority: Medium]
- [x] **DOCULA-15**: [DOC-502] Dual-Stage Exemplar & `diesel-cte-ext` Lineage (`docula-exemplar`) [State: Done, Priority: Medium]
- [x] **DOCULA-16**: [DOC-601] Triage Gates & CorrectionTriads (`docula-hitl`) [State: Done, Priority: Medium]
- [x] **DOCULA-17**: [DOC-602] `diesel-cte-ext` Lineage DAG & Tiered Storage (`docula-hitl`) [State: Done, Priority: Medium]
- [x] **DOCULA-18**: [DOC-701] Pure-Rust Optical Code Mapper (`docula-vision-codes`) [State: Done, Priority: Medium]
- [x] **DOCULA-19**: [DOC-702] Document Screenshots & `oximedia` Region Cropper (`docula-preprocess`) [State: Done, Priority: Medium]
- [x] **DOCULA-20**: [DOC-801] High-Level DoculaPipeline & Self-Repair (`docula`) [State: Done, Priority: Medium]
- [x] **DOCULA-21**: [DOC-802] Terminal CLI Utility (`docula-cli`) [State: Done, Priority: Medium]
- [x] **DOCULA-22**: [DOC-803] cargo-geiger Zero-FFI Safety & Criterion Benchmarks (`docula`) [State: Done, Priority: Medium]
- [x] **DOCULA-23**: [DOC-901] Macro-Driven Invoicing & AP Domain Template (`docula-domains`) [State: Done, Priority: Medium]
- [x] **DOCULA-24**: [DOC-902] Macro-Driven Bank Statement & Reconciliation Template (`docula-domains`) [State: Done, Priority: Medium]
- [x] **DOCULA-25**: [DOC-903] Macro-Driven Receipt & Expense Normalization Template (`docula-domains`) [State: Done, Priority: Medium]
- [x] **DOCULA-26**: [DOC-904] Macro-Driven Logistics & Shipping Manifest Template (`docula-domains`) [State: Done, Priority: Medium]
- [x] **DOCULA-27**: [DOC-1001] Universal Ofimatic Source Primitives & Format Detection (`docula-core`) [State: Done, Priority: Medium]
- [x] **DOCULA-28**: [DOC-1002] Pure-Rust OOXML & Delimited Spreadsheet Ingestion (`docula-ingest`) [State: Done, Priority: Medium]
- [x] **DOCULA-29**: [DOC-1003] Pure-Rust Presentation & Word Document Ingestion (`docula-ingest`) [State: Done, Priority: Medium]
- [x] **DOCULA-30**: [DOC-1004] Ofimatic Visual Rasterization & Pipeline Facade Integration (`docula-preprocess`, `docula`) [State: Done, Priority: Medium]
- [ ] **DOCULA-31**: [DOC-1101] Migration of `PageRasterBuffer` to `oxipix::FrameBuffer` & Zero-Copy Sub-View Cropping (`docula-core`, `docula-preprocess`) [State: To Do, Priority: Medium]
- [ ] **DOCULA-32**: [DOC-1102] Sovereign Patent-Clean JPEG Crop Export via `oxipix` (`docula-preprocess`) [State: To Do, Priority: Medium]
- [ ] **DOCULA-33**: [DOC-1103] Workspace-Wide Drop of `image` Crate Across `docula-ocr`, `docula-compliance`, `docula-vision-codes`, `docula-cli` [State: To Do, Priority: Medium]
- [ ] **DOCULA-34**: [DOC-1104] End-to-End Accelerator Verification & Zero-FFI Geiger Audit (`docula`) [State: To Do, Priority: Medium]
- [ ] **DOCULA-35**: [DOC-1201] Real HTTP vision LlmBackend (Anthropic / OpenAI / Ollama-compatible) (`docula-llm`) [State: Backlog, Priority: Urgent]
- [ ] **DOCULA-36**: [DOC-1202] Wire the specified neural OCR backends (rusto-rs PP-OCRv6 primary, ocrs optional) into docula-ocr (`docula-ocr`) [State: Backlog, Priority: High]
- [ ] **DOCULA-37**: [DOC-1204] Mexican bank statement CSV profiles + OFX/QFX import (`docula-ingest`) [State: Backlog, Priority: High]
- [ ] **DOCULA-38**: [DOC-1205] Integer fixed-point money mode (i64 × 10⁴) instead of f64 in domain types (`docula-domains`) [State: Backlog, Priority: High]
- [ ] **DOCULA-39**: [DOC-1203] CFDI 4.0 XML parser + RFC validation (`docula-domains`) [State: Backlog, Priority: High]
- [ ] **DOCULA-40**: [DOC-1206] Per-field bounding box, page and confidence in ExtractionResult (`docula`) [State: Backlog, Priority: Medium]
- [ ] **DOCULA-41**: [DOC-1207] Deterministic line-item parser for Mexican thermal receipts (`docula-domains`) [State: Backlog, Priority: Medium]
- [ ] **DOCULA-42**: [DOC-1208] Replace relative `../../../embedag` path dependency with optional git/workspace dependency (`docula-rag`) [State: Backlog, Priority: Medium]
- [ ] **DOCULA-43**: [DOC-1209] Mobile/wasm-friendly feature profile (`docula`) [State: Backlog, Priority: Medium]
- [ ] **DOCULA-44**: [DOC-1210] Reconcile DEV_STATUS and specs with implementation (OCR, LLM backends) + deduplicate Plane milestones (`docula`) [State: Backlog, Priority: Medium]

### DPLANE

- [ ] **DPLANE-1**: project.archive and project.delete sidecar methods [State: To Verify, Priority: Medium]
- [ ] **DPLANE-002**: Sidecar: issue assignees, labels, user.me and comment list/edit (protocol 1.3.0) [State: To Verify, Priority: Medium]
- [ ] **DPLANE-3**: user.me: fall back to /api/users/me/ when /api/v1/users/me/ is 404 [State: To Verify, Priority: Medium]
- [ ] **DPLANE-4**: issue.get: fill state_detail from the states list (Plane returns only the state id) [State: To Verify, Priority: Medium]
- [ ] **DPLANE-5**: Rust quality pass 2: rstest crate and zero duplication in dendrite-plane-rs [State: To Verify, Priority: High]

### DRAG

- [ ] **DRAG-1**: dendrite-rag-rs: BM25+vector hybrid retrieval and memory sync engine [State: Backlog, Priority: Medium]
- [ ] **DRAG-2**: [DEN-964] memory modes: local-only / edge (WAL) / push (+compaction) (was DEN #189) [State: Backlog, Priority: Medium]
- [ ] **DRAG-3**: [DEN-803] Structured Topic Memory Parser & Zero-Extrapolation Fidelity Guard (was DEN #131) [State: To Verify, Priority: Medium]
- [ ] **DRAG-4**: [DEND-203] [Local-First RAG & Shared Library Deduplication]: Zero-cost nomic embeddings & CAS library store (was DEN #108) [State: In Progress, Priority: Urgent]
- [ ] **DRAG-5**: [DEN-060] Store Inspection, Chunk Status Telemetry & Explicit User Purge Lifecycler (`pkg/store/status`, `pkg/ui/store`) [State: To Verify, Priority: Medium]
- [ ] **DRAG-6**: [DEN-059] BLAKE3 Content-Addressed Persistent Segmented Store & Chunk Checkpointer (`pkg/store/segmented`, `cmd/dendrite`) [State: To Verify, Priority: Medium]
- [ ] **DRAG-7**: [DEN-037] Hybrid Agent Search, TEI Embeddings & Semantic Routing Engine (`pkg/agentsearch`, `cmd/dendrite`) [State: In Progress, Priority: High]
- [x] **DRAG-8**: docs: own technical specification (ARCHITECTURE.md + specs/), drop imported copies [State: Done, Priority: High]
- [x] **DRAG-9**: feat(crate): dendrite-rag crate and binary, proto/ schemas and JSON-RPC health [State: Done, Priority: High]
- [x] **DRAG-10**: feat(chunk): gitignore-aware walker, cAST tree-sitter chunker and contextual chunk headers [State: Done, Priority: High]
- [x] **DRAG-11**: feat(index): redb ledger, Merkle incremental indexing and code-aware Tantivy BM25 [State: Done, Priority: High]
- [x] **DRAG-12**: feat(embed): Embedder trait (LiteLLM gateway, model2vec), embedding cache, flat/HNSW vector index [State: Done, Priority: High]
- [x] **DRAG-13**: feat(search): weighted RRF hybrid search, symbol-graph expansion and token-budgeted context packing [State: Done, Priority: High]
- [x] **DRAG-14**: feat(rpc): serve index/search/context/status/purge over JSON-RPC and dev CLI [State: Done, Priority: High]
- [x] **DRAG-15**: test(eval): golden query set, recall@10/MRR report and criterion benchmark gate [State: Done, Priority: Medium]
- [x] **DRAG-16**: feat(history): searchable git history (commit messages and per-file diffs) as a retrieval source [State: Done, Priority: Medium]
- [x] **DRAG-17**: feat(index): time budget for the embedding phase so callers never block on a first full embedding [State: Done, Priority: High]
- [x] **DRAG-18**: feat(embed): Ollama and Hugging Face Inference embedding backends; user-selected backend and model from .dendrite/rag.toml or env; embedders listing [State: Done, Priority: High]
- [x] **DRAG-19**: feat(embed): in-process candle embedder (BERT, NomicBERT, JinaBERT) with Metal/accelerate features [State: Done, Priority: High]
- [x] **DRAG-20**: feat(embed): in-process OxiLLaMa GGUF embedder (Qwen3-Embedding and other decoder embedders) [State: Done, Priority: High]
- [x] **DRAG-21**: feat(embed): auto backend prefers GPU: candle (Metal/CUDA), then a Hugging Face TEI endpoint, then Ollama; oxillama documented as CPU-only for embeddings; eval results for every backend [State: Done, Priority: High]
- [x] **DRAG-22**: fix(vector): crash-safe vectors.bin (fsync, u16 lengths, checksum) that self-heals from the embedding cache [State: Done, Priority: High]
- [x] **DRAG-23**: perf(search): LRU cache of query embeddings keyed by model and query [State: Done, Priority: High]
- [x] **DRAG-24**: docs: drop the removed Go fallback and hnsw_rs from ARCHITECTURE and retrieval specs [State: Done, Priority: Medium]
- [x] **DRAG-25**: test(eval): golden sets for embedag and dendrite-rag-rs plus broad questions [State: Done, Priority: High]
- [x] **DRAG-26**: feat(search): per-embedder fusion weights in rag.toml tuned by an eval sweep [State: Done, Priority: High]
- [x] **DRAG-27**: feat(rerank): in-process candle cross-encoder reranker (bge-reranker) selectable in rag.toml [State: Done, Priority: Medium]
- [x] **DRAG-28**: feat(graph): typed call/type/import edges, import resolution per language, ledger schema v2 [State: Done, Priority: High]
- [x] **DRAG-29**: feat(graph): pure-Rust Leiden communities stored per file and chunk in the ledger [State: Done, Priority: High]
- [x] **DRAG-30**: refactor(store): Store traits behind Engine with LocalStore (redb, Tantivy, FlatIndex) [State: Done, Priority: High]
- [x] **DRAG-31**: feat(search): community-aware context expansion and per-community summary chunks [State: Done, Priority: High]
- [x] **DRAG-32**: feat(rpc): graph.communities, graph.god_nodes, graph.path, graph.explain (protocol 1.1.0) and CLI [State: Done, Priority: High]
- [x] **DRAG-33**: feat(store): PgStore remote mode on Postgres + pgvector with SQL BM25 and shared embeddings [State: Done, Priority: High]
- [x] **DRAG-34**: feat(store): edge mode with WAL journal and replay, Merkle-root pull, pull --slice [State: Done, Priority: Medium]
- [x] **DRAG-35**: feat(store): push with Tier 2 compaction and --dry-run report [State: Done, Priority: Medium]
- [x] **DRAG-36**: feat(llm): optional LiteLLM concept extraction, community summaries and contextual headers [State: Done, Priority: Low]
- [x] **DRAG-37**: feat(vector): HNSW behind VectorIndex for large corpora (recall@10 >= 0.95) [State: Done, Priority: Low]
- [x] **DRAG-38**: feat(ask): retrieval-augmented generation in the sidecar (ask RPC: prompts, citations, local via gateway, hybrid and paid tiers) [State: Done, Priority: High]
- [x] **DRAG-39**: feat(rank): rank RPC for hybrid ranking of supplied documents (agent routing) [State: Done, Priority: High]
- [ ] **DRAG-40**: ci(quality): max 500 lines per code file and jscpd duplicate gate; split the 14 long Rust files [State: Backlog, Priority: High]
- [x] **DRAG-41**: perf(vector): benchmark flat vs HNSW from 50k to 400k vectors and set the auto threshold from the crossover [State: Done, Priority: Medium]
- [x] **DRAG-42**: perf(store): measure cold edge indexing at 50k+ chunks and decide on index segment pull [State: Done, Priority: Medium]
- [ ] **DRAG-43**: perf(vector): replace or rewrite HNSW so an approximate index beats the flat scan above 200k vectors [State: Backlog, Priority: Medium]
- [x] **DRAG-44**: perf(graph): code graph build peaks at 463 MB at 49k chunks; stream chunks without their text to stay under the 200 MB budget [State: Done, Priority: High]
- [x] **DRAG-45**: test(eval): bootstrap confidence intervals and paired compare in loom-eval [State: Done, Priority: High]
- [x] **DRAG-46**: perf(bench): criterion benches and dhat peak-heap gate [State: Done, Priority: Medium]
- [x] **DRAG-47**: feat(lexical): tunable BM25 k1/b and q-log IDF in Tantivy, PostgreSQL and rank, with a sweep [State: Done, Priority: High]
- [x] **DRAG-48**: test(eval): go/no-go experiment for LLM contextual chunk prefixes on code [State: Done, Priority: Medium]
- [x] **DRAG-49**: feat(store): SQLite + sqlite-vec local ledger backend (opt-in), Turso rejected [State: Done, Priority: High]
- [ ] **DRAG-50**: feat(enrich): enrichment pipeline - blocked by the DRAG-58 re-run [State: Backlog, Priority: Medium]
- [x] **DRAG-51**: feat(retrieve): reranker-calibrated abstention and outline fallback in context packs [State: Done, Priority: Medium]
- [x] **DRAG-52**: perf(embed): Qwen3 candle embedder/reranker, paid code embedders and external eval subsets [State: Done, Priority: Low]
- [x] **DRAG-53**: docs(eval): DRAG-48 is inconclusive, not no-go; correct the embedder dimension limit [State: Done, Priority: High]
- [x] **DRAG-54**: test(eval): SWE-bench Verified importer and loom-eval swebench runner [State: Done, Priority: High]
- [ ] **DRAG-55**: test(eval): SWE-bench baselines, pooling and query variants (local models) [State: In Progress, Priority: High]
- [x] **DRAG-56**: feat(embed): embedding dimensions (MRL), model@dims cache key, retries and token budget [State: Done, Priority: Medium]
- [ ] **DRAG-57**: test(embed): paid code embedder evaluation (needs keys and consent) [State: To Do, Priority: Medium]
- [ ] **DRAG-58**: test(eval): pre-registered re-run of the LLM contextual prefix experiment [State: Backlog, Priority: Medium]
- [x] **DRAG-59**: docs: crate READMEs and a documentation audit of the whole project [State: Done, Priority: High]
- [ ] **DRAG-60**: feat(dendrite-rag): improvements for the Dendrite integration [State: To Verify, Priority: Medium]
- [x] **DRAG-61**: test(retrieve): measure or revert adaptive fusion weights, corpus auto routing and ALL_CAPS symbols [State: Done, Priority: High]
- [x] **DRAG-62**: fix: review findings in embeddings (dimensions, retries, budget), SWE-bench metrics and the RPC surface [State: Done, Priority: High]
- [x] **DRAG-63**: test: parameterize repetitive unit tests with rstest [State: Done, Priority: Low]
- [x] **DRAG-64**: Governance 2.1: loom-gate LOC counter with test stripping [State: Done, Priority: High]
- [ ] **DRAG-65**: Governance 2.2: structural clone detector and rstest check [State: To Verify, Priority: High]
- [ ] **DRAG-66**: Governance 4.2: graph.symbols and edit.move_symbols [State: Backlog, Priority: Medium]
- [ ] **DRAG-67**: loom-gate rstest: use the rstest crate (0.28.0), not hand-written helpers [State: To Verify, Priority: High]
- [ ] **DRAG-68**: Rust quality pass 2: loom-gate rstest, clones and rstest dependency clean in dendrite-rag-rs [State: To Verify, Priority: High]
- [ ] **DRAG-69**: loom-gate rstest strict: helpers with parameters repeated by tests must be fixtures too [State: To Verify, Priority: Medium]
- [ ] **DRAG-70**: ask streams its answer: ask.chunk notifications (protocol 1.12.0) [State: Backlog, Priority: High]
- [ ] **DRAG-71**: Clone batch R1 (rag-rs): embed and vector [State: To Verify, Priority: High]
- [ ] **DRAG-72**: Clone batch R2 (rag-rs): stores, ledger and engine tests [State: To Verify, Priority: High]
- [ ] **DRAG-73**: rag-rs clone batch R3: symbols, chunk, graph, quality tests, loom-eval baselines [State: To Verify, Priority: Medium]
- [ ] **DRAG-74**: rag-rs clone batch R4: dendrite-rag settings/server and loom-eval runs [State: To Verify, Priority: Medium]

### EMBEDAG

- [x] **EMBEDAG-001**: [1.1.1] Core Traits & Data Representations (`embedag-core`) [State: Done, Priority: Medium]
- [x] **EMBEDAG-002**: [1.1.2] In-Memory DAG Arena (`MemoryDag`) via `petgraph` (`embedag-core`) [State: Done, Priority: Medium]
- [x] **EMBEDAG-003**: [1.1.3] Cycle Detection & Acyclic Guardrails (`embedag-core`) [State: Done, Priority: Medium]
- [x] **EMBEDAG-004**: [1.1.4] Typestate Mutation Plan (`embedag-core`) [State: Done, Priority: Medium]
- [x] **EMBEDAG-005**: [1.2.1] Continuous Scalar Encoders & Log Scaling (`embedag-core`) [State: Done, Priority: Medium]
- [x] **EMBEDAG-006**: [1.2.2] Cyclical Temporal Encoders (`embedag-core`) [State: Done, Priority: Medium]
- [x] **EMBEDAG-007**: [1.2.3] Uber H3 Geospatial Indexing via `h3o` (`embedag-core`) [State: Done, Priority: Medium]
- [x] **EMBEDAG-008**: [1.2.4] Composite Hybrid Distance Metric (`embedag-core`) [State: Done, Priority: Medium]
- [x] **EMBEDAG-009**: [1.3.1] `#[derive(AsGraphContext)]` (`embedag-derive`) [State: Done, Priority: Medium]
- [x] **EMBEDAG-010**: [1.3.2] `#[derive(GraphTarget)]` (`embedag-derive`) [State: Done, Priority: Medium]
- [x] **EMBEDAG-011**: [2.1.1] Universal JSON Schema Generation with `schemars` (`embedag-llm`) [State: Done, Priority: Medium]
- [x] **EMBEDAG-012**: [2.1.2] GBNF Grammar Generation & Pushdown Automata (`embedag-llm`) [State: Done, Priority: Medium]
- [x] **EMBEDAG-013**: [2.1.3] Draft-Then-Decode Envelope & Prompt Formatting with Field Context (`embedag-llm`) [State: Done, Priority: Medium]
- [x] **EMBEDAG-014**: [2.1.4] Evaluation of `oxibonsai` vs sovereign `oxillama` & `candle` (`embedag-llm`) [State: Done, Priority: Medium]
- [x] **EMBEDAG-015**: [2.1.5] Structure-Agnostic Semantic Delta Synthesizer (`ScalarRatioRule`, `DeltaSynthesizerConfig`) (`embedag-llm`) [State: Done, Priority: Medium]
- [x] **EMBEDAG-016**: [2.1.6] Sovereign `oxillama` GGUF & `candle-core` Inference Providers (`embedag-llm`) [State: Done, Priority: Medium]
- [x] **EMBEDAG-017**: [2.1.7] Developer-Defined Entity Properties & Schema Macro (`entity_properties!`, `EntityProperties`) (`embedag-llm`) [State: Done, Priority: Medium]
- [x] **EMBEDAG-018**: [2.2.1] Tier 1 Deterministic Alias Store & Inverted Index (`embedag-llm`) [State: Done, Priority: Medium]
- [x] **EMBEDAG-019**: [2.2.2] Tier 2 Hybrid Similarity Matching & Pure Rust Embeddings (`embedag-llm`) [State: Done, Priority: Medium]
- [x] **EMBEDAG-020**: [2.2.3] In-Memory Changeset Diffing Engine (`embedag-llm`) [State: Done, Priority: Medium]
- [x] **EMBEDAG-021**: [3.1.1] Relational Schema Migrations (PostgreSQL & SQLite) (`embedag-diesel`) [State: Done, Priority: Medium]
- [x] **EMBEDAG-022**: [3.1.2] Asynchronous Transactions & Savepoints with `diesel-async` (`embedag-diesel`) [State: Done, Priority: Medium]
- [x] **EMBEDAG-023**: [3.1.3] High-Throughput Batch Upserts (`embedag-diesel`) [State: Done, Priority: Medium]
- [x] **EMBEDAG-024**: [3.1.4] Dynamic Graph Mutation Lifecycle Execution (`embedag-diesel`) [State: Done, Priority: Medium]
- [x] **EMBEDAG-025**: [3.1.5] Mandatory Testcontainers Harness for Persistence Layer (`embedag-diesel`) [State: Done, Priority: Medium]
- [x] **EMBEDAG-026**: [3.2.1] Typed `WITH RECURSIVE` Traversal Builder via `diesel-cte-ext` (`embedag-diesel`) [State: Done, Priority: Medium]
- [x] **EMBEDAG-027**: [3.2.2] SQL Cycle Guardrails & Deduplication via `with_recursive_not_all` (`embedag-diesel`) [State: Done, Priority: Medium]
- [x] **EMBEDAG-028**: [3.3.1] Capability Sub-Traits Refactoring (`embedag-core` / `embedag-diesel`) [State: Done, Priority: Medium]
- [x] **EMBEDAG-029**: [3.4.1] Structure-Agnostic Upsert Processor & Custom Conflict Strategies (`embedag-diesel`) [State: Done, Priority: Medium]
- [x] **EMBEDAG-030**: [3.4.2] Generic Hierarchical CTE Traversal via `diesel-cte-ext` for SQLite & PostgreSQL (`embedag-diesel`) [State: Done, Priority: Medium]
- [x] **EMBEDAG-031**: [3.4.3] External Domain Ingestion & Diamond Deduplication Demo (`embedag-example-receipts`) [State: Done, Priority: Medium]
- [x] **EMBEDAG-032**: [3.4.4] Dynamic On-the-Fly LLM Taxonomy Pipeline & Deduplication (`embedag-example-receipts`) [State: Done, Priority: Medium]
- [x] **EMBEDAG-033**: [3.4.5] Finance Invoice Graph Pipeline with Dual-Mode Density Categorization, Time-Dependency, and Dynamic User Reclassification (Candle & OxiLLaMa) (`embedag-example-receipts`) [State: Done, Priority: Medium]
- [x] **EMBEDAG-034**: [3.5.1] Turso / libSQL Distributed Persistence via Diesel & Diesel-Async (`embedag-diesel`) [State: Done, Priority: Medium]
- [x] **EMBEDAG-035**: [4.1.1] Dense Vector & Sparse FTS Indexing with `oxirag` Echo Store (`embedag-rag`) [State: Done, Priority: Medium]
- [x] **EMBEDAG-036**: [4.1.2] Reciprocal Rank Fusion (RRF) Implementation (`embedag-rag`) [State: Done, Priority: Medium]
- [x] **EMBEDAG-037**: [4.2.1] Bounded Subgraph Expansion & Distance Decay (`oxirag` Layer 4: Graph) (`embedag-rag`) [State: Done, Priority: Medium]
- [x] **EMBEDAG-038**: [4.2.2] Grounded Prompt Evidence Packager with Citations (`embedag-rag`) [State: Done, Priority: Medium]
- [x] **EMBEDAG-039**: [4.2.3] Formal SMT-Based Logic Verification via `oxirag` OxiZ Solver (`embedag-rag`) [State: Done, Priority: Medium]
- [x] **EMBEDAG-040**: [5.1.1] Root Facade Crate API Re-exports (`embedag`) [State: Done, Priority: Medium]
- [x] **EMBEDAG-041**: [5.1.2] End-to-End Pipeline Integration Test (`embedag`) [State: Done, Priority: Medium]
- [x] **EMBEDAG-042**: [6.1.1] Mermaid & Graphviz DOT Exporters (`embedag-core`) [State: Done, Priority: Medium]
- [x] **EMBEDAG-043**: [6.1.2] GraphML & RDF-Star / JSON-LD Serialization via `oxirs` (`embedag-core`) [State: Done, Priority: Medium]
- [x] **EMBEDAG-044**: [6.2.1] Graph Migration Engine (`GraphMigration`) (`embedag-core`) [State: Done, Priority: Medium]
- [x] **EMBEDAG-045**: [6.3.1] In-Memory Community Detection (`embedag-rag`) [State: Done, Priority: Medium]
- [x] **EMBEDAG-046**: [6.4.1] Temporal Edge Versioning (`valid_from`, `valid_to`) (`embedag-diesel`) [State: Done, Priority: Medium]
- [x] **EMBEDAG-047**: [6.5.1] Pure-Rust Multimedia Ingestion & Forensic Vision via `oximedia` (`embedag-core`) [State: Done, Priority: Medium]
- [x] **EMBEDAG-048**: [6.5.2] SPARQL 1.2 Semantic Query & Reasoning Interface via `oxirs` (`embedag-rag`) [State: Done, Priority: Medium]
- [x] **EMBEDAG-049**: [7.1.1] SurrealDB Multi-Model Storage Implementation (`embedag-surrealdb`) [State: Done, Priority: Medium]
- [x] **EMBEDAG-050**: [7.2.1] OpenCypher Bolt Protocol Adapter (`embedag-neo4j`) [State: Done, Priority: Medium]
- [x] **EMBEDAG-051**: [7.3.1] Native OxiRS TDB2 Persistent Storage Engine (`embedag-oxirs`) [State: Done, Priority: Medium]
- [x] **EMBEDAG-052**: [8.1.1] Streaming Traversal Iterators & Early Termination (`embedag-core`) [State: Done, Priority: Medium]
- [x] **EMBEDAG-053**: [8.1.2] Degree Pagination & Hub Neighbor Slicing (`embedag-core`) [State: Done, Priority: Medium]
- [x] **EMBEDAG-054**: [8.1.3] Localized Differential Cycle Validation (`embedag-core`) [State: Done, Priority: Medium]
- [x] **EMBEDAG-055**: [8.2.1] Hub-Bounded GraphRAG Subgraph Expansion (`embedag-rag`) [State: Done, Priority: Medium]
- [x] **EMBEDAG-056**: [8.3.1] Windowed & Degree-Bounded Recursive CTE Traversals (`embedag-diesel`) [State: Done, Priority: Medium]
- [x] **EMBEDAG-057**: [9.1.1] Slim facade: embedag::dag resolution, Merkle delta trees and embedag-matcher behind core,matcher features [State: Done, Priority: Medium]
- [ ] **EMBEDAG-058**: [9.2.1] Multi-root dependency manifests: MemoryDag from per-directory manifests, cycle hazards and deterministic build order (`embedag-core`) [State: To Do, Priority: Medium]
- [ ] **EMBEDAG-059**: [9.2.2] Work-item prioritisation: unblock weights with distance decay and critical-path scheduling over item/milestone DAGs (`embedag-core`) [State: To Do, Priority: Medium]
- [ ] **EMBEDAG-060**: [9.3.1] Merkle journal verification: root recomputation for append-only state journals and canonical tree wire format (`embedag-core`) [State: To Do, Priority: Medium]
- [ ] **EMBEDAG-061**: [9.4.1] Leiden community clustering & god-node index for code-knowledge graphs (`embedag-rag`) [State: To Do, Priority: Medium]
- [ ] **EMBEDAG-062**: [9.5.1] Public SCC detection & condensation API for possibly-cyclic graphs (cyclic input → condensed DAG + component membership) (`embedag-core`) [State: To Do, Priority: Medium]
- [x] **EMBEDAG-063**: [9.0.1] docs: describe embedag in its own domain (dependency resolution & Merkle delta API spec), drop imported copies [State: Done, Priority: High]
- [ ] **EMBEDAG-064**: [10.1.1] Embedder trait + real text embeddings per spec 08 (trustformers/oxirag pure-Rust; fastembed/ONNX optional) [State: Backlog, Priority: High]
- [ ] **EMBEDAG-065**: [10.1.2] Diesel VectorSearchRepository for pgvector and SQLite (BLOB brute-force / sqlite-vec) [State: Backlog, Priority: High]
- [ ] **EMBEDAG-066**: [10.2.1] Fuzzy merchant-descriptor normalization (bank descriptor cleaning + trigram/Jaro-Winkler) [State: Backlog, Priority: High]
- [ ] **EMBEDAG-67**: [10.3.1] Real HTTP LlmProvider (Anthropic / OpenAI / Ollama-compatible) for Tier-3 [State: Backlog, Priority: High]
- [ ] **EMBEDAG-068**: [10.4.1] HierarchicalModel: discriminator-scoped hierarchies + configurable ON DELETE [State: Backlog, Priority: High]
- [ ] **EMBEDAG-069**: [10.2.2] kNN categorizer over labelled history producing CandidateMatch [State: Backlog, Priority: Medium]
- [ ] **EMBEDAG-070**: [10.2.3] Recurring/subscription series detection + amount-drift alerts [State: Backlog, Priority: Medium]
- [ ] **EMBEDAG-071**: [10.3.2] Lean default features (test-harness and full off by default) [State: Backlog, Priority: Medium]
- [ ] **EMBEDAG-072**: [10.4.2] Brand hierarchy + trait facet with inheritance in the finance categorization engine [State: Backlog, Priority: Medium]
- [ ] **EMBEDAG-073**: [10.5.1] Incremental Merkle trees with prefix-subtree digests for replica anti-entropy (builds on 9.3.1 wire format) [State: Backlog, Priority: Medium]
- [x] **EMBEDAG-074**: [10.0.1] Local inference priority: GPU-capable candle first, Ollama-compatible HTTP next, oxillama last (CPU-only prefill) [State: Done, Priority: High]
- [ ] **EMBEDAG-75**: Rust quality pass 2: rstest crate, 500-line limit and zero duplication in embedag [State: To Verify, Priority: High]
- [ ] **EMBEDAG-76**: embedag clone batch E1 core, derive, matcher [State: To Verify, Priority: Medium]
- [ ] **EMBEDAG-77**: embedag clone batch E2 neo4j, surrealdb, oxirs, rag backends [State: To Verify, Priority: Medium]
- [ ] **EMBEDAG-78**: embedag clone batch E3 diesel, llm, root crate [State: To Verify, Priority: Medium]
- [ ] **EMBEDAG-79**: embedag clone batch E4 example-receipts and tests [State: To Verify, Priority: Medium]
- [ ] **EMBEDAG-80**: embedag CI green on develop: audit advisories, complexity step, macOS and Windows libpq [State: To Verify, Priority: Medium]
- [ ] **EMBEDAG-81**: Decide how to treat the RUSTSEC advisories pinned by third-party crates [State: To Verify, Priority: Medium]

### KAKEBO

*(No active tickets)*

### KAKEBODM

- [x] **KAKEBODM-1**: [KAK-001] Publish architecture review + KAK-ADR-001…007 [State: Done, Priority: Medium]
- [x] **KAKEBODM-2**: [KAK-002] Publish second assessment (tagging, sharing, sync) + KAK-ADR-008/009 + superseded banners [State: Done, Priority: Medium]
- [ ] **KAKEBODM-3**: [KAK-003] Unify money scale to 10⁴ in features.md, database_types_reference.md, input_validations.md, purchase_registry_integration.md [State: To Do, Priority: Medium]
- [ ] **KAKEBODM-4**: [KAK-004] Settle Argon2id password hashing across access_control_and_auth.md, workspace_architecture.md, development_plan.md [State: To Do, Priority: Medium]
- [ ] **KAKEBODM-5**: [KAK-005] New spec — Sync wire protocol per KAK-ADR-009 (op API, cursors, rebase sections, op classes, Merkle exchange, bootstrap) [State: To Do, Priority: Medium]
- [ ] **KAKEBODM-6**: [KAK-006] New spec — Shared expenses lifecycle & UX (participants, claim invites, consent states, disputes as revisions, settlements, payer privacy) [State: To Do, Priority: Medium]
- [ ] **KAKEBODM-7**: [KAK-007] Correct inflated statuses (KAK-M0-T04/T07/T20) and stale Scheduled DDL rows in development_plan.md; map old KAK-Mx-Tyy IDs to KAK-NNN [State: To Do, Priority: Medium]
- [ ] **KAKEBODM-8**: [KAK-008] Fix stale tooling docs (diesel.toml, item_schema.rs), README table list, sharing_instructions placeholders, ../../src line-anchor links, AGENTS.md re-export claim [State: To Do, Priority: Medium]
- [ ] **KAKEBODM-9**: [KAK-009] New spec — Documents, OCR & extraction pipeline [State: To Do, Priority: Medium]
- [ ] **KAKEBODM-10**: [KAK-010] New spec — Bank/CFDI import & reconciliation [State: To Do, Priority: Medium]
- [ ] **KAKEBODM-11**: [KAK-011] New spec — Audit log, key management/rotation, export & backup [State: To Do, Priority: Medium]
- [x] **KAKEBODM-12**: [KAK-101] Re-baseline migrations 001–005 into per-context 01…08; fix D1/D3/D4/D14; exact down.sql; engine-identical CHECKs; FK indexes [State: Done, Priority: Medium]
- [x] **KAKEBODM-13**: [KAK-102] Deterministic UUIDv5 seed IDs shared by SQLite & Postgres; create join tables before seeds [State: Done, Priority: Medium]
- [x] **KAKEBODM-14**: [KAK-103] Standard sync columns (created_at, updated_at, deleted_at, version, hlc, origin_device_id) on all syncable tables [State: Done, Priority: Medium]
- [x] **KAKEBODM-15**: [KAK-104] Index every foreign key + hot paths (journal_entries.account_id, transactions(workspace_id,date)) [State: Done, Priority: Medium]
- [x] **KAKEBODM-16**: [KAK-105] Workspaces (personal|group) + participants replacing groups, group_members, shadow users and user_aliases [State: Done, Priority: Medium]
- [x] **KAKEBODM-17**: [KAK-106] ActorContext resolving participant role per workspace for every repository call [State: Done, Priority: Medium]
- [x] **KAKEBODM-18**: [KAK-107] Immutable ledger — transaction status + revisions (open month) + RESTRICT on journal_entries [State: Done, Priority: Medium]
- [x] **KAKEBODM-19**: [KAK-108] base_amount + Σ base_amount = 0 guard with i128 + banker's residue; Postgres deferred constraint trigger [State: Done, Priority: Medium]
- [x] **KAKEBODM-20**: [KAK-109] Multi-payer payer_shares, item_splits, and settlements debt simplification [State: Done, Priority: Medium]
- [x] **KAKEBODM-21**: [KAK-110] share_lenses per-participant item categorization lenses [State: Done, Priority: Medium]
- [x] **KAKEBODM-22**: [KAK-111] Faceted tag_nodes (natural, trait, purchase, invoice, merchant) with same-facet composite parent FK + tag_assignments with facet→target CHECKs [State: Done, Priority: Medium]
- [x] **KAKEBODM-23**: [KAK-112] Conjunctive tag_rules (item pattern, descriptor, brand, merchant, location, participant, amount range) with specificity + rule_id explanation [State: Done, Priority: Medium]
- [x] **KAKEBODM-24**: [KAK-113] Workspace-scoped merchants/locations/items (NULL = global); lat/lng E6; merged_into_id merge redirects + catalog_merges [State: Done, Priority: Medium]
- [x] **KAKEBODM-25**: [KAK-114] Currency column on every amount; rename amount_cents; remove SQLite Decimal = f32 [State: Done, Priority: Medium]
- [x] **KAKEBODM-26**: [KAK-115] BPS newtype + banker's split remainder allocator in kakebo-core [State: Done, Priority: Medium]
- [x] **KAKEBODM-27**: [KAK-116] Regenerate src/schemas, update models/repos/DTOs; wire kakebo-core & arbor into src; dedupe LedgerError/CoreError and migration runner [State: Done, Priority: Medium]
- [x] **KAKEBODM-28**: [KAK-117] Regression tests D1–D14 on both engines [State: Done, Priority: Medium]
- [x] **KAKEBODM-29**: [KAK-118] Brands tree (parent_brand_id) + items.brand_id; brand/merchant trait inheritance via arbor::inherit_by [State: Done, Priority: Medium]
- [x] **KAKEBODM-30**: [KAK-119] effective_tags CTE (natural ancestors ∪ item/brand/merchant traits ∪ purchase ∪ invoice) + pillar resolution order + derived fixed/variable [State: Done, Priority: Medium]
- [ ] **KAKEBODM-31**: [KAK-601] shared_expenses + revisions + paid/consumed shares with Σ paid = Σ consumed = total (KAK-ADR-008) [State: To Do, Priority: Medium]
- [ ] **KAKEBODM-32**: [KAK-602] Private funding_links + deterministic personal projection function (UUIDv5 rows) with property tests [State: To Do, Priority: Medium]
- [ ] **KAKEBODM-33**: [KAK-603] Participation state machine (pending/accepted/disputed/auto_accepted from delivered_at) + dispute reallocation as revisions (single & multi-payer) [State: To Do, Priority: Medium]
- [ ] **KAKEBODM-34**: [KAK-604] Settlements with payee confirmation + per-currency balances + min-cash-flow proposed settlements [State: To Do, Priority: Medium]
- [ ] **KAKEBODM-35**: [KAK-605] Pair workspaces from accepted friendships + participant eligibility validated at op apply [State: To Do, Priority: Medium]
- [ ] **KAKEBODM-36**: [KAK-606] Versioned split agreements (effective_from) with participant acceptance [State: To Do, Priority: Medium]
- [ ] **KAKEBODM-37**: [KAK-607] share_lenses — per-participant pillar, purchase tags and private notes (personal lens) [State: To Do, Priority: Medium]
- [ ] **KAKEBODM-38**: [KAK-608] Participant claim by invite (shadow upgrade) + leave/archive with zero-balance guard [State: To Do, Priority: Medium]
- [ ] **KAKEBODM-39**: [KAK-609] Multi-currency groups — base_currency, rate snapshot per revision, report conversion [State: To Do, Priority: Medium]
- [ ] **KAKEBODM-40**: [KAK-610] Snap & split — assign OCR receipt lines (stable line_id) to participants' consumed shares [State: To Do, Priority: Medium]
- [ ] **KAKEBODM-41**: [KAK-701] devices + ops intent log + change_log per workspace with server_seq cursors [State: To Do, Priority: Medium]
- [ ] **KAKEBODM-42**: [KAK-702] HLC + field-level LWW patches for metadata rows [State: To Do, Priority: Medium]
- [ ] **KAKEBODM-43**: [KAK-703] Op apply engine — validation, participant-role authz, op classes ordering, idempotency, compensations [State: To Do, Priority: Medium]
- [ ] **KAKEBODM-44**: [KAK-704] Revision rebase for disjoint sections + conflict revisions & review [State: To Do, Priority: Medium]
- [ ] **KAKEBODM-45**: [KAK-705] Deterministic IDs for derived records (recurring drafts, projections, seeds) [State: To Do, Priority: Medium]
- [ ] **KAKEBODM-46**: [KAK-706] Merkle anti-entropy per workspace (embedag merkle) [State: To Do, Priority: Medium]
- [ ] **KAKEBODM-47**: [KAK-707] Bootstrap snapshot + change_log compaction [State: To Do, Priority: Medium]
- [ ] **KAKEBODM-48**: [KAK-708] Offline→cloud claim migration (no ID rewrite) + quarantine of invalid aggregates [State: To Do, Priority: Medium]
- [ ] **KAKEBODM-49**: [KAK-709] Web online-first op API surface + ts-rs bindings [State: To Do, Priority: Medium]
- [ ] **KAKEBODM-50**: [KAK-710] Month locks freeze revisions; post-lock corrections as adjustments in the open month [State: To Do, Priority: Medium]
- [ ] **KAKEBODM-51**: [KAK-501] audit_events append-only log [State: To Do, Priority: Medium]
- [x] **KAKEBODM-52**: [KAK-502] Encrypted TOTP secret (totp_secret_enc, key_id) + hashed single-use mfa_backup_codes [State: Done, Priority: Medium]
- [x] **KAKEBODM-53**: [KAK-503] Session token_hash + revoked_at soft revoke [State: Done, Priority: Medium]
- [ ] **KAKEBODM-54**: [KAK-504] Device encryption at rest: evaluate libSQL AES-256 encryption (KAK-ADR-010) vs SQLCipher; key derivation without string-formatted PRAGMA [State: To Do, Priority: Medium]
- [ ] **KAKEBODM-55**: [KAK-201] documents, document_extractions, extraction_fields, document_links migrations + repos (KAK-ADR-005) (`kakebo-documents`) [State: In Progress, Priority: Medium]
- [x] **KAKEBODM-56**: [KAK-202] Receipts upgrade — merchant, nullable location, totals, tip, discount, folio, document link; receipt_items line_total/raw_text [State: Done, Priority: Medium]
- [ ] **KAKEBODM-57**: [KAK-203] docula ReceiptData/InvoiceData → receipts mapper with f64 → i64×10⁴ banker's conversion [State: To Do, Priority: Medium]
- [ ] **KAKEBODM-58**: [KAK-204] cfdi_invoices, cfdi_concepts, cfdi_taxes + deducciones personales query (`kakebo-documents`) [State: In Progress, Priority: Medium]
- [ ] **KAKEBODM-59**: [KAK-205] Document review queue (needs_review) and correction write-back to extraction_fields [State: To Do, Priority: Medium]
- [ ] **KAKEBODM-60**: [KAK-301] import_batches + statement_lines staging (`kakebo-ledger`) [State: In Progress, Priority: Medium]
- [ ] **KAKEBODM-61**: [KAK-302] transaction_fingerprints dedup (account, date, amount, descriptor hash) (`kakebo-ledger`) [State: In Progress, Priority: Medium]
- [ ] **KAKEBODM-62**: [KAK-303] reconciliation_matches and statement → draft transaction promotion (`kakebo-ledger`) [State: In Progress, Priority: Medium]
- [ ] **KAKEBODM-63**: [KAK-304] merchant_aliases table + descriptor normalization hook (`kakebo-metadata`) [State: In Progress, Priority: Medium]
- [ ] **KAKEBODM-64**: [KAK-401] kakebo-intelligence crate with features parse-cfdi, import-bank, ocr-local, llm-cloud, llm-local, embed-local, vectors-*, categorize + ai-local/ai-cloud/ai-hybrid bundles [State: To Do, Priority: Medium]
- [ ] **KAKEBODM-65**: [KAK-402] InferencePolicy runtime routing + locality/cost recording [State: To Do, Priority: Medium]
- [ ] **KAKEBODM-66**: [KAK-403] CI feature matrix (none, ai-local, ai-cloud, ai-hybrid; mobile profile without tokio/full, wgpu) [State: To Do, Priority: Medium]
- [ ] **KAKEBODM-67**: [KAK-404] CFDI XML/QR ingestion via docula (parse-cfdi) [State: To Do, Priority: Medium]
- [ ] **KAKEBODM-68**: [KAK-405] Bank CSV/OFX ingestion via docula-ingest (import-bank) [State: To Do, Priority: Medium]
- [ ] **KAKEBODM-69**: [KAK-406] Receipt photo pipeline — oxipix preprocess → OCR/vision LLM → extraction_fields [State: To Do, Priority: Medium]
- [ ] **KAKEBODM-70**: [KAK-407] categorization_suggestions + rules → aliases → kNN (per-participant lens) → LLM waterfall (embedag) [State: To Do, Priority: Medium]
- [ ] **KAKEBODM-71**: [KAK-408] embeddings table (SQLite BLOB / pgvector) behind vectors-* features [State: To Do, Priority: Medium]
- [ ] **KAKEBODM-72**: [KAK-409] Recurring/subscription detection & price-increase alerts (feeds derived fixed/variable) [State: To Do, Priority: Medium]
- [ ] **KAKEBODM-73**: [KAK-410] Duplicate-charge detection [State: To Do, Priority: Medium]
- [ ] **KAKEBODM-74**: [KAK-411] Item price history across merchants + warranty vault queries [State: To Do, Priority: Medium]
- [ ] **KAKEBODM-75**: [KAK-412] Card optimizer (MSI + rewards + fees) recommendation [State: To Do, Priority: Medium]
- [ ] **KAKEBODM-76**: [KAK-413] Natural-language ledger Q&A + monthly Kakebo reflection summary [State: To Do, Priority: Medium]
- [ ] **KAKEBODM-77**: [KAK-414] Multi-facet item classification via embedag — OCR line → item/brand alias, natural waterfall, brand traits, purchase context (gift), invoice class [State: To Do, Priority: Medium]
- [ ] **KAKEBODM-78**: [KAK-415] Categorization review inbox ranked by (1 − confidence) × amount + bulk accept + 'Why this tag?' explanation [State: To Do, Priority: Medium]
- [ ] **KAKEBODM-79**: [KAK-416] Correction learning — feed AliasStore/kNN and suggest conjunctive rules after N consistent corrections [State: To Do, Priority: Medium]
- [x] **KAKEBODM-80**: [KAK-120] Turso / libSQL backend: `turso` feature on the SQLite dialect, TursoConnectionManager (local, remote, embedded replica), LibSql enum impls, kakebo-migrations runner [State: Done, Priority: Medium]
- [x] **KAKEBODM-81**: [KAK-121] diesel-libsql fork: SQLite insert semantics (no DEFAULT keyword, multi-row/row-by-row batches), chrono feature, cte-ext feature; upstream PRs yackey-labs/diesel-libsql#5 and #6 (`diesel-libsql (fork)`) [State: Done, Priority: Medium]
- [ ] **KAKEBODM-82**: [KAK-122] Switch kakebo, kakebo-migrations and arbor back to crates.io diesel-libsql once upstream PRs #5/#6 are released [State: To Do, Priority: Medium]
- [ ] **KAKEBODM-83**: [KAK-123] Live Turso CI: run the ignored remote + embedded-replica test against a disposable Turso database [State: To Do, Priority: Medium]
- [x] **KAKEBODM-84**: [KAK-124] arbor: general-purpose tree crate replacing kakebo-tree — forests, cycle guard, checked rollups, inherit_by, typed recursive CTEs on diesel-cte-ext for SQLite/Postgres/libSQL [State: Done, Priority: Medium]
- [x] **KAKEBODM-85**: [KAK-125] Account subtree balance query in kakebo-ledger composed from arbor::sql [State: Done, Priority: Medium]
- [x] **KAKEBODM-86**: [KAK-126] diesel-cte-ext: WithCte static QueryId reused the wrong prepared statement — confirmed on SQLite and PostgreSQL, reported leynos/diesel-cte-ext#195, fixed in #196 (`diesel-cte-ext (fork)`) [State: Done, Priority: Medium]
- [x] **KAKEBODM-87**: [KAK-127] Split monolithic root src/ modules into domain sub-crates under crates/ and convert root src/lib.rs into a unified facade [State: Done, Priority: Medium]
- [x] **KAKEBODM-88**: [KAK-130] Taxes Pillar, Faceted Tags Multi-Context & Conflict Resolution, Insurances, Savings Goals & Debts [State: Done, Priority: High]
- [x] **KAKEBODM-89**: [KAK-128] Retire legacy ownership tables (groups, group_members, user_aliases + unused tag link tables); move ReBAC queries to workspace_members [State: Done, Priority: Medium]
- [x] **KAKEBODM-90**: [KAK-129] payment_instruments replaces debit_cards (full D1 fix) [State: Done, Priority: Medium]
- [x] **KAKEBODM-91**: [KAK-131] Retire legacy tags/user_tags registry; point items.tag_id, split_agreements.tag_id and user_tags at tag_nodes [State: Done, Priority: Medium]
- [x] **KAKEBODM-92**: [KAK-132] Engine-neutral schema manifest golden test (SQLite/libSQL/Postgres parity) [State: Done, Priority: Medium]
- [x] **KAKEBODM-93**: [KAK-133] RecordIdExt/TimestampExt helpers replace cfg(postgres)/parse_str duplication [State: Done, Priority: Medium]
- [x] **KAKEBODM-94**: [KAK-134] Normalize schema: countries, insurance_providers, loyalty_programs, msi_allowed_terms, postal_areas, payment_instruments, budget FK [State: Done, Priority: Medium]
- [x] **KAKEBODM-95**: [KAK-135] Close D6/D8/D9: workspace-scoped names, currency on every amount, standard sync columns [State: Done, Priority: Medium]
- [x] **KAKEBODM-96**: [KAK-136] Tagging contract test + stale tagging doc refresh [State: Done, Priority: Medium]

### KAKEBOSPEC

*(No active tickets)*

### LEARNWGPU

*(No active tickets)*

### MAPIFE

*(No active tickets)*

### OXI

- [x] **OXI-101**: 64-Byte Aligned Storage & `AlignedBuffer` [State: Done, Priority: Medium]
- [x] **OXI-102**: Typestate `FrameBuffer` Primitives [State: Done, Priority: Medium]
- [x] **OXI-103**: `PixelFormat` Trait & Color Domain Types [State: Done, Priority: Medium]
- [x] **OXI-104**: `ZeroizeOnDrop` RAII Privacy Wrapper [State: Done, Priority: Medium]
- [x] **OXI-201**: Clean-Room ISO/IEC 10918-1 JFIF Parser & Writer [State: Done, Priority: Medium]
- [x] **OXI-202**: Fast Integer AAN Forward DCT & Quantizer [State: Done, Priority: Medium]
- [x] **OXI-203**: Huffman Bitstream Packer & JpegEncoder [State: Done, Priority: Medium]
- [x] **OXI-204**: Baseline JPEG Decoder & Marker Parser [State: Done, Priority: Medium]
- [x] **OXI-301**: Accelerated PNG & APNG Engine [State: Done, Priority: Medium]
- [x] **OXI-302**: WebP Lossy & Lossless Decoder/Encoder Bridge [State: Done, Priority: Medium]
- [x] **OXI-303**: GIF Multi-Frame & LZW Animation Engine [State: Done, Priority: Medium]
- [x] **OXI-304**: TIFF, BMP, QOI, AVIF & OpenEXR Federation [State: Done, Priority: Medium]
- [x] **OXI-305**: PNM, Farbfeld, HDR, DDS & ICO Clean-Room Engines [State: Done, Priority: Medium]
- [x] **OXI-401**: Dynamic Runtime CPU Feature Detector [State: Done, Priority: Medium]
- [x] **OXI-402**: ARM NEON 128-bit Vectorized Kernels [State: Done, Priority: Medium]
- [x] **OXI-403**: x86_64 AVX2 & AVX-512 Vectorized Kernels [State: Done, Priority: Medium]
- [x] **OXI-404**: SIMD Kernel Bridge & Vector Math [State: Done, Priority: Medium]
- [x] **OXI-501**: `SubPixView` Strided Zero-Allocation Slicer [State: Done, Priority: Medium]
- [x] **OXI-502**: Multi-Kernel Resizing Engine (Nearest & SIMD) [State: Done, Priority: Medium]
- [x] **OXI-503**: Spatial Filters & Convolutions [State: Done, Priority: Medium]
- [x] **OXI-504**: Binarization (Otsu/Bradley) [State: Done, Priority: Medium]
- [x] **OXI-601**: WGPU Compute Context & Pipeline Initialization [State: Done, Priority: Medium]
- [x] **OXI-602**: Unified Memory Zero-Copy Mapping [State: Done, Priority: Medium]
- [x] **OXI-603**: WGSL Compute Shaders [State: Done, Priority: Medium]
- [x] **OXI-701**: `DynamicImage` & `GenericImageView` Basic Facade [State: Done, Priority: Medium]
- [x] **OXI-702**: `ImageReader`, Format Sniffing & Limits Engine [State: Done, Priority: Medium]
- [x] **OXI-703**: Complete 100% Drop-In API Facade Re-exporting upstream `image` v0.25 with zero-copy `FrameBuffer` bridging [State: Done, Priority: Medium]
- [x] **OXI-704**: Reference Images & Codec Parity [State: Done, Priority: Medium]
- [x] **OXI-705**: Truncation & Limits Parity [State: Done, Priority: Medium]
- [x] **OXI-801**: Testcontainers Ephemeral Fixture Repository [State: Done, Priority: Medium]
- [x] **OXI-802**: Criterion Performance & Memory Regression Bench [State: Done, Priority: Medium]
- [x] **OXI-803**: Scrum Master Audit & DoD Certification [State: Done, Priority: Medium]
- [x] **OXI-901**: 2D Drawing Primitives & Anti-Aliased Lines/Shapes Facade & Bridge [State: Done, Priority: Medium]
- [x] **OXI-902**: Text & Font Glyph Rasterization Engine Facade & Bridge [State: Done, Priority: Medium]
- [x] **OXI-903**: Edge Detection & Gradients (Sobel, Scharr, Canny) [State: Done, Priority: Medium]
- [x] **OXI-904**: Morphological Operators (Dilate, Erode, Open, Close) [State: Done, Priority: Medium]
- [x] **OXI-905**: Hough Feature Detectors & Affine/Warp Transforms [State: Done, Priority: Medium]
- [x] **OXI-906**: Connected Components & Contour Tracing (Suzuki-Abe) [State: Done, Priority: Medium]
- [x] **OXI-1001**: Vectorized Horizontal/Vertical Convolution Kernels with SIMD Accelerator Bridge [State: Done, Priority: Medium]
- [x] **OXI-1002**: Mathematical Resampling Filters & Coefficients (Lanczos3, Mitchell, Bilinear) [State: Done, Priority: Medium]
- [x] **OXI-1003**: Multi-Channel Pipelines (`U8x1..4`, `U16x1..4`, `F32`) [State: Done, Priority: Medium]
- [x] **OXI-1004**: SIMD Alpha Premultiplication / Demultiplication [State: Done, Priority: Medium]
- [x] **OXI-1005**: Multi-Threaded Tiled Resizing Scheduler & Drop-In Facade [State: Done, Priority: Medium]
- [x] **OXI-1101**: Hybrid Binarizer & BitMatrix Primitives Facade & Bridge [State: Done, Priority: Medium]
- [x] **OXI-1102**: Galois Field $GF(2^8)$ Reed-Solomon Codec [State: Done, Priority: Medium]
- [x] **OXI-1103**: QR Code & Micro QR Encoder / Decoder Engine Facade & Bridge [State: Done, Priority: Medium]
- [x] **OXI-1104**: Data Matrix & Aztec 2D Codec Engine [State: Done, Priority: Medium]
- [x] **OXI-1105**: PDF417 High-Density 2D Codec Engine [State: Done, Priority: Medium]
- [x] **OXI-1106**: Sovereign 1D Barcode Suite (EAN, UPC, Code 128, ITF) [State: Done, Priority: Medium]
- [x] **OXI-1201**: Drop-In Compatibility Facades (`oxipix::compat::image`, `oxipix::compat::imageproc`, `oxipix::compat::fast_image_resize`, `oxipix::compat::rxing`) [State: Done, Priority: Medium]
- [x] **OXI-1202**: Upstream Test Parity Integration Suite [State: Done, Priority: Medium]
- [x] **OXI-1203**: Criterion Golden Fixtures & Regression Benchmarks [State: Done, Priority: Medium]
- [x] **OXI-1301**: Complete Elimination of Upstream `image` from `oxipix-compat-image` (sovereign `DynamicImage`, `ImageBuffer`, `RgbImage`, `RgbaImage` directly backed by `FrameBuffer<Rgba8, state::Initialized>`). [State: Done, Priority: Medium]
- [x] **OXI-1302**: SIMD Pixel Iteration & Color Conversion Engine in `oxipix-simd` (vectorized `rgba_to_rgb`, `rgb_to_rgba`, `rgba_to_gray`, `gray_to_rgba`, `invert_rgba_inplace`). [State: Done, Priority: Medium]
- [x] **OXI-1303**: Sovereign Barcode & QR Matrix Codec in `oxipix-barcode` (zero `rxing`/`image` dependencies, SIMD Bradley-Roth/Otsu binarizer, GF(2^8) Reed-Solomon polynomial math, direct `FrameBuffer` decoding). [State: Done, Priority: Medium]
- [x] **OXI-1304**: Headless WGSL Compute Pipelines in `oxipix-gpu` (2D convolution, 3x3 Gaussian spatial blur, homography perspective rectification shader, and Apple Silicon unified memory `MTLResourceStorageModeShared` zero-copy coherency). [State: Done, Priority: Medium]
- [x] **OXI-1401**: Complete Removal of `image` & `rxing` across all workspace crates (including `oxipix-ops` and root `Cargo.toml`). [State: Done, Priority: Medium]
- [x] **OXI-1402**: Complete Purge of Unused Legacy Codecs (`zune-jpeg`, `png`, `gif`, `tiff`, `qoi`, `image-webp` eliminated from `oxipix-codecs/Cargo.toml`). [State: Done, Priority: Medium]
- [x] **OXI-1403**: Sovereign Resampling & CV Engine (`fast_image_resize` and `imageproc` purged; replaced by sovereign `oxipix-ops::resample`, `oxipix-ops::draw`, and `oxipix-ops::cv`). [State: Done, Priority: Medium]
- [x] **OXI-1404**: Pure Sovereign Benchmark Suite in `benches/crate_comparison.rs` with zero external crate dependencies. [State: Done, Priority: Medium]
- [ ] **OXI-1501**: Real JPEG (baseline + progressive) and PNG decoding with EXIF orientation — federated per 00_TECHNICAL_ASSESSMENT §2.4 [State: Backlog, Priority: Urgent]
- [ ] **OXI-1502**: Document/receipt quad detection + homography estimation + bilinear warp [State: Backlog, Priority: High]
- [ ] **OXI-1503**: Thermal-receipt enhancement: CLAHE, illumination/shadow removal, denoise, sharpen [State: Backlog, Priority: High]
- [ ] **OXI-1504**: Robust QR finder-pattern detection under rotation & perspective (CFDI QR) [State: Backlog, Priority: High]
- [ ] **OXI-1505**: Make `gpu` (wgpu) a non-default feature; document mobile/wasm build [State: Backlog, Priority: Medium]
- [ ] **OXI-1506**: Reconcile DEV_STATUS with code (codec claims, Hough/contours, 569-test parity) [State: Backlog, Priority: Medium]

### PLOTLYRS

*(No active tickets)*

### RENDER

*(No active tickets)*

### TAXCALC

*(No active tickets)*

