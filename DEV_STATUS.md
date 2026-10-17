# Dendrite Project Development Status & Roadmap

This document maintains the real-time development status, milestones, and active ticket execution pipeline for
**Dendrite**, managed following the autonomous ticket lifecycle protocol.

---

## Current Status Overview

- **Active Milestone**: Milestone 17 — Performance Testing, Inference Benchmarking & Gateway Load Harness (FEAT-021)
- **Current Focus**: `DEN-981: GitHub Copilot access via LiteLLM: persisted device login and configurable model`
- **Last Updated**: 2026-10-17
- **Summary**: Backlog: 58 | To Do: 24 | In Progress: 20 | To Verify: 87 | Done: 21 (Total: 210)
- **Overall Progress**: 21 / 210 Tickets Completed (10.0%)

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
- [ ] **DEN-901**: Untrack runtime artifacts and committed secret [State: In Progress, Priority: Urgent]
- [ ] **DEN-902**: EnsureLiteLLMMasterKey + compose wiring (P4) [State: Backlog, Priority: Urgent]
- [ ] **DEN-976**: Spawned projects get lifecycle states (To Verify) and exact ticket-ID matching [State: To Verify, Priority: High]
- [ ] **DEN-977**: plane create/update accept Markdown spec files [State: To Verify, Priority: High]
- [ ] **DEN-978**: Hermetic agent tests: no live daemon probing or real agent execution [State: Backlog, Priority: Urgent]
- [ ] **DEN-979**: De-mock pkg/story/fork tests with real git repos [State: To Verify, Priority: High]
- [ ] **DEN-980**: dendrite commit pushes after committing when a remote is configured [State: To Verify, Priority: Urgent]
- [ ] **DEN-981**: GitHub Copilot access via LiteLLM: persisted device login and configurable model [State: In Progress, Priority: Urgent]
- [ ] **DEN-983**: dev ticket set accepts human-readable state names (To Verify) [State: To Verify, Priority: Urgent]
- [ ] **DEN-984**: plane replicate reflects every ticket and the To Verify state in DEV_STATUS.md [State: Backlog, Priority: High]
- [ ] **DEN-985**: Enforced documentation: generated CLI/MCP reference, README and docs audit gate [State: Backlog, Priority: High]
- [ ] **DEN-986**: Per-package coverage ratchet towards the 100% goal [State: Backlog, Priority: High]
- [ ] **DEN-987**: Stop env replicate and root tests from reverting the LiteLLM control stack [State: Backlog, Priority: Urgent]

### Milestone 1: Core Foundation, Developer Tools & Ollama Engine

- [x] **DEN-001**: Initialize Development Status Ledger & Execution Roadmap [State: Done, Priority: Medium]
- [x] **DEN-002**: Implement Idiomatic Ollama Client & Metal Engine Support [State: Done, Priority: Medium]
- [x] **DEN-003**: Developer Productivity Tools on the Fly & CLI Scaffold (`pkg/devtool`, `cmd/dendrite`) [State: Done, Priority: Medium]
- [x] **DEN-004**: Ticket Module & Local Status Synchronization Engine [State: Done, Priority: Medium]
- [ ] **DEN-701**: Git Branch Guard: main lock, develop default & atomic CommitAndPush [State: To Verify, Priority: Urgent]
- [ ] **DEN-702**: Ticket Domain Model, Triage & Low-Budget DAG Prioritizer [State: To Verify, Priority: Urgent]
- [ ] **DEN-703**: GitHub Issues Two-Way Synchronizer & Label Provisioner [State: To Verify, Priority: Urgent]
- [ ] **DEN-704**: Speculative Stacked Branch Orchestrator & Degraded Budget Mode [State: To Verify, Priority: Urgent]
- [ ] **DEN-705**: dendrite audit-changes CLI Command & Vendor LLM Rebase/Merge Engine [State: To Verify, Priority: Urgent]
- [ ] **DEN-801**: Topic Zero State Prompt Engine & Multi-Turn State Machine [State: In Progress, Priority: High]
- [ ] **DEN-802**: Welcome Prompt Suite & Cross-Source Synthesis Dispatcher [State: To Verify, Priority: High]
- [ ] **DEN-803**: Structured Topic Memory Parser & Zero-Extrapolation Fidelity Guard [State: To Verify, Priority: Medium]
- [ ] **DEN-804**: System Installer Helpers: Pre-Flight Audit & Port/Hardware Prober [State: To Verify, Priority: High]
- [ ] **DEN-805**: Local LLM Model Provisioner & LiteLLM Gateway Routing Bridge [State: To Verify, Priority: Medium]
- [ ] **DEN-806**: Dynamic 10-Style Greeting Matrix & Cliché Suppression Engine [State: To Verify, Priority: Medium]
- [ ] **DEN-807**: Canvas Interactive Writing & Direct Creative Action Handlers [State: To Verify, Priority: Medium]
- [ ] **DEN-808**: Personal Context MCP Connectors: Workspace & Enterprise Tool Adapters [State: Backlog, Priority: Medium]
- [ ] **DEN-809**: Local LLM Inference Concurrency Limiter & FIFO Priority Queue [State: To Verify, Priority: High]
- [ ] **DEN-817**: Decouple Git Commit Synthesis Engine: OxiLLaMa HTTP Adapter & Configurable Fallbacks [State: To Verify, Priority: Medium]
- [ ] **DEN-822**: Modernize Root CLI with Cobra-style Command Architecture and Local Model Delegation [State: To Verify, Priority: High]

