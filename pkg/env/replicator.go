package env

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"time"
)

// Replicator executes the replication plan on the target environment.
type Replicator struct {
	runner CommandRunner
	host   HostInfo
}

// NewReplicator constructs a Replicator instance.
func NewReplicator(runner CommandRunner, host HostInfo) *Replicator {
	if runner == nil {
		runner = &DefaultCommandRunner{}
	}
	return &Replicator{
		runner: runner,
		host:   host,
	}
}

// ExecutePlan executes each action in the ReplicationPlan sequentially.
func (r *Replicator) ExecutePlan(ctx context.Context, plan *ReplicationPlan, dryRun bool) (*ReplicationResult, error) {
	start := time.Now()
	res := &ReplicationResult{
		Mode:           plan.Mode,
		Success:        true,
		ActionResults:  make([]ActionExecutionResult, 0),
		GeneratedFiles: make([]string, 0),
		Components:     make([]ComponentStatus, 0),
	}

	for _, action := range plan.Actions {
		actionStart := time.Now()
		actRes := ActionExecutionResult{
			ActionID: action.ID,
			Success:  true,
		}

		if dryRun {
			actRes.Skipped = true
			actRes.Output = fmt.Sprintf("[DRY-RUN] Would execute: %s (%s)", action.Command, action.Description)
			actRes.Duration = time.Since(actionStart)
			res.ActionResults = append(res.ActionResults, actRes)
			continue
		}

		// Specialized handling for compose rendering
		if action.Type == ActionRenderCompose {
			renderedPath, err := r.handleRenderAction(action)
			if err != nil {
				actRes.Success = false
				actRes.Error = err.Error()
				actRes.Duration = time.Since(actionStart)
				res.ActionResults = append(res.ActionResults, actRes)
				if action.Mandatory {
					res.Success = false
					res.Error = fmt.Sprintf("mandatory action failed: %s (%s)", action.ID, err)
					return res, nil
				}
				continue
			}
			res.GeneratedFiles = append(res.GeneratedFiles, renderedPath)
			actRes.Output = fmt.Sprintf("rendered specification to %s", renderedPath)
			actRes.Duration = time.Since(actionStart)
			res.ActionResults = append(res.ActionResults, actRes)
			continue
		}

		// Standard command execution
		out, err := r.runner.Run(ctx, action.Command, nil)
		actRes.Output = out
		actRes.Duration = time.Since(actionStart)

		if err != nil {
			actRes.Success = false
			actRes.Error = err.Error()
			res.ActionResults = append(res.ActionResults, actRes)

			if action.Mandatory {
				res.Success = false
				res.Error = fmt.Sprintf("mandatory action %s failed: %v", action.ID, err)

				// Rollback if defined
				if action.Rollback != "" {
					_, _ = r.runner.Run(ctx, action.Rollback, nil)
				}
				return res, nil
			}
		} else {
			res.ActionResults = append(res.ActionResults, actRes)
		}
	}

	res.TotalDuration = time.Since(start)
	res.Components = r.AuditComponents(ctx)
	return res, nil
}

func (r *Replicator) handleRenderAction(action PlanAction) (string, error) {
	switch action.Target {
	case TargetLinear:
		cfg := DefaultLinearConfig()
		yamlContent := GenerateLinearCompose(cfg)
		outPath := filepath.Join("stacks", "compose.linear.yaml")
		if err := os.MkdirAll(filepath.Dir(outPath), 0755); err != nil {
			return "", err
		}
		if err := os.WriteFile(outPath, []byte(yamlContent), 0644); err != nil {
			return "", err
		}
		return outPath, nil

	case TargetPlane:
		cfg := DefaultPlaneConfig()
		yamlContent := GeneratePlaneCompose(cfg)
		outPath := filepath.Join("stacks", "compose.plane.yaml")
		if err := os.MkdirAll(filepath.Dir(outPath), 0755); err != nil {
			return "", err
		}
		if err := os.WriteFile(outPath, []byte(yamlContent), 0644); err != nil {
			return "", err
		}
		return outPath, nil

	case TargetPenpot:
		cfg := DefaultPenpotConfig()
		yamlContent := GeneratePenpotCompose(cfg)
		outPath := filepath.Join("stacks", "compose.penpot.yaml")
		if err := os.MkdirAll(filepath.Dir(outPath), 0755); err != nil {
			return "", err
		}
		if err := os.WriteFile(outPath, []byte(yamlContent), 0644); err != nil {
			return "", err
		}
		return outPath, nil

	case TargetLiteLLM, TargetPostgres, TargetRedis:
		yamlContent := GenerateControlCompose()
		outPath := filepath.Join("stacks", "compose.control.yaml")
		if err := os.MkdirAll(filepath.Dir(outPath), 0755); err != nil {
			return "", err
		}
		if err := os.WriteFile(outPath, []byte(yamlContent), 0644); err != nil {
			return "", err
		}
		return outPath, nil

	default:
		return "", fmt.Errorf("unknown compose target: %s", action.Target)
	}
}

