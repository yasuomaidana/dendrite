# Dendrite Project Development Status & Roadmap

This document maintains the real-time development status, milestones, and active ticket execution pipeline for
**Dendrite**, managed following the autonomous ticket lifecycle protocol.

---

## Current Status Overview

- **Active Milestone**: Milestone 17 — Performance Testing, Inference Benchmarking & Gateway Load Harness (FEAT-021)
- **Current Focus**: `DEN-828: DEV_STATUS replicates only its own Plane project`
- **Last Updated**: 2026-11-02
- **Summary**: Backlog: 104 | To Do: 31 | In Progress: 42 | To Verify: 182 | Done: 43 (Total: 402)
- **Overall Progress**: 43 / 402 Tickets Completed (10.7%)

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

- [ ] **DEN-024**: Agent runner harness: bouton/agentjobs runner shape for current executors [State: To Verify, Priority: Medium]
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
- [ ] **DEN-375**: Governance 1.1: ticketlock package [State: To Verify, Priority: High]
- [ ] **DEN-376**: Governance 1.2: Plane assignee and labels via sidecar [State: To Verify, Priority: High]
- [ ] **DEN-377**: Governance 1.3: MCP and CLI ticket lifecycle tools [State: To Verify, Priority: High]
- [ ] **DEN-378**: Governance 1.4: pre-edit hook, plan-saved hook, git pre-commit, MCP config writers [State: To Verify, Priority: High]
- [ ] **DEN-379**: Governance 1.5: finish command, two-stage assertion, auto-Done [State: To Verify, Priority: High]
- [x] **DEN-380**: Governance 2.3: dendrite directive command [State: Done, Priority: High]
- [x] **DEN-381**: Governance 2.4: allostasis gate and 100% coverage floor [State: Done, Priority: High]
- [ ] **DEN-382**: Governance 3.1: pkg/lang adapter and 5 profiles [State: Backlog, Priority: High]
- [ ] **DEN-383**: Governance 3.2: templated CI/Makefile/config rendering [State: Backlog, Priority: High]
- [ ] **DEN-384**: Governance 3.3: Python/TS/Go/Kotlin template sets [State: Backlog, Priority: Medium]
- [ ] **DEN-385**: Governance 3.4: unknown-language adapter generation [State: Backlog, Priority: Medium]
- [ ] **DEN-386**: Governance 4.1: afferent check three-tier reuse [State: Backlog, Priority: Medium]
- [ ] **DEN-387**: Governance 4.3: dendrite meiosis [State: Backlog, Priority: Medium]
- [ ] **DEN-388**: Governance 5.1: immune update [State: Backlog, Priority: Medium]
- [ ] **DEN-389**: Governance 5.2: immune scan [State: Backlog, Priority: High]
- [ ] **DEN-390**: Governance 5.3: immune enforce [State: Backlog, Priority: Medium]
- [ ] **DEN-393**: Governance 1.6: ticketlock review fixes [State: To Verify, Priority: High]
- [ ] **DEN-394**: Chore: gofmt the ~20 unformatted Go files [State: To Verify, Priority: Medium]
- [ ] **DEN-395**: Governance 1.7: dogfood - soma prepare installs the ticket lock in the Dendrite repo [State: To Verify, Priority: Medium]
- [x] **DEN-396**: Bug: soma prepare overwrites a hand-maintained .github/workflows/ci.yml with the generic template [State: Done, Priority: High]
- [ ] **DEN-397**: Bug: AGENTS.md Synapses section says Plane project unknown for peers that have one [State: To Verify, Priority: Medium]
- [ ] **DEN-398**: Governance 1.8: record agent questions and decisions on the ticket [State: To Verify, Priority: High]
- [ ] **DEN-399**: Governance: decisions and plan [State: To Verify, Priority: Medium]
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
- [ ] **DEN-1017**: fix(agentjobs): AGY model catalog, per-family cooldown, load spreading and safe transient retry [State: Backlog, Priority: High]
- [ ] **DEN-1018**: docs(subprojects): complete sub-project doc imports and fix their indexes [State: Backlog, Priority: Medium]
- [ ] **DEN-1019**: refactor(devtool): shared guarded push and repo inspection [State: Backlog, Priority: Medium]
- [ ] **DEN-1020**: feat(sync): dendrite sync fans out across synapse peers [State: Backlog, Priority: High]
- [ ] **DEN-1021**: feat(commit): dendrite commit --synapse commits and checks peers [State: Backlog, Priority: High]
- [ ] **DEN-1022**: feat(sync): continuous synapse sync via --watch [State: Backlog, Priority: Medium]
- [ ] **DEN-1033**: fix(github): derive github-sync and hitl-sync from Plane and git, isolate tests, fix DEV_STATUS sync [State: To Verify, Priority: High]
- [ ] **DEN-1034**: feat(plane): every editable repository is a Plane project; related repositories share a synapse [State: To Verify, Priority: High]
- [ ] **DEN-1035**: feat(install): branch-driven install and enforced synapse branches [State: To Verify, Priority: High]
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
- [x] **DEN-1046**: feat(rag): graph MCP tools and HTTP endpoints over the dendrite-rag graph.* methods [State: Done, Priority: High]
- [x] **DEN-1047**: feat(stacks): pgvector Postgres and dendrite_rag database for the shared RAG tier; DENDRITE_RAG_PG_URL in prepare [State: Done, Priority: Medium]
- [x] **DEN-1048**: fix(bouton): jobs wait/show --ticket find the job in the ticket's project and fail when none exists [State: Done, Priority: Medium]
- [ ] **DEN-1049**: refactor(rag): ask delegates generation to the dendrite-rag ask RPC; Go keeps the client and the escalation prompt [State: Backlog, Priority: High]
- [x] **DEN-1050**: refactor(agentsearch): rank agents through the dendrite-rag rank RPC; remove the Go BM25, embeddings and RRF [State: Done, Priority: High]
- [ ] **DEN-1051**: ci(quality): max 500 lines per code file and jscpd duplicate gate; fix the 19 clones [State: Backlog, Priority: High]
- [x] **DEN-1052**: refactor: split Go files over 500 lines in cmd/dendrite, tracker/plane, mcpserver and server [State: Done, Priority: Medium]
- [x] **DEN-1053**: refactor: split Go files over 500 lines in agentjobs, agent, devtool, quality, vcs and workcopy [State: Done, Priority: Medium]
- [ ] **DEN-1054**: refactor: split Go files over 500 lines in inference, env, gateway, synapse, node and pool [State: In Progress, Priority: Medium]
- [x] **DEN-1055**: refactor: split Go files over 500 lines in ast, docgen, docusaurus, personalcontext and story [State: Done, Priority: Medium]

