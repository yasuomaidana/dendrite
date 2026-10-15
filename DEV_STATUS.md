# Dendrite Project Development Status & Roadmap

This document maintains the real-time development status, milestones, and active ticket execution pipeline for
**Dendrite**, managed following the autonomous ticket lifecycle protocol.

---

## Current Status Overview

- **Active Milestone**: Milestone 24 — Autonomous Pool DAG & Multi-IDE Project Synchronization (FEAT-029)
- **Current Focus**: [DEN-051] Autonomous Dependency Inference & Multi-Directory DAG Solver
- **Last Updated**: 2026-10-14
- **Overall Progress**: 50 / 90 Tickets Completed (55.6%)

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
|      **6**      | **DEN-036** | Milestone 17 | FEAT-021   |     8 SP     |     7.5 / 10     | `pkg/perf`, `cmd/dendrite`, `tests/perf`                     | **COMPLETED [x]** |
|      **8**      | **DEN-037** | Milestone 18 | FEAT-022   |    13 SP     |     8.5 / 10     | `pkg/agentsearch`, `cmd/dendrite`                            |    **COMPLETED [x]**     |
|      **9**      | **DEN-038** | Milestone 19 | FEAT-023   |    13 SP     |     9.0 / 10     | `pkg/testgen`, `pkg/uitest`, `cmd/dendrite`                  |    **COMPLETED [x]**     |
|      **7**      | **DEN-039** | Milestone 20 | FEAT-024   |     8 SP     |     8.0 / 10     | `pkg/rea`, `pkg/ai/import`, `tests/showcase`, `cmd/dendrite` |    **COMPLETED [x]**     |
|     **10**      | **DEN-051** | Milestone 24 | FEAT-029   |     5 SP     |     4.0 / 10     | `pkg/pool/dag`, `cmd/dendrite`                               |       TODO [ ]        |
|     **11**      | **DEN-052** | Milestone 24 | FEAT-029   |     8 SP     |     5.5 / 10     | `pkg/pool/switcher`, `pkg/jetbrains`                         |       TODO [ ]        |
|     **12**      | **DEN-053** | Milestone 24 | FEAT-029   |     5 SP     |     4.5 / 10     | `pkg/pool/split`, `pkg/ui/modal`                             |       TODO [ ]        |
|     **13**      | **DEN-054** | Milestone 25 | FEAT-030   |     8 SP     |     6.5 / 10     | `pkg/query/slice`, `cmd/dendrite`                            |       TODO [ ]        |
|     **14**      | **DEN-055** | Milestone 25 | FEAT-030   |     8 SP     |     7.0 / 10     | `pkg/query/swap`, `pkg/rea`                                  |       TODO [ ]        |
|     **15**      | **DEN-056** | Milestone 25 | FEAT-030   |    13 SP     |     8.5 / 10     | `pkg/query/transpile`, `tests/differential`                  |       TODO [ ]        |
|     **16**      | **DEN-057** | Milestone 25 | FEAT-030   |     8 SP     |     7.5 / 10     | `pkg/query/fuse`, `pkg/pool`                                 |       TODO [ ]        |
|     **17**      | **DEN-058** | Milestone 25 | FEAT-030   |     5 SP     |     5.0 / 10     | `pkg/query/reuse`, `pkg/store`                               |       TODO [ ]        |
|     **18**      | **DEN-059** | Milestone 26 | FEAT-031   |     8 SP     |     7.0 / 10     | `pkg/store/segmented`, `cmd/dendrite`                        |       TODO [ ]        |
|     **19**      | **DEN-060** | Milestone 26 | FEAT-031   |     5 SP     |     4.5 / 10     | `pkg/store/status`, `pkg/ui/store`                           |       TODO [ ]        |
|     **20**      | **DEN-061** | Milestone 26 | FEAT-031   |     8 SP     |     7.0 / 10     | `pkg/scheduler/token`, `pkg/gateway`                         |       TODO [ ]        |
|     **21**      | **DEN-062** | Milestone 26 | FEAT-031   |     5 SP     |     4.5 / 10     | `pkg/scheduler/governor`, `cmd/dendrite`                     |       TODO [ ]        |
|     **22**      | **DEN-063** | Milestone 27 | FEAT-032   |     8 SP     |     6.5 / 10     | `pkg/polyglot/contract`, `pkg/jetbrains`                     |       TODO [ ]        |
|     **23**      | **DEN-064** | Milestone 27 | FEAT-032   |     8 SP     |     7.0 / 10     | `pkg/story/fork`, `pkg/perf`                                 |       TODO [ ]        |
|     **24**      | **DEN-065** | Milestone 27 | FEAT-032   |     8 SP     |     6.5 / 10     | `pkg/story/drift`, `pkg/query`                               |       TODO [ ]        |
|     **25**      | **DEN-066** | Milestone 27 | FEAT-032   |     5 SP     |     4.0 / 10     | `pkg/story/resume`, `cmd/dendrite`                           |       TODO [ ]        |
|     **26**      | **DEN-067** | Milestone 28 | FEAT-033   |    13 SP     |     8.5 / 10     | `pkg/node/migrate`, `cmd/dendrite`                           |       TODO [ ]        |
|     **27**      | **DEN-068** | Milestone 28 | FEAT-033   |     8 SP     |     7.5 / 10     | `pkg/node/wal`, `pkg/store`                                  |       TODO [ ]        |
|     **28**      | **DEN-069** | Milestone 28 | FEAT-033   |     8 SP     |     7.0 / 10     | `pkg/backup/engine`, `cmd/dendrite`                          |       TODO [ ]        |
|     **29**      | **DEN-070** | Milestone 29 | FEAT-034   |     5 SP     |     4.5 / 10     | `pkg/config/engine`, `cmd/dendrite`                          |       TODO [ ]        |
|     **30**      | **DEN-071** | Milestone 29 | FEAT-034   |     8 SP     |     6.0 / 10     | `.github/workflows`, `.goreleaser.yaml`                      |       TODO [ ]        |
|     **31**      | **DEN-072** | Milestone 30 | FEAT-035   |     5 SP     |     4.5 / 10     | `pkg/agent/runtime`, `cmd/dendrite`                          |       TODO [ ]        |
|     **32**      | **DEN-073** | Milestone 30 | FEAT-035   |     8 SP     |     6.5 / 10     | `pkg/agent/gitgov`, `pkg/pool`                               |       TODO [ ]        |
|     **33**      | **DEN-074** | Milestone 31 | FEAT-036   |     8 SP     |     6.0 / 10     | `pkg/stories/dag`, `pkg/tracker`                             |       TODO [ ]        |
|     **34**      | **DEN-075** | Milestone 31 | FEAT-036   |     5 SP     |     4.5 / 10     | `pkg/stories/next`, `pkg/jetbrains`                          |       TODO [ ]        |
|     **35**      | **DEN-076** | Milestone 32 | FEAT-037   |     8 SP     |     6.0 / 10     | `pkg/ui/penpot`, `pkg/mcpserver`                             |       TODO [ ]        |
|     **36**      | **DEN-077** | Milestone 32 | FEAT-037   |     5 SP     |     4.5 / 10     | `pkg/tracker/plane`, `pkg/ui/penpot`                         |       TODO [ ]        |
|     **37**      | **DEN-078** | Milestone 33 | FEAT-038   |     5 SP     |     4.0 / 10     | `pkg/update/lifecycle`, `cmd/dendrite`                       |       TODO [ ]        |
|     **38**      | **DEN-079** | Milestone 33 | FEAT-038   |     5 SP     |     4.0 / 10     | `pkg/update/devmode`, `docs/DEVELOPMENT.md`                  |       TODO [ ]        |
|     **39**      | **DEN-080** | Milestone 34 | FEAT-039   |     5 SP     |     4.5 / 10     | `pkg/env/bootstrap`, `cmd/dendrite`                          |       TODO [ ]        |
|     **40**      | **DEN-081** | Milestone 34 | FEAT-039   |     8 SP     |     5.5 / 10     | `pkg/env/patterns`, `.agents/skills`                         |       TODO [ ]        |
|     **41**      | **DEN-082** | Milestone 34 | FEAT-039   |     5 SP     |     5.0 / 10     | `pkg/env/updater`, `cmd/dendrite`                            |       TODO [ ]        |
|     **42**      | **DEN-083** | Milestone 35 | FEAT-040   |     8 SP     |     7.0 / 10     | `pkg/cache/turso`, `pkg/store`                               |       TODO [ ]        |
|     **43**      | **DEN-084** | Milestone 35 | FEAT-040   |     8 SP     |     6.5 / 10     | `pkg/cache/outbox`, `pkg/tracker`                            |       TODO [ ]        |
|     **44**      | **DEN-085** | Milestone 35 | FEAT-040   |     3 SP     |     2.5 / 10     | `pkg/cache/hygiene`, `pkg/env`                               |       TODO [ ]        |
|     **45**      | **DEN-086** | Milestone 36 | FEAT-041   |     5 SP     |     4.5 / 10     | `pkg/security/secretgate`, `pkg/agent`                       |       TODO [ ]        |
|     **46**      | **DEN-087** | Milestone 36 | FEAT-041   |     5 SP     |     4.5 / 10     | `pkg/quality/harness`, `pkg/agent`                           |       TODO [ ]        |
|     **47**      | **DEN-088** | Milestone 37 | FEAT-041   |     8 SP     |     6.5 / 10     | `pkg/scheduler/failover`, `pkg/gateway`                      |       TODO [ ]        |
|     **48**      | **DEN-089** | Milestone 37 | FEAT-041   |     8 SP     |     6.5 / 10     | `pkg/agent/worktree`, `pkg/pool`                             |       TODO [ ]        |
|     **49**      | **DEN-090** | Milestone 37 | FEAT-041   |     5 SP     |     4.0 / 10     | `pkg/audit/logger`, `cmd/dendrite`                           |       TODO [ ]        |

