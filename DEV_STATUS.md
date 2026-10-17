# Dendrite Project Development Status & Roadmap

This document maintains the real-time development status, milestones, and active ticket execution pipeline for
**Dendrite**, managed following the autonomous ticket lifecycle protocol.

---

## Current Status Overview

- **Active Milestone**: Milestone 17 — Performance Testing, Inference Benchmarking & Gateway Load Harness (FEAT-021)
- **Current Focus**: `DEN-015: Real-Time Telemetry Gauges & Thermal Monitoring Daemon`
- **Last Updated**: 2026-10-16
- **Overall Progress**: 24 / 153 Tickets Completed (15.7%)

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

### Milestone 16: Pluggable Multi-Provider Inference Engines (FEAT-020)

- [ ] **DEN-035**: Unified Inference Engine Gateway: OxiLLaMa, Hugging Face TGI/TEI, vLLM, llama.cpp & Ollama Metal (`pkg/inference`, `pkg/gateway`)

### Milestone 17: Performance Testing, Inference Benchmarking & Gateway Load Harness (FEAT-021)

- [ ] **DEN-036**: k6 Performance Testing, TTFT/TPS Benchmarking & Cascade Failover Load Harness (`pkg/perf`, `cmd/dendrite`, `tests/perf`)

### Milestone 18: Agent Search, Capability Discovery & Semantic Routing Engine (FEAT-022)

- [ ] **DEN-037**: Hybrid Agent Search, TEI Embeddings & Semantic Routing Engine (`pkg/agentsearch`, `cmd/dendrite`)

### Milestone 19: Developer Test Automation, Self-Healing Harness & UI Integration Suite (FEAT-023)

- [ ] **DEN-038**: Automated Test Synthesizer, Self-Healing Convergence Loop & Web UI Test Harness (`pkg/testgen`, `pkg/uitest`, `cmd/dendrite`)

### Milestone 20: Open-Source REA Showcase Pipeline & Cleanroom Parity Verification Suite (FEAT-024)

- [ ] **DEN-039**: Open-Source Reverse Engineering Showcases & Cleanroom Parity Verification Suite (`pkg/rea`, `pkg/ai/import`, `tests/showcase`, `cmd/dendrite`)

### Milestone 21: Model Context Protocol (MCP) Server & Tool Registry Bridge (FEAT-025)

- [ ] **DEN-040**: Dendrite Native MCP Server & Tool Registry Bridge (`pkg/mcpserver`, `cmd/dendrite`)

### Milestone 22: Distributed Codebase Ecosystem, Modular REA & Cross-Project Interoperability Coordinator (FEAT-026)

- [ ] **DEN-041**: Distributed Codebase Multi-Source Ecosystem & Priority Resolver (pkg/distcodebase, cmd/dendrite)
- [ ] **DEN-042**: Scenario 1: REA Modular Decomposition & Ecosystem Reuse Discovery Engine (pkg/distcodebase, pkg/rea, cmd/dendrite)
- [ ] **DEN-043**: Scenario 2: Parallel Multi-Project Workspace & Repository Spawner (pkg/distcodebase, pkg/workspace, cmd/dendrite)
- [ ] **DEN-044**: Scenario 3: Local LLM Cross-Project Interoperability Coordinator (pkg/distcodebase, pkg/ollama, cmd/dendrite)
- [ ] **DEN-045**: Scenario 4: Generative Satellite Project Synthesis & Autonomous Plane Project Provisioning (pkg/distcodebase, pkg/tracker/plane, cmd/dendrite)
- [ ] **DEN-046**: JetBrains IDE Toolset Integration, 19 MCP Tools & Agentic Developer Workflow Guide (pkg/jetbrains, pkg/mcpserver, cmd/dendrite)

