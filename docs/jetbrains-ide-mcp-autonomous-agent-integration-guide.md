# JetBrains IDE MCP & Autonomous Agent Integration Guide

# JetBrains IDE MCP & Autonomous Agent Integration Guide

# JetBrains IDE MCP & Autonomous Agent Integration Guide

# JetBrains IDE MCP & Autonomous Agent Integration Guide

## 1. Overview & Architecture

Dendrite provides bidirectional integration with the **JetBrains IDE Ecosystem** (GoLand, IntelliJ IDEA, PyCharm, RustRover, WebStorm, CLion) through two complementary communication channels:

1. **JetBrains Native MCP Client (IDE $\to$ Dendrite)**:

JetBrains IDEs (2024.3+, 2025+, 2026+) natively support the **Model Context Protocol (MCP 2024-11-05)**. By configuring Dendrite's native MCP server (`bin/dendrite mcp serve`), JetBrains AI Assistant, Junie, and embedded agents gain direct access to Dendrite's **20 operational MCP tools** (ticket tracking, multi-engine inference probing, topology advisor, agent runners, distributed codebase registry, modular decomposition, cross-project interop, satellite project spawning, environment replication, and telemetry).

2. **Dendrite JetBrains Bridge (Dendrite $\to$ IDE)**:

Through `pkg/jetbrains` and `dendrite jetbrains <probe|open|problems|symbols|build|run|setup>`, Dendrite's autonomous agents can reach back into the running JetBrains IDE over the IDE's built-in MCP server / port `63342` to navigate active editor tabs, trigger background compilations, query PSI symbols, and extract live compiler diagnostics.

flowchart LR

subgraph &#34;JetBrains IDE (GoLand, IntelliJ, RustRover)&#34;

