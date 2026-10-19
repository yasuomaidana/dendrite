---
id: FEAT-097
title: "Plane Doc Sync Taxonomy Guard & Markdown-to-HTML Robust Formatter"
status: approved
plane_epic: PROJ-E-12
dependencies: [FEAT-008, FEAT-012, FEAT-016]
repository: dendrite
tags: [plane, docs, taxonomy, markdown, html, code-fence, tables, cleanliness]
---

# FEAT-097: Plane Doc Sync Taxonomy Guard & Markdown-to-HTML Robust Formatter

## 1. Executive Summary & Problem Statement

### The Problem

During development iterations and autonomous agent loops, synchronizing documentation between local repository markdown files and self-hosted Plane v2 project management documentation pages (`/pages/`) encountered two major architectural defects:

- **Taxonomy Loss & Root Directory Pollution**:

- `pullDocsFromPlane` previously wrote all pulled pages directly to `filepath.Join(targetDir, filename)` (e.g. `docs/<sanitized-title>.md`), completely ignoring existing file directory taxonomies (`docs/features/`, `docs/architecture/`, `docs/guides/`, `docs/quality/`, `docs/roadmap/`).
- This caused up to 63 dirty, untracked duplicate markdown files to pollute the root `docs/` folder whenever a new feature was registered in Plane or when bidirectional sync executed.
- Hand-crafted markdown files suffered from lossy overwriting and duplicated top-level headings.

- **Broken Markdown-to-HTML Semantic Rendering in Plane UI**:

- Regex-based code block extraction (`codeFenceRegex`) failed to match indented code blocks (e.g. `  ```json` inside lists), causing code fence delimiters to invert and treat comments as `<h1>` headings and emit broken tags like `<p>&lt;/code&gt;&lt;/pre&gt;</p>`.
- Markdown tables (`| Col1 | Col2 |`) were not converted into semantic HTML `<table>` elements and displayed as unformatted text.
- Ordered lists (`1. `, `2. `) were rendered as plain paragraphs.

### The Solution

FEAT-097 implements an enterprise-grade, taxonomy-aware documentation synchronization engine and robust Markdown-to-HTML parser:

- **Taxonomy Guard**: In-memory recursive index of existing repository documents by frontmatter `id`, `title`, and sanitized filename. In-place updating preserves existing folder hierarchies and skips writes when content is identical.
- **Stateful Line-by-Line Markdown Parser**: Code fences (with arbitrary indentation and language specifiers) preserve verbatim lines inside `<pre><code>...</code></pre>`, escaping special HTML characters without leaking or breaking outer tags.
- **Full Semantic HTML Support**: Tables are rendered with `<table><thead><tr><th>...</th></tr></thead><tbody><tr><td>...</td></tr></tbody></table>`, ordered lists with `<ol><li>`, and inline elements (`<strong>`, `<em>`, `<code>`, `<a href>`).

---

## 2. Technical Architecture & Invariants

```
flowchart TD
    subgraph "Local Codebase Taxonomy (docs/)"
        F1["docs/features/FEAT-*.md"]
        F2["docs/architecture/*.md"]
        F3["docs/guides/*.md"]
        F4["docs/quality/*.md"]
        F5["docs/adr/ADR-*.md"]
    end

    subgraph "Plane Doc Sync Engine (pkg/tracker/plane)"
        Parser["Stateful Markdown-to-HTML Parser(Verbatim Code Fences, Tables, Lists)"]
        TaxonomyGuard["Taxonomy Guard & Local Doc Indexer(Fuzzy & Exact Spec ID Matching)"]
        SyncEngine["Bi-Directional Doc Syncer(dendrite_plane_doc_sync)"]
    end

    subgraph "Plane v2 Service (:8090 / UI :8091)"
        Pages["Plane Documentation Pages(/api/workspaces/.../pages/)"]
    end

    F1 & F2 & F3 & F4 & F5 -->|Extract Title & Body| Parser
    Parser -->|Push Clean Semantic HTML| Pages
    Pages -->|Pull Pages| TaxonomyGuard
    TaxonomyGuard -->|Match In-Place / No Dirty Docs| F1 & F2 & F3 & F4 & F5

```

### Core Invariants

- **Zero Dirty Docs**: Running `dendrite codex plane <push|pull|sync>` must never create loose untracked markdown files in the root `docs/` directory.
- **Taxonomy Fidelity**: Documents originating from `features/`, `architecture/`, `guides/`, `quality/`, or `adr/` must always be synchronized strictly within their designated directory.
- **HTML Semantic Validity**: Code fences in Plane pages must maintain matched `<pre><code>` and `</code></pre>` counts, with no escaped closing tags (`&lt;/code&gt;`). Tables must render semantic `<table>` elements.

---

## 3. Operational Verification Matrix

| Verification Target | Command / MCP Tool | Expected Result | Status |
| :--- | :--- | :--- | :--- |
| **MCP Tool Registration** | `dendrite_plane_doc_sync` | Synchronizes docs bidirectionally with Plane | PASS |
| **Taxonomy Preservation** | `go test -v ./pkg/tracker/plane/...` | `TestTaxonomyPreservation_Pull` passes with 0 root files | PASS |
| **Stateful Formatter** | `go test -v ./pkg/tracker/plane/...` | `TestMarkdownToHTML_IndentedCodeAndTables` passes | PASS |
| **Plane UI Rendering** | Curl Page `e418008a-c399-464b-bff6-03359eb82f72` | 12 `<pre><code>` blocks, 12 `</code></pre>` blocks, 0 escaped tags | PASS |
| **Git Cleanliness** | `git status --short docs/` | 0 untracked files in root docs | PASS |