### QA-M1: Truthful Assertion (Pillar 1)

- [ ] **DEN-910**: ASTProvider iface + Go go/ast impl (changed symbols, fold, last-node for Go) [State: Backlog, Priority: High]
- [ ] **DEN-911**: AST mock-reachability analyzer (replaces ScanFilesForMocks) [State: Backlog, Priority: High]
- [ ] **DEN-912**: Stage B toolchain adapter (go/cargo/bun/uv) + changed-pkg coverage & cyclomatic [State: Backlog, Priority: High]
- [ ] **DEN-913**: LLM assertion query + strict JSON verdict (per-AC evidence) [State: Backlog, Priority: High]
- [ ] **DEN-914**: Delete stub analyzers; rewire AuditFinishedTicket to pkg/assert; 85% gate [State: Backlog, Priority: High]
- [ ] **DEN-915**: TODO extractor + Plane child-ticket spawner [State: Backlog, Priority: High]
- [ ] **DEN-916**: GatedTracker (ironclad state gate) [State: Backlog, Priority: High]
- [ ] **DEN-917**: Backfill: re-audit the 37 under-specified tickets and all To Verify [State: Backlog, Priority: High]
- [ ] **DEN-982**: Replace production mock types with real implementations [State: Backlog, Priority: Urgent]

### Milestone 2: Multi-Node Topology & Cloud-Burst Infrastructure

- [x] **DEN-005**: Topology Configuration Model & Scenario Placement Types [State: Done, Priority: Medium]
- [x] **DEN-006**: Terraform Cloud-Init & Compose Profile Runner [State: Done, Priority: Medium]
- [x] **DEN-016**: Terraform AWS Spot EC2 Module & Cloud-Init Bootstrap [State: Done, Priority: Medium]
- [x] **DEN-017**: Persistent EBS Volume Lifecycler & Zero-Ingress Security Group Validator [State: Done, Priority: Medium]
- [ ] **DEN-029**: Development Environment Replication, Host Installer & Containerized Tooling (`pkg/env`, `stacks`) [State: To Verify, Priority: Medium]

### QA-M2: Router, Cooldown & Escalation Ladder (Pillar 2)

- [ ] **DEN-920**: Capability catalog (models.toml) + bench ingestion [State: Backlog, Priority: High]
- [ ] **DEN-921**: Capability scorer + OxiLLaMa tie-break +  small-model rule [State: Backlog, Priority: High]
- [ ] **DEN-922**: Lifecycle manager: leases, derived 30m countdown, unload [State: Backlog, Priority: High]
- [ ] **DEN-923**: Probe + auto-start + VRAM check [State: Backlog, Priority: High]
- [ ] **DEN-924**: Escalation ladder + HandoverCapsule + retry budgets [State: Backlog, Priority: High]
- [ ] **DEN-925**: Route all inference through router/lease (incl. devtool oxillama run exec) [State: Backlog, Priority: High]

### Milestone 3: Token Budget Governance & Fallback Gateway

- [x] **DEN-007**: LiteLLM Configuration Template & Dynamic Renderer [State: Done, Priority: Medium]
- [x] **DEN-008**: Sliding Window Budget Limiter & Ollama Failover Engine [State: Done, Priority: Medium]

### QA-M3: Smart Commit (Pillar 8)

- [ ] **DEN-932**: Hunk extractor + symbol-dependency graph [State: Backlog, Priority: High]
- [ ] **DEN-933**: Clusterer + LLM plan + validators (exactly-once, topo order, per-commit build) [State: Backlog, Priority: High]
- [ ] **DEN-934**: --local-review stacked branches + git notes telemetry + review audit/land [State: Backlog, Priority: High]

### Milestone 4: Autonomous Coding Agent Harness & Tool Orchestration

- [ ] **DEN-024**: Agent Runner Harness & Environment Injector for Aider, OpenHands, and OpenCode [State: To Do, Priority: Medium]
- [x] **DEN-025**: Agent Sandbox Isolation & LiteLLM Gateway Budget Binding [State: Done, Priority: Medium]
- [ ] **DEN-026**: Agent Session Tracker, Git Commit Provenance & Issue Tracker Sync [State: In Progress, Priority: Medium]
- [ ] **DEN-819**: Externalize Agent Runner Gateway Credentials & Virtual Key Governance [State: Backlog, Priority: Medium]

### Milestone 4: Autonomous Coding Agent Harness &amp; Tool Orchestration (FEAT-013)

- [ ] **DEN-103**: & Completed Agent Modules: Exhaustive Test Suite & 100% Statement Coverage [State: In Progress, Priority: Medium]