---

## Milestone Execution Pipeline

### Milestone 23: Hardening, Benchmark Scenarios, Cleanroom Invariants & Local Agent Triage (FEAT-020, FEAT-021, FEAT-024, FEAT-028)

- [x] **DEN-047**: Fix Goroutine Deadlock & Dynamic Buffer Starvation in Synthetic k6 Benchmark Runner (`pkg/perf`, `cmd/dendrite`)
- [x] **DEN-048**: Automatic Zero-Latency Fallback Cascade for Unreachable Inference Upstreams in Benchmark Scenarios (`pkg/perf`, `pkg/gateway`, `pkg/inference`)
- [x] **DEN-049**: Multi-Digest Test Vector Errata & 63-Byte Alignment Invariant Suite for REA Showcases (`pkg/rea/showcase`, `tests/showcase`)
- [x] **DEN-050**: Local Agent Triage Gateway, Token-Efficient Prompt Router & JetBrains AI Chat Multi-MCP Orchestrator (`pkg/agent/triage`, `pkg/jetbrains`, `pkg/mcpserver`, `pkg/gateway`)

### Milestone 24: Autonomous Pool DAG & Multi-IDE Project Synchronization (FEAT-029)

- [ ] **DEN-051**: Autonomous Dependency Inference & Multi-Directory DAG Solver (`pkg/pool/dag`, `cmd/dendrite`)
- [ ] **DEN-052**: Multi-Mode Dependency Switcher: Published, Branch, Local Lockstep & Remote Fork (`pkg/pool/switcher`, `pkg/jetbrains`)
- [ ] **DEN-053**: Mid-Flight Project Splitter & Mandatory Interactive Target Directory Gate (`pkg/pool/split`, `pkg/ui/modal`)

### Milestone 25: Universal Query Engine & 3-Tier Library Reuse Auditor (FEAT-030)

- [ ] **DEN-054**: Universal Intent Slicing & Binary/Repo Debloating Engine (`pkg/query/slice`, `cmd/dendrite`)
- [ ] **DEN-055**: Subsystem Trait Boundary Adapter & Graphics Engine Swapper (`pkg/query/swap`, `pkg/rea`)
- [ ] **DEN-056**: Idiomatic Binary/Repo-to-Rust Transpiler & Differential Test Harness (`pkg/query/transpile`, `tests/differential`)
- [ ] **DEN-057**: Multi-Artifact Architecture Fusion & In-Process Channel Synthesizer (`pkg/query/fuse`, `pkg/pool`)
- [ ] **DEN-058**: Strict 3-Tier Library Reuse Hierarchy Lookup & ADR Provenance Recorder (`pkg/query/reuse`, `pkg/store`)

### Milestone 26: Persistent Segmented Store & Token-Aware Hybrid Scheduler (FEAT-031)

- [ ] **DEN-059**: BLAKE3 Content-Addressed Persistent Segmented Store & Chunk Checkpointer (`pkg/store/segmented`, `cmd/dendrite`)
- [ ] **DEN-060**: Store Inspection, Chunk Status Telemetry & Explicit User Purge Lifecycler (`pkg/store/status`, `pkg/ui/store`)
- [ ] **DEN-061**: Token-Aware Hybrid Scheduler: Night Local Window & 48h Pre-Reset Cloud Sprint (`pkg/scheduler/token`, `pkg/gateway`)
- [ ] **DEN-062**: Quota Floor Safety Guard (20%/0% Drain Override) & Continuous Analysis Pause Engine (`pkg/scheduler/governor`, `cmd/dendrite`)

### Milestone 27: Advanced Polyglot Contract Sync, Speculative Branching & Upstream Drift (FEAT-032)

- [ ] **DEN-063**: Quad-IDE Cross-Language Schema & IPC Contract Drift Monitor (`pkg/polyglot/contract`, `pkg/jetbrains`)
- [ ] **DEN-064**: Speculative Git Worktree Branching & Differential Architecture Benchmark Harness (`pkg/story/fork`, `pkg/perf`)
- [ ] **DEN-065**: Sliced Upstream Commit Drift Analyzer & Bug Fix Porting Generator (`pkg/story/drift`, `pkg/query`)
- [ ] **DEN-066**: Multi-Repo Context Re-Hydration & IDE Scope Restorer (`pkg/story/resume`, `cmd/dendrite`)

### Milestone 28: Zero-Data-Loss Infrastructure Mobility & Smart Repo-Aware Backup Engine (FEAT-033)

- [ ] **DEN-067**: 5-Phase Atomic Service Migration Engine: Local <-> Debian <-> Azure (`pkg/node/migrate`, `cmd/dendrite`)
- [ ] **DEN-068**: Write-Ahead Log (WAL) State Journaling & Merkle Digest Verification Protocol (`pkg/node/wal`, `pkg/store`)
- [ ] **DEN-069**: Smart Repo-Aware Backup Engine with Git-Persisted Blob Eviction Priority (`pkg/backup/engine`, `cmd/dendrite`)

### Milestone 29: Dual-Mode Configuration Parity & Multi-Channel Go CI/CD Automation (FEAT-034)

- [ ] **DEN-070**: Unified Configuration Subsystem with nvim/vim Transactional Staging & Taplo Schema (`pkg/config/engine`, `cmd/dendrite`)
- [ ] **DEN-071**: Multi-Channel CI/CD Distribution Pipeline: Brew Tap, Debian .deb, GHCR & GoReleaser (`.github/workflows`, `.goreleaser.yaml`)

### Milestone 13: Remote SSH Hardware Discovery & Automated Service Placement Router (FEAT-017)

- [x] **DEN-032**: Remote SSH Node Topology Provisioner & Dynamic Service Placement Router (`pkg/topology/advisor`, `cmd/dendrite`)

### Milestone 15: Azure Cloud-Burst Infrastructure & Managed Storage (FEAT-019)

- [x] **DEN-034**: Terraform Azure Spot VM Module & Persistent Managed Disk Lifecycler (`infra/modules/azure-burst`, `pkg/orchestrator/azure`)

### Milestone 21: Model Context Protocol (MCP) Server & Tool Registry Bridge (FEAT-025)

- [x] **DEN-040**: Dendrite Native MCP Server & Tool Registry Bridge (`pkg/mcpserver`, `cmd/dendrite`)

### Milestone 22: Distributed Codebase Ecosystem, Modular REA & Cross-Project Interoperability Coordinator (FEAT-026)

- [x] **DEN-041**: Distributed Codebase Multi-Source Ecosystem & Priority Resolver (pkg/distcodebase, cmd/dendrite)
- [x] **DEN-042**: Scenario 1: REA Modular Decomposition & Ecosystem Reuse Discovery Engine (pkg/distcodebase, pkg/rea, cmd/dendrite)
- [x] **DEN-043**: Scenario 2: Parallel Multi-Project Workspace & Repository Spawner (pkg/distcodebase, pkg/workspace, cmd/dendrite)
- [x] **DEN-044**: Scenario 3: Local LLM Cross-Project Interoperability Coordinator (pkg/distcodebase, pkg/ollama, cmd/dendrite)
- [x] **DEN-045**: Scenario 4: Generative Satellite Project Synthesis & Autonomous Plane Project Provisioning (pkg/distcodebase, pkg/tracker/plane, cmd/dendrite)
- [x] **DEN-046**: JetBrains IDE Toolset Integration, 19 MCP Tools & Agentic Developer Workflow Guide (pkg/jetbrains, pkg/mcpserver, cmd/dendrite)

### Milestone 16: Pluggable Multi-Provider Inference Engines (FEAT-020)

- [x] **DEN-035**: Unified Inference Engine Gateway: OxiLLaMa, Hugging Face TGI/TEI, vLLM, llama.cpp & Ollama Metal (`pkg/inference`, `pkg/gateway`)

### Milestone 17: Performance Testing, Inference Benchmarking & Gateway Load Harness (FEAT-021)

- [x] **DEN-036**: k6 Performance Testing, TTFT/TPS Benchmarking & Cascade Failover Load Harness (`pkg/perf`, `cmd/dendrite`, `tests/perf`)

### Milestone 20: Open-Source REA Showcase Pipeline & Cleanroom Parity Verification Suite (FEAT-024)

- [x] **DEN-039**: Open-Source Reverse Engineering Showcases & Cleanroom Parity Verification Suite (`pkg/rea`, `pkg/ai/import`, `tests/showcase`, `cmd/dendrite`)

### Milestone 18: Agent Search, Capability Discovery & Semantic Routing Engine (FEAT-022)

