---
id: FEAT-098
title: "Autonomous Git Commit Engine with Local Model Message Delegation & Zero-Cost Synthesis"
status: approved
plane_epic: PROJ-E-30
dependencies: [FEAT-024, FEAT-026, FEAT-040, FEAT-050]
repository: dendrite
tags: [git, commit, local-models, qwen, ollama, mcp, zero-cost, automation]
---

# FEAT-098: Autonomous Git Commit Engine with Local Model Message Delegation & Zero-Cost Synthesis

## 1. Executive Summary & Problem Statement

### The Problem
During development iterations and autonomous agent lifecycles, creating structured git commits requires:
1. Synthesizing conventional commit messages following strict project naming rules (`[<TICKET_ID>] <type>(<scope>): <summary>`).
2. Staging modified and untracked files without accidentally committing temporary artifacts, scratch files, or sensitive environment credentials.
3. Tracking commit provenance and execution metrics.

Previously, git commits were either manual or performed by cloud-hosted agents (AGY, Gemini Pro, Claude 3.5 Sonnet) consuming metered cloud tokens for mundane commit message generation. This violated the project's **Token Economy & Delegation Matrix**, which mandates that repetitive tasks and chores should be offloaded to zero-cost local models (`local-qwen3-27b` / Ollama Metal) running with < 15ms latency and $0.00 cost.

### The Solution
FEAT-098 introduces a first-class autonomous Git commit engine in Dendrite:
- **Local Model Delegation Directive**: Automatically routes commit message generation to local Ollama / LiteLLM engines (`local-qwen3-27b` / `qwen3.8:27b` / `local-qwen-coder`) when available, synthesizing Conventional Commits with zero cloud API token spend.
- **Unified CLI Command (`dendrite commit`)**: Offers flags `--ticket`, `--message`, `--all`, `--files`, `--model`, `--local`, and `--dry-run`.
- **Native MCP Tool (`dendrite_commit`)**: Enables autonomous agents and MCP clients (e.g. JetBrains, Cline, Antigravity) to stage and commit directly through standard JSON-RPC.
- **Fail-Safe Fallback**: If local model endpoints are unavailable or timeout, falls back gracefully to deterministic ticket-bound conventional messages without blocking the developer workflow.

---

## 2. Technical Architecture & Component Design

```
flowchart TD
    subgraph "Git Working Directory"
        Files["Modified & Untracked Files"]
        Diff["Staged Git Diff / Stat"]
    end

    subgraph "Dendrite Commit Engine (pkg/devtool)"
        CommitRunner["Commit Engine & Stage Manager"]
        DiffExtractor["Diff Extractor & Token Truncator"]
        MessageDelegator["Local Model Message Delegator"]
    end

    subgraph "Zero-Cost Local Inference"
        OllamaMetal["Ollama Metal (:11434)\nqwen3.8:27b / qwen3.5:9b"]
        LiteLLM["LiteLLM (:4000/v1)\nlocal-qwen3-27b / local-qwen-coder"]
    end

    subgraph "Interfaces"
        CLI["dendrite commit [--ticket DEN-XXX]"]
        MCP["MCP: dendrite_commit"]
    end

    Files --> CommitRunner
    CommitRunner --> DiffExtractor
    DiffExtractor --> Diff
    Diff --> MessageDelegator
    MessageDelegator -->|Query Diff| OllamaMetal
    MessageDelegator -->|Fallback| LiteLLM
    OllamaMetal -->|Conventional Commit Msg| MessageDelegator
    MessageDelegator --> CommitRunner
    CLI --> CommitRunner
    MCP --> CommitRunner
```

---

## 3. Specifications & API Contracts

### CLI Usage
```bash
# Basic usage with auto-staged changes and local model message synthesis
./bin/dendrite commit --ticket DEN-098

# Custom explicit message
./bin/dendrite commit --ticket DEN-098 --message "feat(git): implement commit engine"

# Specific files only
./bin/dendrite commit --ticket DEN-098 --files pkg/devtool/commit.go,pkg/mcpserver/tools.go
```

### Native MCP Tool: `dendrite_commit`
- **Name**: `dendrite_commit`
- **Parameters**:
  - `ticket_id` (string, optional): Authoritative Plane Ticket ID (e.g. `DEN-098`).
  - `message` (string, optional): Explicit commit message. When omitted, delegated to local model.
  - `stage_all` (bool, optional, default `true`): Stage all modified and untracked files (`git add -A`).
  - `files` (array of strings, optional): Specific file paths to stage.
  - `use_local_model` (bool, optional, default `true`): Delegate message generation to local zero-cost model.
  - `model` (string, optional): Target local model name (default `qwen3.8:27b` / `local-qwen3-27b`).
  - `dry_run` (bool, optional): Preview commit message and staged diff without modifying git history.

---

## 4. Acceptance Criteria & Test Coverage
1. `TestCommit_LocalModelSynthesis`: Verifies commit message generation given a git diff.
2. `TestCommit_FallbackDeterministic`: Verifies fallback to deterministic commit message if local model is offline.
3. `TestCommit_CLIAndMCP`: Verifies CLI flag parsing and MCP tool registration and response format.
4. Statement coverage in `pkg/devtool` $\ge 85\%$.
