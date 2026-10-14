# Dendrite Project Development Status & Roadmap

This document maintains the real-time development status, milestones, and active ticket execution pipeline for **Dendrite**, managed following the autonomous ticket lifecycle protocol.

---

## Current Status Overview

- **Active Milestone**: Milestone 11 — Pluggable Mesh Networking & Zero-Trust Connectivity (FEAT-015)
- **Current Focus**: `DEN-030: Multi-Mesh Networking Abstraction: Cloudflare Tunnels, SSH Port Forwarding, WireGuard, and Direct LAN`
- **Last Updated**: 2026-10-14
- **Overall Progress**: 29 / 33 Tickets Completed (87.9%)

---

## Milestone Execution Pipeline

### Milestone 11: Pluggable Mesh Networking & Zero-Trust Connectivity (FEAT-015) (CURRENT FOCUS)
- [ ] **DEN-030**: Multi-Mesh Networking Abstraction: Cloudflare Tunnels, SSH Port Forwarding, WireGuard, and Direct LAN (`pkg/topology/network`, `pkg/orchestrator`)

### Milestone 12: Open-Source Project Management & Design Tooling (FEAT-016)
- [x] **DEN-031**: Self-Hosted Plane v2 & Penpot Design Token Stacks & Replicator (`pkg/env`, `stacks/compose.plane.yaml`, `stacks/compose.penpot.yaml`)

### Milestone 13: Remote SSH Hardware Discovery & Automated Service Placement Router (FEAT-017)
- [ ] **DEN-032**: Remote SSH Node Topology Provisioner & Dynamic Service Placement Router (`pkg/topology/advisor`, `cmd/dendrite`)

### Milestone 14: Hybrid Telemetry Architecture & Prometheus/Grafana Observability Suite (FEAT-018)
- [ ] **DEN-033**: Prometheus `/metrics` Exporter & Grafana Observability Dashboard Integration Pack (`pkg/telemetry/exporter`, `stacks/compose.grafana.yaml`)

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

## Detailed Active Ticket Specification

### Ticket `DEN-030`: Multi-Mesh Networking Abstraction: Cloudflare Tunnels, SSH Port Forwarding, WireGuard, and Direct LAN

- **Target Package**: `pkg/topology`, `pkg/orchestrator`
- **Governing Specs**: `docs/features/FEAT-015-pluggable-mesh-networking.md`
- **Goal**: Decouple Dendrite from hardcoded Tailscale endpoints by creating a pluggable network provider interface supporting Cloudflare Zero Trust Tunnels (`cloudflared`), dynamic/multiplexed SSH port forwarding, standalone WireGuard, and Direct LAN/Localhost (zero-overlay).
- **Acceptance Criteria**:
  - [ ] Pluggable `NetworkProvider` interface (`TailscaleProvider`, `CloudflareProvider`, `SSHTunnelProvider`, `DirectLANProvider`).
  - [ ] Support `cloudflared` tunnel ingress configuration generation for control plane and telemetry ports.
  - [ ] Implement SSH port-forwarding and reverse-tunnel manager for instant zero-dependency remote node routing.
  - [ ] Update cloud-init and compose templates to conditionally bootstrap the chosen network provider without forcing Tailscale.
  - [ ] Table-driven unit test suite achieving $\ge 85\%$ statement coverage.

---

## Completed Ticket Archive

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