### QA-M4: Token-Limit Recovery (Pillar 13)

- [ ] **DEN-940**: TokenCounter impls + Budget.Fit (25% reserve, 80% trigger) [State: Backlog, Priority: High]
- [ ] **DEN-941**: <StateCheckpoint> schema + Engram flush [State: Backlog, Priority: High]
- [ ] **DEN-942**: Truncation splicer (Go via go/ast; other languages after ) [State: Backlog, Priority: High]
- [ ] **DEN-943**: BLAKE3 chunk ledger + WAL + resume [State: Backlog, Priority: High]
- [ ] **DEN-944**: 429 backoff + quota-floor downshift / PAUSED_QUOTA_FLOOR [State: Backlog, Priority: High]
- [ ] **DEN-945**: LiteLLM enable_pre_call_checks + per-local-alias context_window_fallbacks [State: Backlog, Priority: High]

### Milestone 5: Reverse Engineering & Sandboxed Intelligence

- [ ] **DEN-009**: REA MCP Bridge & Disassembler Protocol Client [State: To Do, Priority: Medium]
- [x] **DEN-010**: Tree-sitter AST & Complexity Analyzer [State: Done, Priority: Medium]
- [x] **DEN-011**: Ephemeral Hardened Container Sandbox with RAII Drop [State: Done, Priority: Medium]

### QA-M5: CLI, Tokens & Tool Registry (Pillars 10-12)

- [ ] **DEN-950**: Cobra noun-verb tree; port all 28 commands; hidden deprecated aliases [State: Backlog, Priority: Medium]
- [ ] **DEN-951**: config edit transactional editor flow [State: Backlog, Priority: Medium]
- [ ] **DEN-952**: 4-source token aggregator + tokens live TUI/SSE [State: Backlog, Priority: Medium]
- [ ] **DEN-953**: Tool registry (tools.toml + SQLite) + installer enforcement [State: Backlog, Priority: Medium]
- [ ] **DEN-954**: Supervisor pre/stream/post hooks [State: Backlog, Priority: Medium]

### Milestone 6: AI-Assisted Token Optimization & Cleanroom Import

- [x] **DEN-018**: AST Context Compaction & LiteLLM Prefix Cache Harness [State: Done, Priority: Medium]
- [ ] **DEN-019**: Cross-Repository Capability Dissection & Cleanroom Synthesizer [State: To Do, Priority: Medium]

### QA-M6: Native AI, Lifecycle Schema & Memory (Pillars 3,5,6,7)

- [ ] **DEN-930**: Spike: Penpot official MCP server deployment shape [State: Backlog, Priority: Medium]
- [ ] **DEN-931**: Spike: Plane AI env/edition verification [State: Backlog, Priority: Medium]
- [ ] **DEN-960**: Penpot MCP service + hub registration [State: Backlog, Priority: Medium]
- [ ] **DEN-961**: Plane AI → LiteLLM virtual key + local-coder alias [State: Backlog, Priority: Medium]
- [ ] **DEN-962**: TicketSpec schema + vagueness linter + /plan auto-trigger [State: Backlog, Priority: Medium]
- [ ] **DEN-963**: Single-source skill generation for .agents/.gemini lifecycle skills [State: Backlog, Priority: Medium]
- [ ] **DEN-964**: memory modes: local-only / edge (WAL) / push (+compaction) [State: Backlog, Priority: Medium]

### Milestone 7: Polyglot Translation & Documentation Reflection

- [x] **DEN-020**: Intermediate Semantic Representation ISR Tree-sitter Parser [State: Done, Priority: Medium]
- [ ] **DEN-021**: Idiomatic Target Code Synthesizer & Dual Property Test Parity Generator [State: In Progress, Priority: Medium]
- [x] **DEN-022**: AST Docs-as-Code Synthesizer for FEAT/ADR Specs & OpenAPI 3.1 [State: Done, Priority: Medium]
- [x] **DEN-023**: Cross-Project Documentation Reflection Engine & Docusaurus Sync [State: Done, Priority: Medium]

### QA-M7: Go/Rust Decomposition & Pool Dogfood (Pillar 9)

- [ ] **DEN-970**: Sidecar JSON-RPC contract + Go client with in-process fallback [State: Backlog, Priority: Low]
- [ ] **DEN-971**: Extract dendrite-mcp-hub (history-preserving) [State: Backlog, Priority: Low]
- [ ] **DEN-972**: dendrite-core-rs: tree-sitter fold / last-node / complexity / blake3 / tokenize [State: Backlog, Priority: Low]
- [ ] **DEN-973**: dendrite-git-rs: gix hunk graph + clusterer [State: Backlog, Priority: Low]
- [ ] **DEN-974**: dendrite-rag-rs: BM25+vector hybrid + sync engine [State: Backlog, Priority: Low]
- [ ] **DEN-975**: Pool dogfood E2E across 5 sibling dirs (GoLand + RustRover scopes) [State: Backlog, Priority: Low]

### Milestone 8: Multi-Project Workspaces & Pluggable Trackers