- [x] **DEN-037**: Hybrid Agent Search, TEI Embeddings & Semantic Routing Engine (`pkg/agentsearch`, `cmd/dendrite`)

### Milestone 19: Developer Test Automation, Self-Healing Harness & UI Integration Suite (FEAT-023)

- [x] **DEN-038**: Automated Test Synthesizer, Self-Healing Convergence Loop & Web UI Test Harness (`pkg/testgen`, `pkg/uitest`, `cmd/dendrite`)

### Milestone 11: Pluggable Mesh Networking & Zero-Trust Connectivity (FEAT-015)

- [x] **DEN-030**: Multi-Mesh Networking Abstraction: Cloudflare Tunnels, SSH Port Forwarding, WireGuard, and Direct LAN (`pkg/topology/network`, `pkg/orchestrator`)

### Milestone 14: Hybrid Telemetry Architecture & Prometheus/Grafana Observability Suite (FEAT-018)

- [x] **DEN-033**: Prometheus `/metrics` Exporter & Grafana Observability Dashboard Integration Pack (`pkg/telemetry/exporter`, `stacks/compose.grafana.yaml`)

### Milestone 12: Open-Source Project Management & Design Tooling (FEAT-016)

- [x] **DEN-031**: Self-Hosted Plane v2 & Penpot Design Token Stacks & Replicator (`pkg/env`, `stacks/compose.plane.yaml`, `stacks/compose.penpot.yaml`)

### Milestone 10: IDE Intelligence & JetBrains MCP Bridge (FEAT-014)

- [x] **DEN-028**: JetBrains IDE MCP Integration & Communication Bridge

### Milestone 9: Web UI & Dashboard Engine (FEAT-009)

- [x] **DEN-014**: Embedded HTTP Control Server & WebSocket Telemetry
- [x] **DEN-015**: Real-Time Telemetry Gauges & Thermal Monitoring Daemon

### Milestone 8: Multi-Project Workspaces & Pluggable Trackers (FEAT-005, FEAT-008)

- [x] **DEN-012**: Multi-Project Workspace Registry & Context Switcher
- [x] **DEN-013**: Pluggable Issue Tracker Interface & Atlassian Jira Adapter
- [x] **DEN-027**: Plane v2 & Linear Strategy Adapters

### Milestone 7: Polyglot Translation & Documentation Reflection (FEAT-011, FEAT-012)

- [x] **DEN-020**: Intermediate Semantic Representation ISR Tree-sitter Parser
- [x] **DEN-021**: Idiomatic Target Code Synthesizer & Dual Property Test Parity Generator
- [x] **DEN-022**: AST Docs-as-Code Synthesizer for FEAT/ADR Specs & OpenAPI 3.1
- [x] **DEN-023**: Cross-Project Documentation Reflection Engine & Docusaurus Sync

### Milestone 6: AI-Assisted Token Optimization & Cleanroom Import (FEAT-010)

- [x] **DEN-018**: AST Context Compaction & LiteLLM Prefix Cache Harness
- [x] **DEN-019**: Cross-Repository Capability Dissection & Cleanroom Synthesizer

### Milestone 5: Reverse Engineering & Sandboxed Intelligence (FEAT-003, FEAT-004, FEAT-006)

- [x] **DEN-009**: REA MCP Bridge & Disassembler Protocol Client
- [x] **DEN-010**: Tree-sitter AST & Complexity Analyzer
- [x] **DEN-011**: Ephemeral Hardened Container Sandbox with RAII Drop

### Milestone 4: Autonomous Coding Agent Harness & Tool Orchestration (FEAT-013)

- [x] **DEN-024**: Agent Runner Harness & Environment Injector for Aider, OpenHands, and OpenCode
- [x] **DEN-025**: Agent Sandbox Isolation & LiteLLM Gateway Budget Binding
- [x] **DEN-026**: Agent Session Tracker, Git Commit Provenance & Issue Tracker Sync

### Milestone 3: Token Budget Governance & Fallback Gateway (FEAT-002)

- [x] **DEN-007**: LiteLLM Configuration Template & Dynamic Renderer
- [x] **DEN-008**: Sliding Window Budget Limiter & Ollama Failover Engine

### Milestone 2: Multi-Node Topology & Cloud-Burst Infrastructure (FEAT-001, FEAT-007)

- [x] **DEN-005**: Topology Configuration Model & Scenario Placement Types
- [x] **DEN-006**: Terraform Cloud-Init & Compose Profile Runner
- [x] **DEN-016**: Terraform AWS Spot EC2 Module & Cloud-Init Bootstrap
- [x] **DEN-017**: Persistent EBS Volume Lifecycler & Zero-Ingress Security Group Validator
- [x] **DEN-029**: Development Environment Replication, Host Installer & Containerized Tooling (`pkg/env`, `stacks`)

### Milestone 1: Core Foundation, Developer Tools & Ollama Engine

- [x] **DEN-001**: Initialize Development Status Ledger & Execution Roadmap
- [x] **DEN-002**: Implement Idiomatic Ollama Client & Metal Engine Support
- [x] **DEN-003**: Developer Productivity Tools on the Fly & CLI Scaffold (`pkg/devtool`, `cmd/dendrite`)
- [x] **DEN-004**: Ticket Module & Local Status Synchronization Engine

---

## Detailed Specifications for Active & Queued Tickets

### Ticket `DEN-036`: Performance Testing, Inference Benchmarking & Gateway Load Harness (k6)

- **Target Package**: `pkg/perf`, `cmd/dendrite`, `tests/perf`
- **Governing Specs**: [`docs/features/FEAT-021-performance-testing-k6-benchmark.md`](docs/features/FEAT-021-performance-testing-k6-benchmark.md)
- **Goal**: Integrate k6 runner wrapper and scenario definitions to benchmark Time-To-First-Token (TTFT), Tokens-Per-Second (TPS), concurrent agent load, Apple Silicon thermal correlation, and LiteLLM cascade failovers under injected faults.
- **Acceptance Criteria**:
    - [ ] Implement `k6` benchmark scenario runner with TTFT/TPS metrics parsing.
    - [ ] Fault injection harness simulating HTTP 429 and HTTP 503 failover triggers.
    - [ ] CLI subcommand `dendrite bench <inference|gateway|matrix>`.
    - [ ] Statement coverage $\ge 85\%$.

### Ticket `DEN-039`: Open-Source Reverse Engineering Showcases & Cleanroom Parity Verification Suite

- **Target Package**: `pkg/rea`, `pkg/ai/import`, `tests/showcase`, `cmd/dendrite`
- **Governing Specs**: [`docs/features/FEAT-024-open-source-showcases.md`](docs/features/FEAT-024-open-source-showcases.md)
- **Goal**: Implement automated end-to-end showcase runners and cleanroom verification suites using 5 open-source target archetypes.
- **Acceptance Criteria**:
    - [ ] CLI subcommand `dendrite rea showcase <aegis|dxball|ctf|th04|mattermost|all>` executing automated analysis and verification loops.
    - [ ] Bounded evidence bundling conforming to $< 4,000$ tokens per showcase probe.
    - [ ] Cleanroom synthesis pipeline producing test-verified Go/Rust implementations and ADR documents with YAML frontmatter.
    - [ ] Table-driven integration and property test suite achieving $\ge 85\%$ statement coverage.

### Ticket `DEN-041`: Distributed Codebase Multi-Source Ecosystem & Priority Resolver

- **Target Package**: `pkg/distcodebase`, `cmd/dendrite`
- **Governing Specs**: [`docs/features/FEAT-026-distributed-codebase-ecosystem.md`](docs/features/FEAT-026-distributed-codebase-ecosystem.md)
- **Goal**: Implement multi-source registry prioritizing user/organization-owned repositories (P1), curated high-performance OSS engines (P2: wgpu, turso, diesel, diesel-cte-ext), and public registries (P3), forced modular decomposition, local LLM interoperability coordinator, and parallel/satellite project spawner with Plane v2 integration.
- **Acceptance Criteria**:
    - [x] Multi-source priority resolver prioritizing owned repos (P1) > curated OSS (P2) > public registries (P3).
    - [x] Pre-configured libraries: `wgpu`, `yasuomaidana` repositories, `turso`, `diesel`, `diesel-cte-ext`.
    - [x] Forced modular decomposition engine for REA evidence bundles and legacy systems.
    - [x] Cross-project interoperability coordinator with `TODO(interop:...)` fallbacks and automatic reconciliation.
    - [x] Parallel repository and Plane v2 satellite project spawner.
    - [x] Native MCP tools (`dendrite_dist_sources`, `dendrite_dist_decompose`, `dendrite_dist_interop`, `dendrite_dist_spawn`) and CLI subcommands.
    - [x] Statement test coverage $\ge 85\%$ (achieved 89.2%).


### Ticket `DEN-051`: Autonomous Dependency Inference & Multi-Directory DAG Solver