IDE_Core[&#34;IDE Core &amp; Editor Engine&#34;]

AIAssistant[&#34;JetBrains AI Assistant / Junie&#34;]

IDE_MCP_Srv[&#34;JetBrains Built-in MCP Server&lt;br/&gt;Port 63342 / Stdio&#34;]

ConfigProject[&#34;Project MCP: .ai/mcp/mcp.json&#34;]

ConfigGlobal[&#34;Global MCP: ~/.ai/mcp/mcp.json&#34;]

RunConfigs[&#34;Run Configurations (.idea/runConfigurations)&#34;]

ExtTools[&#34;External Tools (Context Menu)&#34;]

end

subgraph &#34;Dendrite Control Plane&#34;

DendriteCLI[&#34;Dendrite CLI / Binary&lt;br/&gt;./bin/dendrite&#34;]

MCPServer[&#34;Dendrite MCP Server&lt;br/&gt;20 Operational Tools&#34;]

DistEngine[&#34;Distributed Codebase Engine&lt;br/&gt;pkg/distcodebase&#34;]

PlaneAdapter[&#34;Plane v2 Adapter&lt;br/&gt;pkg/tracker/plane&#34;]

JBBridge[&#34;JetBrains Bridge&lt;br/&gt;pkg/jetbrains&#34;]

AgentRunner[&#34;Agent Harness&lt;br/&gt;Aider / AGY / OpenHands&#34;]

end

ConfigProject &amp; ConfigGlobal --&gt;|Configures| AIAssistant

AIAssistant --&gt;|&#34;tools/call (JSON-RPC stdio)&#34;| MCPServer

MCPServer --&gt; DistEngine

MCPServer --&gt; PlaneAdapter

MCPServer --&gt; AgentRunner

RunConfigs &amp; ExtTools --&gt;|Executes Actions| DendriteCLI

AgentRunner --&gt;|Uses| JBBridge

JBBridge --&gt;|Port 63342 / MCP| IDE_MCP_Srv

IDE_MCP_Srv --&gt;|Controls| IDE_Core

---

## 2. Automated Routine: `dendrite jetbrains setup`

Dendrite includes an automated routine to configure all necessary JetBrains files in one command:

# Full automated setup (project MCP, global MCP, run configurations, external tools)

./bin/dendrite jetbrains setup

# Target only the current project (.ai/mcp/mcp.json and .idea/)

./bin/dendrite jetbrains setup --project-only

# Target only global user settings (~/.ai/mcp/mcp.json)

./bin/dendrite jetbrains setup --global-only

Alternatively, this routine can be triggered dynamically from any AI agent via the MCP tool `dendrite_jetbrains_setup`:

./bin/dendrite mcp call dendrite_jetbrains_setup &#39;{&#34;project_only&#34;: true}&#39;</code></pre>

### What the Routine Configures:

1. **`.ai/mcp/mcp.json` (Project Root)**: Registers `dendrite` as an active project MCP server.

2. **`~/.ai/mcp/mcp.json` (User Home)**: Registers `dendrite` as a globally available MCP server across all installed JetBrains IDEs.

3. **`.idea/runConfigurations/`**: Generates 6 ready-to-run GoLand / IntelliJ configurations:

- `Dendrite_MCP_Server.xml` (`dendrite mcp serve`)
- `Dendrite_Agent_Aider.xml` (`dendrite agent run aider --ticket DEN-036`)
- `Dendrite_Agent_Antigravity.xml` (`dendrite agent run agy`)
- `Dendrite_Web_UI.xml` (`dendrite ui --port 3456`)
- `Dendrite_Doctor.xml` (`dendrite doctor`)
- `Dendrite_JetBrains_Probe.xml` (`dendrite jetbrains probe`)

4. **`.idea/tools/External_Tools.xml`**: Adds context-menu IDE external tools:

- `Dendrite: Run Aider on Active File`
- `Dendrite: Run Antigravity on Active File`
- `Dendrite: Probe MCP Diagnostics`
- `Dendrite: List Registered MCP Tools`
- `Dendrite: Telemetry Snapshot`

---

## 3. Manual Configuration in JetBrains

If you prefer to configure JetBrains manually or verify your IDE settings, follow these methods:

### Method A: Configuration via `.ai/mcp/mcp.json` (Recommended)

JetBrains IDEs automatically detect MCP configuration files matching standard MCP JSON schemas:

- **Project Scope**: `<project-root>/.ai/mcp/mcp.json`
- **Global Scope**: `~/.ai/mcp/mcp.json`

Add the following configuration:

{

&#34;mcpServers&#34;: {

&#34;dendrite&#34;: {

&#34;command&#34;: &#34;/Users/yasuomaidana/GoLandProjects/Dendrite/bin/dendrite&#34;,

&#34;args&#34;: [

&#34;mcp&#34;,

&#34;serve&#34;

],

&#34;env&#34;: {

&#34;DENDRITE_WORKSPACE&#34;: &#34;/Users/yasuomaidana/GoLandProjects/Dendrite&#34;,

&#34;DENDRITE_ENV&#34;: &#34;development&#34;,

&#34;PLANE_API_KEY&#34;: &#34;plane_sec_dendrite_token_12345&#34;,

&#34;PLANE_URL&#34;: &#34;http://localhost:8090&#34;

}

}

}

}</code></pre>

Once saved, JetBrains's `McpProjectServerConfigurationService` reloads the file via VFS listeners without requiring an IDE restart.

---

### Method B: Configuration via JetBrains IDE GUI

1. Open **Settings / Preferences** (`Cmd + ,` on macOS, `Ctrl + Alt + S` on Linux/Windows).

2. Navigate to **Tools $\to$ Model Context Protocol (MCP)** or **AI Assistant $\to$ Model Context Protocol (MCP)**.

3. Click the **`+` (Add)** icon and select **Command (stdio)**.

4. Fill in the fields:

- **Name**: `dendrite`
- **Command**: `/Users/yasuomaidana/GoLandProjects/Dendrite/bin/dendrite`
- **Arguments**: `mcp serve`
- **Environment Variables**:
- `DENDRITE_WORKSPACE`: `/Users/yasuomaidana/GoLandProjects/Dendrite`
- `DENDRITE_ENV`: `development`
- `PLANE_API_KEY`: `plane_sec_dendrite_token_12345`
- `PLANE_URL`: `http://localhost:8090`

5. Click **Apply** and **OK**.

6. The IDE status bar MCP widget will display `dendrite (20 tools connected)`.

---

## 4. Comprehensive Catalog of Dendrite's 19 Native MCP Tools

| Tool Identifier | Description & Capability | Primary Input Parameters |

|:---|:---|:---|

| `dendrite_ticket_list` | Query active tickets from Plane v2 with status, priority, and milestone. | `{}` |

| `dendrite_ticket_update` | Mutate ticket state (e.g. Backlog $\to$ Todo $\to$ In Progress $\to$ Done). | `ticket_id`, `status` |

| `dendrite_plane_issues` | Comprehensive Plane v2 issue search with filter criteria. | `project_id`, `state` |

| `dendrite_plane_push_status`| Bulk push and reconcile status ledgers to Plane v2. | `file_path`, `dry_run` |

| `dendrite_plane_doc_sync` | Synchronize markdown docs into Plane project documentation pages. | `docs_dir`, `project_id`|

| `dendrite_plane_replicate` | Replicate Plane v2 project structure, states, and modules to target. | `target_project_name` |

| `dendrite_dist_sources` | **Priority-tiered multi-source registry** ($P_1$: Own, $P_2$: Curated OSS, $P_3$: Registries). | `query`, `min_priority` |

| `dendrite_dist_decompose` | **Scenario 1**: Force modular decomposition and search ecosystem reuse. | `target`, `user_requested_custom` |

| `dendrite_dist_interop` | **Scenario 3**: Local LLM cross-project interop coordinator (fallback & TODO). | `source_project`, `target_project`, `action` |

| `dendrite_dist_spawn` | **Scenario 2 & 4**: Parallel repository spawner & Plane v2 project creator. | `project_name`, `language`, `create_plane_project` |

| `dendrite_agent_run` | Dispatch autonomous coding loop (Aider, OpenHands, OpenCode, AGY). | `agent_type`, `ticket_id`, `task` |

| `dendrite_telemetry` | Real-time Apple Silicon SoC thermals and LiteLLM spend window totals. | `{}` |

| `dendrite_jetbrains_setup`| Auto-configure JetBrains MCP server, Run Configurations, and Tools. | `project_only`, `global_only` |

| `dendrite_ollama_probe` | Probe local Ollama Metal GPU instance, check models and local latency. | `{}` |

| `dendrite_inference_list` | List all operational engines (Ollama, OxiLLaMa, vLLM, TGI, LiteLLM). | `{}` |

| `dendrite_inference_probe`| Health-check and sample TTFT/TPS latency on target engine. | `engine_type` |

| `dendrite_topology_advisor`| Recommend hardware placement topologies for target scenario. | `scenario` |

| `dendrite_gateway_render` | Dynamically render LiteLLM gateway routing configurations. | `template_path` |

| `dendrite_env_status` | Pre-flight diagnostics for Docker, Go, Git, Plane, and environment. | `{}` |

---

## 5. Technical Workflows: Distributed Codebase Scenarios 1 to 4 in JetBrains

JetBrains AI Assistant or Junie can execute complex distributed codebase tasks through conversational prompts:

### Scenario 1: Modular REA Decomposition & Ecosystem Reuse

Monolithic legacy targets are automatically divided into reusable sub-modules. The tool checks registered priority sources before authoring custom code:

- **Prompt in JetBrains AI Chat**:

> *"Decompose the monolithic target 'game-renderer' into modular sub-packages and find existing libraries using `dendrite_dist_decompose`."*

- **MCP Call Triggered**:

{

&#34;tool&#34;: &#34;dendrite_dist_decompose&#34;,

&#34;arguments&#34;: {

&#34;target&#34;: &#34;game-renderer&#34;,

&#34;user_requested_custom&#34;: false

}

}

- **Result**: Core rendering is mapped to Priority 2 `wgpu`, data queries to Priority 2 `diesel-cte-ext`, and edge storage to `turso`. Custom development is only scheduled for unique game state logic.

---

### Scenario 2: Parallel Multi-Project &amp;amp; Repository Spawning

Scaffold independent repositories concurrently with language manifests and git initialization:

- **Prompt in JetBrains AI Chat**:

&amp;gt; *&amp;#34;Spawn a new Rust project named &amp;#39;sensor-stream-rs&amp;#39; with full Git initialization using `dendrite_dist_spawn`.&amp;#34;*

- **MCP Call Triggered**:

</code></pre>

json

{

&#34;tool&#34;: &#34;dendrite_dist_spawn&#34;,

&#34;arguments&#34;: {

&#34;project_name&#34;: &#34;sensor-stream-rs&#34;,

&#34;language&#34;: &#34;rust&#34;,

&#34;create_plane_project&#34;: false

}

}

---

### Scenario 3: Cross-Project Interoperability Coordinator (Docula $\to$ Oxipix)

When working on `Docula` which depends on user-owned `Oxipix` and `semdag`:

1. Request a capability in `Oxipix` (e.g., `simd-blend-modes`).

2. Generate an interim fallback in `Docula` tagged with `// TODO(interop:Oxipix): Replace with Oxipix::simd_blend once implemented`.

3. Unblock `Docula` development immediately.

4. Once `Oxipix` is updated, apply reconciliation patch.

- **Prompt in JetBrains AI Chat**:

&amp;gt; *&amp;#34;Coordinate the SIMD blend feature between Docula and Oxipix using `dendrite_dist_interop`.&amp;#34;*

- **MCP Call Triggered**:

</code></pre>

json

{

&#34;tool&#34;: &#34;dendrite_dist_interop&#34;,

&#34;arguments&#34;: {

&#34;source_project&#34;: &#34;Docula&#34;,

&#34;target_project&#34;: &#34;Oxipix&#34;,

&#34;feature&#34;: &#34;simd-blend-modes&#34;,

&#34;action&#34;: &#34;request&#34;

}

}

---

### Scenario 4: Generative Satellite Projects (Carla-to-Rust) &amp;amp; Plane Lifecycle

When executing full language translations (e.g. Carla simulator to 100% Rust), spin off missing libraries into separate Plane projects:

- **Prompt in JetBrains AI Chat**:

&amp;gt; *&amp;#34;Create a generative satellite project for Carla sensor suite in Rust with a dedicated Plane project using `dendrite_dist_spawn`.&amp;#34;*

- **MCP Call Triggered**:

</code></pre>

json

{

&#34;tool&#34;: &#34;dendrite_dist_spawn&#34;,

&#34;arguments&#34;: {

&#34;project_name&#34;: &#34;carla-sensor-rs&#34;,

&#34;language&#34;: &#34;rust&#34;,

&#34;create_plane_project&#34;: true

}

}

- **Result**: Provisions `carla-sensor-rs` locally with Git and creates project `carla-sensor-rs` on Plane v2 (`http://localhost:8090`).

---

## 6. How to Use Tools via JetBrains External Tools Menu

You can invoke Dendrite operations directly on the file you are actively editing:

1. Right-click anywhere in the editor code area or in the Project view.

2. Select **External Tools** $\to$ **Dendrite**:

- **`Dendrite: Run Aider on Active File`**: Launches an autonomous pair-programming loop targeted at the active file.
- **`Dendrite: Run Antigravity on Active File`**: Invokes the high-cognition strategic agent to inspect the current file.
- **`Dendrite: Probe MCP Diagnostics`**: Validates stdio MCP communication and lists tool status.
- **`Dendrite: List Registered MCP Tools`**: Displays all 19 MCP tools with schema descriptions in the IDE Run window.
- **`Dendrite: Telemetry Snapshot`**: Displays real-time Apple Silicon thermals and LiteLLM token usage in the IDE console.

---

## 7. How to Use Tools via JetBrains Run Configurations

In the top-right toolbar of GoLand or IntelliJ, click the Run Configuration dropdown:

| Configuration Name | Action Performed | Shortcut |

|:---|:---|:---:|

| `Dendrite_Web_UI` | Starts the embedded HTTP dashboard and WebSocket telemetry server on `http://localhost:3456`. | `Shift + F10` |

| `Dendrite_Doctor` | Runs environment diagnostics checking Docker, Ollama, Plane, LiteLLM, and Go compiler. | `Shift + F10` |

| `Dendrite_MCP_Server` | Launches the Model Context Protocol stdio server for external MCP debuggers. | `Shift + F10` |

| `Dendrite_Agent_Aider` | Launches the Aider autonomous coding loop bound to virtual LiteLLM keys. | `Shift + F10` |

| `Dendrite_Agent_Antigravity` | Launches the Antigravity high-cognition cloud agent session. | `Shift + F10` |

| `Dendrite_JetBrains_Probe` | Probes the running JetBrains IDE&amp;#39;s MCP port `63342` and verifies IDE control tools. | `Shift + F10` |

---

## 8. Controlling JetBrains from Autonomous Agents (Reverse Channel)

Dendrite&amp;#39;s autonomous coding agents (Aider, OpenHands, AGY) can leverage the running IDE&amp;#39;s internal capabilities through `pkg/jetbrains`:

&lt;/code&gt;&lt;/pre&gt;

bash

# Verify IDE connectivity from terminal

./bin/dendrite jetbrains probe

# Navigate the IDE editor to a specific file and line

./bin/dendrite jetbrains open pkg/distcodebase/registry.go 45

# Extract live IDE inspection problems and compiler errors

./bin/dendrite jetbrains problems pkg/distcodebase/registry.go

# Search project PSI symbol index for structs, interfaces, and methods

./bin/dendrite jetbrains symbols ResolveCapability

# Trigger background compilation inside the IDE

./bin/dendrite jetbrains build

---

## 9. Verification &amp;amp; Operational Health Check

To verify that your JetBrains IDE and Dendrite MCP server are operating in perfect synchrony:

&lt;/code&gt;&lt;/pre&gt;

bash

# 1. Check all 19 MCP tools

./bin/dendrite mcp tools

# 2. Test tool execution directly via CLI

./bin/dendrite mcp call dendrite_dist_sources &#39;{&#34;query&#34;:&#34;wgpu&#34;}&#39;

# 3. Check Plane connectivity

./bin/dendrite plane status

</code></pre>
