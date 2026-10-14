# Dendrite Project Development Status & Roadmap

This document maintains the real-time development status, milestones, and active ticket execution pipeline for
**Dendrite**, managed following the autonomous ticket lifecycle protocol.

---

## Current Status Overview

- **Active Milestone**: Milestone 17 — Performance Testing, Inference Benchmarking & Gateway Load Harness (FEAT-021)
- **Current Focus**: `DEN-036: k6 Performance Testing, TTFT/TPS Benchmarking & Cascade Failover Load Harness`
- **Last Updated**: 2026-10-14
- **Overall Progress**: 35 / 40 Tickets Completed (87.5%)

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
|      **7**      | **DEN-039** | Milestone 20 | FEAT-024   |     8 SP     |     8.0 / 10     | `pkg/rea`, `pkg/ai/import`, `tests/showcase`, `cmd/dendrite` |    **QUEUED [ ]**     |
|      **8**      | **DEN-037** | Milestone 18 | FEAT-022   |    13 SP     |     8.5 / 10     | `pkg/agentsearch`, `cmd/dendrite`                            |    **QUEUED [ ]**     |
|      **9**      | **DEN-038** | Milestone 19 | FEAT-023   |    13 SP     |     9.0 / 10     | `pkg/testgen`, `pkg/uitest`, `cmd/dendrite`                  |    **QUEUED [ ]**     |

---

## Milestone Execution Pipeline

### Milestone 13: Remote SSH Hardware Discovery & Automated Service Placement Router (FEAT-017)

- [x] **DEN-032**: Remote SSH Node Topology Provisioner & Dynamic Service Placement Router (`pkg/topology/advisor`, `cmd/dendrite`)

### Milestone 15: Azure Cloud-Burst Infrastructure & Managed Storage (FEAT-019)

- [x] **DEN-034**: Terraform Azure Spot VM Module & Persistent Managed Disk Lifecycler (`infra/modules/azure-burst`, `pkg/orchestrator/azure`)

### Milestone 21: Model Context Protocol (MCP) Server & Tool Registry Bridge (FEAT-025)

- [x] **DEN-040**: Dendrite Native MCP Server & Tool Registry Bridge (`pkg/mcpserver`, `cmd/dendrite`)

### Milestone 16: Pluggable Multi-Provider Inference Engines (FEAT-020)

- [x] **DEN-035**: Unified Inference Engine Gateway: OxiLLaMa, Hugging Face TGI/TEI, vLLM, llama.cpp & Ollama Metal (`pkg/inference`, `pkg/gateway`)

### Milestone 17: Performance Testing, Inference Benchmarking & Gateway Load Harness (FEAT-021)

- [ ] **DEN-036**: k6 Performance Testing, TTFT/TPS Benchmarking & Cascade Failover Load Harness (`pkg/perf`, `cmd/dendrite`, `tests/perf`)

### Milestone 20: Open-Source REA Showcase Pipeline & Cleanroom Parity Verification Suite (FEAT-024)

- [ ] **DEN-039**: Open-Source Reverse Engineering Showcases & Cleanroom Parity Verification Suite (`pkg/rea`, `pkg/ai/import`, `tests/showcase`, `cmd/dendrite`)

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

- [x] **DEN-028**: JetBrains IDE MCP Integration & Communication Bridge (`pkg/jetbrains`)

### Milestone 9: Web UI & Dashboard Engine (FEAT-009)

- [x] **DEN-014**: Embedded HTTP Control Server & WebSocket Telemetry (`pkg/server`)
- [x] **DEN-015**: Real-Time Telemetry Gauges & Thermal Monitoring Daemon (`pkg/telemetry`)

### Milestone 8: Multi-Project Workspaces & Pluggable Trackers (FEAT-005, FEAT-008)

- [x] **DEN-012**: Multi-Project Workspace Registry & Context Switcher (`pkg/workspace`)
- [x] **DEN-013**: Pluggable Issue Tracker Interface & Atlassian Jira Adapter (`pkg/tracker/jira`)
- [x] **DEN-027**: Plane v2 & Linear Strategy Adapters (`pkg/tracker/plane`)