- **Target Package**: `pkg/pool/dag`, `cmd/dendrite`
- **Governing Specs**: [`docs/features/FEAT-029-autonomous-pool-dag-synchronization.md`](docs/features/FEAT-029-autonomous-pool-dag-synchronization.md)
- **Goal**: Implement multi-directory dependency scanner that automatically inspects package manifests (`go.mod`, `Cargo.toml`, `package.json`, `pyproject.toml`), exported AST symbol tables, and IPC boundaries to compute topological dependency edges (`A depends_on B`) across local projects without manual wiring.
- **Acceptance Criteria**:
    - [ ] `dendrite pool add <dir1> <dir2> ...` registers multiple project directories in `.dendrite/pool.json`.
    - [ ] `dendrite pool resolve` computes a deterministic DAG, detecting circular dependency hazards with error variants.
    - [ ] Table-driven tests validating multi-ecosystem manifest parsing (Go, Rust, TypeScript, Python) with statement coverage >= 85%.
    - [ ] Zero unwrap/panic policy with typed error hierarchy (`ErrCyclicDependency`, `ErrManifestMalformed`).
    - [ ] Cyclomatic complexity <= 15, Cognitive complexity <= 20 per function.

### Ticket `DEN-052`: Multi-Mode Dependency Switcher: Published, Branch, Local Lockstep & Remote Fork

- **Target Package**: `pkg/pool/switcher`, `pkg/jetbrains`
- **Governing Specs**: [`docs/features/FEAT-029-autonomous-pool-dag-synchronization.md`](docs/features/FEAT-029-autonomous-pool-dag-synchronization.md)
- **Goal**: Implement atomic dependency mode switcher allowing developers to transition any pool dependency across Published (`--mode published --version <semver>`), Remote Git Branch (`--mode branch --branch <name>`), Live Local (`--mode local`), or Custom Fork (`--mode fork --fork-url <url>`) while keeping active JetBrains IDE scopes synchronized.
- **Acceptance Criteria**:
    - [ ] Mode `published` strips local overrides and pins exact registry version in manifests and lockfiles.
    - [ ] Mode `branch` rewrites dependency to track experimental remote branch with lockfile commit SHA pin.
    - [ ] Mode `local` injects `go.work use` (GoLand), `[patch.crates-io]` (RustRover), `link:` (WebStorm), or `tool.uv.sources` (PyCharm).
    - [ ] Mode `fork` pushes local branch changes to fork remote and rewires manifests to the fork URL.
    - [ ] Typestate pattern used to guard mode transition validity.
    - [ ] All manifest operations are transactional (reverts on syntax or lockfile resolution failure).
    - [ ] Statement test coverage >= 85%.

### Ticket `DEN-053`: Mid-Flight Project Splitter & Mandatory Interactive Target Directory Gate

- **Target Package**: `pkg/pool/split`, `pkg/ui/modal`
- **Governing Specs**: [`docs/features/FEAT-029-autonomous-pool-dag-synchronization.md`](docs/features/FEAT-029-autonomous-pool-dag-synchronization.md)
- **Goal**: Implement module extractor and pool member generator that moves an AST slice and its unit tests into a new project, enforcing a mandatory interactive destination directory prompt in both CLI and Web UI before performing any disk modifications.
- **Acceptance Criteria**:
    - [ ] CLI command `dendrite pool split --from <dir> --module <pkg> --name <member>` halts and prompts user for target directory: `? Specify the target directory for <member> (required):`.
    - [ ] Web UI modal displays inferred DAG edge (`parent -> new_member`) and requires explicit directory picker selection before proceeding.
    - [ ] Extracts target AST slice, moves corresponding unit tests, scaffolds new ecosystem manifest, and wires local mode in pool.
    - [ ] Mandatory integration test verifying atomic rollback if destination directory cannot be created or user cancels prompt.
    - [ ] Statement test coverage >= 85%.

### Ticket `DEN-054`: Universal Intent Slicing & Binary/Repo Debloating Engine

- **Target Package**: `pkg/query/slice`, `cmd/dendrite`
- **Governing Specs**: [`docs/features/FEAT-030-universal-query-engine-3tier-library-reuse.md`](docs/features/FEAT-030-universal-query-engine-3tier-library-reuse.md)
- **Goal**: Implement universal query slicing engine supporting `--entrypoint <symbol>` forward/backward reachability slicing on compiled binaries (CFG) and source repositories (AST), pruning unneeded features (GUI, telemetry, updaters) into a lean standalone project.
- **Acceptance Criteria**:
    - [ ] `dendrite query --sources <bin|repo> --entrypoint <symbol> "<intent>"` extracts reachable call graphs and data flow boundaries.
    - [ ] Strips dead code and trims unrelated manifest dependencies (reducing lines and dependencies by >= 80% on bloated targets).
    - [ ] Generates golden I/O test vectors asserting input/output parity with the original binary or module.
    - [ ] Cyclomatic complexity <= 15, Cognitive complexity <= 20.
    - [ ] Statement test coverage >= 85%.

### Ticket `DEN-055`: Subsystem Trait Boundary Adapter & Graphics Engine Swapper

- **Target Package**: `pkg/query/swap`, `pkg/rea`
- **Governing Specs**: [`docs/features/FEAT-030-universal-query-engine-3tier-library-reuse.md`](docs/features/FEAT-030-universal-query-engine-3tier-library-reuse.md)
- **Goal**: Implement subsystem swap transformer capable of preserving domain kinematics and state machines while swapping core rendering or I/O engines (e.g., replacing legacy OpenGL/DirectX FFI calls with `wgpu` in Rust or Three.js/WebGPU in WebStorm).
- **Acceptance Criteria**:
    - [ ] Isolates boundary between application domain simulation updates and legacy graphics FFI calls.
    - [ ] Generates idiomatic `RenderBackend` trait / interface boundary separating domain state from renderer.
    - [ ] Synthesizes adapter crate wiring `wgpu` or Three.js without modifying simulation step logic.
    - [ ] Verified via property tests comparing scene graph node matrices before and after swap.
    - [ ] Statement test coverage >= 85%.

### Ticket `DEN-056`: Idiomatic Binary/Repo-to-Rust Transpiler & Differential Test Harness

- **Target Package**: `pkg/query/transpile`, `tests/differential`
- **Governing Specs**: [`docs/features/FEAT-030-universal-query-engine-3tier-library-reuse.md`](docs/features/FEAT-030-universal-query-engine-3tier-library-reuse.md)
- **Goal**: Implement safe Rust transpilation generator that translates compiled C/Go/Python binaries or repositories into safe, idiomatic Rust, replacing decompiled boilerplate with registered source references and ecosystem crates.
- **Acceptance Criteria**:
    - [ ] `dendrite query --sources <artifact> --to rust --audit-library-reuse "<prompt>"` generates pure-Rust crates (`no_std` compatible where specified).
    - [ ] Enforces Zero-Cost Abstractions, borrows (`&str`, `&[T]`) over unnecessary clones, and Newtype pattern for domain IDs.
    - [ ] Zero `unsafe` blocks emitted unless explicitly gated behind audited FFI modules.
    - [ ] Differential test harness comparing outputs of original artifact against synthesized Rust crate with epsilon <= 1e-6.
    - [ ] Statement test coverage >= 85%.

### Ticket `DEN-057`: Multi-Artifact Architecture Fusion & In-Process Channel Synthesizer

- **Target Package**: `pkg/query/fuse`, `pkg/pool`
- **Governing Specs**: [`docs/features/FEAT-030-universal-query-engine-3tier-library-reuse.md`](docs/features/FEAT-030-universal-query-engine-3tier-library-reuse.md)
- **Goal**: Implement binary and repository fusion engine that merges two distinct projects into a unified workspace, deduplicating shared static libraries and replacing inter-process socket/pipe serialization with zero-copy in-process channels.
- **Acceptance Criteria**:
    - [ ] `dendrite query --sources "bin_a,bin_b" --to rust --audit-library-reuse "Merge..."` identifies overlapping modules (e.g. protobuf, linear algebra).
    - [ ] Deduplicates overlapping dependencies against Source References and Pool crates.
    - [ ] Synthesizes zero-copy in-process ring buffer channel (`crossbeam-channel` or Go buffered channels).
    - [ ] Triggers mandatory target directory prompt before scaffolding merged pool member.
    - [ ] Statement test coverage >= 85%.

### Ticket `DEN-058`: Strict 3-Tier Library Reuse Hierarchy Lookup & ADR Provenance Recorder

- **Target Package**: `pkg/query/reuse`, `pkg/store`
- **Governing Specs**: [`docs/features/FEAT-030-universal-query-engine-3tier-library-reuse.md`](docs/features/FEAT-030-universal-query-engine-3tier-library-reuse.md)
- **Goal**: Implement strict 3-tier library reuse verification engine (`Tier 1: Source References` > `Tier 2: Active Pool Members` > `Tier 3: Ecosystem Registries`) executed whenever `--audit-library-reuse` is triggered, recording exact provenance in story ADRs.
- **Acceptance Criteria**:
    - [ ] Tier 1: Slices or imports implementation directly from `dendrite ref` registered repositories.
    - [ ] Tier 2: Wires dependency edge to existing pool members or proposes extraction.
    - [ ] Tier 3: Maps decompiled patterns to canonical ecosystem packages (crates.io `nalgebra`, `serde`, `diesel`; pkg.go.dev `ent`).
    - [ ] Emits architecture decision record (`docs/adr/ADR-*.md`) with YAML frontmatter capturing reuse provenance.
    - [ ] Statement test coverage >= 85%.

