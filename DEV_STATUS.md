# Dendrite Project Development Status & Roadmap

This document maintains the real-time development status, milestones, and active ticket execution pipeline for
**Dendrite**, managed following the autonomous ticket lifecycle protocol.

---

## Current Status Overview

- **Active Milestone**: Milestone 17 — Performance Testing, Inference Benchmarking & Gateway Load Harness (FEAT-021)
- **Current Focus**: `DEN-039: Open-Source Reverse Engineering Showcases & Cleanroom Parity Verification Suite (`pkg/rea`, `pkg/ai/import`, `tests/showcase`, `cmd/dendrite`)`
- **Last Updated**: 2026-10-14
- **Overall Progress**: 43 / 46 Tickets Completed (93.5%)

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
|      **8**      | **DEN-037** | Milestone 18 | FEAT-022   |    13 SP     |     8.5 / 10     | `pkg/agentsearch`, `cmd/dendrite`                            |    **QUEUED [ ]**     |
|      **9**      | **DEN-038** | Milestone 19 | FEAT-023   |    13 SP     |     9.0 / 10     | `pkg/testgen`, `pkg/uitest`, `cmd/dendrite`                  |    **QUEUED [ ]**     |
|      **7**      | **DEN-039** | Milestone 20 | FEAT-024   |     8 SP     |     8.0 / 10     | `pkg/rea`, `pkg/ai/import`, `tests/showcase`, `cmd/dendrite` |    **QUEUED [ ]**     |

---

## Milestone Execution Pipeline

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

- [ ] **DEN-039**: Open-Source Reverse Engineering Showcases & Cleanroom Parity Verification Suite (`pkg/rea`, `pkg/ai/import`, `tests/showcase`, `cmd/dendrite`) *(IN PROGRESS)*

### Milestone 18: Agent Search, Capability Discovery & Semantic Routing Engine (FEAT-022)

- [ ] **DEN-037**: Hybrid Agent Search, TEI Embeddings & Semantic Routing Engine (`pkg/agentsearch`, `cmd/dendrite`)

### Milestone 19: Developer Test Automation, Self-Healing Harness & UI Integration Suite (FEAT-023)

- [ ] **DEN-038**: Automated Test Synthesizer, Self-Healing Convergence Loop & Web UI Test Harness (`pkg/testgen`, `pkg/uitest`, `cmd/dendrite`)

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

