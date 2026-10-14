---
id: FEAT-006
title: Ephemeral Throwaway Sandbox Execution & Dynamic Analysis
status: approved
plane_epic: PROJ-E-06
dependencies: [FEAT-001, FEAT-004]
repository: dendrite
tags: [sandbox, security, ephemeral, container, raii]
---

# FEAT-006: Ephemeral Throwaway Sandbox Execution & Dynamic Analysis

## 1. User Story & Business Objective
As a systems engineer analyzing potentially malicious, untrusted, or volatile third-party repositories and binaries, I need a completely ephemeral sandbox command that boots an isolated, network-restricted container, runs static/dynamic inspection or builds, and guarantees total resource reclamation (RAII / Drop) upon completion, leaving zero persistent artifacts, daemon processes, or file locks behind.

---

## 2. Technical Boundaries & Specifications

1. **Security & Sandboxing Hardening**:
   - `cap_drop: [ALL]` (all Linux capabilities dropped).
   - `security_opt: [no-new-privileges:true]`.
   - Read-only root filesystem (`--read-only`).
   - Isolated RAM tmpfs for scratch storage (`--tmpfs /tmp:rw,noexec,nosuid,size=1G`).
   - Network isolation: `--network none` by default; optional internal mesh egress only if fetching submodules.
2. **Ephemeral Lifecycle (RAII Semantics)**:
   - On command invocation (`dendrite sandbox run --ephemeral ...` or `dendrite analyze repo <url> --ephemeral`), the container is tagged with a UUID.
   - Upon process termination (success, failure, or `SIGINT`/`SIGTERM`), a runtime Drop/defer handler ensures:
     - Container is forcefully killed and removed (`docker rm -f`).
     - Temporary bind mounts or cloned disk caches are scrubbed.
     - Process exit codes are propagated transparently to caller.

```mermaid
sequenceDiagram
    autonumber
    actor Dev as Developer / Agent
    participant CLI as Dendrite CLI
    participant Daemon as Docker Daemon (Debian/Azure)
    participant Box as Ephemeral Sandbox (--read-only, cap-drop)

    Dev->>CLI: dendrite analyze repo <url> --ephemeral
    CLI->>Daemon: Create throwaway container & isolated tmpfs
    Daemon-->>Box: Boot container with dropped capabilities
    CLI->>Box: Execute Tree-sitter / ast-grep inspection
    Box-->>CLI: Stream structured JSON results / metrics
    Note over CLI,Box: RAII Drop Handler Triggered (Normal Exit or SIGINT)
    CLI->>Daemon: docker rm -f <container-id>
    Daemon-->>CLI: Confirm zero lingering state
    CLI-->>Dev: Present sanitized report to developer
```

---

## 3. Acceptance Criteria & Verification Matrix

- [x] **AC-1**: Any execution with `--ephemeral` guarantees that `docker ps -a` exhibits zero orphaned containers after execution.
- [x] **AC-2**: Interrupted runs via `SIGINT` (Ctrl+C) trigger the cleanup trap within $500\text{ms}$.
- [x] **AC-3**: Sandboxed containers cannot write to host directories outside the explicitly bound, ephemeral scratch volume.
- [x] **AC-4**: Zero root escalations or kernel privilege acquisitions are possible inside the sandbox.

---

## 4. Definition of Done (DoD) & Persistence Verification

- **Automated Tests**: Integration tests assert container cleanup upon simulated panic, `SIGINT`, and non-zero exit codes.
- **Persistent Layer Verification**: Confirms that host persistent stores (PostgreSQL, Docusaurus, Graphify DB) remain unaffected by untrusted code execution.
- **Complexity Guardrails**: Process management code uses Go RAII/`defer` patterns or Rust `Drop` without nested callback pyramids; cyclomatic complexity $\le 8$.