### Ticket `DEN-059`: BLAKE3 Content-Addressed Persistent Segmented Store & Chunk Checkpointer

- **Target Package**: `pkg/store/segmented`, `cmd/dendrite`
- **Governing Specs**: [`docs/features/FEAT-031-persistent-store-hybrid-token-scheduler.md`](docs/features/FEAT-031-persistent-store-hybrid-token-scheduler.md)
- **Goal**: Implement content-addressed segmented store (`~/.dendrite/store/`) indexing files, functions, and binary CFG blocks via BLAKE3 hashes in SQLite with Zstandard compressed graph blobs, committing completed chunks immediately.
- **Acceptance Criteria**:
    - [ ] Incremental chunk checkpointing allows queries to execute on partially analyzed projects (state `ANALYZING_PARTIAL`).
    - [ ] Stored analyses never expire automatically; persist until explicit manual user purge.
    - [ ] Mandatory Persistent Layer Testing (DoD): Validated via `testcontainers-go` SQLite integration tests verifying ACID transactions and crash recovery.
    - [ ] Zero data corruption on simulated process kill mid-chunk write.
    - [ ] Statement test coverage >= 85%.

### Ticket `DEN-060`: Store Inspection, Chunk Status Telemetry & Explicit User Purge Lifecycler

- **Target Package**: `pkg/store/status`, `pkg/ui/store`
- **Governing Specs**: [`docs/features/FEAT-031-persistent-store-hybrid-token-scheduler.md`](docs/features/FEAT-031-persistent-store-hybrid-token-scheduler.md)
- **Goal**: Implement store inspection CLI commands and Web UI views displaying real-time chunk status (`STORED`, `ANALYZING_PARTIAL`, `QUEUED_NIGHT_LOCAL`, `PAUSED_QUOTA_FLOOR`), with chunk-level drill-down and manual deletion routines.
- **Acceptance Criteria**:
    - [ ] `dendrite store status` tabular CLI output showing STORE ID, NAME, KIND, STATE, PROGRESS, TOKENS, and UPDATED.
    - [ ] `dendrite store inspect <id>` details chunk-by-chunk analysis state.
    - [ ] `dendrite store rm <id>` safely removes SQLite records and unlinks blob storage with confirmation gate.
    - [ ] Parity with Web UI Store Inspector tab and delete actions.
    - [ ] Statement test coverage >= 85%.

### Ticket `DEN-061`: Token-Aware Hybrid Scheduler: Night Local Window & 48h Pre-Reset Cloud Sprint

- **Target Package**: `pkg/scheduler/token`, `pkg/gateway`
- **Governing Specs**: [`docs/features/FEAT-031-persistent-store-hybrid-token-scheduler.md`](docs/features/FEAT-031-persistent-store-hybrid-token-scheduler.md)
- **Goal**: Implement token-aware background scheduler that executes AST extraction immediately at 0 tokens, runs semantic analysis via local Ollama/Metal models during scheduled night windows (23:00–06:00), and activates paid cloud Gemini sprint 48 hours prior to weekly quota reset to prevent token waste.
- **Acceptance Criteria**:
    - [ ] `dendrite analyze policy --night-window "23:00-06:00" --weekly-reset "SUN 23:59" --sprint-before-reset 48h` sets scheduler directive.
    - [ ] Outside sprint window: only zero-cost local Ollama models used; cloud token spend = $0.00.
    - [ ] Inside 48h sprint window: accelerates pending chunks using Gemini Pro via LiteLLM gateway.
    - [ ] Unit and mock clock tests verifying transitions at 48h boundary.
    - [ ] Statement test coverage >= 85%.

### Ticket `DEN-062`: Quota Floor Safety Guard (20%/0% Drain Override) & Continuous Analysis Pause Engine

- **Target Package**: `pkg/scheduler/governor`, `cmd/dendrite`
- **Governing Specs**: [`docs/features/FEAT-031-persistent-store-hybrid-token-scheduler.md`](docs/features/FEAT-031-persistent-store-hybrid-token-scheduler.md)
- **Goal**: Implement quota governor halting cloud sprint at 20% remaining weekly quota reserve, supporting a manual override switch to allow draining to 0%, and an `always` mode analyzing continuously until paused.
- **Acceptance Criteria**:
    - [ ] Stops cloud analysis automatically and transitions project to `PAUSED_QUOTA_FLOOR` when remaining weekly quota hits 20%.
    - [ ] `dendrite analyze switch --allow-zero on|off` enables/disables draining past 20% down to 0%.
    - [ ] `dendrite analyze mode always|policy` switches between continuous and windowed policy analysis.
    - [ ] `dendrite analyze pause` and `resume` commands persist current chunk progress immediately.
    - [ ] Statement test coverage >= 85%.

### Ticket `DEN-063`: Quad-IDE Cross-Language Schema & IPC Contract Drift Monitor

- **Target Package**: `pkg/polyglot/contract`, `pkg/jetbrains`
- **Governing Specs**: [`docs/features/FEAT-032-polyglot-speculative-drift-workflows.md`](docs/features/FEAT-032-polyglot-speculative-drift-workflows.md)
- **Goal**: Implement polyglot contract analyzer connecting RustRover, GoLand, WebStorm, and PyCharm projects to detect schema and IPC contract drift across Protobuf, OpenAPI, and JSON-Schema definitions in real time.
- **Acceptance Criteria**:
    - [ ] `dendrite story start "<story>" --sync-contracts` monitors shared schema directories across pool members.
    - [ ] Detects field deletions, type mismatches, and serialization tag discrepancies across Go structs, Rust serde models, TS interfaces, and Pydantic models.
    - [ ] Emits IDE diagnostic markers and triggers notification in active JetBrains IDE.
    - [ ] Statement test coverage >= 85%.

### Ticket `DEN-064`: Speculative Git Worktree Branching & Differential Architecture Benchmark Harness

- **Target Package**: `pkg/story/fork`, `pkg/perf`
- **Governing Specs**: [`docs/features/FEAT-032-polyglot-speculative-drift-workflows.md`](docs/features/FEAT-032-polyglot-speculative-drift-workflows.md)
- **Goal**: Implement speculative story branching engine that creates isolated Git worktrees for competing implementation variants, runs comparative performance benchmarks, and resolves to the winning implementation.
- **Acceptance Criteria**:
    - [ ] `dendrite story fork "<story>" --variants "v1,v2"` creates isolated worktrees under `.dendrite/worktrees/`.
    - [ ] Automated benchmark harness collects TTFT, TPS, memory, and CPU metrics for both variants.
    - [ ] `dendrite story resolve "<story>" --winner "<name>"` merges winning worktree, saves benchmark metrics in `dendrite store`, and purges alternate worktree.
    - [ ] RAII cleanup pattern prevents orphan worktrees or locked git indexes.
    - [ ] Statement test coverage >= 85%.

### Ticket `DEN-065`: Sliced Upstream Commit Drift Analyzer & Bug Fix Porting Generator

- **Target Package**: `pkg/story/drift`, `pkg/query`
- **Governing Specs**: [`docs/features/FEAT-032-polyglot-speculative-drift-workflows.md`](docs/features/FEAT-032-polyglot-speculative-drift-workflows.md)
- **Goal**: Implement upstream drift detection engine that inspects upstream repository commits against sliced downstream codebases, ignores changes in pruned modules, and generates PR stories for relevant security and bug fixes.
- **Acceptance Criteria**:
    - [ ] `dendrite ref sync --check-drift` maps upstream commit deltas against retained AST symbols.
    - [ ] Filters out commits touching stripped packages with zero false-positive drift warnings.
    - [ ] Synthesizes bug fix patch branches targeting sliced local modules.
    - [ ] Statement test coverage >= 85%.

### Ticket `DEN-066`: Multi-Repo Context Re-Hydration & IDE Scope Restorer

- **Target Package**: `pkg/story/resume`, `cmd/dendrite`
- **Governing Specs**: [`docs/features/FEAT-032-polyglot-speculative-drift-workflows.md`](docs/features/FEAT-032-polyglot-speculative-drift-workflows.md)
- **Goal**: Implement one-shot context re-hydration command that restores pool links, summarizes completed vs pending analysis chunks, refreshes JetBrains IDE scopes, and presents an actionable summary of next tasks after weeks away.
- **Acceptance Criteria**:
    - [ ] `dendrite story resume` executes in < 3 seconds without consuming cloud tokens.
    - [ ] Verifies validity of all local pool directory paths and manifest links.
    - [ ] Emits concise summary scorecard: active story, pending chunks, uncompleted acceptance criteria, next CLI/IDE action.
    - [ ] Regenerates `.idea/scopes/` XML for active JetBrains IDE.
    - [ ] Statement test coverage >= 85%.

### Ticket `DEN-067`: 5-Phase Atomic Service Migration Engine: Local <-> Debian <-> Azure