- [x] **DEN-012**: Multi-Project Workspace Registry & Context Switcher [State: Done, Priority: Medium]
- [x] **DEN-013**: Pluggable Issue Tracker Interface & Atlassian Jira Adapter [State: Done, Priority: Medium]
- [ ] **DEN-027**: Plane v2 & Linear Strategy Adapters [State: To Verify, Priority: Medium]
- [ ] **DEN-812**: Bidirectional Multi-Vendor Migration & Real-Time Sync Engine (Plane <-> Jira, Penpot <-> Figma) [State: Backlog, Priority: High]

### Milestone 9: Web UI & Dashboard Engine

- [x] **DEN-014**: Embedded HTTP Control Server & WebSocket Telemetry [State: Done, Priority: Medium]
- [ ] **DEN-015**: Real-Time Telemetry Gauges & Thermal Monitoring Daemon [State: In Progress, Priority: Medium]

### Milestone 10: IDE Intelligence & JetBrains MCP Bridge

- [ ] **DEN-028**: JetBrains IDE MCP Integration & Communication Bridge [State: To Verify, Priority: Medium]

### Milestone 11: Pluggable Mesh Networking & Zero-Trust Connectivity

- [ ] **DEN-030**: Multi-Mesh Networking Abstraction: Cloudflare Tunnels, SSH Port Forwarding, WireGuard, and Direct LAN (`pkg/topology/network`, `pkg/orchestrator`) [State: To Verify, Priority: Medium]

### Milestone 12: Open-Source Project Management & Design Tooling

- [ ] **DEN-031**: Self-Hosted Plane v2 & Penpot Design Token Stacks & Replicator (`pkg/env`, `stacks/compose.plane.yaml`, `stacks/compose.penpot.yaml`) [State: To Verify, Priority: Medium]
- [ ] **DEN-097**: Plane Doc Sync Taxonomy Guard & Markdown-to-HTML Robust Formatter [State: To Verify, Priority: Medium]

### Milestone 13: Remote SSH Hardware Discovery & Automated Service Placement Router

- [ ] **DEN-032**: Remote SSH Node Topology Provisioner & Dynamic Service Placement Router (`pkg/topology/advisor`, `cmd/dendrite`) [State: To Verify, Priority: Medium]

### Milestone 14: Hybrid Telemetry Architecture & Prometheus/Grafana Observability Suite

- [ ] **DEN-033**: Prometheus `/metrics` Exporter & Grafana Observability Dashboard Integration Pack (`pkg/telemetry/exporter`, `stacks/compose.grafana.yaml`) [State: To Verify, Priority: Low]

### Milestone 15: Azure Cloud-Burst Infrastructure & Managed Storage

- [ ] **DEN-034**: Terraform Azure Spot VM Module & Persistent Managed Disk Lifecycler (`infra/modules/azure-burst`, `pkg/orchestrator/azure`) [State: To Verify, Priority: Medium]

### Milestone 16: Pluggable Multi-Provider Inference Engines

- [ ] **DEN-035**: Unified Inference Engine Gateway: OxiLLaMa, Hugging Face TGI/TEI, vLLM, llama.cpp & Ollama Metal (`pkg/inference`, `pkg/gateway`) [State: To Verify, Priority: High]
- [ ] **DEN-809**: Model Migration Engine: Migrate Models from Ollama to OxiLLaMa & Evict Local Blobs [State: Backlog, Priority: High]
- [ ] **DEN-815**: Decouple Hardcoded Inference Engine Endpoints, Cascade Sequence & Model Catalog [State: To Verify, Priority: High]

### Milestone 17: Performance Testing, Inference Benchmarking & Gateway Load Harness

- [ ] **DEN-036**: k6 Performance Testing, TTFT/TPS Benchmarking & Cascade Failover Load Harness (`pkg/perf`, `cmd/dendrite`, `tests/perf`) [State: To Verify, Priority: High]

### Milestone 18: Agent Search, Capability Discovery & Semantic Routing Engine

- [ ] **DEN-037**: Hybrid Agent Search, TEI Embeddings & Semantic Routing Engine (`pkg/agentsearch`, `cmd/dendrite`) [State: In Progress, Priority: High]

### Milestone 19: Developer Test Automation, Self-Healing Harness & UI Integration Suite

- [ ] **DEN-038**: Automated Test Synthesizer, Self-Healing Convergence Loop & Web UI Test Harness (`pkg/testgen`, `pkg/uitest`, `cmd/dendrite`) [State: In Progress, Priority: Urgent]
- [ ] **DEN-821**: Standardize Toolchain Migration on uv (Python) and bun (Node/TS) across Website, Scripts & CI/CD [State: Backlog, Priority: High]

### Milestone 20: Open-Source REA Showcase Pipeline & Cleanroom Parity Verification Suite

- [ ] **DEN-039**: Open-Source Reverse Engineering Showcases & Cleanroom Parity Verification Suite (`pkg/rea`, `pkg/ai/import`, `tests/showcase`, `cmd/dendrite`) [State: To Verify, Priority: High]

