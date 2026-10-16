import urllib.request
import json
import re
import sys

PLANE_URL = "http://localhost:8090"
PLANE_WS = "dendrite-ws"
PLANE_PROJ = "e6380603-1428-4a66-995e-e06039915d52"
PLANE_KEY = "plane_sec_dendrite_token_12345"
OXILLAMA_URL = "http://127.0.0.1:8080/v1/chat/completions"

TICKETS_METADATA = {
    "5e90a5ce-ef2f-4bd7-9fcd-ce3d995155d9": {
        "title": "[DEN-811] Modular CLI Command Registry & Dispatcher Architecture",
        "seq": 140,
        "packages": "pkg/cli, cmd/dendrite",
        "specs": "Architecture Refactoring & Command Routing",
        "summary": "Refactors monolithic switch-case CLI dispatch in cmd/dendrite/main.go into an extensible, modular command router in pkg/cli/router.go with subcommand discovery, alias resolution, and execution interceptors.",
        "ac": [
            "AC-1 (Command Registry): Implements CommandRegistry in pkg/cli with support for registering subcommands, aliases, help text, and nested command groups.",
            "AC-2 (Dispatcher & Routing): Provides CommandDispatcher parsing os.Args, resolving flags and subcommands, and invoking registered handlers with context.Context.",
            "AC-3 (Backward Compatibility): Fully preserves all existing 30+ dendrite CLI subcommands (plane, dev, inference, bench, quality, etc.) with identical UX.",
            "AC-4 (DoD & Coverage): Statement test coverage >= 85% across pkg/cli with zero mocks."
        ]
    },
    "1253eca7-aa9f-4fb5-8f93-91940513fe30": {
        "title": "[DEN-809] Local LLM Inference Concurrency Limiter & FIFO Priority Queue",
        "seq": 139,
        "packages": "pkg/inference",
        "specs": "FEAT-101",
        "summary": "Implements a hardware-aware sliding-window concurrency limiter and priority queue in pkg/inference/queue.go to prevent Metal GPU and edge compute memory exhaustion during parallel local LLM requests.",
        "ac": [
            "AC-1 (Concurrency Limiter): Bounds concurrent inference execution slots (default: 1 on Apple Silicon Metal) to prevent GPU memory saturation.",
            "AC-2 (FIFO Priority Queue): Implements priority levels (High, Medium, Low) allowing interactive developer prompts to preempt background batch evaluation.",
            "AC-3 (Context & Cancellation): Cleanly handles client timeouts, context cancellations, and queue reordering without leaking goroutines.",
            "AC-4 (DoD & Coverage): Statement test coverage >= 85% across pkg/inference queue components with race detector enabled."
        ]
    },
    "b2b7b9ff-df4b-484c-8dc0-ad11569c7c4a": {
        "title": "[DEN-810] Penpot Service Lifecycle & Web UI Development Synchronized with Plane",
        "seq": 138,
        "packages": "pkg/ui/penpot, cmd/dendrite",
        "specs": "FEAT-037, FEAT-047",
        "summary": "Manages containerized Penpot service lifecycle (http://localhost:9000), frame deep-linking with Plane tickets (DEN-XXX), and bidirectional design token synchronization.",
        "ac": [
            "AC-1 (Service Lifecycle): Probes, starts, and halts local Penpot container stack via dendrite service commands.",
            "AC-2 (Plane Ticket Binding): Relates Plane ticket IDs and UI features to specific Penpot frame UUIDs and web URLs.",
            "AC-3 (MCP Integration): Exposes dendrite_ui_penpot_sync MCP tool for automated UI frame inspection and ticket comment synchronization.",
            "AC-4 (DoD & Coverage): Statement test coverage >= 85% across pkg/ui/penpot with zero mocks."
        ]
    },
    "402121d5-7afb-47ed-97e8-d689d363f620": {
        "title": "[DEN-809] Model Migration Engine: Migrate Models from Ollama to OxiLLaMa & Evict Local Blobs",
        "seq": 137,
        "packages": "pkg/inference",
        "specs": "FEAT-080",
        "summary": "Delivers zero-copy hardlink migration of GGUF model layers from Ollama store (~/.ollama) into OxiLLaMa sovereign store (~/.oxillama), verifying layer digests and evicting source models to reclaim memory and disk space.",
        "ac": [
            "AC-1 (Zero-Copy Hardlinking): Uses os.Link with streaming copy fallback to transfer GGUF layers without duplicating disk footprint.",
            "AC-2 (OxiLLaMa Manifest & Integrity): Synthesizes oxillama-manifest.json and validates all layer SHA-256 digests.",
            "AC-3 (Zero-Cost Registry Sync): Registers migrated models into .dendrite/zero_cost_models.json for LiteLLM budget exemption.",
            "AC-4 (Source Model Eviction): Safely invokes DELETE /api/delete on Ollama daemon to reclaim tens of gigabytes of disk/memory.",
            "AC-5 (DoD & Coverage): Statement test coverage >= 85% across pkg/inference/migration.go with zero mocks."
        ]
    },
    "7b22ed57-5950-4928-9be4-5cf966b15e4d": {
        "title": "[DEN-808] Personal Context MCP Connectors: Workspace & Enterprise Tool Adapters",
        "seq": 136,
        "packages": "pkg/personalcontext, pkg/mcpserver",
        "specs": "FEAT-101",
        "summary": "Exposes Model Context Protocol (MCP 2024-11-05) connector tools bridging personal context topic memories, user traits, and enterprise tool integrations (Plane, Linear, Penpot) into agent conversations.",
        "ac": [
            "AC-1 (Memory Query MCP): Registers MCP tools for searching, querying, and updating structured personal context memories.",
            "AC-2 (Enterprise Tool Adapters): Resolves active tickets, roadmap milestones, and design frames across Plane and Penpot.",
            "AC-3 (Compact Token Governance): Formats MCP responses compactly to preserve agent context windows and minimize token overhead.",
            "AC-4 (DoD & Coverage): Statement test coverage >= 85% across personal context MCP tools with zero mocks."
        ]
    },
    "1dcb5eb2-5f5e-4e97-a4e6-081f48d20251": {
        "title": "[DEN-806] Dynamic 10-Style Greeting Matrix & Cliché Suppression Engine",
        "seq": 134,
        "packages": "pkg/personalcontext/greeting, cmd/dendrite, pkg/mcpserver",
        "specs": "FEAT-101",
        "summary": "Implements the 10-style personalized headline greeting engine in pkg/personalcontext/greeting with strict cliché suppression, sliding history deduplication, and journey context resolution.",
        "ac": [
            "AC-1 (10 Persona Styles): Implements 10 distinct headline greeting styles (Formal, Direct, Playful, Stoic, Poetic, etc.).",
            "AC-2 (Strict Cliché Suppression): Rejects overused tropes, hollow phrases, and repetitive greetings.",
            "AC-3 (Sliding History Ring Buffer): Persists recent greeting hashes to guarantee non-repeating greetings across sessions.",
            "AC-4 (CLI & MCP Exposure): Exposes CLI dendrite greeting and MCP tool dendrite_pcontext_greeting_generate.",
            "AC-5 (DoD & Coverage): Statement test coverage >= 85% across pkg/personalcontext/greeting with zero mocks."
        ]
    },
    "92e5fb7f-e315-49e3-a5a7-cedd5dd4e7bc": {
        "title": "[DEN-805] Local LLM Model Provisioner & LiteLLM Gateway Routing Bridge",
        "seq": 133,
        "packages": "pkg/installer/models, pkg/gateway",
        "specs": "FEAT-101",
        "summary": "Automated provisioner for local GGUF models (qwen3.5:9b, qwen3.8:27b, hermes3:8b) linking them to LiteLLM proxy configuration and registering zero-cost financial exemptions.",
        "ac": [
            "AC-1 (Automated Model Pulling): Inspects and pulls required local models into Ollama or OxiLLaMa runtime stores.",
            "AC-2 (Gateway Routing Bridge): Updates LiteLLM proxy YAML configuration with local upstream model endpoints.",
            "AC-3 (Zero-Cost Accounting): Registers models in .dendrite/zero_cost_models.json to exempt local execution from commercial token limits.",
            "AC-4 (DoD & Coverage): Statement test coverage >= 85% across pkg/installer/models with zero mocks."
        ]
    },
    "1a387aaa-e539-4b53-9dec-2d3af0ebbb0f": {
        "title": "[DEN-804] System Installer Helpers: Pre-Flight Audit & Port/Hardware Prober",
        "seq": 132,
        "packages": "pkg/installer, scripts/",
        "specs": "FEAT-101",
        "summary": "Provides pre-flight system diagnostics auditing host compute resources, Apple Silicon Metal GPU memory, port bindings (11434, 8080, 4000, 8090), and developer toolchains.",
        "ac": [
            "AC-1 (Port & Service Probing): Checks availability of required service ports with conflict resolution guidance.",
            "AC-2 (Hardware & Metal Audit): Inspects CPU cores, system RAM, and Apple Silicon unified memory for model fit.",
            "AC-3 (Toolchain Verification): Validates required runtime binaries (Docker, Git, Go, Rust, Ollama).",
            "AC-4 (DoD & Coverage): Statement test coverage >= 85% across pkg/installer with zero mocks."
        ]
    },
    "ad081172-3d76-4384-a5cb-96a7c0e84e78": {
        "title": "[DEN-803] Structured Topic Memory Parser & Zero-Extrapolation Fidelity Guard",
        "seq": 131,
        "packages": "pkg/personalcontext/memory",
        "specs": "FEAT-101",
        "summary": "Extracts and persists structured user topics, preferences, and workspace entities into persistent store with a Zero-Extrapolation Fidelity Guard preventing hallucinations.",
        "ac": [
            "AC-1 (Structured Entity Extraction): Parses conversational turns into typed topic memory structs.",
            "AC-2 (Zero-Extrapolation Guard): Strictly rejects ungrounded or extrapolated statements, storing only explicit user facts.",
            "AC-3 (Persistence & Checkpoints): Atomic storage with WAL journaling and query interfaces.",
            "AC-4 (DoD & Coverage): Statement test coverage >= 85% across pkg/personalcontext/memory with zero mocks."
        ]
    },
    "e7de0677-e928-45e0-b7cc-501db2cf8d0a": {
        "title": "[DEN-802] Welcome Prompt Suite & Cross-Source Synthesis Dispatcher",
        "seq": 130,
        "packages": "pkg/personalcontext/welcome",
        "specs": "FEAT-101",
        "summary": "Synthesizes cross-source context (git commits, Plane ticket progress, personal topic memory) into personalized welcome prompts and conversational entry points.",
        "ac": [
            "AC-1 (Cross-Source Synthesis): Gathers real-time progress from git history, Plane project state, and memory cache.",
            "AC-2 (Context Prompt Generation): Synthesizes dynamic, concise welcome headlines and recommended action suggestions.",
            "AC-3 (Graceful Fallback): Provides sensible default prompts when external trackers or memory stores are offline.",
            "AC-4 (DoD & Coverage): Statement test coverage >= 85% across pkg/personalcontext/welcome with zero mocks."
        ]
    },
    "bf64b1ef-e98c-462d-92b4-7769739fe16e": {
        "title": "[DEN-801] Topic Zero State Prompt Engine & Multi-Turn State Machine",
        "seq": 129,
        "packages": "pkg/personalcontext, pkg/prompt",
        "specs": "FEAT-101",
        "summary": "Multi-turn conversational state machine guiding user onboarding, topic selection, task clarification, and agent skill activation from zero state.",
        "ac": [
            "AC-1 (State Machine Engine): Implements deterministic transitions (Init -> TopicSelect -> Clarification -> ActiveSession).",
            "AC-2 (Intent Classification): Classifies user inputs and routes them to optimal agent skills or commands.",
            "AC-3 (Session Recovery): Persists conversational zero-state to survive terminal restarts.",
            "AC-4 (DoD & Coverage): Statement test coverage >= 85% across pkg/personalcontext with zero mocks."
        ]
    }
}