- **Target Package**: `pkg/node/migrate`, `cmd/dendrite`
- **Governing Specs**: [`docs/features/FEAT-033-infrastructure-mobility-smart-backup.md`](docs/features/FEAT-033-infrastructure-mobility-smart-backup.md)
- **Goal**: Implement 5-phase atomic service migration protocol (`Pre-Flight Backup` -> `WAL Journaling` -> `BLAKE3 Sync` -> `Merkle Verification` -> `Atomic Cutover`) allowing background analysis workers and store state to move seamlessly across Local, Debian, and Azure nodes with zero data loss.
- **Acceptance Criteria**:
    - [ ] `dendrite node migrate --from <src> --to <dst> --verify-checksum` executes the 5 atomic phases in sequence.
    - [ ] Cryptographic parity verified (`source_blake3 == target_blake3`) before initiating cutover.
    - [ ] Automatic rollback if any migration phase fails, restoring source node as active with zero downtime.
    - [ ] Mandatory Persistent Layer Testing (DoD): Multi-node migration verified using `testcontainers-go` (SSH container, PostgreSQL container).
    - [ ] Statement test coverage >= 85%.

### Ticket `DEN-068`: Write-Ahead Log (WAL) State Journaling & Merkle Digest Verification Protocol

- **Target Package**: `pkg/node/wal`, `pkg/store`
- **Governing Specs**: [`docs/features/FEAT-033-infrastructure-mobility-smart-backup.md`](docs/features/FEAT-033-infrastructure-mobility-smart-backup.md)
- **Goal**: Implement WAL journaling and Merkle tree state verification engine that buffers live story updates during migration and asserts byte-level cryptographic consistency prior to active node promotion.
- **Acceptance Criteria**:
    - [ ] Diverts live IDE story updates and telemetry events to `.dendrite/wal.journal` during state transfer.
    - [ ] Computes hierarchical Merkle tree digests over all SQLite pages and chunk blob archives.
    - [ ] Target node replays buffered WAL entries upon promotion with zero dropped transactions.
    - [ ] Statement test coverage >= 85%.

### Ticket `DEN-069`: Smart Repo-Aware Backup Engine with Git-Persisted Blob Eviction Priority

- **Target Package**: `pkg/backup/engine`, `cmd/dendrite`
- **Governing Specs**: [`docs/features/FEAT-033-infrastructure-mobility-smart-backup.md`](docs/features/FEAT-033-infrastructure-mobility-smart-backup.md)
- **Goal**: Implement smart repository-aware backup engine enforcing a 2-version cap by default, space quotas, and a 3-priority eviction algorithm that frees space by evicting Git-persisted data first (replacing committed files with 40-byte Git SHA pointers).
- **Acceptance Criteria**:
    - [ ] `dendrite backup init --target-dir <dir> --max-versions 2 --max-size <limit>` configures backup retention.
    - [ ] When quota is reached, Priority 1 evicts objects verified in `git rev-list --objects --all` and writes 40-byte pointer.
    - [ ] Priority 2 evicts recomputable static AST indexes; Priority 3 (LLM semantic store) is protected from automated eviction.
    - [ ] Backup restoration reconstructs full filesystem state from Git commit blobs and backup pointers.
    - [ ] Mandatory Persistent Layer Testing (DoD): Backup, eviction, and restoration cycles tested with `testcontainers-go`.
    - [ ] Statement test coverage >= 85%.

### Ticket `DEN-070`: Unified Configuration Subsystem with nvim/vim Transactional Staging & Taplo Schema

- **Target Package**: `pkg/config/engine`, `cmd/dendrite`
- **Governing Specs**: [`docs/features/FEAT-034-dual-mode-config-multichannel-distribution.md`](docs/features/FEAT-034-dual-mode-config-multichannel-distribution.md)
- **Goal**: Implement unified configuration manager providing 100% parity across Web UI, Terminal UI, CLI (`config get/set`), and direct editing in `nvim`/`vim` with transactional staging, `:wq` validation loops, cursor section jumping, and taplo TOML JSON schema autocompletion.
- **Acceptance Criteria**:
    - [ ] `dendrite config edit --editor nvim [--section <name>]` opens transactional staging copy with cursor positioned at table header.
    - [ ] On `:wq`, validates TOML syntax and value ranges; re-opens `nvim +<line>` on validation failure.
    - [ ] Generates `.dendrite/config.toml` containing taplo `#:schema` directive and vim modeline.
    - [ ] Changes made in CLI, UI, or nvim propagate immediately to live background daemon.
    - [ ] Statement test coverage >= 85%.

### Ticket `DEN-071`: Multi-Channel CI/CD Distribution Pipeline: Brew Tap, Debian .deb, GHCR & GoReleaser

- **Target Package**: `.github/workflows`, `.goreleaser.yaml`
- **Governing Specs**: [`docs/features/FEAT-034-dual-mode-config-multichannel-distribution.md`](docs/features/FEAT-034-dual-mode-config-multichannel-distribution.md)
- **Goal**: Implement automated GitHub Actions and GoReleaser v2 distribution pipeline building multi-platform binaries, publishing Homebrew formulas to `yasuomaidana/homebrew-tap`, building Debian `.deb` packages with systemd units, and publishing multi-arch Docker containers to GHCR upon tag push.
- **Acceptance Criteria**:
    - [ ] `.goreleaser.yaml` configures cross-compilation for linux/darwin/windows on amd64 and arm64 with CGO_ENABLED=0.
    - [ ] Generates Debian `.deb` packaging `deploy/systemd/dendrite-daemon.service` into `/lib/systemd/system/`.
    - [ ] Pushes multi-architecture Docker container images to `ghcr.io/yasuomaidana/dendrite`.
    - [ ] GitHub Actions workflow `.github/workflows/ci-release.yml` triggers on `v*.*.*` tags with clean verification steps.
    - [ ] Coverage and test matrix passing in CI before release job triggers.
### Milestone 30: Multi-Provider AI Runtime & Semantic Hierarchical Git Governance (FEAT-035)

- [ ] **DEN-072**: Multi-Provider AI Inference Adapter: GitHub Copilot CLI & Native/Bridged Ollama (`pkg/agent/runtime`, `cmd/dendrite`)
- [ ] **DEN-073**: 5-Tier Version Strategy Interceptor & Semantic Hierarchical Branch/Merge Engine (`pkg/agent/gitgov`, `pkg/pool`)

### Milestone 31: Ticket-Milestone DAG Linker & Story Prioritization Engine (FEAT-036)

- [ ] **DEN-074**: Ticket-Milestone Relational Ingestion & Dependency DAG Solver (`pkg/stories/dag`, `pkg/tracker`)
- [ ] **DEN-075**: Context-Aware `dendrite stories next` Scheduler & Active Story Context Injector (`pkg/stories/next`, `pkg/jetbrains`)

### Milestone 32: Bidirectional Penpot UI Documentation & MCP Integration (FEAT-037)

- [ ] **DEN-076**: UI File Mutation Watcher & Penpot Documentation Token Diff Synthesizer (`pkg/ui/penpot`, `pkg/mcpserver`)
- [ ] **DEN-077**: Bidirectional Traceability: Ticket Comment Frame Deep-Linking & Design Drift Sync (`pkg/tracker/plane`, `pkg/ui/penpot`)

### Milestone 33: Self-Update Lifecycle & Developer Mode Dual-Source Engine (FEAT-038)

- [ ] **DEN-078**: Self-Update Lifecycle Manager with Checksum Verification & Binary Rollback (`pkg/update/lifecycle`, `cmd/dendrite`)
- [ ] **DEN-079**: Developer Mode (`--dev`) Dual-Source Engine: Local-First Build & Remote Override (`pkg/update/devmode`, `docs/DEVELOPMENT.md`)

### Milestone 34: Polyglot Environment Bootstrapper & GoF Pattern Analyzer (FEAT-039)

- [ ] **DEN-080**: Polyglot Stack Fingerprinter & Agent Environment Initializer (`pkg/env/bootstrap`, `cmd/dendrite`)
- [ ] **DEN-081**:  GoF Design-Pattern Analyzer Skill Generator (`pkg/env/patterns`, `.agents/skills`)
- [ ] **DEN-082**: SOTA Best-Practices Web/Advisory Diff Inspector & Automated Updater (`pkg/env/updater`, `cmd/dendrite`)

### Milestone 35: Offline-First Local Cache & Outbox Sync Engine (FEAT-040)

- [ ] **DEN-083**: Embedded Turso/libSQL Local Cache with WAL Journaling & Testcontainers Integration (`pkg/cache/turso`, `pkg/store`)
- [ ] **DEN-084**: Transactional Outbox Mutation Queue & Exponential Backoff Replay Worker (`pkg/cache/outbox`, `pkg/tracker`)
- [ ] **DEN-085**: Automated Gitignore Binary & Cache Artifact Exclusion Guardian (`pkg/cache/hygiene`, `pkg/env`)

### Milestone 36: Pre-Commit Secret Scanner & Pre-PR/Merge Quality Gate (FEAT-041)

- [ ] **DEN-086**: Deterministic Pre-Commit Entropy & Credential Scanner Interceptor (`pkg/security/secretgate`, `pkg/agent`)
- [ ] **DEN-087**: Automated Pre-PR & Pre-Merge Build, Lint, and Test Verification Gate (`pkg/quality/harness`, `pkg/agent`)

### Milestone 37: Quota Failover, Worktree Isolation & Immutable Agent Audit Logging (FEAT-041)