### Milestone 40: Synapse Cross-Project Links &amp; Multi-Agent Arbitration Mesh

- [x] **DEN-323**: refactor(agentsearch): rank agents through the dendrite-rag sidecar, no Go BM25/embeddings/RRF [State: Done, Priority: Medium]
- [ ] **DEN-324**: Reproduce and fix Plane create/update failures [State: To Verify, Priority: High]
- [ ] **DEN-325**: MCP tool profiles (core|full) with allowlist flag/env [State: To Verify, Priority: High]
- [ ] **DEN-326**: Read-only MCP probe harness [State: To Verify, Priority: Medium]
- [ ] **DEN-327**: Default-enable only passing MCP tools [State: To Verify, Priority: Medium]
- [ ] **DEN-328**: PreToolUse hook generator [State: To Verify, Priority: High]
- [ ] **DEN-329**: AGY rule parity and rg/dendrite telemetry [State: To Verify, Priority: Medium]
- [ ] **DEN-330**: Render Plane tickets to statedir markdown [State: To Verify, Priority: High]
- [ ] **DEN-331**: tickets corpus in dendrite-rag sidecar [State: To Verify, Priority: High]
- [ ] **DEN-332**: dendrite ask --corpus tickets and MCP tool [State: To Verify, Priority: Medium]
- [ ] **DEN-333**: Synapse sync: RAG index step per member [State: To Verify, Priority: High]
- [ ] **DEN-334**: Synapse federated search [State: To Verify, Priority: Medium]
- [ ] **DEN-335**: Ticket lifecycle guard [State: To Verify, Priority: High]
- [ ] **DEN-336**: Delegation playbook for local/AGY models [State: To Verify, Priority: Low]
- [ ] **DEN-337**: Idempotent Plane upsert keyed on external id [State: Backlog, Priority: Medium]
- [ ] **DEN-338**: Plane smart-select and hitl-sync take 28s/50s [State: To Verify, Priority: High]
- [ ] **DEN-339**: MCP tool to set a Plane ticket status [State: To Verify, Priority: High]
- [ ] **DEN-340**: Blocked and Cancelled ticket states in the tooling [State: To Verify, Priority: High]
- [ ] **DEN-341**: Lifecycle round-trip scratch [State: Cancelled, Priority: Low]
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
- [ ] **DEN-362**: rag-rs: choose which code languages are parsed [State: Backlog, Priority: Medium]
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
- [ ] **DEN-414**: Code debt: remove the repeated code [State: In Progress, Priority: High]
- [ ] **DEN-415**: Coverage batch 1: telemetry/exporter synapse sandbox story/fork tracker [State: To Verify, Priority: Medium]
- [ ] **DEN-416**: Coverage batch 2: orchestrator/aws ui/store agent scheduler/governor statedir [State: To Verify, Priority: Medium]
- [ ] **DEN-417**: Coverage batch 3: rea/showcase ast orchestrator/azure synapsesync tracker/dag [State: To Verify, Priority: Medium]
- [ ] **DEN-418**: Coverage batch 4: docgen/reflect sidecar query/swap scheduler/token vcs [State: To Verify, Priority: Medium]
- [ ] **DEN-419**: Clone batch 1: pkg/topology and pkg/tracker/plane [State: To Verify, Priority: Medium]
- [ ] **DEN-420**: Clone batch 2: pkg/inference [State: To Verify, Priority: Medium]
- [ ] **DEN-421**: Clone batch 3: pkg/agent, pkg/agentjobs, pkg/devtool [State: To Verify, Priority: Medium]
- [ ] **DEN-440**: agy zero-trust: opt out of telemetry and find the settings.json key [State: Backlog, Priority: Medium]
- [x] **DEN-1056**: dendrite ask finds the LiteLLM key outside the Dendrite checkout [State: Done, Priority: High]
- [x] **DEN-1057**: API keys and vendor configuration are configured once in $DENDRITE_HOME/.env [State: Done, Priority: Urgent]