def generate_html_context(meta):
    ac_items = "".join([f"<li><b>{ac.split(':')[0]}</b>:{':'.join(ac.split(':')[1:])}</li>" if ':' in ac else f"<li>{ac}</li>" for ac in meta["ac"]])
    return f"""<div>
<h3>Executive Summary &amp; Objective</h3>
<p>{meta["summary"]}</p>
<h3>Architecture &amp; Target Components</h3>
<ul>
  <li><strong>Target Packages:</strong> <code>{meta["packages"]}</code></li>
  <li><strong>Governing Specifications:</strong> <code>{meta["specs"]}</code></li>
</ul>
<h3>Deterministic Acceptance Criteria</h3>
<ul>
  {ac_items}
</ul>
<h3>Definition of Done (DoD)</h3>
<ul>
  <li>Zero mocks policy strictly enforced using real data models and test servers.</li>
  <li>Statement test coverage &gt;= 85% across target packages.</li>
  <li>Cyclomatic complexity &lt;= 15 and cognitive complexity &lt;= 20 per function.</li>
</ul>
</div>"""

print(f"Starting ticket context updates for {len(TICKETS_METADATA)} empty tickets in Plane...")

updated_count = 0
for issue_id, meta in TICKETS_METADATA.items():
    html_desc = generate_html_context(meta)
    
    # Send PATCH request to Plane
    url = f"{PLANE_URL}/api/v1/workspaces/{PLANE_WS}/projects/{PLANE_PROJ}/issues/{issue_id}/"
    payload = json.dumps({"description_html": html_desc}).encode("utf-8")
    req = urllib.request.Request(url, data=payload, headers={
        "Content-Type": "application/json",
        "x-api-key": PLANE_KEY
    }, method="PATCH")
    
    try:
        with urllib.request.urlopen(req, timeout=10) as resp:
            if resp.status in (200, 201):
                data = json.loads(resp.read().decode("utf-8"))
                print(f"[✓] Successfully updated ticket #{meta['seq']} ({meta['title'][:45]}...) in Plane")
                updated_count += 1
            else:
                print(f"[-] Unexpected status {resp.status} for ticket #{meta['seq']}")
    except Exception as e:
        print(f"[✗] Failed to update ticket #{meta['seq']}: {e}")

print(f"\nCompleted: {updated_count}/{len(TICKETS_METADATA)} tickets updated in Plane.")