### Milestone 21: Model Context Protocol (MCP) Server & Tool Registry Bridge

- [ ] **DEN-040**: Dendrite Native MCP Server & Tool Registry Bridge (`pkg/mcpserver`, `cmd/dendrite`) [State: To Verify, Priority: Medium]

### Milestone 22: Distributed Codebase Ecosystem, Modular REA & Cross-Project Interoperability Coordinator

- [ ] **DEN-041**: Distributed Codebase Multi-Source Ecosystem & Priority Resolver (pkg/distcodebase, cmd/dendrite) [State: To Verify, Priority: Medium]
- [ ] **DEN-042**: Scenario 1: REA Modular Decomposition & Ecosystem Reuse Discovery Engine (pkg/distcodebase, pkg/rea, cmd/dendrite) [State: To Verify, Priority: Medium]
- [ ] **DEN-043**: Scenario 2: Parallel Multi-Project Workspace & Repository Spawner (pkg/distcodebase, pkg/workspace, cmd/dendrite) [State: To Verify, Priority: Medium]
- [ ] **DEN-044**: Scenario 3: Local LLM Cross-Project Interoperability Coordinator (pkg/distcodebase, pkg/ollama, cmd/dendrite) [State: To Verify, Priority: Medium]
- [ ] **DEN-045**: Scenario 4: Generative Satellite Project Synthesis & Autonomous Plane Project Provisioning (pkg/distcodebase, pkg/tracker/plane, cmd/dendrite) [State: To Verify, Priority: Medium]
- [ ] **DEN-046**: JetBrains IDE Toolset Integration, 19 MCP Tools & Agentic Developer Workflow Guide (pkg/jetbrains, pkg/mcpserver, cmd/dendrite) [State: To Verify, Priority: Medium]

### Milestone 23: Hardening, Benchmark Scenarios & Cleanroom Invariant Governance

- [ ] **DEN-047**: Fix Goroutine Deadlock & Dynamic Buffer Starvation in Synthetic k6 Benchmark Runner (`pkg/perf`, `cmd/dendrite`) [State: To Verify, Priority: Medium]
- [ ] **DEN-048**: Automatic Zero-Latency Fallback Cascade for Unreachable Inference Upstreams in Benchmark Scenarios (`pkg/perf`, `pkg/gateway`, `pkg/inference`) [State: To Verify, Priority: Medium]
- [ ] **DEN-049**: Multi-Digest Test Vector Errata & 63-Byte Alignment Invariant Suite for REA Showcases (`pkg/rea/showcase`, `tests/showcase`) [State: To Verify, Priority: Medium]

### Milestone 23: Hardening, Benchmark Scenarios, Cleanroom Invariants & Local Agent Triage

- [ ] **DEN-050**: Local Agent Triage Gateway, Token-Efficient Prompt Router & JetBrains AI Chat Multi-MCP Orchestrator (`pkg/agent/triage`, `pkg/jetbrains`, `pkg/mcpserver`, `pkg/gateway`) [State: To Verify, Priority: Medium]
- [ ] **DEN-816**: Dynamic Triage Engine Hardware Probing & Model Precedence Resolution [State: In Progress, Priority: High]

### Milestone 24: Autonomous Pool DAG & Multi-IDE Project Synchronization

- [ ] **DEN-051**: Autonomous Dependency Inference & Multi-Directory DAG Solver (`pkg/pool/dag`, `cmd/dendrite`) [State: To Verify, Priority: Medium]
- [ ] **DEN-052**: Multi-Mode Dependency Switcher: Published, Branch, Local Lockstep & Remote Fork (`pkg/pool/switcher`, `pkg/jetbrains`) [State: To Verify, Priority: Medium]
- [ ] **DEN-053**: Mid-Flight Project Splitter & Mandatory Interactive Target Directory Gate (`pkg/pool/split`, `pkg/ui/modal`) [State: To Verify, Priority: Medium]
- [ ] **DEN-813**: Project Environment Preparation, Agent/Skill Sync & CI/CD Enforcement with GitHub Copilot [State: To Verify, Priority: High]

### Milestone 25: Universal Query Engine & 3-Tier Library Reuse Auditor

- [ ] **DEN-054**: Universal Intent Slicing & Binary/Repo Debloating Engine (`pkg/query/slice`, `cmd/dendrite`) [State: To Verify, Priority: Medium]
- [ ] **DEN-055**: Subsystem Trait Boundary Adapter & Graphics Engine Swapper (`pkg/query/swap`, `pkg/rea`) [State: To Verify, Priority: Medium]
- [ ] **DEN-056**: Idiomatic Binary/Repo-to-Rust Transpiler & Differential Test Harness (`pkg/query/transpile`, `tests/differential`) [State: To Verify, Priority: Medium]
- [ ] **DEN-057**: Multi-Artifact Architecture Fusion & In-Process Channel Synthesizer (`pkg/query/fuse`, `pkg/pool`) [State: To Verify, Priority: Medium]
- [ ] **DEN-058**: Strict 3-Tier Library Reuse Hierarchy Lookup & ADR Provenance Recorder (`pkg/query/reuse`, `pkg/store`) [State: To Verify, Priority: Medium]

