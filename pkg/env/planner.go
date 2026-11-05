package env

import (
	"Dendrite/pkg/stacks"
	"fmt"
	"path/filepath"
	"time"
)

// Planner generates sequenced execution plans for environment replication.
type Planner struct {
	host HostInfo
}

// NewPlanner creates a Planner configured with the target host profile.
func NewPlanner(host HostInfo) *Planner {
	return &Planner{host: host}
}

// PlanReplication creates a deterministic ReplicationPlan based on the request and host capabilities.
func (p *Planner) PlanReplication(req ReplicationRequest) (*ReplicationPlan, error) {
	if !req.Mode.IsValid() {
		return nil, fmt.Errorf("invalid install mode: %q (must be container, host, or hybrid)", req.Mode)
	}

	targets := req.Targets
	if len(targets) == 0 {
		targets = []ReplicationTarget{TargetDocker, TargetPostgres, TargetRedis, TargetLiteLLM, TargetOllama}
	}
	if req.WithLinear && !containsTarget(targets, TargetLinear) {
		targets = append(targets, TargetLinear)
	}
	if req.WithPlane && !containsTarget(targets, TargetPlane) {
		targets = append(targets, TargetPlane)
	}
	if req.WithPenpot && !containsTarget(targets, TargetPenpot) {
		targets = append(targets, TargetPenpot)
	}
	if req.WithGrafana && !containsTarget(targets, TargetGrafana) {
		targets = append(targets, TargetGrafana)
	}

	plan := &ReplicationPlan{
		CreatedAt:     time.Now().UTC(),
		Mode:          req.Mode,
		TargetHost:    p.host,
		Actions:       make([]PlanAction, 0),
		ComposeFiles:  make([]string, 0),
		EstimatedSecs: 0,
	}

	switch req.Mode {
	case ModeContainer:
		p.planContainerMode(req, targets, plan)
	case ModeHost:
		p.planHostMode(req, targets, plan)
	case ModeHybrid:
		p.planHybridMode(req, targets, plan)
	}

	return plan, nil
}

