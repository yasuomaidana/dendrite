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

// writeStackFileIfAbsent writes a generated stack file only when it does not exist yet,
// so user-maintained compose files (keys, mounts, extra services) are never reverted.
func writeStackFileIfAbsent(outPath, content string) error {
	if _, err := os.Stat(outPath); err == nil {
		return nil
	} else if !os.IsNotExist(err) {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(outPath), 0755); err != nil {
		return err
	}
	return os.WriteFile(outPath, []byte(content), 0644)
}

func (r *Replicator) handleRenderAction(action PlanAction) (string, error) {
	switch action.Target {
	case TargetLinear:
		cfg := DefaultLinearConfig()
		yamlContent := GenerateLinearCompose(cfg)
		outPath := filepath.Join("stacks", "compose.linear.yaml")
		if err := writeStackFileIfAbsent(outPath, yamlContent); err != nil {
			return "", err
		}
		return outPath, nil

	case TargetPlane:
		cfg := DefaultPlaneConfig()
		yamlContent := GeneratePlaneCompose(cfg)
		outPath := filepath.Join("stacks", "compose.plane.yaml")
		if err := writeStackFileIfAbsent(outPath, yamlContent); err != nil {
			return "", err
		}
		proxyConfPath := filepath.Join(filepath.Dir(outPath), "plane_proxy.conf")
		if _, err := os.Stat(proxyConfPath); os.IsNotExist(err) {
			_ = os.WriteFile(proxyConfPath, []byte(PlaneProxyNginxConfig), 0644)
		}
		return outPath, nil

	case TargetPenpot:
		cfg := DefaultPenpotConfig()
		yamlContent := GeneratePenpotCompose(cfg)
		outPath := filepath.Join("stacks", "compose.penpot.yaml")
		if err := writeStackFileIfAbsent(outPath, yamlContent); err != nil {
			return "", err
		}
		return outPath, nil

	case TargetLiteLLM, TargetPostgres, TargetRedis:
		yamlContent := GenerateControlCompose()
		outPath := filepath.Join("stacks", "compose.control.yaml")
		if err := writeStackFileIfAbsent(outPath, yamlContent); err != nil {
			return "", err
		}
		initSqlPath := filepath.Join(filepath.Dir(outPath), "init-rag.sql")
		if _, err := os.Stat(initSqlPath); os.IsNotExist(err) {
			_ = os.WriteFile(initSqlPath, []byte(RagInitSQL), 0644)
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
	planeUIReachable := CheckPlaneUIHealth(ctx, "http://localhost:8091", 1*time.Second)
	planeDetails := "Plane v2 Project Management (Self-Hosted / Containerized)"
	planeEndpoint := "http://localhost:8090"
	if planeUIReachable {
		planeDetails = "Plane v2 Project Management & Web UI (Self-Hosted / Containerized)"
		planeEndpoint = "http://localhost:8090 (UI: http://localhost:8091)"
	}
	statuses = append(statuses, ComponentStatus{
		Target:    TargetPlane,
		Mode:      ModeContainer,
		Installed: planeReachable || planeUIReachable,
		Reachable: planeReachable || planeUIReachable,
		Endpoint:  planeEndpoint,
		Details:   planeDetails,
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

// RagInitSQL initializes the dendrite_rag database and pgvector extension for the shared tier.
const RagInitSQL = `-- dendrite-rag shared tier: the dendrite_rag role and database with pgvector.
-- Runs on a fresh volume through /docker-entrypoint-initdb.d; idempotent, so an existing
-- stack can apply it too:
--   docker exec -i dendrite-postgres psql -U dendrite -d postgres < stacks/init-rag.sql
SELECT 'CREATE ROLE dendrite_rag WITH LOGIN PASSWORD ''dendrite_rag'''
WHERE NOT EXISTS (SELECT FROM pg_roles WHERE rolname = 'dendrite_rag')\gexec
SELECT 'CREATE DATABASE dendrite_rag OWNER dendrite_rag'
WHERE NOT EXISTS (SELECT FROM pg_database WHERE datname = 'dendrite_rag')\gexec
\connect dendrite_rag
CREATE EXTENSION IF NOT EXISTS vector;
GRANT ALL ON SCHEMA public TO dendrite_rag;
`

// GenerateControlCompose returns standard docker compose YAML for the control plane.
// GenerateControlCompose returns the control-plane stack (Postgres, Redis, LiteLLM).
// It must stay identical to stacks/compose.control.yaml (guarded by TestGenerateControlComposeMatchesCommittedStack).
func GenerateControlCompose() string {
	return `version: "3.8"

networks:
  dendrite-mesh:
    driver: bridge

volumes:
  litellm-copilot-auth:
  dendrite-postgres-data:
  dendrite-redis-data:

services:
  postgres:
    image: pgvector/pgvector:pg16
    container_name: dendrite-postgres
    restart: unless-stopped
    environment:
      POSTGRES_DB: ${POSTGRES_DB:-dendrite}
      POSTGRES_USER: ${POSTGRES_USER:-dendrite}
      POSTGRES_PASSWORD: ${POSTGRES_PASSWORD:-dendrite_password}
    ports:
      - "${POSTGRES_PORT:-5432}:5432"
    volumes:
      - dendrite-postgres-data:/var/lib/postgresql/data
      - ./init-rag.sql:/docker-entrypoint-initdb.d/init-rag.sql:ro
    networks:
      - dendrite-mesh
    healthcheck:
      test: ["CMD-SHELL", "pg_isready -U ${POSTGRES_USER:-dendrite} -d ${POSTGRES_DB:-dendrite}"]
      interval: 5s
      timeout: 3s
      retries: 5

  redis:
    image: redis:7-alpine
    container_name: dendrite-redis
    restart: unless-stopped
    command: redis-server --appendonly yes
    ports:
      - "${REDIS_PORT:-6379}:6379"
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
    command: ["--config", "/etc/litellm/config.yaml", "--port", "${LITELLM_PORT:-4000}"]
    ports:
      - "${LITELLM_PORT:-4000}:4000"
    environment:
      DATABASE_URL: "postgresql://${POSTGRES_USER:-dendrite}:${POSTGRES_PASSWORD:-dendrite_password}@postgres:5432/${POSTGRES_DB:-dendrite}"
      STORE_MODEL_IN_DB: "${STORE_MODEL_IN_DB:-True}"
      # Generated by 'dendrite env secrets ensure' (DEN-902); compose refuses to start without it.
      LITELLM_MASTER_KEY: "${LITELLM_MASTER_KEY:?LITELLM_MASTER_KEY is unset - run 'dendrite env secrets ensure'}"
      # Encrypts credentials stored in Postgres; must stay stable across master-key rotations.
      LITELLM_SALT_KEY: "${LITELLM_SALT_KEY:?LITELLM_SALT_KEY is unset - run 'dendrite env secrets ensure'}"
    volumes:
      - ./litellm_config.yaml:/etc/litellm/config.yaml
      # GitHub Copilot OAuth token from the one-time device login (dendrite gateway copilot-login)
      - litellm-copilot-auth:/root/.config/litellm/github_copilot
    extra_hosts:
      - "host.docker.internal:host-gateway"
    networks:
      - dendrite-mesh
    depends_on:
      postgres:
        condition: service_healthy
`
}