### Milestone 26: Persistent Segmented Store & Token-Aware Hybrid Scheduler

- [ ] **DEN-059**: BLAKE3 Content-Addressed Persistent Segmented Store & Chunk Checkpointer (`pkg/store/segmented`, `cmd/dendrite`) [State: To Verify, Priority: Medium]
- [ ] **DEN-060**: Store Inspection, Chunk Status Telemetry & Explicit User Purge Lifecycler (`pkg/store/status`, `pkg/ui/store`) [State: To Verify, Priority: Medium]
- [ ] **DEN-061**: Token-Aware Hybrid Scheduler: Night Local Window & 48h Pre-Reset Cloud Sprint (`pkg/scheduler/token`, `pkg/gateway`) [State: To Verify, Priority: Medium]
- [ ] **DEN-062**: Quota Floor Safety Guard (20%/0% Drain Override) & Continuous Analysis Pause Engine (`pkg/scheduler/governor`, `cmd/dendrite`) [State: To Verify, Priority: Medium]

### Milestone 27: Advanced Polyglot Contract Sync, Speculative Branching & Upstream Drift

- [ ] **DEN-063**: Quad-IDE Cross-Language Schema & IPC Contract Drift Monitor (`pkg/polyglot/contract`, `pkg/jetbrains`) [State: To Verify, Priority: Medium]
- [ ] **DEN-064**: Speculative Git Worktree Branching & Differential Architecture Benchmark Harness (`pkg/story/fork`, `pkg/perf`) [State: To Verify, Priority: Medium]
- [ ] **DEN-065**: Sliced Upstream Commit Drift Analyzer & Bug Fix Porting Generator (`pkg/story/drift`, `pkg/query`) [State: To Do, Priority: Medium]
- [ ] **DEN-066**: Multi-Repo Context Re-Hydration & IDE Scope Restorer (`pkg/story/resume`, `cmd/dendrite`) [State: To Verify, Priority: Medium]

### Milestone 28: Zero-Data-Loss Infrastructure Mobility & Smart Repo-Aware Backup Engine

- [ ] **DEN-067**: 5-Phase Atomic Service Migration Engine: Local <-> Debian <-> Azure (`pkg/node/migrate`, `cmd/dendrite`) [State: To Verify, Priority: Medium]
- [ ] **DEN-068**: Write-Ahead Log (WAL) State Journaling & Merkle Digest Verification Protocol (`pkg/node/wal`, `pkg/store`) [State: To Do, Priority: Medium]
- [ ] **DEN-069**: Smart Repo-Aware Backup Engine with Git-Persisted Blob Eviction Priority (`pkg/backup/engine`, `cmd/dendrite`) [State: To Do, Priority: Medium]

### Milestone 29: Dual-Mode Configuration Parity & Multi-Channel Go CI/CD Automation

- [ ] **DEN-070**: Unified Configuration Subsystem with nvim/vim Transactional Staging & Taplo Schema (`pkg/config/engine`, `cmd/dendrite`) [State: To Verify, Priority: Medium]
- [ ] **DEN-071**: Multi-Channel CI/CD Distribution Pipeline: Brew Tap, Debian .deb, GHCR & GoReleaser (`.github/workflows`, `.goreleaser.yaml`) [State: To Do, Priority: Medium]
- [ ] **DEN-818**: Externalize Hardcoded Tailscale IPs & Topology Mesh Endpoints [State: Backlog, Priority: Medium]

### Milestone 30: Multi-Provider AI Runtime & Semantic Hierarchical Git Governance

- [ ] **DEN-072**: Multi-Provider AI Inference Adapter: GitHub Copilot CLI & Native/Bridged Ollama (`pkg/agent/runtime`, `cmd/dendrite`) [State: To Do, Priority: High]
- [ ] **DEN-073**: 5-Tier Version Strategy Interceptor & Semantic Hierarchical Branch/Merge Engine (`pkg/agent/gitgov`, `pkg/pool`) [State: To Do, Priority: Urgent]
- [ ] **DEN-098**: Autonomous Git Commit Engine with Local Model Message Delegation & Zero-Cost Synthesis [State: To Verify, Priority: High]

### Milestone 31: Ticket-Milestone DAG Linker & Story Prioritization Engine

- [ ] **DEN-074**: Ticket-Milestone Relational Ingestion & Dependency DAG Solver (`pkg/stories/dag`, `pkg/tracker`) [State: To Verify, Priority: High]
- [ ] **DEN-075**: Context-Aware `dendrite stories next` Scheduler & Active Story Context Injector (`pkg/stories/next`, `pkg/jetbrains`) [State: In Progress, Priority: High]

### Milestone 32: Bidirectional Penpot UI Documentation & MCP Integration