func (p *Planner) planContainerMode(req ReplicationRequest, targets []ReplicationTarget, plan *ReplicationPlan) {
	// 1. Dependency: Docker Engine & Compose
	plan.Actions = append(plan.Actions, PlanAction{
		ID:          "check-docker",
		Type:        ActionCheckDependency,
		Target:      TargetDocker,
		Description: "Verify Docker engine and Compose availability",
		Command:     "docker info && docker compose version",
		Mandatory:   true,
	})
	plan.EstimatedSecs += 5

	outDir := req.OutputDir
	if outDir == "" {
		outDir = stacks.Dir()
	}

	// 2. Control plane / Dev compose stack
	controlCompose := filepath.Join(outDir, "compose.control.yaml")
	plan.ComposeFiles = append(plan.ComposeFiles, controlCompose)
	plan.Actions = append(plan.Actions, PlanAction{
		ID:          "render-control-compose",
		Type:        ActionRenderCompose,
		Target:      TargetLiteLLM,
		Description: "Render control plane Docker Compose specification",
		Command:     fmt.Sprintf("dendrite compose render --target=control --out=%s", controlCompose),
		Mandatory:   true,
	})
	plan.Actions = append(plan.Actions, PlanAction{
		ID:          "start-control-stack",
		Type:        ActionStartContainer,
		Target:      TargetLiteLLM,
		Description: "Launch persistent database, Redis cache and LiteLLM gateway containers",
		Command:     fmt.Sprintf("docker compose -f %s up -d", controlCompose),
		Rollback:    fmt.Sprintf("docker compose -f %s down", controlCompose),
		Mandatory:   true,
	})
	plan.EstimatedSecs += 30

	// 3. Linear Container Stack (if requested)
	if containsTarget(targets, TargetLinear) {
		linearCompose := filepath.Join(outDir, "compose.linear.yaml")
		plan.ComposeFiles = append(plan.ComposeFiles, linearCompose)
		plan.Actions = append(plan.Actions, PlanAction{
			ID:          "render-linear-compose",
			Type:        ActionRenderCompose,
			Target:      TargetLinear,
			Description: "Render containerized Linear server and dedicated PostgreSQL compose configuration",
			Command:     fmt.Sprintf("dendrite compose render --target=linear --out=%s", linearCompose),
			Mandatory:   true,
		})
		plan.Actions = append(plan.Actions, PlanAction{
			ID:          "start-linear-stack",
			Type:        ActionStartContainer,
			Target:      TargetLinear,
			Description: "Start containerized Linear mock/relay engine and database",
			Command:     fmt.Sprintf("docker compose -f %s up -d", linearCompose),
			Rollback:    fmt.Sprintf("docker compose -f %s down", linearCompose),
			Mandatory:   true,
		})
		plan.Actions = append(plan.Actions, PlanAction{
			ID:          "verify-linear-health",
			Type:        ActionVerifyHealth,
			Target:      TargetLinear,
			Description: "Assert HTTP 200 OK on containerized Linear health probe",
			Command:     "curl -sSf http://localhost:8085/health",
			Mandatory:   true,
		})
		plan.EstimatedSecs += 25
	}

	// 4. Plane Container Stack (if requested)
	if containsTarget(targets, TargetPlane) {
		planeCompose := filepath.Join(outDir, "compose.plane.yaml")
		plan.ComposeFiles = append(plan.ComposeFiles, planeCompose)
		plan.Actions = append(plan.Actions, PlanAction{
			ID:          "render-plane-compose",
			Type:        ActionRenderCompose,
			Target:      TargetPlane,
			Description: "Render self-hosted Plane v2 project management stack configuration",
			Command:     fmt.Sprintf("dendrite compose render --target=plane --out=%s", planeCompose),
			Mandatory:   true,
		})
		plan.Actions = append(plan.Actions, PlanAction{
			ID:          "start-plane-stack",
			Type:        ActionStartContainer,
			Target:      TargetPlane,
			Description: "Start containerized Plane v2 backend, frontend UI, PostgreSQL and Redis services",
			Command:     fmt.Sprintf("docker compose -f %s up -d", planeCompose),
			Rollback:    fmt.Sprintf("docker compose -f %s down", planeCompose),
			Mandatory:   true,
		})
		plan.Actions = append(plan.Actions, PlanAction{
			ID:          "verify-plane-health",
			Type:        ActionVerifyHealth,
			Target:      TargetPlane,
			Description: "Assert HTTP 200 OK on self-hosted Plane health probe",
			Command:     "curl -sSf http://localhost:8090/api/instances/ || curl -sSf http://localhost:8090/api/health/",
			Mandatory:   true,
		})
		if req.WithPlaneUI == nil || *req.WithPlaneUI {
			plan.Actions = append(plan.Actions, PlanAction{
				ID:          "verify-plane-ui-health",
				Type:        ActionVerifyHealth,
				Target:      TargetPlane,
				Description: "Assert HTTP 200 OK on self-hosted Plane web UI interface",
				Command:     "curl -sSf http://localhost:8091/",
				Mandatory:   true,
			})
		}
		plan.EstimatedSecs += 30
	}

	// 5. Penpot Container Stack (if requested)
	if containsTarget(targets, TargetPenpot) {
		penpotCompose := filepath.Join(outDir, "compose.penpot.yaml")
		plan.ComposeFiles = append(plan.ComposeFiles, penpotCompose)
		plan.Actions = append(plan.Actions, PlanAction{
			ID:          "render-penpot-compose",
			Type:        ActionRenderCompose,
			Target:      TargetPenpot,
			Description: "Render self-hosted Penpot design and prototyping stack configuration",
			Command:     fmt.Sprintf("dendrite compose render --target=penpot --out=%s", penpotCompose),
			Mandatory:   true,
		})
		plan.Actions = append(plan.Actions, PlanAction{
			ID:          "start-penpot-stack",
			Type:        ActionStartContainer,
			Target:      TargetPenpot,
			Description: "Start containerized Penpot frontend, backend, PostgreSQL and Redis services",
			Command:     fmt.Sprintf("docker compose -f %s up -d", penpotCompose),
			Rollback:    fmt.Sprintf("docker compose -f %s down", penpotCompose),
			Mandatory:   true,
		})
		plan.Actions = append(plan.Actions, PlanAction{
			ID:          "verify-penpot-health",
			Type:        ActionVerifyHealth,
			Target:      TargetPenpot,
			Description: "Assert HTTP 200 OK on self-hosted Penpot frontend probe",
			Command:     "curl -sSf http://localhost:9000/",
			Mandatory:   true,
		})
		plan.EstimatedSecs += 35
	}

	// 6. Grafana Observability Stack (if requested)
	if containsTarget(targets, TargetGrafana) {
		grafanaCompose := filepath.Join(outDir, "compose.grafana.yaml")
		plan.ComposeFiles = append(plan.ComposeFiles, grafanaCompose)
		plan.Actions = append(plan.Actions, PlanAction{
			ID:          "render-grafana-compose",
			Type:        ActionRenderCompose,
			Target:      TargetGrafana,
			Description: "Render Prometheus and Grafana telemetry observability stack",
			Command:     fmt.Sprintf("dendrite compose render --target=grafana --out=%s", grafanaCompose),
			Mandatory:   true,
		})
		plan.Actions = append(plan.Actions, PlanAction{
			ID:          "start-grafana-stack",
			Type:        ActionStartContainer,
			Target:      TargetGrafana,
			Description: "Launch Prometheus and Grafana telemetry containers",
			Command:     fmt.Sprintf("docker compose -f %s up -d", grafanaCompose),
			Rollback:    fmt.Sprintf("docker compose -f %s down", grafanaCompose),
			Mandatory:   true,
		})
		plan.Actions = append(plan.Actions, PlanAction{
			ID:          "verify-grafana-health",
			Type:        ActionVerifyHealth,
			Target:      TargetGrafana,
			Description: "Assert HTTP 200 OK on Grafana dashboard service",
			Command:     "curl -sSf http://localhost:3000/api/health",
			Mandatory:   true,
		})
		plan.EstimatedSecs += 20
	}
}

