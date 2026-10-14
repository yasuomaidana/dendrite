package env

import (
	"context"
	"fmt"
	"net/http"
	"strings"
	"time"
)

// PlaneConfig defines parameters for containerized Plane v2 installation.
type PlaneConfig struct {
	Port          int    `json:"port"`
	DBPort        int    `json:"db_port"`
	RedisPort     int    `json:"redis_port"`
	ContainerName string `json:"container_name"`
	LocalEndpoint string `json:"local_endpoint"`
	DockerImage   string `json:"docker_image"`
	SecretKey     string `json:"secret_key"`
}

// DefaultPlaneConfig returns sensible defaults for replicating Plane in containers.
func DefaultPlaneConfig() PlaneConfig {
	return PlaneConfig{
		Port:          8090,
		DBPort:        5434,
		RedisPort:     6380,
		ContainerName: "dendrite-plane-api",
		LocalEndpoint: "http://localhost:8090",
		DockerImage:   "makeplane/plane-backend:v0.20-dev",
		SecretKey:     "dendrite_plane_secret_key_mock_local",
	}
}

// GeneratePlaneCompose renders a valid docker-compose configuration for self-hosted Plane.
func GeneratePlaneCompose(cfg PlaneConfig) string {
	if cfg.Port <= 0 {
		cfg.Port = 8090
	}
	if cfg.DBPort <= 0 {
		cfg.DBPort = 5434
	}
	if cfg.RedisPort <= 0 {
		cfg.RedisPort = 6380
	}
	if cfg.ContainerName == "" {
		cfg.ContainerName = "dendrite-plane-api"
	}
	if cfg.DockerImage == "" {
		cfg.DockerImage = "makeplane/plane-backend:v0.20-dev"
	}
	if cfg.SecretKey == "" {
		cfg.SecretKey = "dendrite_plane_secret_key_mock_local"
	}

	return fmt.Sprintf(`version: "3.8"

networks:
  dendrite-mesh:
    driver: bridge

volumes:
  plane-postgres-data:
  plane-redis-data:
  plane-uploads:

services:
  plane-postgres:
    image: postgres:16-alpine
    container_name: dendrite-plane-db
    restart: unless-stopped
    environment:
      POSTGRES_DB: plane_dev
      POSTGRES_USER: plane_user
      POSTGRES_PASSWORD: plane_password
    ports:
      - "%d:5432"
    volumes:
      - plane-postgres-data:/var/lib/postgresql/data
    networks:
      - dendrite-mesh
    healthcheck:
      test: ["CMD-SHELL", "pg_isready -U plane_user -d plane_dev"]
      interval: 5s
      timeout: 3s
      retries: 5

  plane-redis:
    image: redis:7-alpine
    container_name: dendrite-plane-redis
    restart: unless-stopped
    ports:
      - "%d:6379"
    volumes:
      - plane-redis-data:/data
    networks:
      - dendrite-mesh
    healthcheck:
      test: ["CMD", "redis-cli", "ping"]
      interval: 5s
      timeout: 3s
      retries: 5

  plane-api:
    image: %s
    container_name: %s
    restart: unless-stopped
    depends_on:
      plane-postgres:
        condition: service_healthy
      plane-redis:
        condition: service_healthy
    environment:
      DATABASE_URL: "postgresql://plane_user:plane_password@plane-postgres:5432/plane_dev"
      REDIS_URL: "redis://plane-redis:6379/0"
      PORT: %d
      WEB_URL: "http://localhost:%d"
      SECRET_KEY: "%s"
    ports:
      - "%d:%d"
    volumes:
      - plane-uploads:/app/uploads
    networks:
      - dendrite-mesh
    healthcheck:
      test: ["CMD", "wget", "-qO-", "http://localhost:%d/api/health/"]
      interval: 10s
      timeout: 5s
      retries: 3
`, cfg.DBPort, cfg.RedisPort, cfg.DockerImage, cfg.ContainerName, cfg.Port, cfg.Port, cfg.SecretKey, cfg.Port, cfg.Port, cfg.Port)
}

// GenerateHostPlaneInstallCommand returns host installation instructions for Plane CLI.
func GenerateHostPlaneInstallCommand(info HostInfo) string {
	if info.HasNpm {
		return "npm install -g @makeplane/plane-cli"
	}
	if info.PackageManager == PkgBrew {
		return "brew install makeplane/tap/plane"
	}
	return "curl -sSL https://raw.githubusercontent.com/makeplane/plane/develop/install.sh | bash"
}

// CheckPlaneHealth tests the reachability of a running Plane service instance.
func CheckPlaneHealth(ctx context.Context, endpoint string, timeout time.Duration) bool {
	if endpoint == "" {
		endpoint = "http://localhost:8090"
	}
	trimmed := strings.TrimRight(endpoint, "/")
	healthURL := fmt.Sprintf("%s/api/health/", trimmed)

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