- [ ] **DEN-076**: UI File Mutation Watcher & Penpot Documentation Token Diff Synthesizer (`pkg/ui/penpot`, `pkg/mcpserver`) [State: To Verify, Priority: Medium]
- [ ] **DEN-077**: Bidirectional Traceability: Ticket Comment Frame Deep-Linking & Design Drift Sync (`pkg/tracker/plane`, `pkg/ui/penpot`) [State: To Verify, Priority: Medium]
- [ ] **DEN-810**: Penpot Service Lifecycle & Web UI Development Synchronized with Plane [State: To Verify, Priority: High]

### Milestone 33: Self-Update Lifecycle & Developer Mode Dual-Source Engine

- [ ] **DEN-078**: Self-Update Lifecycle Manager with Checksum Verification & Binary Rollback (`pkg/update/lifecycle`, `cmd/dendrite`) [State: To Verify, Priority: High]
- [ ] **DEN-079**: Developer Mode (`--dev`) Dual-Source Engine: Local-First Build & Remote Override (`pkg/update/devmode`, `docs/DEVELOPMENT.md`) [State: To Do, Priority: High]

### Milestone 34: Polyglot Environment Bootstrapper & GoF Pattern Analyzer

- [ ] **DEN-080**: Polyglot Stack Fingerprinter & Agent Environment Initializer (`pkg/env/bootstrap`, `cmd/dendrite`) [State: To Verify, Priority: Medium]
- [ ] **DEN-081**: GoF Design-Pattern Analyzer Skill Generator (`pkg/env/patterns`, `.agents/skills`) [State: To Do, Priority: Medium]
- [ ] **DEN-082**: SOTA Best-Practices Web/Advisory Diff Inspector & Automated Updater (`pkg/env/updater`, `cmd/dendrite`) [State: To Verify, Priority: Medium]

### Milestone 35: Offline-First Local Cache & Outbox Sync Engine

- [ ] **DEN-083**: Embedded Turso/libSQL Local Cache with WAL Journaling & Testcontainers Integration (`pkg/cache/turso`, `pkg/store`) [State: To Do, Priority: Medium]
- [ ] **DEN-084**: Transactional Outbox Mutation Queue & Exponential Backoff Replay Worker (`pkg/cache/outbox`, `pkg/tracker`) [State: To Do, Priority: Medium]
- [ ] **DEN-085**: Automated Gitignore Binary & Cache Artifact Exclusion Guardian (`pkg/cache/hygiene`, `pkg/env`) [State: To Do, Priority: Medium]

### Milestone 36: Pre-Commit Secret Scanner & Pre-PR/Merge Quality Gate

- [ ] **DEN-086**: Deterministic Pre-Commit Entropy & Credential Scanner Interceptor (`pkg/security/secretgate`, `pkg/agent`) [State: To Verify, Priority: Urgent]
- [ ] **DEN-087**: Automated Pre-PR & Pre-Merge Build, Lint, and Test Verification Gate (`pkg/quality/harness`, `pkg/agent`) [State: In Progress, Priority: Urgent]

### Milestone 37: Quota Failover, Worktree Isolation & Immutable Agent Audit Logging

- [ ] **DEN-088**: Hybrid Quota Scheduler & Mid-Story Copilot-to-Ollama Graceful Failover (`pkg/scheduler/failover`, `pkg/gateway`) [State: To Do, Priority: Medium]
- [ ] **DEN-089**: Ephemeral Git Worktree Isolation Engine & RAII Rollback Controller (`pkg/agent/worktree`, `pkg/pool`) [State: In Progress, Priority: Medium]
- [ ] **DEN-090**: Immutable libSQL Agent Audit Log & `dendrite audit` Trace CLI (`pkg/audit/logger`, `cmd/dendrite`) [State: To Verify, Priority: Medium]

### Milestone 38: Artifact & Package Publisher MCP Service & Registry Governance

- [ ] **DEN-091**: Pluggable Registry Adapter Interface & Cargo Workspace Topological Publisher (`pkg/mcp/publisher`, `internal/mcp/publisher`) [State: To Do, Priority: Medium]
- [ ] **DEN-092**: Pre-Flight Validation, Dry-Run SemVer Guard & Secret-Isolated Execution Gate (`pkg/mcp/publisher`, `cmd/dendrite`) [State: To Do, Priority: Medium]
- [ ] **DEN-093**: Post-Publish Git Tag Lifecycle & Stdio JSON-RPC MCP Tool Registry (`pkg/mcp/publisher`, `pkg/mcpserver`) [State: To Do, Priority: Medium]

### Milestone 39: Local & Custom-Server Ephemeral CI/CD Routine & Mesh Runner

- [ ] **DEN-094**: Ephemeral Container Sandbox Runner & Automated Dynamic On-Demand Activation (`pkg/routines/cicd`, `internal/routines/cicd`) [State: To Verify, Priority: Medium]
- [ ] **DEN-095**: Remote Tailscale Mesh SSH/Docker Execution Target & One-Time GitHub Ephemeral Runner (`pkg/routines/cicd`, `pkg/topology/network`) [State: To Do, Priority: Medium]
- [ ] **DEN-096**: Token-Efficient Structured Log Tailing & MCP Routine Control Tools (`pkg/routines/cicd`, `pkg/mcpserver`, `cmd/dendrite`) [State: To Do, Priority: Medium]
- [ ] **DEN-814**: Dynamic LiteLLM Routing Table Updater & Daily Scheduler with Manual Trigger [State: To Verify, Priority: High]
- [ ] **DEN-820**: Dynamic Gateway Renderer Model Routing by Live Engine Intelligence [State: To Verify, Priority: High]