func (p *Planner) planHostMode(req ReplicationRequest, targets []ReplicationTarget, plan *ReplicationPlan) {
	for _, t := range targets {
		switch t {
		case TargetLinear:
			installCmd := GenerateHostLinearInstallCommand(p.host)
			plan.Actions = append(plan.Actions, PlanAction{
				ID:          "install-host-linear-cli",
				Type:        ActionInstallHostTool,
				Target:      TargetLinear,
				Description: "Install Linear CLI on target host",
				Command:     installCmd,
				Mandatory:   false,
			})
			plan.EstimatedSecs += 20

		case TargetPlane:
			installCmd := GenerateHostPlaneInstallCommand(p.host)
			plan.Actions = append(plan.Actions, PlanAction{
				ID:          "install-host-plane-cli",
				Type:        ActionInstallHostTool,
				Target:      TargetPlane,
				Description: "Install Plane CLI on target host",
				Command:     installCmd,
				Mandatory:   false,
			})
			plan.EstimatedSecs += 20

		case TargetOllama:
			if !p.host.HasOllama {
				var cmd string
				if p.host.OS == "darwin" && p.host.PackageManager == PkgBrew {
					cmd = "brew install ollama"
				} else {
					cmd = "curl -fsSL https://ollama.com/install.sh | sh"
				}
				plan.Actions = append(plan.Actions, PlanAction{
					ID:          "install-host-ollama",
					Type:        ActionInstallHostTool,
					Target:      TargetOllama,
					Description: "Install Ollama local inference engine on host",
					Command:     cmd,
					Mandatory:   false,
				})
				plan.EstimatedSecs += 35
			}

		case TargetTailscale:
			if !p.host.HasTailscale {
				var cmd string
				if p.host.OS == "darwin" && p.host.PackageManager == PkgBrew {
					cmd = "brew install --cask tailscale"
				} else {
					cmd = "curl -fsSL https://tailscale.com/install.sh | sh"
				}
				plan.Actions = append(plan.Actions, PlanAction{
					ID:          "install-host-tailscale",
					Type:        ActionInstallHostTool,
					Target:      TargetTailscale,
					Description: "Install Tailscale mesh daemon on host",
					Command:     cmd,
					Mandatory:   false,
				})
				plan.EstimatedSecs += 30
			}

		case TargetCloudflare:
			var cmd string
			if p.host.OS == "darwin" && p.host.PackageManager == PkgBrew {
				cmd = "brew install cloudflared"
			} else {
				cmd = "curl -fsSL https://pkg.cloudflare.com/cloudflared-ascii.repo | sh"
			}
			plan.Actions = append(plan.Actions, PlanAction{
				ID:          "install-host-cloudflared",
				Type:        ActionInstallHostTool,
				Target:      TargetCloudflare,
				Description: "Install Cloudflare Tunnel daemon on host",
				Command:     cmd,
				Mandatory:   false,
			})
			plan.EstimatedSecs += 25
		}
	}
}

func (p *Planner) planHybridMode(req ReplicationRequest, targets []ReplicationTarget, plan *ReplicationPlan) {
	// Hybrid mode: Docker containers for persistent services (Postgres, Redis, LiteLLM, Plane, Linear, Penpot)
	// Host installation for developer tools (Ollama Metal, Tailscale, Cloudflare, Plane/Linear CLI)
	p.planContainerMode(req, targets, plan)

	if containsTarget(targets, TargetOllama) && !p.host.HasOllama {
		var cmd string
		if p.host.OS == "darwin" && p.host.PackageManager == PkgBrew {
			cmd = "brew install ollama"
		} else {
			cmd = "curl -fsSL https://ollama.com/install.sh | sh"
		}
		plan.Actions = append(plan.Actions, PlanAction{
			ID:          "install-host-ollama-metal",
			Type:        ActionInstallHostTool,
			Target:      TargetOllama,
			Description: "Install Ollama Metal on target host for zero-cost hardware acceleration",
			Command:     cmd,
			Mandatory:   false,
		})
		plan.EstimatedSecs += 35
	}

	if containsTarget(targets, TargetPlane) {
		plan.Actions = append(plan.Actions, PlanAction{
			ID:          "install-host-plane-cli",
			Type:        ActionInstallHostTool,
			Target:      TargetPlane,
			Description: "Install Plane CLI on target host",
			Command:     GenerateHostPlaneInstallCommand(p.host),
			Mandatory:   false,
		})
		plan.EstimatedSecs += 20
	}
}

func containsTarget(targets []ReplicationTarget, t ReplicationTarget) bool {
	for _, item := range targets {
		if item == t {
			return true
		}
	}
	return false
}
