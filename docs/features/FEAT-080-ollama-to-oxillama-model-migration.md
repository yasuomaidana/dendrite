---
id: FEAT-080
title: "Ollama to OxiLLaMa Model Migration & Blob Eviction Engine"
status: approved
plane_epic: PROJ-E-16
plane_ticket: DEN-809
dependencies: [FEAT-020, FEAT-028]
repository: dendrite
tags: [inference, ollama, oxillama, migration, memory-optimization, models, zero-cost]
---

# FEAT-080: Ollama to OxiLLaMa Model Migration & Blob Eviction Engine

## 1. Executive Summary & Problem Statement

### The Problem
Large Language Models running under local Ollama daemons consume substantial memory and disk space (e.g. 18 GB for `qwen3.8:27b` and 7.6 GB for `qwen3.5:9b`). **OxiLLaMa (`cool-japan/oxillama`)** is an ultra-efficient, 100% pure-Rust GGUF inference engine and OpenAI-compatible server that eliminates C/C++ FFI overhead and provides superior memory efficiency, deterministic execution, and zero runtime crashes. However, developers previously had no automated mechanism to migrate local models between Ollama and OxiLLaMa without manual downloading, duplicate disk consumption, or uncoordinated model registry updates.

### User Story & Architectural Pillars
As an AI engineer and systems developer optimizing sovereign edge and workstation hardware, I need an automated migration function that inspects Ollama model layers, links them into OxiLLaMa's structured model storage, validates target integrity, registers the model in Dendrite's zero-cost accounting system, and safely evicts the source model from Ollama to reclaim memory and storage.

```mermaid
flowchart TD
    subgraph OllamaSource["Source: Ollama Daemon & Store (~/.ollama)"]
        OM["Ollama Manifest<br/>(manifests/.../qwen3.5/9b)"]
        OB["Ollama Blobs (7.1 GB)<br/>(blobs/sha256-*)"]
    end

    subgraph MigratorEngine["Dendrite Model Migrator (pkg/inference)"]
        Parser["Manifest & Layer Parser"]
        Transfer["Zero-Copy Hardlink Linker<br/>(os.Link / io.Copy fallback)"]
        Verifier["Integrity & Size Verifier"]
        RegSync["ZeroCostRegistry & OxiLLaMa Sync"]
        Evictor["Source Eviction Controller<br/>(DELETE /api/delete)"]
    end

    subgraph OxiLLaMaTarget["Target: OxiLLaMa Sovereign Store (~/.oxillama)"]
        XM["OxiLLaMa Manifest<br/>(models/qwen3.5_9b/oxillama-manifest.json)"]
        XB["OxiLLaMa Blobs<br/>(blobs/sha256-*)"]
    end

    OM --> Parser
    OB --> Transfer
    Parser --> Transfer
    Transfer --> Verifier
    Verifier -->|Pass| XM
    Verifier -->|Pass| XB
    Verifier -->|Pass| RegSync
    RegSync --> Evictor
    Evictor -->|Reclaim Memory| OM
```

---

## 2. Technical Architecture & Idiomatic Design Patterns

### 2.1 Zero-Copy Hardlink Migration
Rather than copying tens of gigabytes across storage boundaries on the same filesystem (which would double disk usage and cause thermal throttling), `ModelMigrator` utilizes atomic OS hardlinks (`os.Link`) with automatic fallback to streaming copies (`io.Copy`) across device partitions.
- **Source Paths**:
  - `~/.ollama/models/manifests/registry.ollama.ai/library/<model>/<tag>`
  - `~/.ollama/models/manifests-v2/ollama.com/library/<model>/<tag>`
  - `~/.ollama/models/blobs/sha256-<digest>`
- **Destination Paths**:
  - `~/.oxillama/models/<model_tag>/oxillama-manifest.json`
  - `~/.oxillama/blobs/sha256-<digest>`
  - `~/.oxillama/models/<model_tag>/<named_layer>` (e.g., `config.json`, `tokenizer.json`, `weights.safetensors`)

### 2.2 Pre-Eviction Verification Gate
Source deletion never occurs blindly. Before triggering eviction on Ollama, the migrator executes strict verification:
1. Target manifest existence and JSON schema validity.
2. Every layer digest verified present in `~/.oxillama/blobs`.
3. Byte length comparison between layer specification and target file size.
4. If verification encounters any discrepancy, eviction is aborted and source data remains untouched.

### 2.3 Post-Migration Registration & Zero-Cost Governance
Migrated models are automatically registered into Dendrite's `ZeroCostRegistry` (`.dendrite/zero_cost_models.json`) under both `oxillama/<model>` and `<model>`, exempting them from commercial token budgets in `pkg/gateway`.

### 2.4 OpenAI-Compatible OxiLLaMa Server (`:8080`)
To allow coding agents, external tools, and developer scripts to execute completions against migrated models, Dendrite includes an OpenAI-compatible HTTP server:
- **CLI Command**: `dendrite dev inference serve [--port 8080] [--daemon]`
- **Endpoints**:
  - `GET /v1/models`: Enumerate migrated models directly from `~/.oxillama/models`.
  - `POST /v1/chat/completions`: Standard OpenAI chat completion format (JSON & streaming SSE).
  - `GET /health`: Health and active models inspection.

---

## 3. Behavioral Scenarios (Gherkin)

### Scenario 1: Dry-Run Model Migration
- **Given** model `qwen3.5:9b` is registered in Ollama
- **When** the developer executes `dendrite dev inference migrate qwen3.5:9b --dry-run`
- **Then** the migrator reports blob count and byte size without modifying disk or contacting deletion endpoints.

### Scenario 2: Successful Migration with Memory Reclaim
- **Given** models `qwen3.5:9b` (7.1 GB) and `qwen3.8:27b` (16.9 GB) exist in Ollama
- **When** migration is executed with `--delete-source`
- **Then** layers are transferred to `~/.oxillama`, target descriptors are created, and Ollama is instructed to evict the models, reclaiming over 24 GB of storage and releasing memory.

### Scenario 3: Verification Abort on Corrupted Layer
- **Given** a corrupted layer where blob size does not match manifest specification
- **When** migration executes
- **Then** target verification fails and source eviction is aborted to prevent data loss.

---

## 4. Acceptance Criteria & Definition of Done

- [x] **AC-1 (Domain Migrator)**: `ModelMigrator` supports migrating models between Ollama and OxiLLaMa with typed options (`pkg/inference/migration.go`).
- [x] **AC-2 (Zero-Copy Linking)**: Preserves storage and avoids redundant writes via atomic hardlinks.
- [x] **AC-3 (Verification Gate)**: Confirms all blobs and descriptors prior to source deletion.
- [x] **AC-4 (Source Eviction)**: Removes migrated models from Ollama daemon via API and disk manifests to save memory.
- [x] **AC-5 (CLI & MCP Tooling)**: Exposed via CLI `dendrite dev inference migrate` and native MCP tool `dendrite_inference_migrate_model`.
- [x] **AC-6 (Unit Test Coverage)**: Comprehensive unit tests in `pkg/inference/migration_test.go` achieving $\ge 85\%$ coverage on migration logic.
