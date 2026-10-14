# Dendrite Project Development Status & Roadmap

This document maintains the real-time development status, milestones, and active ticket execution pipeline for **Dendrite**, managed following the autonomous ticket lifecycle protocol.

---

## Current Status Overview

- **Active Milestone**: Milestone 6 — AI-Assisted Token Optimization & Cleanroom Import (FEAT-010)
- **Current Focus**: `DEN-019: Cross-Repository Capability Dissection & Cleanroom Synthesizer (pkg/ai/import)`
- **Last Updated**: 2026-10-14
- **Overall Progress**: 15 / 28 Tickets Completed (53.6%)

---

## Milestone Execution Pipeline

### Milestone 10: IDE Intelligence & JetBrains MCP Bridge (FEAT-014) (TOP 1 PRIORITY)
- [x] **DEN-028**: JetBrains IDE MCP Integration & Communication Bridge (`pkg/jetbrains`)

### Milestone 1: Core Foundation, Developer Tools & Ollama Engine
- [x] **DEN-001**: Initialize Development Status Ledger & Execution Roadmap (`DEV_STATUS.md`)
- [x] **DEN-002**: Implement Idiomatic Ollama Client & Metal Engine Support (`pkg/ollama`)
- [x] **DEN-003**: Developer Productivity Tools on the Fly & CLI Scaffold (`pkg/devtool`, `cmd/dendrite`)
- [x] **DEN-004**: Ticket Module & Local Status Synchronization Engine (`pkg/tracker`)

### Milestone 2: Multi-Node Topology & Cloud-Burst Infrastructure (FEAT-001, FEAT-007)
- [x] **DEN-005**: Topology Configuration Model & Scenario Placement Types (`pkg/topology`)
- [x] **DEN-006**: Terraform Cloud-Init & Compose Profile Runner (`pkg/orchestrator`)
- [x] **DEN-016**: Terraform AWS Spot EC2 Module & Cloud-Init Bootstrap (`infra/modules/aws-burst`)
- [x] **DEN-017**: Persistent EBS Volume Lifecycler & Zero-Ingress Security Group Validator (`pkg/orchestrator/aws`)

### Milestone 3: Token Budget Governance & Fallback Gateway (FEAT-002)
- [x] **DEN-007**: LiteLLM Configuration Template & Dynamic Renderer (`pkg/gateway`)
- [x] **DEN-008**: Sliding Window Budget Limiter & Ollama Failover Engine (`pkg/gateway`)

### Milestone 4: Autonomous Coding Agent Harness & Tool Orchestration (FEAT-013)
- [x] **DEN-024**: Agent Runner Harness & Environment Injector for Aider, OpenHands, and OpenCode (`pkg/agent`)
- [x] **DEN-025**: Agent Sandbox Isolation & LiteLLM Gateway Budget Binding (`pkg/agent/runner`)
- [x] **DEN-026**: Agent Session Tracker, Git Commit Provenance & Issue Tracker Sync (`pkg/agent/session`)

### Milestone 5: Reverse Engineering & Sandboxed Intelligence (FEAT-003, FEAT-004, FEAT-006)
- [x] **DEN-009**: REA MCP Bridge & Disassembler Protocol Client (`pkg/rea`)
- [x] **DEN-010**: Tree-sitter AST & Complexity Analyzer (`pkg/ast`)
- [x] **DEN-011**: Ephemeral Hardened Container Sandbox with RAII Drop (`pkg/sandbox`)

### Milestone 6: AI-Assisted Token Optimization & Cleanroom Import (FEAT-010)
- [x] **DEN-018**: AST Context Compaction & LiteLLM Prefix Cache Harness (`pkg/ai/tokenopt`)
- [x] **DEN-019**: Cross-Repository Capability Dissection & Cleanroom Synthesizer (`pkg/ai/import`)

### Milestone 7: Polyglot Translation & Documentation Reflection (FEAT-011, FEAT-012)
- [x] **DEN-020**: Intermediate Semantic Representation ISR Tree-sitter Parser (`pkg/ai/translate`)
- [x] **DEN-021**: Idiomatic Target Code Synthesizer & Dual Property Test Parity Generator (`pkg/ai/translate/verify`)
- [x] **DEN-022**: AST Docs-as-Code Synthesizer for FEAT/ADR Specs & OpenAPI 3.1 (`pkg/docgen`)
- [ ] **DEN-023**: Cross-Project Documentation Reflection Engine & Docusaurus Sync (`pkg/docgen/reflect`) *(IN PROGRESS)*