### General

- [ ] **DEN-101**: Multi-Vendor Agent Architecture Review: Evaluate Anthropic Claude Code, OpenAI Operator, and Cursor Integration [State: To Do, Priority: Medium]
- [ ] **DEN-102**: Autonomous Agent Harness: OpenCode Real Execution Validation & Sandboxing [State: In Progress, Priority: Medium]
- [ ] **DEN-701**: - Coverage Gap: Reach 100% coverage in pkg/vcs/guard.go [State: To Verify, Priority: Urgent]
- [ ] **DEN-702**: - Coverage Gap: Reach 100% coverage in pkg/tracker/routing.go [State: In Progress, Priority: Urgent]
- [ ] **DEN-703**: - Coverage Gap: Reach 100% coverage in pkg/tracker/github/github.go [State: To Verify, Priority: Urgent]
- [ ] **DEN-704**: - Coverage Gap: Reach 100% coverage in pkg/vcs/orchestrator.go & remote_sync.go [State: In Progress, Priority: Urgent]
- [ ] **DEN-705**: - Coverage Gap: Reach 100% coverage in pkg/audit/auditor.go [State: To Verify, Priority: Urgent]
- [ ] **DEN-811**: Modular CLI Command Registry & Dispatcher Architecture [State: To Verify, Priority: High]

---

## Other Plane Projects

### DCORE

- [ ] **DCORE-1**: dendrite-core-rs: tree-sitter fold / last-node / complexity / blake3 / tokenize sidecar [State: Backlog, Priority: Medium]

### DEND

- [ ] **DEND-101**: [Anti-Mock Guard]: Tree-sitter AST mock detection & pre-commit hook [State: To Verify, Priority: Urgent]
- [ ] **DEND-102**: [Verification Routine]: Plane To Verify intermediate state & dendrite verify skill [State: In Progress, Priority: High]
- [ ] **DEND-103**: [Toolchain Migration]: Standardize build & test tooling on uv and bun [State: To Verify, Priority: Medium]
- [ ] **DEND-201**: [Code Knowledge MCP]: Local AST query MCP server [State: To Verify, Priority: High]
- [ ] **DEND-202**: [Graphify Community Graph]: Leiden clustering & god-node dependency index [State: To Verify, Priority: High]
- [ ] **DEND-203**: [Local-First RAG & Shared Library Deduplication]: Zero-cost nomic embeddings & CAS library store [State: In Progress, Priority: Urgent]
- [ ] **DEND-301**: [JetBrains Validation]: Audit live IDE socket bridge & state detection without mocks [State: In Progress, Priority: High]
- [ ] **DEND-302**: [Plugin - JetBrains]: JetBrains Plugin (dendrite-jetbrains) with local AI chat & Graphify markers [State: To Do, Priority: Urgent]
- [ ] **DEND-303**: [Plugin - VS Code]: VS Code Extension (dendrite-vscode) built with bun & MCP context [State: To Do, Priority: High]
- [ ] **DEND-304**: [Plugin - Neovim]: Headless Lua RPC client (dendrite.nvim) [State: To Verify, Priority: Medium]
- [ ] **DEND-305**: [Plugin - Zed]: Rust WASM extension (dendrite-zed) [State: To Do, Priority: Medium]
- [x] **DEND-401**: [Infra - Migration]: Zero-credential SSH service migration (move <svc> to --ssh) [State: Done, Priority: Urgent]
- [ ] **DEND-402**: [Infra - Start]: Mock-free service lifecycle manager with Penpot compose/MCP provisioning [State: To Verify, Priority: High]
- [ ] **DEND-403**: [Infra - Sync]: Cross-node endpoint & schema update propagation (dendrite cluster sync) [State: To Verify, Priority: High]
- [ ] **DEND-501**: [REA]: macOS PE .EXE analysis, pause/resume checkpointing, multi-tier LLM triage & purge [State: In Progress, Priority: High]
- [ ] **DEND-601**: [Tickets Skill]: Ticket Connection & Priority Updater (dendrite skill ticket-relate) [State: To Verify, Priority: Urgent]

### DGIT

- [ ] **DGIT-1**: dendrite-git-rs: gix hunk graph + semantic clusterer sidecar [State: Backlog, Priority: Medium]

### DMCP

- [ ] **DMCP-1**: dendrite-mcp-hub: extract pkg/mcpserver into a standalone Go module [State: Backlog, Priority: Medium]

### DRAG

- [ ] **DRAG-1**: dendrite-rag-rs: BM25+vector hybrid retrieval and memory sync engine [State: Backlog, Priority: Medium]