- [ ] **DEN-088**: Hybrid Quota Scheduler & Mid-Story Copilot-to-Ollama Graceful Failover (`pkg/scheduler/failover`, `pkg/gateway`)
- [ ] **DEN-089**: Ephemeral Git Worktree Isolation Engine & RAII Rollback Controller (`pkg/agent/worktree`, `pkg/pool`)
- [ ] **DEN-090**: Immutable libSQL Agent Audit Log & `dendrite audit` Trace CLI (`pkg/audit/logger`, `cmd/dendrite`)




### Ticket `DEN-072`: Multi-Provider AI Inference Adapter: GitHub Copilot CLI & Native/Bridged Ollama

- **Target Package**: `pkg/agent/runtime`, `cmd/dendrite`
- **Governing Specs**: [`docs/features/FEAT-035-multiprovider-agent-governance.md`](docs/features/FEAT-035-multiprovider-agent-governance.md)
- **Goal**: Implement unified AI runtime adapter supporting direct native HTTP streaming to Ollama instances (`/api/chat`), OpenAI/Copilot-compatible proxy bridging, GitHub Copilot CLI (`gh copilot`) wrapper, and runtime environment auto-negotiation.
- **Acceptance Criteria**:
    - [ ] Auto-negotiates execution provider (`gh` copilot vs native local Ollama) based on host reachability and latency probing.
    - [ ] Communicates directly with local/LAN Ollama HTTP endpoints with zero external wrapper overhead.
    - [ ] Emits structured token streaming and tool-call events to terminal and telemetry bus.
    - [ ] Statement test coverage >= 85%.

### Ticket `DEN-073`: 5-Tier Version Strategy Interceptor & Semantic Hierarchical Branch/Merge Engine

- **Target Package**: `pkg/agent/gitgov`, `pkg/pool`
- **Governing Specs**: [`docs/features/FEAT-035-multiprovider-agent-governance.md`](docs/features/FEAT-035-multiprovider-agent-governance.md), [`docs/features/FEAT-029-autonomous-pool-dag-synchronization.md`](docs/features/FEAT-029-autonomous-pool-dag-synchronization.md)
- **Goal**: Implement deterministic Git and GitHub CLI policy interceptor enforcing the 5-tier autonomy governance matrix (`conservative`, `super-safe`, `safe`, `normal`, `yolo`) and enforcing semantic hierarchical multi-level branching and merging (`milestone/*` -> `story/*` -> `feature/*`).
- **Acceptance Criteria**:
    - [ ] Validates every `git` and `gh` execution against active autonomy level at process boundary.
    - [ ] In Level 1, blocks commits/branches/PRs; in Level 2, isolates on `agent/*`; in Level 3, enforces strict 1:1 PRs; in Level 4, enforces milestone-level consolidation.
    - [ ] Enforces 3-tier hierarchy: feature branches merge to story, stories merge to milestone, milestone opens consolidated PR to main.
    - [ ] Statement test coverage >= 85%.

### Ticket `DEN-074`: Ticket-Milestone Relational Ingestion & Dependency DAG Solver

- **Target Package**: `pkg/stories/dag`, `pkg/tracker`
- **Governing Specs**: [`docs/features/FEAT-036-ticket-milestone-dag-story-prioritization.md`](docs/features/FEAT-036-ticket-milestone-dag-story-prioritization.md)
- **Goal**: Implement relational graph layer ingesting tickets and milestones from Plane API, GitHub Issues, and local cache, constructing an acyclic dependency graph, and detecting circular blockers.
- **Acceptance Criteria**:
    - [ ] `dendrite stories link --analyze` constructs relational ticket-milestone DAG.
    - [ ] Detects and reports circular dependency deadlocks with offending cycle trace.
    - [ ] Persists graph edges in local database and syncs relationships to Plane tracker.
    - [ ] Statement test coverage >= 85%.

### Ticket `DEN-075`: Context-Aware `dendrite stories next` Scheduler & Active Story Context Injector

- **Target Package**: `pkg/stories/next`, `pkg/jetbrains`
- **Governing Specs**: [`docs/features/FEAT-036-ticket-milestone-dag-story-prioritization.md`](docs/features/FEAT-036-ticket-milestone-dag-story-prioritization.md)
- **Goal**: Implement priority scheduler that calculates critical path unblock weight for uncompleted tickets and automatically checks out the highest-priority story branch rooted under its parent milestone, refreshing JetBrains IDE scopes.
- **Acceptance Criteria**:
    - [ ] Calculates unblock weights: direct story points + weighted downstream dependent impact.
    - [ ] `dendrite stories next` checks out `story/<id>` under `milestone/<id>`.
    - [ ] Injects JetBrains `.idea/scopes/Dendrite_Active_Story.xml` and run configuration.
    - [ ] Statement test coverage >= 85%.

### Ticket `DEN-076`: UI File Mutation Watcher & Penpot Documentation Token Diff Synthesizer

- **Target Package**: `pkg/ui/penpot`, `pkg/mcpserver`
- **Governing Specs**: [`docs/features/FEAT-037-penpot-ui-documentation-mcp-integration.md`](docs/features/FEAT-037-penpot-ui-documentation-mcp-integration.md)
- **Goal**: Implement frontend asset watcher monitoring modifications across `.svelte`, `.tsx`, `.vue`, CSS, and Tailwind files, extracting component states and token diffs, and generating structured specification frames in Penpot via MCP.
- **Acceptance Criteria**:
    - [ ] Triggers on UI asset modifications during story execution.
    - [ ] Extracts component props, variants, and design token differences against git baseline.
    - [ ] Invokes Penpot MCP tool to create or update design review frame in Penpot project.
    - [ ] Statement test coverage >= 85%.

### Ticket `DEN-077`: Bidirectional Traceability: Ticket Comment Frame Deep-Linking & Design Drift Sync

- **Target Package**: `pkg/tracker/plane`, `pkg/ui/penpot`
- **Governing Specs**: [`docs/features/FEAT-037-penpot-ui-documentation-mcp-integration.md`](docs/features/FEAT-037-penpot-ui-documentation-mcp-integration.md)
- **Goal**: Implement bidirectional integration linking synthesized Penpot design frames to Plane tickets and GitHub PR descriptions with actionable deep-links and design review badges.
- **Acceptance Criteria**:
    - [ ] Formats markdown comment with Penpot frame URL and embeds it into associated Plane issue.
    - [ ] Appends Penpot design specification badge to GitHub PR body.
    - [ ] Propagates designer comments from Penpot back into Plane ticket activity log.
    - [ ] Statement test coverage >= 85%.

### Ticket `DEN-078`: Self-Update Lifecycle Manager with Checksum Verification & Binary Rollback

- **Target Package**: `pkg/update/lifecycle`, `cmd/dendrite`
- **Governing Specs**: [`docs/features/FEAT-038-self-update-lifecycle-devmode.md`](docs/features/FEAT-038-self-update-lifecycle-devmode.md)
- **Goal**: Implement production self-update lifecycle querying GitHub Releases, verifying SHA-256 signatures, executing atomic in-place binary swaps, and safely rolling back from backup on failure.
- **Acceptance Criteria**:
    - [ ] `dendrite update` queries latest GitHub release, downloads target OS/arch binary, and verifies SHA-256.
    - [ ] Renames active binary to `.bak` before swapping to prevent `text file busy` errors.
    - [ ] Runs `--version` smoke test post-swap; restores backup automatically if test fails.
    - [ ] Statement test coverage >= 85%.

### Ticket `DEN-079`: Developer Mode (`--dev`) Dual-Source Engine: Local-First Build & Remote Override

- **Target Package**: `pkg/update/devmode`, `docs/DEVELOPMENT.md`
- **Governing Specs**: [`docs/features/FEAT-038-self-update-lifecycle-devmode.md`](docs/features/FEAT-038-self-update-lifecycle-devmode.md)
- **Goal**: Implement developer mode engine where `dendrite update --dev` compiles and installs from the local Go repository by default, while supporting an explicit `--source remote` override and maintaining developer documentation in `docs/DEVELOPMENT.md`.
- **Acceptance Criteria**:
    - [ ] `dendrite update --dev` compiles `cmd/dendrite` from local source tree and installs binary.
    - [ ] `dendrite update --dev --source remote` queries remote release candidate with confirmation prompt.
    - [ ] Complete workflow guide and developer commands documented in `docs/DEVELOPMENT.md`.
    - [ ] Statement test coverage >= 85%.

### Ticket `DEN-080`: Polyglot Stack Fingerprinter & Agent Environment Initializer

- **Target Package**: `pkg/env/bootstrap`, `cmd/dendrite`
- **Governing Specs**: [`docs/features/FEAT-039-polyglot-env-bootstrapper-gof-patterns.md`](docs/features/FEAT-039-polyglot-env-bootstrapper-gof-patterns.md)
- **Goal**: Implement polyglot stack analyzer inspecting manifests (`go.mod`, `Cargo.toml`, `package.json`, `pyproject.toml`, `CMakeLists.txt`) and scaffolding stack-specific rules across `.agents/`, `.ai/`, `.gemini/`, and `AGENTS.md`.
- **Acceptance Criteria**:
    - [ ] Detects primary language, test runner, linters, and architectural conventions.
    - [ ] Replaces mismatched rules (e.g. Rust borrow-checker in Go repo) with stack-specific idioms.
    - [ ] Idempotently configures repository `.gitignore` for agent environment artifacts.
    - [ ] Statement test coverage >= 85%.