### Milestone 7: Polyglot Translation & Documentation Reflection (FEAT-011, FEAT-012)

- [x] **DEN-020**: Intermediate Semantic Representation ISR Tree-sitter Parser (`pkg/ai/translate`)
- [x] **DEN-021**: Idiomatic Target Code Synthesizer & Dual Property Test Parity Generator (`pkg/ai/translate/verify`)
- [x] **DEN-022**: AST Docs-as-Code Synthesizer for FEAT/ADR Specs & OpenAPI 3.1 (`pkg/docgen`)
- [x] **DEN-023**: Cross-Project Documentation Reflection Engine & Docusaurus Sync (`pkg/docgen/reflect`)

### Milestone 6: AI-Assisted Token Optimization & Cleanroom Import (FEAT-010)

- [x] **DEN-018**: AST Context Compaction & LiteLLM Prefix Cache Harness (`pkg/ai/tokenopt`)
- [x] **DEN-019**: Cross-Repository Capability Dissection & Cleanroom Synthesizer (`pkg/ai/import`)

### Milestone 5: Reverse Engineering & Sandboxed Intelligence (FEAT-003, FEAT-004, FEAT-006)

- [x] **DEN-009**: REA MCP Bridge & Disassembler Protocol Client (`pkg/rea`)
- [x] **DEN-010**: Tree-sitter AST & Complexity Analyzer (`pkg/ast`)
- [x] **DEN-011**: Ephemeral Hardened Container Sandbox with RAII Drop (`pkg/sandbox`)

### Milestone 4: Autonomous Coding Agent Harness & Tool Orchestration (FEAT-013)

- [x] **DEN-024**: Agent Runner Harness & Environment Injector for Aider, OpenHands, and OpenCode (`pkg/agent`)
- [x] **DEN-025**: Agent Sandbox Isolation & LiteLLM Gateway Budget Binding (`pkg/agent/runner`)
- [x] **DEN-026**: Agent Session Tracker, Git Commit Provenance & Issue Tracker Sync (`pkg/agent/session`)

### Milestone 3: Token Budget Governance & Fallback Gateway (FEAT-002)

- [x] **DEN-007**: LiteLLM Configuration Template & Dynamic Renderer (`pkg/gateway`)
- [x] **DEN-008**: Sliding Window Budget Limiter & Ollama Failover Engine (`pkg/gateway`)

### Milestone 2: Multi-Node Topology & Cloud-Burst Infrastructure (FEAT-001, FEAT-007)

- [x] **DEN-005**: Topology Configuration Model & Scenario Placement Types (`pkg/topology`)
- [x] **DEN-006**: Terraform Cloud-Init & Compose Profile Runner (`pkg/orchestrator`)
- [x] **DEN-016**: Terraform AWS Spot EC2 Module & Cloud-Init Bootstrap (`infra/modules/aws-burst`)
- [x] **DEN-017**: Persistent EBS Volume Lifecycler & Zero-Ingress Security Group Validator (`pkg/orchestrator/aws`)
- [x] **DEN-029**: Development Environment Replication, Host Installer & Containerized Tooling (`pkg/env`, `stacks`)

### Milestone 1: Core Foundation, Developer Tools & Ollama Engine

- [x] **DEN-001**: Initialize Development Status Ledger & Execution Roadmap (`DEV_STATUS.md`)
- [x] **DEN-002**: Implement Idiomatic Ollama Client & Metal Engine Support (`pkg/ollama`)
- [x] **DEN-003**: Developer Productivity Tools on the Fly & CLI Scaffold (`pkg/devtool`, `cmd/dendrite`)
- [x] **DEN-004**: Ticket Module & Local Status Synchronization Engine (`pkg/tracker`)

---

## Detailed Specifications for Active & Queued Tickets

### Ticket `DEN-034`: Terraform Azure Spot VM Module & Persistent Managed Disk Lifecycler

