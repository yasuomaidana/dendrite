---
id: environment-replication
title: "Development Environment Installation & Replication Guide"
sidebar_label: "Environment Replication"
sidebar_position: 2
tags: [replication, docker, environment, host, penpot, plane]
---

# Development Environment Installation & Replication Guide

This guide details the methodologies, tools, and automation provided by **Dendrite** to replicate, install, and bootstrap the complete development environment on any target computer—either natively on the host OS or completely containerized via Docker.

---

## 1. Overview & Dual Replication Architecture

Dendrite accommodates both isolated, reproducible cloud/local container setups and high-performance native host developer environments:

1. **Docker Containerized Mode (`--mode container`)**:
   - Spins up all services, databases, gateways, and mock backends in Docker containers.
   - Ideal for reproducible setups, sandboxed testing, clean teardown, and hosting local instances of tools (such as **Penpot** design studio, **Plane** issue tracker, or **Linear** GraphQL engine).
   - Zero host pollution, guaranteed port isolation, and automated health checks.
2. **Direct Host Installation Mode (`--mode host`)**:
   - Installs and configures development tooling directly on the target workstation (macOS or Linux).
   - Leverages native package managers (Homebrew on macOS, APT/DNF/Pacman on Linux, or verified installer scripts).
   - Unlocks maximum performance for local Metal inference (Ollama GPU acceleration on Apple Silicon) and native CLI tools (Linear CLI, Go toolchain, Tailscale).
3. **Hybrid Mode (`--mode hybrid`)**:
   - The recommended default for developers: heavy background infrastructure (PostgreSQL 16, Redis 7, LiteLLM Proxy, Penpot, Plane) runs in Docker, while local developer tools (Go, Ollama Metal, Tailscale) run natively on the host computer.

```mermaid
flowchart TD
    User([Developer / Workstation]) --> CLI["dendrite env <action>"]
    
    CLI --> Detector[Host Environment Detector]
    Detector --> Planner[Replication Planner]
    
    Planner --> Mode{Replication Mode}
    
    subgraph "Containerized Mode (Docker)"
        Mode -->|--mode container| ComposeGen[Render Compose Stacks]
        ComposeGen --> DockerRun["docker compose up -d"]
        DockerRun --> PG[(Postgres 16)]
        DockerRun --> Redis[(Redis 7)]
        DockerRun --> LiteLLM[LiteLLM Proxy Gateway]
        DockerRun --> PenpotBox[Penpot Design Suite - Port 9000]
        DockerRun --> PlaneBox[Plane v2 Issue Tracker - Port 8091]
        DockerRun --> LinearBox[Containerized Linear Server - Port 8085]
    end

    subgraph "Direct Host Installation Mode"
        Mode -->|--mode host| HostPkg[Package Manager / Installer]
        HostPkg --> BrewApt["brew / apt / curl script"]
        BrewApt --> NativeOllama[Ollama Metal Engine]
        BrewApt --> NativeLinear[Linear CLI via npm/binary]
        BrewApt --> NativeTailscale[Tailscale Mesh Client]
    end

    subgraph "Verification & Health Probes"
        DockerRun --> Prober[Healthcheck Probe Engine]
        BrewApt --> Prober
        Prober --> StatusReport["Component Status Report (HTTP 200 / Reachable)"]
    end
```

---

## 2. Replicating the Environment with Docker Containers

### Fast Replicate Command
To bootstrap the entire stack in containers:

```bash
# Preview replication actions without making changes
dendrite env replicate --mode container --dry-run

# Replicate and start all containerized services
dendrite env replicate --mode container
```

### Composing Individual Stacks Manually
Dendrite maintains pre-configured, production-grade Docker Compose manifests under `stacks/`:

- **Penpot Design Stack** (`stacks/compose.penpot.yaml`):
  Runs Penpot frontend on port `9000`, backend API on port `9001` (internal `6060`), PostgreSQL on `5435`, and Redis on `6381`.
  ```bash
  docker compose -f stacks/compose.penpot.yaml up -d
  ```

- **Plane v2 Project & Issue Tracker** (`stacks/compose.plane.yaml`):
  Runs Plane proxy and web UI on port `8091`, backend API on port `8090`, PostgreSQL on `5434`, and Redis on `6380`.
  ```bash
  docker compose -f stacks/compose.plane.yaml up -d
  ```

- **All-in-One Dev Stack** (`stacks/compose.dev.yaml`):
  Combines PostgreSQL, Redis, LiteLLM Gateway, and containerized Linear.
  ```bash
  docker compose -f stacks/compose.dev.yaml up -d
  ```