### Milestone 8: Multi-Project Workspaces & Pluggable Trackers (FEAT-005, FEAT-008)
- [ ] **DEN-012**: Multi-Project Workspace Registry & Context Switcher (`pkg/workspace`)
- [ ] **DEN-013**: Pluggable Issue Tracker Interface & Atlassian Jira Adapter (`pkg/tracker/jira`)
- [ ] **DEN-027**: Plane v2 & Linear Strategy Adapters (`pkg/tracker/plane`)

### Milestone 9: Web UI & Dashboard Engine (FEAT-009)
- [ ] **DEN-014**: Embedded HTTP Control Server & WebSocket Telemetry (`pkg/server`)
- [ ] **DEN-015**: Real-Time Telemetry Gauges & Thermal Monitoring Daemon (`pkg/telemetry`)

---

## Detailed Active Ticket Specification

### Ticket `DEN-019`: Cross-Repository Capability Dissection & Cleanroom Synthesizer

- **Target Package**: `pkg/ai/import`
- **Governing Specs**: [`docs/features/FEAT-010-ai-assisted-token-optimization.md`](docs/features/FEAT-010-ai-assisted-token-optimization.md)
- **Goal**: Ingest external or legacy repositories, dissect capabilities into cleanroom semantic envelopes, and re-synthesize them adhering to target project architectures.
- **Acceptance Criteria**:
  - [ ] Multi-repo symbol and signature harvester.
  - [ ] Semantic cleanroom envelope generator eliminating vendor lock-in.
  - [ ] Target architecture synthesizer integrating with `pkg/ai/translate`.
  - [ ] Unit and property tests verifying cleanroom equivalence.

---

## Completed Ticket Archive

### Ticket `DEN-028`: JetBrains IDE MCP Integration & Communication Bridge (TOP 1 Priority)
- **Target Package**: `pkg/jetbrains`
- **Status**: Completed [x]
- **Acceptance Criteria**:
  - [x] Implement JSON-RPC 2.0 MCP protocol types, message schemas, and error envelopes (`pkg/jetbrains/types.go`).
  - [x] Design transport abstraction (`Transport` interface) supporting Stdio execution, HTTP/SSE, and Mock in-memory test transport (`pkg/jetbrains/transport.go`).
  - [x] Implement robust `Client` handling `initialize` handshake, request sequencing, timeouts, and `tools/list` / `tools/call` invocations (`pkg/jetbrains/client.go`).
  - [x] Build high-level `Bridge` providing typed helper functions: `OpenFile`, `GetFileProblems`, `SearchSymbol`, `BuildProject`, `ExecuteRunConfiguration`, `LintFiles` (`pkg/jetbrains/bridge.go`).
  - [x] Integrate JetBrains CLI subcommands (`dendrite jetbrains probe`, `open`, `problems`, `symbols`, `build`, `run`) in `cmd/dendrite` and root `main.go`.
  - [x] Unit test suite achieving 86.5% statement coverage with zero failures and low complexity.

### Ticket `DEN-016`: Terraform AWS Spot EC2 Module & Cloud-Init Bootstrap
- **Target Package**: `infra/modules/aws-burst`
- **Status**: Completed [x]
- **Acceptance Criteria**:
  - [x] Terraform AWS module defining `aws_spot_instance_request` / `aws_instance` with `hashicorp/aws` (~> 5.0).
  - [x] Cloud-init template installing Docker 26+, Tailscale, and bootstrapping Tailnet join.
  - [x] Zero public ingress rules (`0.0.0.0/0:22` prohibited; strictly private mesh or outbound-only).
  - [x] Automated validation via `terraform validate` and integration test mock.

### Ticket `DEN-026`: Agent Session Tracker, Git Commit Provenance & Issue Tracker Sync
- **Target Package**: `pkg/agent/session`
- **Status**: Completed [x]
- **Acceptance Criteria**:
  - [x] Persist agent session records, execution duration, and token spend into local store.
  - [x] Parse generated Git commits and extract ticket tags to push comments/status updates to active issue tracker (`pkg/tracker`).
  - [x] Add session ledger query capabilities and JSON export.
  - [x] Unit test suite achieving 96.4% statement coverage.

### Ticket `DEN-025`: Agent Sandbox Isolation & LiteLLM Gateway Budget Binding
- **Target Package**: `pkg/agent/runner`
- **Status**: Completed [x]
- **Acceptance Criteria**:
  - [x] Connect agent runner to `pkg/gateway` budget limiter to poll real-time virtual key spend.
  - [x] Execute agent commands inside ephemeral, cap-dropped container sandbox with deferred teardown traps (`defer session.Close()`).
  - [x] Ensure non-zero exit codes and curfew interrupts propagate without orphaned containers.
  - [x] Unit test suite achieving 94.5% statement coverage.