### General

- [x] **DEN-323**: refactor(agentsearch): rank agents through the dendrite-rag sidecar, no Go BM25/embeddings/RRF [State: Done, Priority: Medium]
- [x] **DEN-442**: Rotate LITELLM_SALT_KEY: the live value equals the leaked master key [State: Done, Priority: Medium]
- [ ] **DEN-443**: S1 leftovers: ADR for artifact layout, egm-bootstrap run, argon-high research pin [State: Backlog, Priority: Medium]
- [ ] **DEN-444**: Release develop to main: CLI availability note, CHANGELOG, status [State: To Verify, Priority: Medium]
- [ ] **DEN-808**: Cover cmd/dendrite to 100% and clear the remaining clone groups [State: Backlog, Priority: Medium]
- [ ] **DEN-812**: Complexity quality gate: complexity.max_cyclomatic and max_cognitive directives, gate check [State: In Progress, Priority: Medium]
- [ ] **DEN-813**: Use dendrite-core for complexity, folding and token counts; delete the Go analyzers [State: To Verify, Priority: Medium]
- [ ] **DEN-814**: TODO(move) markers on the Go code that moves to Rust or to mcp-hub [State: In Progress, Priority: Medium]
- [ ] **DEN-815**: Second complexity check in CI with gocyclo, gocognit and ruff [State: In Progress, Priority: Medium]
- [ ] **DEN-816**: Clear the Go complexity debt: 785 functions above cyclomatic 10, 450 above cognitive 15 [State: Backlog, Priority: Medium]
- [ ] **DEN-817**: Stack-aligned CI pipeline generation with cyclomatic and cognitive complexity [State: To Verify, Priority: Medium]
- [ ] **DEN-818**: subprojects.md: say who actually consumes embedag, core-rs and git-rs today [State: In Progress, Priority: Medium]
- [ ] **DEN-819**: CHANGELOG 0.1.0: coreclient, complexity and structure gates, pending moves, second-opinion CI [State: In Progress, Priority: Medium]
- [ ] **DEN-820**: Regenerate the CLI reference docs for allostasis and soma [State: To Verify, Priority: Medium]
- [ ] **DEN-821**: Make the telemetry and triage tests independent of the host: coverage ratchet fails on CI since a136a6a [State: To Verify, Priority: Medium]
- [ ] **DEN-822**: Split cmd/dendrite into sub-packages (86 files, 13680 production lines) [State: Backlog, Priority: Medium]
- [ ] **DEN-823**: Split pkg/ticketlock, pkg/env/bootstrap and pkg/devtool into sub-packages (structure gate) [State: Backlog, Priority: Medium]
- [ ] **DEN-824**: Dendrite: clear 821 complexity violations, 51 clone groups and coverage gaps, then adopt generated ci.yml [State: Backlog, Priority: Medium]
- [x] **DEN-825**: Generated Rust CI installs llvm-tools-preview for cargo llvm-cov [State: Done, Priority: Medium]
- [ ] **DEN-826**: pkg/agent/triage: hermetic tests so the coverage ratchet holds on Linux CI [State: To Verify, Priority: Medium]
- [ ] **DEN-827**: Move-as-you-develop: pre-edit notice, resume summary, burn-down ceiling [State: Backlog, Priority: Medium]
- [ ] **DEN-828**: DEV_STATUS replicates only its own Plane project [State: In Progress, Priority: Medium]
- [ ] **DEN-1017**: fix(agentjobs): AGY model catalog, per-family cooldown, load spreading and safe transient retry [State: Backlog, Priority: High]
- [ ] **DEN-1018**: docs(subprojects): complete sub-project doc imports and fix their indexes [State: Backlog, Priority: Medium]
- [ ] **DEN-1058**: Remove the 4 clone groups the 0.1.0 release still adds (unblocks DEN-444) [State: To Verify, Priority: High]
- [ ] **DEN-1059**: Clear the remaining legacy clone groups and decide include_tests [State: To Do, Priority: Medium]
- [ ] **DEN-1060**: Wire coverage into the quality gate for Rust peers [State: To Do, Priority: Medium]