- **Target Package**: `infra/modules/azure-burst`, `pkg/orchestrator/azure`
- **Governing Specs**: [`docs/features/FEAT-019-azure-cloud-burst.md`](docs/features/FEAT-019-azure-cloud-burst.md)
- **Status**: Completed [x]
- **Goal**: Implement the Azure cloud-burst infrastructure module supporting Linux Spot VMs, persistent Managed Disks, zero public ingress NSG rules, and multi-mesh bootstrap.
- **Acceptance Criteria**:
    - [x] Terraform Azure Spot VM compute and persistent Managed Disk module (`infra/modules/azure-burst/main.tf`, `variables.tf`, `outputs.tf`).
    - [x] Cloud-Init multi-mesh bootstrap configuring Docker 26+, deployer user, Tailscale overlay join, and persistent Managed Disk auto-mounting at `/var/lib/dendrite/storage` (`infra/modules/azure-burst/cloud-init.yaml.tftpl`).
    - [x] Zero Public Ingress security validator enforcing prohibition of 0.0.0.0/0 and ::/0 rules (`pkg/orchestrator/azure/validator.go`).
    - [x] Persistent Managed Disk lifecycler with thread-safe dynamic attachment, detachment, and storage tier validation (`pkg/orchestrator/azure/disk.go`).
    - [x] Comprehensive test suite achieving 96.9% statement coverage in `pkg/orchestrator/azure` and Terraform module structure verification (`pkg/orchestrator/azure_module_test.go`).

### Ticket `DEN-040`: Dendrite Native Model Context Protocol (MCP) Server & Tool Registry Bridge

- **Target Package**: `pkg/mcpserver`, `cmd/dendrite`
- **Status**: Completed [x]
- **Goal**: Expose Dendrite platform capabilities natively as a standard Model Context Protocol (MCP 2024-11-05) JSON-RPC 2.0 server for agentic IDEs, Antigravity/AGY, Cursor, and Claude Desktop.
- **Acceptance Criteria**:
    - [x] Native MCP JSON-RPC 2.0 server implementation with stdio transport (`pkg/mcpserver/server.go`, `pkg/mcpserver/types.go`).
    - [x] Standard tool registry with 9 exposed operational tools: `dendrite_env_status`, `dendrite_ticket_list`, `dendrite_ticket_update`, `dendrite_plane_issues`, `dendrite_ollama_probe`, `dendrite_topology_advisor`, `dendrite_gateway_render`, `dendrite_agent_run`, `dendrite_telemetry` (`pkg/mcpserver/tools.go`).
    - [x] Full CLI commands `dendrite mcp serve`, `dendrite mcp tools`, `dendrite mcp call <name> [args]` in `cmd/dendrite/mcp.go`.
    - [x] 100% statement coverage unit test suite for handshake, tool listing, execution, and error handling (`pkg/mcpserver/server_test.go`).

### Ticket `DEN-035`: Unified Inference Engine Gateway: OxiLLaMa, Hugging Face TGI/TEI, vLLM, llama.cpp & Ollama Metal