### Ticket `DEN-024`: Agent Runner Harness & Environment Injector for Aider, OpenHands, and OpenCode
- **Target Package**: `pkg/agent`
- **Status**: Completed [x]
- **Acceptance Criteria**:
  - [x] Model and validate `AgentConfig` supporting `aider`, `openhands`, and `opencode`.
  - [x] Environment variable generator binding LiteLLM gateway credentials and token limits.
  - [x] Argument list builder supporting tickets, model overrides, map tokens, and ephemeral sandbox modes.
  - [x] Standard commit message formatter with ticket ID and agent provenance tags.
  - [x] Unit test suite achieving 100% statement coverage.

### Ticket `DEN-011`: Ephemeral Hardened Container Sandbox with RAII Drop
- **Target Package**: `pkg/sandbox`
- **Status**: Completed [x]
- **Acceptance Criteria**:
  - [x] Enforce `--read-only`, `--cap-drop ALL`, and isolated tmpfs for container executions.
  - [x] Guarantee RAII Drop container cleanup via deferred `session.Close()` during normal termination, non-zero exits, or `context.Canceled`.
  - [x] Unit test suite achieving 92.1% statement coverage.

### Ticket `DEN-010`: Tree-sitter AST & Complexity Analyzer
- **Target Package**: `pkg/ast`
- **Status**: Completed [x]
- **Acceptance Criteria**:
  - [x] Tree-sitter AST symbol extractor and language grammar detection.
  - [x] Compute cyclomatic and cognitive complexity metrics.
  - [x] Unit test suite achieving 99.0% statement coverage.

### Ticket `DEN-009`: REA MCP Bridge & Disassembler Protocol Client
- **Target Package**: `pkg/rea`
- **Status**: Completed [x]
- **Acceptance Criteria**:
  - [x] Hopper and Ghidra disassembler protocol bridge models and transport.
  - [x] Decompilation dispatch with timeout handling.
  - [x] Unit test suite achieving 94.7% statement coverage.

### Ticket `DEN-008`: Sliding Window Budget Limiter & Ollama Failover Engine
- **Target Package**: `pkg/gateway`
- **Status**: Completed [x]
- **Acceptance Criteria**:
  - [x] Sliding window rate limiter for 24h and 7d financial windows.
  - [x] Dynamic failover routing to local Ollama Metal endpoint on HTTP 429 quota exhaustion.
  - [x] Rate-bounded throttling calculation adhering to supervisory formula ($R_{\text{target}}$).
  - [x] Unit tests achieving 94.1% statement coverage.

### Ticket `DEN-007`: LiteLLM Configuration Template & Dynamic Renderer
- **Target Package**: `pkg/gateway`
- **Status**: Completed [x]
- **Acceptance Criteria**:
  - [x] Render dynamic `litellm_config.yaml` based on active topology scenario.
  - [x] Configure upstream routes for `gemini-2.5-pro`, `copilot-sonnet`, `gemini-2.5-flash`, and `local-qwen-coder`.
  - [x] Enforce zero-cost token exemption for local Ollama instances.
  - [x] Unit tests achieving 94.1% statement coverage.

### Ticket `DEN-006`: Terraform Cloud-Init & Compose Profile Runner
- **Target Package**: `pkg/orchestrator`
- **Status**: Completed [x]
- **Acceptance Criteria**:
  - [x] Cloud-init YAML generator for Tailscale auto-join and Docker daemon bootstrapping.
  - [x] Compose profile runner generating arguments for `docker compose --profile <profile> up -d`.
  - [x] Topology-driven orchestrator tying `pkg/topology` to node deployment commands.
  - [x] Unit tests achieving 94.2% statement coverage.

### Ticket `DEN-005`: Topology Configuration Model & Scenario Placement Types
- **Target Package**: `pkg/topology`
- **Status**: Completed [x]
- **Acceptance Criteria**:
  - [x] Support scenario enum with validation: `local-hybrid`, `azure-burst`, `aws-burst`, `distributed`.
  - [x] Map compute nodes (`Debian`, `MacBook`, `Azure`, `AWS`) to stack components.
  - [x] Provide YAML/JSON serialization and deserialization for scenario configurations.
  - [x] Unit tests achieving 94.5% statement coverage.