### Technology Integration Roadmap (2026-11)

- [ ] **DEN-422**: Technology integration roadmap (EGM, Engram, Hermes executor, SkillSpector, dual-protocol): epic [State: Backlog, Priority: High]
- [x] **DEN-423**: Roadmap S0: tickets-RAG health + Plane board reconciliation [State: Done, Priority: Medium]
- [x] **DEN-424**: Roadmap S1: bootstrap - adopt EGM, speq-skill, superpowers; SkillSpector pre-scan; agy zero-trust [State: Done, Priority: Medium]
- [ ] **DEN-425**: Roadmap S2: dendrite immune - SkillSpector gate wrapper [State: Backlog, Priority: Medium]
- [ ] **DEN-426**: Roadmap S3: real local inference - kill the spike serve mock; finish capability catalog [State: Backlog, Priority: Medium]
- [ ] **DEN-427**: Roadmap S4: Engram adoption + frozen session-start snapshot [State: Backlog, Priority: Medium]
- [ ] **DEN-428**: Roadmap S5: consolidation and curation - Engram-native dream job, HRR gap evaluation, minimal skill ledger [State: Backlog, Priority: Medium]
- [ ] **DEN-429**: Roadmap S6: Harness2-lite - tools.toml 8-component schema (DEN-953) + Level-2 rule extraction; k=2 deferred [State: Backlog, Priority: Medium]
- [ ] **DEN-430**: Roadmap S7: EGM pipeline wiring - 7-lens config, goldfish routing to 3.8-flash, drift-report finish gate [State: Backlog, Priority: Medium]
- [ ] **DEN-431**: Roadmap S8: routing completion - scorer/ladder/router (DEN-921/924/925), tier tags, vesicle attribution [State: Backlog, Priority: Medium]
- [ ] **DEN-432**: Roadmap S9: Hermes Agent as fourth executor - sandboxed install, bouton runner, memory isolation, bench [State: Backlog, Priority: Medium]
- [ ] **DEN-433**: Roadmap S10: scoped MCP over HTTP - adopt official go-sdk transport + .well-known lazy schemas [State: Backlog, Priority: Medium]
- [ ] **DEN-434**: Roadmap S11: ACP blackboard + semantic leases on rag-rs symbol hashes [State: Backlog, Priority: Medium]
- [ ] **DEN-435**: Roadmap S12: saga fork --mode parallel + deterministic resolve composer [State: Backlog, Priority: Medium]
- [ ] **DEN-436**: Roadmap S13: closeout - meiosis (DEN-387), cadence scheduler, backlog reconciliation + retro [State: Backlog, Priority: Medium]
- [x] **DEN-437**: CLI validation sweep: verify every documented command works, fix plan for failures, redesign GH issues as human test scripts [State: Done, Priority: High]
- [ ] **DEN-438**: Validation sweep follow-ups: re-scope DEN-024, fold coverage-gap issues into DEN-413, cloud-env tags for #41/#37, park Linear, narrow DEN-917, refresh DEND-402/601 [State: To Verify, Priority: High]
- [ ] **DEN-439**: chronos finish: detect [AC-n] criteria written inline in one paragraph, not only one-per-line [State: To Verify, Priority: High]
- [ ] **DEN-441**: Secrets rotation UX: 'ensure --rotate' alone silently no-ops, and 'motor restart' keeps a container's stale env [State: Backlog, Priority: Medium]