- **Target Package**: `pkg/inference`, `pkg/gateway`, `cmd/dendrite`, `pkg/mcpserver`
- **Governing Specs**: [`docs/features/FEAT-020-pluggable-inference-engines.md`](docs/features/FEAT-020-pluggable-inference-engines.md)
- **Status**: Completed [x]
- **Goal**: Extend Dendrite beyond Ollama to support OxiLLaMa (pure-Rust zero-FFI GGUF runtime), Hugging Face TGI/TEI, vLLM, and llama.cpp with unified LiteLLM token governance, zero-cost local accounting, and automatic fallback cascades.
- **Acceptance Criteria**:
    - [x] Implement `InferenceEngine` interface and runtime adapters for Ollama, OxiLLaMa (`cool-japan/oxillama`), Hugging Face TGI, TEI, vLLM, and llama.cpp (`pkg/inference/types.go`, `ollama.go`, `oxillama.go`, `huggingface_tgi.go`, `huggingface_tei.go`, `huggingface_endpoint.go`, `vllm.go`, `llamacpp.go`, `manager.go`).
    - [x] Dynamic upstream route rendering into `litellm_config.yaml` with cascade ordering (`pkg/gateway/renderer.go`).
    - [x] Health checker polling engine readiness and triggering dynamic failovers within $< 2\text{ seconds}$ (`pkg/inference/manager.go`).
    - [x] Zero-cost financial budget exemption for self-hosted engines (`pkg/gateway/governance.go`).
    - [x] Table-driven unit test suite achieving 86.3% statement coverage in `pkg/inference` and 91.3% in `pkg/gateway`.
    - [x] CLI commands `dendrite dev inference <list|probe|switch|render>` in `cmd/dendrite` and native MCP tools `dendrite_inference_list`, `dendrite_inference_probe` in `pkg/mcpserver`.

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
- **Governing Specs**: [`docs/features/FEAT-024-open-source-showcases.md`](docs/features/FEAT-024-open-source-showcases.md), [`docs/features/FEAT-003-reverse-engineering-rea.md`](docs/features/FEAT-003-reverse-engineering-rea.md), [`docs/features/FEAT-010-ai-assisted-token-optimization.md`](docs/features/FEAT-010-ai-assisted-token-optimization.md)
- **Goal**: Implement automated end-to-end showcase runners and cleanroom verification suites using 5 open-source target archetypes (preventing legal, licensing, DMCA, and ToS issues associated with proprietary software like Notion).
- **Showcase Portfolio**:
    1. **Aegis Authenticator** (`beemdevelopment/Aegis`, GPL-3.0): Android APK Dalvik bytecode analysis, TOTP calculation extraction, and RFC 6238 cleanroom generator synthesis.
    2. **DX-Ball Reconstruction** (`N0zoM1z0/dx-ball`, Permissive Open Source): 32-bit Windows PE x86 disassembly, position-to-pan helper extraction, 3,205 test-vector parity verification, and 63-byte binary reproduction.
    3. **DownUnderCTF Flag Checker** (`DownUnderCTF`, MIT): Linux ELF reverse engineering, bitmask/arithmetic equation solver, automated flag discovery.
    4. **TH04 Bullet Math** (`N0zoM1z0/th04`, Open Source): PC-98 DOS 16-bit real-mode instruction recovery, fixed-point trigonometry aiming angle reconstruction in modern C++/Go.
    5. **Mattermost Desktop** (`mattermost/desktop`, Apache-2.0): Open-source Electron replacement for proprietary Notion; ASAR extraction, preload context bridge isolation, and multi-format HTML/text clipboard IPC tracing.
- **Acceptance Criteria**:
    - [ ] CLI subcommand `dendrite rea showcase <aegis|dxball|ctf|th04|mattermost|all>` executing automated analysis and verification loops.
    - [ ] Bounded evidence bundling conforming to $< 4,000$ tokens per showcase probe.
    - [ ] Cleanroom synthesis pipeline producing test-verified Go/Rust implementations and ADR documents with YAML frontmatter.
    - [ ] Zero proprietary or closed-source code dependencies (Notion replaced with open-source Mattermost Desktop to prevent copyright/DMCA issues).
    - [ ] Table-driven integration and property test suite achieving $\ge 85\%$ statement coverage.

### Ticket `DEN-037`: Agent Search, Capability Discovery & Semantic Routing Engine

- **Target Package**: `pkg/agentsearch`, `cmd/dendrite`
- **Governing Specs**: [`docs/features/FEAT-022-agent-search-and-capability-discovery.md`](docs/features/FEAT-022-agent-search-and-capability-discovery.md)
- **Goal**: Implement hybrid dense/sparse search combining Hugging Face TEI vector embeddings and BM25 lexical retrieval with Reciprocal Rank Fusion (RRF) to route tasks to optimal agents (Aider, OpenHands, OpenCode, Google AGY).
- **Acceptance Criteria**:
    - [ ] TEI vector client and in-memory BM25 tokenizer/indexer.
    - [ ] Reciprocal Rank Fusion (RRF) scoring algorithm.
    - [ ] Task-to-Agent routing engine integrated with Graphify AST knowledge.
    - [ ] CLI command `dendrite agent search <query>` and `dendrite agent route --task <desc>`.
    - [ ] Statement coverage $\ge 85\%$.

