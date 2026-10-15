---
id: FEAT-035
title: "Multi-Provider AI Runtime & 5-Tier Semantic Hierarchical Git Governance"
status: approved
plane_epic: PROJ-E-16
dependencies: [FEAT-013, FEAT-020, FEAT-028, FEAT-029]
repository: dendrite
tags: [agent-runtime, git-governance, copilot, ollama, semantic-merging, autonomy-levels, gh-cli]
---

# FEAT-035: Multi-Provider AI Runtime & 5-Tier Semantic Hierarchical Git Governance

## 1. Executive Summary & Problem Statement

### The Problem
Autonomous coding agents (Aider, OpenHands, OpenCode, Google Antigravity / AGY) execute shell commands with varying degrees of agency. When agents are granted unconstrained access to Git and GitHub CLI (`gh`), they produce destructive operations: committing incomplete patches directly to `main`, bypassing CI, creating messy branch topologies, and executing uncoordinated merges across unrelated milestones. Conversely, locking agents in pure read-only sandboxes cripples developer velocity by requiring manual branching, committing, and PR crafting for every trivial task.

Furthermore, existing agent orchestrators often depend on heavyweight external wrappers or third-party proxy bridges to talk to local LLMs, adding latency and failure points when interfacing with local Ollama instances or GitHub Copilot.

### User Story & Architectural Pillars
As an engineering director and autonomous systems architect, I need `dendrite agent` to provide a unified multi-provider execution backend (GitHub CLI `gh`, GitHub Copilot CLI/API, and high-performance native Ollama HTTP streaming), governed by a deterministic **5-Tier Git Version Governance Policy Engine** that validates every Git command before execution and enforces **Semantic Hierarchical Branching and Merging** (`milestone/*` $\to$ `story/*` $\to$ `feature/*` $\to$ level-by-level merge).

```mermaid
flowchart TD
    subgraph "Multi-Provider AI Runtime"
        Invoker["dendrite agent run --story <id>"] --> Probe{"Provider Discovery"}
        Probe -->|gh available| Copilot["GitHub Copilot Engine<br/>(gh copilot / API)"]
        Probe -->|OLLAMA_HOST reachable| OllamaNative["Native Direct Ollama Driver<br/>(HTTP streaming /api/chat)"]
        Probe -->|Third-party tool| OllamaBridge["Compatibility Proxy Bridge<br/>(OpenAI/Copilot adapter)"]
    end

    Copilot & OllamaNative & OllamaBridge --> PolicyGuard{"5-Tier Version Governance Interceptor"}

    subgraph "5-Tier Autonomy Matrix"
        PolicyGuard -->|L1: conservative| L1["Inspect diffs only; commits & PRs strictly blocked"]
        PolicyGuard -->|L2: super-safe| L2["Isolated agent/* branch; local commits allowed; no merges or PRs"]
        PolicyGuard -->|L3: safe| L3["Feature sub-branches merge to story; strict 1:1 PR to main; no auto-merge"]
        PolicyGuard -->|L4: normal| L4["Semantic hierarchical merging: feature -> story -> milestone; aggregated PR"]
        PolicyGuard -->|L5: yolo| L5["Full autonomy; auto-merge after test gate passes"]
    end

    subgraph "Semantic Hierarchical Merging"
        MB["milestone/<id>"] --> SB["story/<id>"]
        SB --> FB["feature/<id>"]
        FB -->|Merge Level 1| SB
        SB -->|Merge Level 2| MB
        MB -->|Merge Level 3 (PR)| MainTrunk["main branch"]
    end
```

---

## 2. Technical Architecture & Idiomatic Design Patterns

### 2.1 The 5-Tier Autonomy Governance Matrix
The policy engine intercepts every `git` and `gh` execution at the operating system process boundary:

| Capability / Git Operation | Level 1: `conservative` | Level 2: `super-safe` | Level 3: `safe` | Level 4: `normal` | Level 5: `yolo` |
|:---|:---|:---|:---|:---|:---|
| **Modify working tree / stage files** | Allowed (human review before commit) | Allowed on agent branches | Allowed on agent branches | Allowed on agent branches | Allowed everywhere |
| **Create commits** | Blocked (human commits or patch export) | Allowed on agent branches | Allowed on agent branches | Allowed on agent branches | Allowed everywhere |
| **Create branches** (`agent/*`, `story/*`, `feature/*`) | Blocked | Allowed (`agent/*`) | Allowed (`agent/*`, `story/*`, `feature/*`) | Allowed (hierarchical) | Allowed |
| **Merge between non-`main` branches** | Blocked | Blocked | Allowed (strictly within single story scope) | Allowed (strictly level-by-level) | Allowed |
| **PR Granularity Constraint** | N/A | N/A | Strict 1:1 (1 Story = 1 PR) | Aggregated N:1 (Milestone = 1 PR) | Configurable / Unconstrained |
| **Create Pull Requests** (`gh pr create`) | Blocked | Blocked | Allowed (story $\to$ `main`) | Allowed (milestone $\to$ `main`) | Allowed |
| **Merge Pull Requests** (`gh pr merge`) | Blocked | Blocked | Blocked (Human merges) | Blocked (Human merges) | Allowed (Auto-merge after CI passes) |
| **Direct push/merge to `main` / `master`** | Blocked | Blocked | Blocked | Blocked | Allowed (fast-forward or PR auto-merge) |