### Ticket `DEN-081`:  GoF Design-Pattern Analyzer Skill Generator

- **Target Package**: `pkg/env/patterns`, `.agents/skills`
- **Governing Specs**: [`docs/features/FEAT-039-polyglot-env-bootstrapper-gof-patterns.md`](docs/features/FEAT-039-polyglot-env-bootstrapper-gof-patterns.md)
- **Goal**: Implement design-pattern skill generator embedding the complete 22  (GoF) patterns from  (Creational, Structural, Behavioral) tailored to the detected repository language.
- **Acceptance Criteria**:
    - [ ] Seeds generated skill with 5 Creational, 7 Structural, and 10 Behavioral GoF design patterns.
    - [ ] Translates pattern applicability into target language idioms (Go interfaces, Rust traits, TS types).
    - [ ] Provides diagnostic smell signals and step-by-step refactoring workflows.
    - [ ] Statement test coverage >= 85%.

### Ticket `DEN-082`: SOTA Best-Practices Web/Advisory Diff Inspector & Automated Updater

- **Target Package**: `pkg/env/updater`, `cmd/dendrite`
- **Governing Specs**: [`docs/features/FEAT-039-polyglot-env-bootstrapper-gof-patterns.md`](docs/features/FEAT-039-polyglot-env-bootstrapper-gof-patterns.md)
- **Goal**: Implement `dendrite env update --sota` querying current language idioms, framework advisories, and AI agent standards, displaying a visual diff against existing agent files, and performing 3-way merge.
- **Acceptance Criteria**:
    - [ ] Queries state-of-the-art best practices for the detected tech stack.
    - [ ] Displays diff of proposed modifications to `.agents/`, `.ai/`, `.gemini/`, and `AGENTS.md`.
    - [ ] Merges improvements while strictly preserving project-specific custom overrides.
    - [ ] Statement test coverage >= 85%.

### Ticket `DEN-083`: Embedded Turso/libSQL Local Cache with WAL Journaling & Testcontainers Integration

- **Target Package**: `pkg/cache/turso`, `pkg/store`
- **Governing Specs**: [`docs/features/FEAT-040-turso-offline-cache-outbox-sync.md`](docs/features/FEAT-040-turso-offline-cache-outbox-sync.md)
- **Goal**: Implement embedded Turso / libSQL local cache at `.dendrite/cache/dendrite.db` operating in WAL mode for sub-millisecond read-through caching of tickets, milestones, and Penpot metadata with offline CLI banner.
- **Acceptance Criteria**:
    - [ ] Operates embedded Turso / libSQL database in WAL journal mode.
    - [ ] Serves cached tickets when remote Plane or Penpot endpoints time out, displaying `[OFFLINE CACHE]`.
    - [ ] Mandatory Persistent Layer Testing (DoD): Concurrency, schema migrations, and crash recovery verified using `testcontainers-go`.
    - [ ] Statement test coverage >= 85%.

### Ticket `DEN-084`: Transactional Outbox Mutation Queue & Exponential Backoff Replay Worker

- **Target Package**: `pkg/cache/outbox`, `pkg/tracker`
- **Governing Specs**: [`docs/features/FEAT-040-turso-offline-cache-outbox-sync.md`](docs/features/FEAT-040-turso-offline-cache-outbox-sync.md)
- **Goal**: Implement transactional outbox queue buffering offline mutations (ticket updates, UI documentation notes) and a background replay worker with exponential backoff and idempotency keys.
- **Acceptance Criteria**:
    - [ ] Writes offline mutations to `outbox_mutations` table atomically.
    - [ ] Replay worker periodically probes connectivity and flushes pending mutations in FIFO order.
    - [ ] Enforces idempotency keys preventing duplicate operations on remote APIs.
    - [ ] Statement test coverage >= 85%.

### Ticket `DEN-085`: Automated Gitignore Binary & Cache Artifact Exclusion Guardian

- **Target Package**: `pkg/cache/hygiene`, `pkg/env`
- **Governing Specs**: [`docs/features/FEAT-040-turso-offline-cache-outbox-sync.md`](docs/features/FEAT-040-turso-offline-cache-outbox-sync.md)
- **Goal**: Implement repository hygiene guardian ensuring `.dendrite/cache/`, `*.db`, `*.db-wal`, and `*.db-shm` are idempotently added to `.gitignore` during initialization and environment updates.
- **Acceptance Criteria**:
    - [ ] Appends database and cache patterns to `.gitignore` idempotently.
    - [ ] Validates git staging area before commits to ensure zero binary cache files are tracked.
    - [ ] Statement test coverage >= 85%.

### Ticket `DEN-086`: Deterministic Pre-Commit Entropy & Credential Scanner Interceptor

- **Target Package**: `pkg/security/secretgate`, `pkg/agent`
- **Governing Specs**: [`docs/features/FEAT-041-agent-security-quality-resilience-guardrails.md`](docs/features/FEAT-041-agent-security-quality-resilience-guardrails.md)
- **Goal**: Implement deterministic pre-commit secret interceptor analyzing staged git diffs for high-entropy strings, API keys (GitHub PATs, Tailscale, Plane, Penpot), private keys, and `.env` files, aborting commits and redacting secrets from LLM context.
- **Acceptance Criteria**:
    - [ ] Analyzes staged diffs in memory before `git commit` execution.
    - [ ] Blocks commits containing high-entropy tokens or known credential patterns.
    - [ ] Redacts detected secrets from outbound prompt context to prevent LLM API leakage.
    - [ ] Statement test coverage >= 85%.

### Ticket `DEN-087`: Automated Pre-PR & Pre-Merge Build, Lint, and Test Verification Gate

- **Target Package**: `pkg/quality/harness`, `pkg/agent`
- **Governing Specs**: [`docs/features/FEAT-041-agent-security-quality-resilience-guardrails.md`](docs/features/FEAT-041-agent-security-quality-resilience-guardrails.md)
- **Goal**: Implement automated verification gate executing build, linter, and test suites defined in `AGENTS.md` before permitting `gh pr create` or `gh pr merge`, with self-healing feedback loops on failure.
- **Acceptance Criteria**:
    - [ ] Executes project verification commands in isolated environment.
    - [ ] Permits PR creation or merge only if exit code is 0.
    - [ ] On failure, feeds error diagnostics back into agent for self-healing before downgrading to human.
    - [ ] Statement test coverage >= 85%.

### Ticket `DEN-088`: Hybrid Quota Scheduler & Mid-Story Copilot-to-Ollama Graceful Failover

- **Target Package**: `pkg/scheduler/failover`, `pkg/gateway`
- **Governing Specs**: [`docs/features/FEAT-041-agent-security-quality-resilience-guardrails.md`](docs/features/FEAT-041-agent-security-quality-resilience-guardrails.md)
- **Goal**: Implement quota scheduler failover intercepting HTTP 429 rate limits or network disconnects during Copilot tasks, checkpointing state in Turso, and routing inference to local Ollama seamlessly.
- **Acceptance Criteria**:
    - [ ] Checkpoints active task state and conversation tokens in Turso cache upon 429 response.
    - [ ] Switches inference provider to local Ollama (`qwen2.5-coder:14b`) and resumes task execution.
    - [ ] Zero crashed agent processes or lost story progress on provider exhaustion.
    - [ ] Statement test coverage >= 85%.

### Ticket `DEN-089`: Ephemeral Git Worktree Isolation Engine & RAII Rollback Controller

- **Target Package**: `pkg/agent/worktree`, `pkg/pool`
- **Governing Specs**: [`docs/features/FEAT-041-agent-security-quality-resilience-guardrails.md`](docs/features/FEAT-041-agent-security-quality-resilience-guardrails.md)
- **Goal**: Implement git worktree isolation controller mounting tasks under `.dendrite/worktrees/<story-id>` with RAII drop cleanup, safe-stopping on merge conflicts, and restoring clean refs.
- **Acceptance Criteria**:
    - [ ] `dendrite agent run --worktree` isolates task in ephemeral directory.
    - [ ] Developer primary working tree remains clean and untouched.
    - [ ] On merge conflict, aborts merge, restores checkpointed ref, and flags story in Plane for human review.
    - [ ] RAII cleanup prunes worktree directory on task resolution or failure.
    - [ ] Statement test coverage >= 85%.

### Ticket `DEN-090`: Immutable libSQL Agent Audit Log & `dendrite audit` Trace CLI

- **Target Package**: `pkg/audit/logger`, `cmd/dendrite`
- **Governing Specs**: [`docs/features/FEAT-041-agent-security-quality-resilience-guardrails.md`](docs/features/FEAT-041-agent-security-quality-resilience-guardrails.md)
- **Goal**: Implement immutable audit logger recording every git command, policy decision, LLM call, and token metric in libSQL, exposed via `dendrite audit --story <id>`.
- **Acceptance Criteria**:
    - [ ] Records all agent actions, policy verdicts, and provider metrics to append-only table.
    - [ ] `dendrite audit --story <id>` displays chronological trace formatted in terminal.
    - [ ] Query filters by story ID, autonomy level, and timestamp range.
    - [ ] Statement test coverage >= 85%.
