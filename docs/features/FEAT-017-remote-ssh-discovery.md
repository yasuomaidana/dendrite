---
id: FEAT-017
title: Remote SSH Hardware Discovery & Automated Service Placement Router
status: approved
plane_epic: PROJ-E-01
dependencies: [FEAT-001, FEAT-015]
repository: dendrite
tags: [topology, ssh, hardware-discovery, placement-advisor, router, multi-node]
---

# FEAT-017: Remote SSH Hardware Discovery & Automated Service Placement Router

## 1. User Story & Business Objective
As a systems engineer managing heterogeneous multi-node infrastructure (e.g. MacBook Air, local Debian server, Azure Spot VMs, and AWS instances), I need an automated remote hardware discovery and placement advisor engine that probes compute nodes over SSH, profiles available CPU, RAM, GPU/VRAM, and storage, and automatically scores and routes Dendrite service components (Ollama Metal, LiteLLM, REA Hopper/Ghidra, Plane v2, Penpot, Postgres/Redis) to the most optimal host, so that GPU-intensive inference runs on Metal/CUDA nodes, persistent databases run on reliable server storage, and no single workstation suffers thermal degradation.

---

## 2. Technical Boundaries & Specifications

1. **Hardware Discovery (`HardwareProbe`)**:
   - Executes remote non-intrusive metric commands over SSH or local shell.
   - Probes:
     - Operating System (`darwin`, `linux`) and architecture (`arm64`, `amd64`).
     - CPU logical cores (`sysctl -n hw.ncpu` or `nproc`).
     - RAM capacity: Total RAM and currently Free/Available RAM in MB (`/proc/meminfo` or `sysctl -n hw.memsize` + `vm_stat`).
     - Disk free space in GB (`df -k`).
     - GPU detection: Apple Silicon Metal GPU, NVIDIA CUDA (`nvidia-smi`), or CPU-only.
     - Network latency: ICMP/TCP ping latency or SSH round-trip latency.
   - Pluggable `CommandExecutor` interface (`SSHExecutor`, `LocalExecutor`, `MockExecutor` for deterministic testing).

2. **Placement Scoring Algorithm (`PlacementAdvisor`)**:
   - Hard Eligibility Filters:
     - GPU requirement: Services requiring Metal or CUDA are disqualified from nodes lacking respective accelerators.
     - OS constraint: Native Mach-O REA tools require Darwin (`macOS`); Linux ELF tools require Linux.
     - Minimum RAM / Disk requirements: Nodes without sufficient headroom are filtered out.
   - Multi-Criteria Soft Scoring:
     - **Resource Margin (40%)**: Ratio of available RAM and CPU cores to requirement.
     - **Hardware Acceleration Match (30%)**: Dedicated VRAM and GPU compute suitability.
     - **Network Locality & Latency (20%)**: Prefers local execution for interactive low-latency CLI/tools; prefers control-plane node for persistent daemons.
     - **Workload Balance (10%)**: Penalizes nodes that are already saturated with multiple heavy workloads.
   - Emits a structured `PlacementPlan` with recommended node assignments, rationales, resource warnings, and ready-to-use `TopologyConfig`.

3. **CLI Integration (`cmd/dendrite`)**:
   - Subcommand: `dendrite topology advisor [--nodes <list>] [--json]`
   - Emits tabular breakdown of discovered node hardware and optimal service placements.

---

## 3. Acceptance Criteria & Verification Matrix

- [x] **AC-1**: `HardwareProbe` collects system metrics (CPU, RAM, GPU, Disk, OS, Arch) via SSH and local command execution with zero external agent dependencies.
- [x] **AC-2**: `PlacementAdvisor` calculates multi-factor placement scores and assigns services (Ollama Metal, LiteLLM, REA Hopper/Ghidra, Plane, Penpot, Database) to eligible nodes.
- [x] **AC-3**: CLI subcommand `dendrite topology advisor` outputs recommendations and warnings.
- [x] **AC-4**: Table-driven unit test suite achieving $\ge 85\%$ statement coverage without requiring live SSH connections.
