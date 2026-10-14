package env

import (
	"context"
	"fmt"
	"net/http"
	"strings"
	"time"
)

// GenerateLinearCompose generates a production-ready docker-compose YAML configuration
// for running a local Linear-compatible service stack with PostgreSQL and GraphQL health probes.
func GenerateLinearCompose(cfg LinearConfig) string {
	if cfg.Port <= 0 {
		cfg.Port = 8085
	}
	if cfg.DBPort <= 0 {
		cfg.DBPort = 5433
	}
	if cfg.ContainerName == "" {
		cfg.ContainerName = "dendrite-linear-server"
	}
	if cfg.DockerImage == "" {
		cfg.DockerImage = "ghcr.io/dendrite-dev/linear-mock:latest"
	}

	return fmt.Sprintf(`version: "3.8"

networks:
  dendrite-mesh:
    driver: bridge

volumes:
  linear-postgres-data:

services:
  linear-postgres:
    image: postgres:16-alpine
    container_name: dendrite-linear-db
    restart: unless-stopped
    environment:
      POSTGRES_DB: linear_dev
      POSTGRES_USER: linear_user
      POSTGRES_PASSWORD: linear_password
    ports:
      - "%d:5432"
    volumes:
      - linear-postgres-data:/var/lib/postgresql/data
    networks:
      - dendrite-mesh
    healthcheck:
      test: ["CMD-SHELL", "pg_isready -U linear_user -d linear_dev"]
      interval: 5s
      timeout: 3s
      retries: 5

  linear-server:
    image: %s
    container_name: %s
    restart: unless-stopped
    depends_on:
      linear-postgres:
        condition: service_healthy
    environment:
      PORT: %d
      DATABASE_URL: "postgres://linear_user:linear_password@linear-postgres:5432/linear_dev?sslmode=disable"
      LINEAR_API_KEY: "%s"
      LINEAR_EMULATOR_MODE: "true"
      LOG_LEVEL: "info"
    ports:
      - "%d:%d"
    networks:
      - dendrite-mesh
    healthcheck:
      test: ["CMD", "wget", "-qO-", "http://localhost:%d/health"]
      interval: 10s
      timeout: 5s
      retries: 3
`, cfg.DBPort, cfg.DockerImage, cfg.ContainerName, cfg.Port, cfg.APIKey, cfg.Port, cfg.Port, cfg.Port)
}

// GenerateHostLinearInstallCommand returns the appropriate host command to install the Linear CLI
// based on package manager and host tools.
func GenerateHostLinearInstallCommand(info HostInfo) string {
	if info.HasNpm {
		return "npm install -g @linear/cli"
	}
	if info.PackageManager == PkgBrew {
		return "brew install linear-cli"
	}
	return "curl -sSL https://raw.githubusercontent.com/linear-app/linear-cli/main/install.sh | bash"
}

// CheckLinearHealth queries the health endpoint of a running containerized or host Linear service.
func CheckLinearHealth(ctx context.Context, endpoint string, timeout time.Duration) bool {
	if endpoint == "" {
		endpoint = "http://localhost:8085"
	}
	trimmed := strings.TrimRight(endpoint, "/")
	healthURL := fmt.Sprintf("%s/health", trimmed)

	client := &http.Client{
		Timeout: timeout,
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, healthURL, nil)
	if err != nil {
		return false
	}

	resp, err := client.Do(req)
	if err != nil {
		return false
	}
	defer resp.Body.Close()

	return resp.StatusCode == http.StatusOK
}