### Ticket `DEN-038`: Developer Test Automation, Self-Healing Harness & UI Integration Suite

- **Target Package**: `pkg/testgen`, `pkg/uitest`, `cmd/dendrite`
- **Governing Specs**: [`docs/features/FEAT-023-developer-test-automation-ui-harness.md`](docs/features/FEAT-023-developer-test-automation-ui-harness.md)
- **Goal**: Provide automated AST test scaffolding, self-healing test convergence loop with local LLM compiler feedback, and headless Web UI integration test runner.
- **Acceptance Criteria**:
    - [ ] AST symbol parser and Testcontainers test code synthesizer.
    - [ ] Self-healing test loop executing `go test`/`cargo test` and refining tests up to 3 turns upon failure.
    - [ ] Headless browser UI test harness validating Web UI user journeys.
    - [ ] Statement coverage $\ge 85\%$.

---

## Open-Source REA Showcase Portfolio & Verification Matrix

To establish a production-grade benchmark for Dendrite's reverse engineering (`pkg/rea`), cleanroom code synthesis (`pkg/ai/import`), and sandboxed container execution (`pkg/sandbox`), we integrate a portfolio of verified, fully open-source targets inspired by the [REA Showcases](https://github.com/morluto/rea#showcases).

### Why Open-Source Showcases are Mandatory

1. **Legal & DMCA Immunity**: Closed-source software (such as Notion Desktop) is bound by proprietary End User License Agreements (EULA) and DMCA anti-circumvention protections. Using open-source projects protects developers, contributors, and enterprise adopters from IP litigation.
2. **Deterministic Ground-Truth Oracles**: Open-source targets offer public source code and established test suites, enabling mathematical property test parity verification between decompiled/synthesized modules and original reference implementations.
3. **CI/CD Reproducibility**: Open-source binaries, APKs, and ASAR packages can be freely cached, built, and tested in headless GitHub Actions and Docker pipelines without authentication tokens or commercial seats.

### Showcase Target Comparison & Substitution Matrix

