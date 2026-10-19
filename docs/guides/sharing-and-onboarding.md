---
id: sharing-and-onboarding
title: "Environment Sharing, Onboarding & Bootstrap Guide"
sidebar_label: "Sharing & Onboarding"
sidebar_position: 4
tags: [onboarding, bootstrap, sharing, teams]
---

# Environment Sharing, Onboarding & Bootstrap Guide

This guide details the methodology for distributing, installing, and sharing the **Dendrite** development environment and multi-project configurations across engineering teams and machines via **GitHub** and encrypted artifacts.

---

## 1. Environment Sharing Architecture & Secret Decoupling

When distributing development environments across multiple team members, two major risks must be mitigated:
1. **Hardcoded Credentials & Leaked API Keys**: Private tokens (Gemini keys, Copilot credentials, database passwords) must never be committed to Git or unencrypted bundles.
2. **Environment Divergence**: Different host OS types (Apple Silicon macOS vs. Linux x86_64) must run identical containerized tooling without architecture conflicts.

### The Dendrite GitHub Sharing Model

```mermaid
flowchart TD
    LeadDev[Team Lead / DevOps] -->|1. Export Profile| ExportCmd["dendrite share export<br/>(Redacts Secrets & Encrypts via age)"]
    ExportCmd --> Bundle[Encrypted Project Bundle: project.dendrite.bundle]
    
    Bundle --> GitHubStore[GitHub Private Repo / Git LFS / Releases]
    GitHubStore --> NewDev[Onboarding Engineer]
    
    NewDev -->|2. GitHub Release Bootstrap| Install["curl -sSL https://raw.githubusercontent.com/dendrite-dev/dendrite/main/scripts/install.sh | bash"]
    Install --> ImportCmd["dendrite share import project.dendrite.bundle"]
    ImportCmd --> ReplicateCmd["dendrite env replicate --mode container"]
    ReplicateCmd --> Ready["dendrite env status<br/>Ready for Production"]
```

---

## 2. Prerequisites & Host Requirements

### Primary Developer Workstation (MacBook Air / macOS)
- **OS**: macOS 14+ (Sonoma or Sequoia) on Apple Silicon (M1/M2/M3/M4).
- **Core Tools**: Docker Desktop or Colima, Homebrew, Git, GitHub CLI (`gh`).
- **Mesh Network**: Tailscale client installed and logged in.
- **Python / Tools**: `uv` package manager (`brew install uv`).

### Persistent Control Plane (Debian Server)
- **OS**: Debian 12 (Bookworm) or Ubuntu 24.04 LTS.
- **Resources**: 2–4 vCPUs, 4–8 GB RAM, 50 GB SSD.
- **Services**: Docker Engine 26+, Docker Compose v2, Tailscale daemon.
- **User Permissions**: Passwordless SSH access for the `deployer` user.

---

## 3. Installation & Updates

### Option A: Build from source (current path)
No release binaries are published yet, so build from a clone:
```bash
git clone https://github.com/yasuomaidana/dendrite.git
cd dendrite
go install ./cmd/dendrite        # installs to $GOBIN or $GOPATH/bin
dendrite version
```

### Option B: Update an existing install
```bash
dendrite update --dev            # inside the checkout: sync synapse siblings, rebuild them and dendrite
dendrite update                  # once releases exist: download, verify SHA-256, swap with rollback
```
Release assets are expected as `dendrite_<os>_<arch>` plus `dendrite_<os>_<arch>.sha256` on `github.com/yasuomaidana/dendrite/releases`. Until a release workflow publishes them, `dendrite update` reports that none exists and points to `--dev`.

See [DEVELOPMENT.md](../DEVELOPMENT.md) for working across the sibling repositories (local vs. remote sources).

---

## 4. Team Onboarding Workflow (Step-by-Step)

### Step 1: Pre-Flight Verification
Run the diagnostics engine to verify local system compatibility:
```bash
dendrite soma diagnose
dendrite env doctor
```

### Step 2: Mesh Network Authentication
Join the team's private Tailnet to gain encrypted, zero-trust access to the Control Plane:
```bash
tailscale up --auth-key=tskey-auth-kXXXXX-XXXXXXXX
```

### Step 3: Decrypted Secret Injection
Dendrite integrates with **SOPS** and **age** (or 1Password CLI) for secret injection.
Create or decrypt the local environment configuration:
```bash
# Decrypt team environment template using your personal age private key
sops -d config/secrets.enc.env > .env
```
Key required environment variables:
```bash
LITELLM_MASTER_KEY="sk-admin-master-XXXXX"
GEMINI_API_KEY="AIzaSyXXXXX"
DB_PASSWORD="super_secure_postgres_pass"
N8N_PASSWORD="secure_n8n_pass"
```

### Step 4: Replicate Development Environment
Replicate all required services either in **Docker containers** or **directly on the host**:

```bash
# Option A: Full Containerized Stack (includes local Linear container + LiteLLM + Postgres)
dendrite env replicate --mode container

# Option B: Direct Host Tool Installation (e.g., Linear CLI, Ollama on host)
dendrite env replicate --mode host

# Verify status of all components
dendrite env status
```

### Step 5: Import Project Bundle from GitHub
Download and import the encrypted project workspace definition shared in the team's GitHub repository:
```bash
# Clone bundle or fetch from GitHub Release / LFS
dendrite share import team-backend-service.dendrite.bundle
```
This initializes:
- Local AST cache in `~/.dendrite/projects/team-backend-service/`.
- Registers dedicated LiteLLM Virtual Key for budget tracking.
- Pre-indexes Tree-sitter symbol definitions.

### Step 6: Launch Environment
```bash
dendrite up local-hybrid
```
Your local GoLand / IDE can now point directly to the LiteLLM gateway (`http://debian-server.ts.net:4000/v1`) using your developer virtual key!

---

## 5. Sharing Method: `dendrite share export`

To share an existing workspace or project environment with a teammate:

```bash
# 1. Export the active project profile
dendrite share export \
  --project backend-service \
  --recipient-key age1ql3z7hjy54pw302... \
  --include-docs \
  --out backend-service.bundle

# 2. Push backend-service.bundle to your project's private GitHub repository or release assets
```

**Security Guarantees of Export Bundles:**
- Plaintext API keys and passwords are automatically scrubbed or encrypted with the recipient's public key.
- Host-specific file paths (e.g. `/Users/username/...` or `/home/user/...`) are sanitized into relative project tokens (`$PROJECT_ROOT`).
- Pre-compiled Graphify AST indices and Docusaurus feature specifications are bundled to eliminate repetitive cold indexing on the recipient's machine.