### 2.2 Semantic Hierarchical Multi-Level Branching & Merging
When merges are permitted (Level 3, Level 4, Level 5), branch operations must strictly follow semantic hierarchy:
1. **Milestone Integration Branch**: When starting a milestone, create `milestone/<milestone-id>` from `main`.
2. **Story Branches**: When beginning a story, create `story/<story-id>` or `agent/<story-id>` branched off `milestone/<milestone-id>`.
3. **Feature Branches**: Granular sub-tasks branch as `feature/<story-id>-<component>` off `story/<story-id>`.
4. **Level-by-Level Merging**:
   - **Level 1**: `feature/*` branches merge into their parent `story/*` branch.
   - **Level 2**: Completed `story/*` branches merge into their parent `milestone/*` branch.
   - **Level 3**: The consolidated `milestone/*` branch opens a single PR against `main`. Direct bypass of intermediate branches is blocked.

### 2.3 Typestate & Process Interceptor Architecture
The governance engine models command execution using Go typed interceptors and typestates:
```go
type AutonomyLevel int

const (
    AutonomyConservative AutonomyLevel = iota + 1 // Level 1
    AutonomySuperSafe                             // Level 2
    AutonomySafe                                  // Level 3
    AutonomyNormal                                // Level 4
    AutonomyYOLO                                  // Level 5
)

type GitCommandInterceptor struct {
    level        AutonomyLevel
    activeScope  BranchScope
    currentStory string
    milestone    string
}

func (g *GitCommandInterceptor) ValidateExecution(cmd string, args []string) error {
    // Intercept git commit, checkout -b, merge, push, gh pr create, gh pr merge
    ...
}
```

---

## 3. Behavioral Scenarios (Gherkin)

### Scenario 1A: Provider Auto-Negotiation with Direct Ollama Fallback
- **Given** the user invokes `dendrite agent run --story PROJ-42`
- **When** Dendrite probes the runtime environment and detects `gh` is authenticated and `ollama serve` is active at `127.0.0.1:11434`
- **Then** Dendrite enables `gh` for repository operations and allows selecting either `copilot` or `ollama:<model>` (via direct native HTTP or bridged mode) without requiring external wrapper scripts.

### Scenario 1B: Level 1 (`conservative`) Human-in-the-Loop Gate
- **Given** the autonomy mode is set to `conservative`
- **When** the agent finishes implementing changes for a task
- **Then** the changes remain staged/unstaged in the working tree (or exported to `.dendrite/patches/`), an interactive diff summary is presented, and any attempt by the agent to run `git commit`, `git checkout -b`, or `gh pr create` is rejected by the policy guard.

### Scenario 1C: Level 2 (`super-safe`) Branch Isolation
- **Given** the autonomy mode is set to `super-safe`
- **When** the agent begins work on story `PROJ-101`
- **Then** it automatically creates and switches to branch `agent/PROJ-101`, commits changes locally or pushes the feature branch to `origin/agent/PROJ-101`, and halts before any `git merge` or `gh pr create` command.

### Scenario 1D: Level 3 (`safe`) Strict 1:1 Story-to-PR Enforcement
- **Given** the autonomy mode is set to `safe`
- **When** the agent completes story `PROJ-102` using helper sub-branches `agent/PROJ-102-part1` and `agent/PROJ-102-part2`
- **Then** the agent is permitted to merge those sub-branches into `agent/PROJ-102` and execute `gh pr create --base main --head agent/PROJ-102`, but is blocked from bundling commits from `PROJ-103` into the same branch/PR and blocked from executing `gh pr merge`.

### Scenario 1E: Level 4 (`normal`) Semantic Hierarchical Multi-Story Consolidation
- **Given** the autonomy mode is set to `normal`
- **When** stories `PROJ-104` and `PROJ-105` belong to milestone `M1-Auth`
- **Then** the agent merges helper `feature/*` branches into their respective `story/PROJ-104` and `story/PROJ-105` branches, merges both story branches into the integration branch `milestone/M1-Auth`, and opens a consolidated PR against `main`, while remaining strictly blocked from merging that PR into `main`.

---

## 4. Acceptance Criteria & Definition of Done

- [ ] **AC-1 (Multi-Provider Runtime)**: Supports GitHub Copilot CLI/API and native direct Ollama HTTP streaming (`/api/chat`, `/api/generate`) with zero wrapper overhead.
- [ ] **AC-2 (5-Tier Version Governance)**: Deterministically validates every `git` and `gh` execution against the active autonomy level (`conservative`, `super-safe`, `safe`, `normal`, `yolo`).
- [ ] **AC-3 (Semantic Hierarchical Branching & Merging)**: Enforces `milestone/*` $\to$ `story/*` $\to$ `feature/*` creation and level-by-level merge gating, blocking orphan cross-level PRs.
- [ ] **AC-4 (DoD & Complexity Thresholds)**: Unit and integration tests achieve $\ge 85\%$ statement coverage; cyclomatic complexity $\le 15$ per function; cognitive complexity $\le 20$.
