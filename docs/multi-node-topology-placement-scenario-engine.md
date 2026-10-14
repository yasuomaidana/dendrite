# Multi-Node Topology & Placement Scenario Engine

# Multi-Node Topology & Placement Scenario Engine

# FEAT-001: Multi-Node Topology & Placement Scenario Engine

## 1. User Story & Business Objective

As a software engineer and DevSecOps architect, I need a declarative topology management engine that allows me to switch between local-hybrid, cloud-burst (Azure or AWS), and distributed multi-node deployment scenarios without modifying service container definitions or manually reconfiguring network addresses, so that my workstation battery is preserved, cloud costs are minimized, and persistent state remains uncorrupted.

---

## 2. Technical Boundaries & Specifications

1. **Topology Scenarios**:

- `local-hybrid`: Debian Server (Control Plane) + macOS Workstation (Execution Plane). Cloud resources off.
- `azure-burst`: Azure VM + Managed Disk (Control Plane & Burst Compute) + macOS Workstation. Debian off.
- `aws-burst` (see [FEAT-007](./FEAT-007-aws-cloud-burst.md)): AWS EC2 Spot/On-Demand VM + Persistent EBS (gp3) + macOS Workstation. Core compute offloading.
- `distributed`: Debian Server (State anchor) + macOS Workstation (Local Dev) + Azure or AWS Cloud Node (Heavy Compute).

2. **IaC Tooling**:

- Terraform 1.7+ utilizing `azurerm`, `hashicorp/aws`, and `kreuzwerker/docker` providers.
- SSH contexts (`ssh://deployer@<node>.ts.net`) mapped over Tailscale.
- Dynamic configuration rendering via `templatefile("config/litellm_config.yaml.tftpl", var.placement_map)`.

3. **Docker Compose Profiles**:

- Profiles: `control-plane`, `execution-sandbox`, `rea-linux-worker`.

---

## 3. Acceptance Criteria & Verification Matrix

- [x] **AC-1**: Switching scenario via `dendrite up <scenario>` or `terraform apply -var-file=scenarios/<scenario>.tfvars` applies placement without container recreation if state is unchanged.
- [x] **AC-2**: All inter-node communication routes through Tailscale overlay IPs (`100.x.y.z`) or localhost. Zero public internet ports are exposed.
- [x] **AC-3**: SSH Docker context authentication completes without manual password prompts using pre-shared Ed25519 keys.
- [x] **AC-4**: Rendering of `litellm_config.yaml` injects the active control plane and execution plane IPs dynamically across local, Azure, and AWS topologies.

---

## 4. Definition of Done (DoD) & Persistence Verification

- **Automated Tests**: Integration tests verify Terraform module validation (`terraform validate`) and Compose configuration syntax (`docker compose config`).
- **Persistent Layer Verification**: PostgreSQL and Redis persistence across scenario switches verified using real throwaway test environments managed via [Testcontainers](https://rust.testcontainers.org). Volume unmounting and remounting preserves test table schemas and records.
- **Complexity Guardrails**: Placement logic in CLI exhibits cyclomatic complexity $\le 8$, cognitive complexity $\le 10$.