// AuditComponents checks the real-time installation and connectivity of all supported targets.
func (r *Replicator) AuditComponents(ctx context.Context) []ComponentStatus {
	statuses := make([]ComponentStatus, 0)

	// Docker
	dockerInstalled := r.host.HasDocker
	dockerReachable := false
	if dockerInstalled {
		if _, err := r.runner.Run(ctx, "docker info", nil); err == nil {
			dockerReachable = true
		}
	}
	statuses = append(statuses, ComponentStatus{
		Target:    TargetDocker,
		Mode:      ModeHost,
		Installed: dockerInstalled,
		Reachable: dockerReachable,
		Details:   "Docker Engine & Container Runtime",
	})

	// Linear
	linearReachable := CheckLinearHealth(ctx, "http://localhost:8085", 1*time.Second)
	statuses = append(statuses, ComponentStatus{
		Target:    TargetLinear,
		Mode:      ModeContainer,
		Installed: r.host.HasLinearCLI || linearReachable,
		Reachable: linearReachable,
		Endpoint:  "http://localhost:8085",
		Details:   "Linear Issue Tracker (Containerized / Local Service)",
	})

	// Plane
	planeReachable := CheckPlaneHealth(ctx, "http://localhost:8090", 1*time.Second)
	statuses = append(statuses, ComponentStatus{
		Target:    TargetPlane,
		Mode:      ModeContainer,
		Installed: planeReachable,
		Reachable: planeReachable,
		Endpoint:  "http://localhost:8090",
		Details:   "Plane v2 Project Management (Self-Hosted / Containerized)",
	})

	// Penpot
	penpotReachable := CheckPenpotHealth(ctx, "http://localhost:9000", 1*time.Second)
	statuses = append(statuses, ComponentStatus{
		Target:    TargetPenpot,
		Mode:      ModeContainer,
		Installed: penpotReachable,
		Reachable: penpotReachable,
		Endpoint:  "http://localhost:9000",
		Details:   "Penpot Open-Source Design & Prototyping Platform",
	})

	// LiteLLM Gateway
	litellmReachable := false
	if _, err := r.runner.Run(ctx, "curl -s http://localhost:4000/health/readiness", nil); err == nil {
		litellmReachable = true
	}
	statuses = append(statuses, ComponentStatus{
		Target:    TargetLiteLLM,
		Mode:      ModeContainer,
		Installed: true,
		Reachable: litellmReachable,
		Endpoint:  "http://localhost:4000",
		Details:   "LiteLLM Token Governance Gateway",
	})

	// Ollama
	ollamaReachable := false
	if _, err := r.runner.Run(ctx, "curl -s http://localhost:11434/api/tags", nil); err == nil {
		ollamaReachable = true
	}
	statuses = append(statuses, ComponentStatus{
		Target:    TargetOllama,
		Mode:      ModeHost,
		Installed: r.host.HasOllama,
		Reachable: ollamaReachable,
		Endpoint:  "http://localhost:11434",
		Details:   "Ollama Local Inference Engine",
	})

	return statuses
}

// GenerateControlCompose returns standard docker compose YAML for the control plane.
func GenerateControlCompose() string {
	return `version: "3.8"

networks:
  dendrite-mesh:
    driver: bridge

volumes:
  dendrite-postgres-data:
  dendrite-redis-data:

services:
  postgres:
    image: postgres:16-alpine
    container_name: dendrite-postgres
    restart: unless-stopped
    environment:
      POSTGRES_DB: dendrite
      POSTGRES_USER: dendrite
      POSTGRES_PASSWORD: dendrite_password
    ports:
      - "5432:5432"
    volumes:
      - dendrite-postgres-data:/var/lib/postgresql/data
    networks:
      - dendrite-mesh
    healthcheck:
      test: ["CMD-SHELL", "pg_isready -U dendrite -d dendrite"]
      interval: 5s
      timeout: 3s
      retries: 5

  redis:
    image: redis:7-alpine
    container_name: dendrite-redis
    restart: unless-stopped
    command: redis-server --appendonly yes
    ports:
      - "6379:6379"
    volumes:
      - dendrite-redis-data:/data
    networks:
      - dendrite-mesh
    healthcheck:
      test: ["CMD", "redis-cli", "ping"]
      interval: 5s
      timeout: 3s
      retries: 5

  litellm:
    image: ghcr.io/berriai/litellm:main-latest
    container_name: dendrite-litellm
    restart: unless-stopped
    ports:
      - "4000:4000"
    environment:
      DATABASE_URL: "postgresql://dendrite:dendrite_password@postgres:5432/dendrite"
      STORE_MODEL_IN_DB: "True"
    networks:
      - dendrite-mesh
    depends_on:
      postgres:
        condition: service_healthy
`
}