|   #   | Showcase Target                 | Source Repository                                                   | License                    | Target Artifact Type                | Reverse-Engineering & Synthesis Objective                                                                                                                                               | Ground-Truth Oracle                             | Legal Status & Substitution Note                                                                      |
|:-----:|:--------------------------------|:--------------------------------------------------------------------|:---------------------------|:------------------------------------|:----------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------|:------------------------------------------------|:------------------------------------------------------------------------------------------------------|
| **1** | **Aegis Authenticator**         | [`beemdevelopment/Aegis`](https://github.com/beemdevelopment/Aegis) | GPL-3.0                    | Android APK / Dalvik Bytecode       | Trace 6-digit TOTP calculation algorithm, 30s clock-step generation, and HMAC-SHA1/SHA256 digest derivation from Dalvik bytecode. Synthesize clean Go `pkg/rea/showcase/otp` generator. | RFC 6238 reference vectors & Aegis test suite   | **Open-source (GPL-3.0)**; safe for public benchmarking.                                              |
| **2** | **DX-Ball Reconstruction**      | [`N0zoM1z0/dx-ball`](https://github.com/N0zoM1z0/dx-ball)           | MIT / Permissive           | 32-bit Windows PE x86 Binary (1996) | Disassemble Windows binary in sandboxed Ghidra container; extract position-to-pan sound helper (`sub_401020`); recover floating-point bounds and convert to idiomatic C/Go.             | 3,205 x86 test cases & 63 function bytes        | **Open-source cleanroom project**; passes byte-level verification.                                    |
| **3** | **DownUnderCTF Flag Checker**   | [`DownUnderCTF`](https://github.com/DownUnderCTF)                   | Open Source / CC           | 64-bit Linux ELF x86_64 Binary      | Disassemble checking function, convert assembly bitmasks and modular sums into algebraic constraint equations, and solve with an automated SMT/algebraic solver.                        | Solved flag string verified by challenge runner | **Open-source CTF challenge**; explicitly created for security analysis.                              |
| **4** | **TH04 Bullet Angle Targeting** | [`N0zoM1z0/th04`](https://github.com/N0zoM1z0/th04)                 | Open Source Reconstruction | 16-bit PC-98 DOS Binary             | Inspect 16-bit real-mode coordinate and angle helper arithmetic; extract fixed-point trigonometry aiming logic; synthesize memory-safe C++/Go function.                                 | Historical Borland C++ ASM parity               | **Open-source cleanroom reconstruction**; no proprietary code.                                        |
| **5** | **Mattermost Desktop**          | [`mattermost/desktop`](https://github.com/mattermost/desktop)       | Apache-2.0                 | Electron ASAR & Preload JavaScript  | Extract ASAR archive; trace preload context isolation boundaries, IPC event listeners, and multi-format HTML/text clipboard serialization.                                              | Mattermost Desktop test suite                   | **Permissive open-source replacement for proprietary Notion**; eliminates commercial EULA/DMCA risks. |

---

## Completed Ticket Archive

### Ticket `DEN-035`: Unified Inference Engine Gateway: OxiLLaMa, Hugging Face TGI/TEI, vLLM, llama.cpp & Ollama Metal

- **Target Package**: `pkg/inference`, `pkg/gateway`, `cmd/dendrite`, `pkg/mcpserver`
- **Status**: Completed [x]
- **Acceptance Criteria**:
    - [x] Pluggable `InferenceEngine` interface and runtime adapters for Ollama Metal, OxiLLaMa (pure-Rust zero-FFI GGUF runtime), Hugging Face TGI, TEI, hosted Endpoints, vLLM, and llama.cpp (`pkg/inference`).
    - [x] Multi-engine dynamic routing into `litellm_config.yaml` supporting declarative scenarios and fallback cascades (`pkg/gateway/renderer.go`).
    - [x] Background health polling (< 2s detection) and dynamic failover cascade router (`pkg/inference/manager.go`).
    - [x] Zero-cost budget exemption for local & self-hosted engines in sliding window budget governor (`pkg/gateway/governance.go`).
    - [x] CLI commands `dendrite dev inference <list|probe|switch|render>` and `dendrite inference <list|probe|switch|render>` (`cmd/dendrite`).
    - [x] Registered native MCP tools `dendrite_inference_list` and `dendrite_inference_probe` in `pkg/mcpserver`.
    - [x] Unit test suites achieving 86.3% statement coverage in `pkg/inference` and 91.3% in `pkg/gateway`.

### Ticket `DEN-034`: Terraform Azure Spot VM Module & Persistent Managed Disk Lifecycler

- **Target Package**: `infra/modules/azure-burst`, `pkg/orchestrator/azure`
- **Status**: Completed [x]
- **Acceptance Criteria**:
    - [x] Terraform Azure Spot VM compute and persistent Managed Disk module (`infra/modules/azure-burst/main.tf`, `variables.tf`, `outputs.tf`).
    - [x] Cloud-Init multi-mesh bootstrap configuring Docker 26+, deployer user, Tailscale overlay join, and persistent Managed Disk auto-mounting at `/var/lib/dendrite/storage` (`infra/modules/azure-burst/cloud-init.yaml.tftpl`).
    - [x] Zero Public Ingress security validator enforcing prohibition of 0.0.0.0/0 and ::/0 rules (`pkg/orchestrator/azure/validator.go`).
    - [x] Persistent Managed Disk lifecycler with thread-safe dynamic attachment, detachment, and storage tier validation (`pkg/orchestrator/azure/disk.go`).
    - [x] Comprehensive test suite achieving 96.9% statement coverage in `pkg/orchestrator/azure` and Terraform module structure verification (`pkg/orchestrator/azure_module_test.go`).

### Ticket `DEN-040`: Dendrite Native Model Context Protocol (MCP) Server & Tool Registry Bridge

- **Target Package**: `pkg/mcpserver`, `cmd/dendrite`
- **Status**: Completed [x]
- **Acceptance Criteria**:
    - [x] Native MCP JSON-RPC 2.0 server implementation with stdio transport (`pkg/mcpserver/server.go`, `pkg/mcpserver/types.go`).
    - [x] Standard tool registry with 9 exposed operational tools: `dendrite_env_status`, `dendrite_ticket_list`, `dendrite_ticket_update`, `dendrite_plane_issues`, `dendrite_ollama_probe`, `dendrite_topology_advisor`, `dendrite_gateway_render`, `dendrite_agent_run`, `dendrite_telemetry` (`pkg/mcpserver/tools.go`).
    - [x] Full CLI commands `dendrite mcp serve`, `dendrite mcp tools`, `dendrite mcp call <name> [args]` in `cmd/dendrite/mcp.go`.
    - [x] 100% statement coverage unit test suite for handshake, tool listing, execution, and error handling (`pkg/mcpserver/server_test.go`).

### Ticket `DEN-032`: Remote SSH Node Topology Provisioner & Dynamic Service Placement Router

- **Target Package**: `pkg/topology/advisor`, `cmd/dendrite`
- **Status**: Completed [x]
- **Acceptance Criteria**:
    - [x] Pluggable `CommandExecutor` interface supporting `LocalExecutor`, `SSHExecutor`, and `MockExecutor` (`pkg/topology/advisor/probe.go`).
    - [x] Hardware discovery engine `HardwareProbe` collecting remote system metrics (CPU cores, total/free RAM, GPU/VRAM presence, disk space, and OS architecture) over SSH or local command execution.
    - [x] `PlacementAdvisor` algorithm calculating multi-factor placement scores based on GPU requirements, RAM footprint, network locality, and workload balance (`pkg/topology/advisor/advisor.go`).
    - [x] CLI command `dendrite topology advisor [--nodes ...]` and `dendrite dev topology advisor` emitting recommended placement mappings and warnings (`cmd/dendrite/main.go`).
    - [x] Unit test suite achieving 88.7% statement coverage in `pkg/topology/advisor` (`pkg/topology/advisor/probe_test.go`, `pkg/topology/advisor/advisor_test.go`).

### Ticket `DEN-033`: Prometheus `/metrics` Exporter & Grafana Observability Dashboard Integration Pack

- **Target Package**: `pkg/telemetry/exporter`, `stacks/compose.grafana.yaml`
- **Status**: Completed [x]
- **Acceptance Criteria**:
    - [x] Standard Prometheus text exposition format (0.0.4) handler (`pkg/telemetry/exporter/exporter.go`).
    - [x] Exposes SoC die temperature, thermal warning/critical flags, 24h & 7d spend, budget ceiling, mesh node reachability, active agent sessions, and gateway request/fallback counters.
    - [x] Embedded HTTP server integration on `/metrics` and `/api/v1/telemetry/metrics` (`pkg/server/server.go`).
    - [x] CLI command `dendrite telemetry metrics` emitting Prometheus metrics.
    - [x] Automated Grafana datasource and dashboard provisioning pack (`stacks/prometheus/prometheus.yml`, `stacks/grafana/dashboards/dendrite_observability.json`, `stacks/compose.grafana.yaml`).
    - [x] Unit test suite achieving 99.2% statement coverage in `pkg/telemetry/exporter` and 88.4% in `pkg/server`.

### Ticket `DEN-030`: Multi-Mesh Networking Abstraction: Cloudflare Tunnels, SSH Port Forwarding, WireGuard, and Direct LAN

- **Target Package**: `pkg/topology/network`, `pkg/orchestrator`
- **Status**: Completed [x]
- **Acceptance Criteria**:
    - [x] Pluggable `NetworkProvider` interface with `TailscaleProvider`, `CloudflareProvider`, `SSHTunnelProvider`, `WireGuardProvider`, and `DirectLANProvider` (`pkg/topology/network/providers.go`).
    - [x] Cloudflare Zero Trust tunnel ingress configuration generator (`GenerateIngressConfig`).
    - [x] Dynamic and reverse SSH port-forwarding argument generator (`BuildForwardArgs`).
    - [x] Standalone WireGuard `wg-quick` configuration generator (`GenerateWGQuickConfig`).
    - [x] Network provider manager with dynamic provider switching (`Manager`).
    - [x] Updated cloud-init bootstrap in `pkg/orchestrator/cloudinit.go` and `pkg/orchestrator/types.go` supporting pluggable providers without forcing Tailscale.
    - [x] Unit test suite achieving 86.7% statement coverage in `pkg/topology/network` and 93.9% in `pkg/orchestrator`.

### Ticket `DEN-031`: Self-Hosted Plane v2 & Penpot Design Token Stacks & Replicator

- **Target Package**: `pkg/env`, `stacks`
- **Status**: Completed [x]
- **Acceptance Criteria**:
    - [x] Self-hosted Plane v2 compose stack (`stacks/compose.plane.yaml`) with PostgreSQL 16 and Redis 7.
    - [x] Self-hosted Penpot design stack (`stacks/compose.penpot.yaml`) with frontend, backend, PostgreSQL, and Redis.
    - [x] Compose generation and health probe checks in `pkg/env/plane.go` and `pkg/env/penpot.go`.
    - [x] Replicator and Planner integration supporting `--with-plane`, `--with-penpot`, and hybrid installation.
    - [x] Test suite achieving 89.0% statement coverage in `pkg/env` with zero warnings.

### Ticket `DEN-014`: Embedded HTTP Control Server & WebSocket Telemetry

- **Target Package**: `pkg/server`
- **Status**: Completed [x]
- **Acceptance Criteria**:
    - [x] REST API endpoints for project switching, scenario state, and token spend.
    - [x] WebSocket streaming channel (`/api/v1/telemetry/ws`) broadcasting real-time metrics.
    - [x] Server-Sent Events (SSE) streaming channel (`/api/v1/telemetry/events`) for unidirectional telemetry.
    - [x] Embedded static HTML dashboard serving for the web control plane.
    - [x] Unit and integration test suite achieving 87.6% statement coverage.

### Ticket `DEN-015`: Real-Time Telemetry Gauges & Thermal Monitoring Daemon

- **Target Package**: `pkg/telemetry`
- **Status**: Completed [x]
- **Acceptance Criteria**:
    - [x] Thermal sensor driver monitoring Apple Silicon SoC temperature via powermetrics and mock fallbacks.
    - [x] Spend budget provider tracking 24h and 7d consumption against daily financial ceilings.
    - [x] Multi-node health pinger for Debian, MacBook, Azure, and AWS compute nodes.
    - [x] Background telemetry sampling daemon with configurable sampling frequency and threshold alerts.
    - [x] Unit test suite achieving 94.4% statement coverage.

### Ticket `DEN-029`: Development Environment Replication, Host Installer & Containerized Tooling

- **Target Package**: `pkg/env`, `stacks`
- **Status**: Completed [x]
- **Acceptance Criteria**:
    - [x] Multi-mode host environment detector probing OS, architecture, package managers (brew, apt, dnf, pacman), and installed tooling (`pkg/env/detector.go`).
    - [x] Replication planner supporting Container mode, Direct Host mode, and Hybrid mode (`pkg/env/planner.go`).
    - [x] Containerized Linear issue tracking compose stack with dedicated PostgreSQL database and health probes (`stacks/compose.linear.yaml`, `pkg/env/linear.go`).
    - [x] Control plane compose stack (`stacks/compose.control.yaml`) and unified dev stack (`stacks/compose.dev.yaml`).
    - [x] Direct host installation generator for Linear CLI and developer tools across macOS and Linux.
    - [x] Replicator execution engine supporting dry-run verification, compose rendering, and real-time component auditing (`pkg/env/replicator.go`).
    - [x] CLI commands `dendrite env <doctor|plan|install|replicate|status>` wired into `cmd/dendrite` and root `main.go`.
    - [x] Comprehensive test suite achieving high statement coverage.
