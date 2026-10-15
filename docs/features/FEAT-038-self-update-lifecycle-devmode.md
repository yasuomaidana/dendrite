---
id: FEAT-038
title: "Self-Update Lifecycle & Developer Mode Dual-Source Engine"
status: approved
plane_epic: PROJ-E-16
dependencies: [FEAT-034, FEAT-035]
repository: dendrite
tags: [self-update, dev-mode, binary-swap, rollback, sha256, local-build]
---

# FEAT-038: Self-Update Lifecycle & Developer Mode Dual-Source Engine

## 1. Executive Summary & Problem Statement

### The Problem
CLI tools operating as autonomous orchestrators must be easily updateable when new capabilities, MCP tools, or security fixes are released. Manually pulling git repositories, rebuilding binaries, and replacing paths in `/usr/local/bin` is error-prone. If an update downloads a corrupt binary or fails checksum validation, the developer's entire orchestration pipeline breaks without an automatic rollback mechanism.

Furthermore, contributors developing Dendrite need a seamless **Developer Mode** (`--dev`). In developer mode, invoking `dendrite update` must default to building and installing from the local working repository instead of overwriting local modifications with remote published releases, while retaining an explicit override flag (`--remote`) when desired.

### User Story & Architectural Pillars
As a developer and contributor to Dendrite, I need `dendrite update` to query releases, verify SHA-256 checksums, and perform atomic in-place binary swaps with rollback safeguards, and I need `dendrite update --dev` to compile and install from the local checked-out Go source tree by default, complete with clear documentation in `docs/DEVELOPMENT.md`.

```mermaid
flowchart TD
    Invoker["dendrite update [flags]"] --> CheckMode{"Is --dev flag set?"}

    subgraph "Production Self-Update (Epic 4)"
        CheckMode -->|No| RemoteQuery["Query GitHub Releases API<br/>(yasuomaidana/dendrite)"]
        RemoteQuery --> Download["Download Platform Binary & .sha256"]
        Download --> Verify["Verify SHA-256 Checksum"]
        Verify -->|Valid| Backup["Backup active binary to .dendrite/bin/dendrite.bak"]
        Backup --> Swap["Atomic In-Place Binary Swap"]
        Swap --> SmokeTest["Execute ./dendrite --version smoke test"]
        SmokeTest -->|Success| Clean["Remove backup & complete"]
        SmokeTest -->|Failure| Rollback["Atomic Rollback from .bak"]
    end

    subgraph "Developer Mode (Epic 5)"
        CheckMode -->|Yes| DevSource{"--source flag"}
        DevSource -->|local (default)| LocalBuild["Execute local Go toolchain:<br/>go build -o /tmp/dendrite cmd/dendrite/main.go"]
        DevSource -->|remote| RemoteQuery
        LocalBuild --> DevSwap["Atomic swap into $GOPATH/bin or active binary path"]
        DevSwap --> DevDocs["Verify docs/DEVELOPMENT.md alignment"]
    end
```

---

## 2. Technical Architecture & Idiomatic Design Patterns

### 2.1 Atomic Binary Swap & RAII Rollback Guard
Replacing an active executable on Unix systems requires renaming the existing inode rather than overwriting in place (to avoid `text file busy` errors):
1. **Stage**: Download release binary to `dendrite.tmp` in the same filesystem directory.
2. **Verify**: Compute crypto SHA-256 and assert match against release checksum manifest.
3. **Backup**: `os.Rename(currentExePath, currentExePath + ".bak")`.
4. **Swap**: `os.Rename(tempPath, currentExePath)`.
5. **Rollback Guard**: If smoke verification (`exec(currentExePath, "--version")`) fails or panics, the defer block automatically restores `currentExePath.bak` over `currentExePath`.

### 2.2 Developer Mode (`--dev`) Resolution Matrix
When running in developer mode:
- **Default (`--dev` or `--dev --source local`)**: Locates root `go.mod`, compiles `cmd/dendrite`, runs unit tests, and copies binary to active install path.
- **Remote Override (`--dev --source remote`)**: Prompts confirmation before fetching remote GitHub Release so local work is never destroyed accidentally.

---

## 3. Behavioral Scenarios (Gherkin)

### Scenario 4A: Release Query, Verification & In-Place Binary Swap
- **Given** a new version `v1.4.0` is published on GitHub Releases with a valid SHA-256 signature
- **When** the user executes `dendrite update`
- **Then** Dendrite queries the release, verifies the SHA-256 checksum, backs up the current executable, swaps the binary in-place, and prints confirmation.

### Scenario 4B: Corrupted Binary Rollback Safeguard
- **Given** a network disruption corrupts the downloaded binary update payload
- **When** the checksum verification fails or the swapped binary fails the initial smoke test
- **Then** Dendrite immediately aborts the update, restores the original binary from backup, and logs a descriptive error.

### Scenario 5A: Dev-Mode Local Source Build as Default
- **Given** a contributor is working inside a cloned Dendrite repository with active local edits
- **When** the developer executes `dendrite update --dev`
- **Then** Dendrite builds the binary directly from the local repository source, installs the fresh build, and prints developer debug info.

### Scenario 5B: Remote Override in Dev Mode & Developer Guide Verification
- **Given** developer mode is active and the developer wants to test a public remote candidate
- **When** the developer executes `dendrite update --dev --source remote`
- **Then** Dendrite prompts for confirmation, fetches the remote binary, and references instructions documented in `docs/DEVELOPMENT.md`.

---

## 4. Acceptance Criteria & Definition of Done

- [ ] **AC-1 (Atomic Production Update)**: `dendrite update` downloads, verifies SHA-256, swaps binary atomically, and handles rollback on failure.
- [ ] **AC-2 (Developer Mode Default)**: `dendrite update --dev` builds from local Go source tree by default without pulling remote.
- [ ] **AC-3 (Remote Override in Dev Mode)**: Supports `--source remote` flag with confirmation prompt.
- [ ] **AC-4 (Documentation Parity)**: Complete developer guide maintained in `docs/DEVELOPMENT.md`.
- [ ] **AC-5 (DoD & Complexity Thresholds)**: Cross-platform tests for macOS/Linux; cyclomatic complexity $\le 15$; cognitive complexity $\le 20$; coverage $\ge 85\%$.