- **Containerized Linear Stack** (`stacks/compose.linear.yaml`):
  Runs a containerized Linear-compatible GraphQL server with a dedicated PostgreSQL database:
  ```bash
  docker compose -f stacks/compose.linear.yaml up -d
  ```

- **Control Plane Stack** (`stacks/compose.control.yaml`):
  Runs persistent state for multi-project token governance and gateways:
  ```bash
  docker compose -f stacks/compose.control.yaml up -d
  ```

> 💡 **New to Penpot and Plane?** Check out our friendly, beginner-oriented walkthrough:  
> [Beginner's Guide to Penpot & Plane](penpot-and-plane-beginner-tutorial.md) for initial account creation, UI navigation, and troubleshooting tips.

### Verifying Service Reachability
Once started, test reachability directly:

```bash
# Verify Penpot
curl -i http://localhost:9000/api/main/methods/get-profile

# Verify Plane
curl -i http://localhost:8091/

# Verify Linear
curl -s http://localhost:8085/health
```

---

## 3. Installing Tools Directly on the Target Computer

For developers who prefer running tools directly on the host without Docker virtualization:

### Guided Host Environment Discovery
Inspect what is currently present on your machine:
```bash
dendrite env doctor
```
Example output:
```
[+] Host Environment Discovery:
    - Operating System:  darwin (arm64)
    - Package Manager:   homebrew
    - Docker Engine:     true
    - Docker Compose:    true
    - Ollama Engine:     true
    - Go Toolchain:      true
    - Node / NPM:        Node: true, NPM: true
    - Tailscale Mesh:    true
    - Linear Host CLI:   true
```

### Installing Specific Tools on the Host
You can instruct Dendrite to generate the exact installation command and execute it:

```bash
# Install Linear CLI on the target host (via npm, brew, or binary)
dendrite env install linear --mode host

# Install and bootstrap Ollama on the host
dendrite env install ollama --mode host

# Install Tailscale mesh daemon on the host
dendrite env install tailscale --mode host
```

---

## 4. Replicating Linear: Container vs. Host

| Capability | Containerized Linear (`--mode container`) | Direct Host Installation (`--mode host`) |
| :--- | :--- | :--- |
| **Primary Use Case** | Local offline testing, CI/CD pipelines, autonomous agent sandbox testing | Team management, interactive ticket triage, CLI integration |
| **Backend Storage** | Isolated PostgreSQL 16 container (`localhost:5433`) | Linear Cloud GraphQL API (`api.linear.app`) |
| **Network Isolation** | Private Docker bridge network `dendrite-mesh` | Host loopback / Internet egress |
| **Port Mapping** | `8085:8085` | N/A (CLI binary) |
| **Health Probe** | `GET http://localhost:8085/health` | `linear --version` |
| **Installation Command**| `dendrite env install linear --mode container` | `dendrite env install linear --mode host` |

---

## 5. Environment Verification & Health Monitoring

To verify all components after installation or replication:

```bash
dendrite env status
```

Example status dashboard:
```
[+] Real-Time Environment Component Status:
    [ONLINE / HEALTHY] docker       - Docker Engine & Container Runtime (Endpoint: )
    [ONLINE / HEALTHY] penpot       - Penpot Design Studio (Endpoint: http://localhost:9000)
    [ONLINE / HEALTHY] plane        - Plane v2 Issue Tracker (Endpoint: http://localhost:8091)
    [ONLINE / HEALTHY] linear       - Linear Issue Tracker (Containerized / Local Service) (Endpoint: http://localhost:8085)
    [ONLINE / HEALTHY] litellm      - LiteLLM Token Governance Gateway (Endpoint: http://localhost:4000)
    [ONLINE / HEALTHY] ollama       - Ollama Local Inference Engine (Endpoint: http://localhost:11434)
```


---

## 6. Querying Replicated Environments & LLM Configuration

Once your environment is replicated and services like Ollama and LiteLLM are online, you can ask Dendrite questions directly about the project and check the state of available LLMs using the local-first RAG engine:

```bash
# Query the project about available LLMs using local RAG
dendrite ask "I want to know how to check available LLMs"

# With automated paid escalation if local inference fails
dendrite ask -y "I want to know how to check available LLMs"
```

For complete details, interactive human escalation workflows, and options, see the dedicated [Project Q&A with Local RAG Tutorial](./asking-questions-to-dendrite.md).
