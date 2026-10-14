package env

import (
	"context"
	"fmt"
	"net/http"
	"strings"
	"time"
)

// PlaneProxyNginxConfig provides reverse-proxy routing between Plane frontend and backend API.
const PlaneProxyNginxConfig = `server {
    listen 80;
    server_name _;

    client_max_body_size 50M;

    location / {
        proxy_pass http://plane-ui:3000;
        proxy_http_version 1.1;
        proxy_set_header Upgrade $http_upgrade;
        proxy_set_header Connection "upgrade";
        proxy_set_header Host $http_host;
        proxy_set_header X-Real-IP $remote_addr;
        proxy_set_header X-Forwarded-For $proxy_add_x_forwarded_for;
        proxy_set_header X-Forwarded-Proto $scheme;
    }

    location /api/ {
        proxy_pass http://plane-api:8090/api/;
        proxy_http_version 1.1;
        proxy_set_header Host $http_host;
        proxy_set_header X-Real-IP $remote_addr;
        proxy_set_header X-Forwarded-For $proxy_add_x_forwarded_for;
        proxy_set_header X-Forwarded-Proto $scheme;
    }

    location /auth/ {
        proxy_pass http://plane-api:8090/auth/;
        proxy_http_version 1.1;
        proxy_set_header Host $http_host;
        proxy_set_header X-Real-IP $remote_addr;
        proxy_set_header X-Forwarded-For $proxy_add_x_forwarded_for;
        proxy_set_header X-Forwarded-Proto $scheme;
    }

    location /uploads/ {
        proxy_pass http://plane-api:8090/uploads/;
        proxy_http_version 1.1;
        proxy_set_header Host $http_host;
        proxy_set_header X-Real-IP $remote_addr;
        proxy_set_header X-Forwarded-For $proxy_add_x_forwarded_for;
        proxy_set_header X-Forwarded-Proto $scheme;
    }
}
`

// PlaneConfig defines parameters for containerized Plane v2 installation.
type PlaneConfig struct {
	Port            int    `json:"port"`
	UIPort          int    `json:"ui_port"`
	DBPort          int    `json:"db_port"`
	RedisPort       int    `json:"redis_port"`
	ContainerName   string `json:"container_name"`
	UIContainerName string `json:"ui_container_name"`
	LocalEndpoint   string `json:"local_endpoint"`
	UIEndpoint      string `json:"ui_endpoint"`
	DockerImage     string `json:"docker_image"`
	UIDockerImage   string `json:"ui_docker_image"`
	SecretKey       string `json:"secret_key"`
	WithUI          *bool  `json:"with_ui,omitempty"`
}

// IsUIEnabled returns true if the Plane Web UI service should be provisioned.
// It is optional and enabled (default on) unless explicitly configured to false.
func (c PlaneConfig) IsUIEnabled() bool {
	if c.WithUI == nil {
		return true
	}
	return *c.WithUI
}

// SetWithUI sets whether the Plane Web UI interface should be included.
func (c *PlaneConfig) SetWithUI(enabled bool) {
	c.WithUI = &enabled
}

// DefaultPlaneConfig returns sensible defaults for replicating Plane in containers.
func DefaultPlaneConfig() PlaneConfig {
	withUI := true
	return PlaneConfig{
		Port:            8090,
		UIPort:          8091,
		DBPort:          5434,
		RedisPort:       6380,
		ContainerName:   "dendrite-plane-api",
		UIContainerName: "dendrite-plane-ui",
		LocalEndpoint:   "http://localhost:8090",
		UIEndpoint:      "http://localhost:8091",
		DockerImage:     "makeplane/plane-backend:v0.20-dev",
		UIDockerImage:   "makeplane/plane-frontend:v0.20-dev",
		SecretKey:       "dendrite_plane_secret_key_mock_local",
		WithUI:          &withUI,
	}
}

// GeneratePlaneCompose renders a valid docker-compose configuration for self-hosted Plane.
func GeneratePlaneCompose(cfg PlaneConfig) string {
	if cfg.Port <= 0 {
		cfg.Port = 8090
	}
	if cfg.UIPort <= 0 {
		cfg.UIPort = 8091
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
	if cfg.UIContainerName == "" {
		cfg.UIContainerName = "dendrite-plane-ui"
	}
	if cfg.DockerImage == "" {
		cfg.DockerImage = "makeplane/plane-backend:v0.20-dev"
	}
	if cfg.UIDockerImage == "" {
		cfg.UIDockerImage = "makeplane/plane-frontend:v0.20-dev"
	}
	if cfg.SecretKey == "" {
		cfg.SecretKey = "dendrite_plane_secret_key_mock_local"
	}

	uiService := ""
	if cfg.IsUIEnabled() {
		uiService = fmt.Sprintf(`
  plane-ui:
    image: %s
    container_name: %s
    restart: unless-stopped
    command: node web/server.js web
    depends_on:
      - plane-api
    environment:
      PORT: 3000
      HOSTNAME: "0.0.0.0"
      NEXT_PUBLIC_API_BASE_URL: "http://localhost:${PLANE_UI_PORT:-%d}"
      NEXT_PUBLIC_WEB_BASE_URL: "http://localhost:${PLANE_UI_PORT:-%d}"
      WEB_URL: "http://localhost:${PLANE_UI_PORT:-%d}"
    expose:
      - "3000"
    networks:
      - dendrite-mesh
    healthcheck:
      test: ["CMD", "wget", "-qO-", "http://127.0.0.1:3000/"]
      interval: 10s
      timeout: 5s
      retries: 3

  plane-proxy:
    image: nginx:alpine
    container_name: dendrite-plane-proxy
    restart: unless-stopped
    depends_on:
      - plane-api
      - plane-ui
    ports:
      - "${PLANE_UI_PORT:-%d}:80"
    volumes:
      - ./plane_proxy.conf:/etc/nginx/conf.d/default.conf:ro
    networks:
      - dendrite-mesh
    healthcheck:
      test: ["CMD", "wget", "-qO-", "http://127.0.0.1:80/"]
      interval: 10s
      timeout: 5s
      retries: 3
`, cfg.UIDockerImage, cfg.UIContainerName, cfg.UIPort, cfg.UIPort, cfg.UIPort, cfg.UIPort)
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
      POSTGRES_DB: ${PLANE_DB_NAME:-plane_dev}
      POSTGRES_USER: ${PLANE_DB_USER:-plane_user}
      POSTGRES_PASSWORD: ${PLANE_DB_PASSWORD:-plane_password}
    ports:
      - "${PLANE_DB_PORT:-%d}:5432"
    volumes:
      - plane-postgres-data:/var/lib/postgresql/data
    networks:
      - dendrite-mesh
    healthcheck:
      test: ["CMD-SHELL", "pg_isready -U ${PLANE_DB_USER:-plane_user} -d ${PLANE_DB_NAME:-plane_dev}"]
      interval: 5s
      timeout: 3s
      retries: 5

  plane-redis:
    image: redis:7-alpine
    container_name: dendrite-plane-redis
    restart: unless-stopped
    ports:
      - "${PLANE_REDIS_PORT:-%d}:6379"
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
    command: python manage.py runserver 0.0.0.0:${PLANE_API_PORT:-%d}
    depends_on:
      plane-postgres:
        condition: service_healthy
      plane-redis:
        condition: service_healthy
    environment:
      DATABASE_URL: "postgresql://${PLANE_DB_USER:-plane_user}:${PLANE_DB_PASSWORD:-plane_password}@plane-postgres:5432/${PLANE_DB_NAME:-plane_dev}"
      REDIS_URL: "redis://plane-redis:6379/0"
      PORT: ${PLANE_API_PORT:-%d}
      WEB_URL: "http://localhost:${PLANE_UI_PORT:-%d}"
      SECRET_KEY: "${PLANE_SECRET_KEY:-%s}"
    ports:
      - "${PLANE_API_PORT:-%d}:${PLANE_API_PORT:-%d}"
    volumes:
      - plane-uploads:/app/uploads
    networks:
      - dendrite-mesh
    healthcheck:
      test: ["CMD", "wget", "-qO-", "http://127.0.0.1:${PLANE_API_PORT:-%d}/api/instances/"]
      interval: 10s
      timeout: 5s
      retries: 3
%s`, cfg.DBPort, cfg.RedisPort, cfg.DockerImage, cfg.ContainerName, cfg.Port, cfg.Port, cfg.UIPort, cfg.SecretKey, cfg.Port, cfg.Port, cfg.Port, uiService)
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

	client := &http.Client{
		Timeout: timeout,
	}

	for _, path := range []string{"/api/instances/", "/api/health/", "/"} {
		healthURL := fmt.Sprintf("%s%s", trimmed, path)
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, healthURL, nil)
		if err != nil {
			continue
		}
		resp, err := client.Do(req)
		if err == nil {
			_ = resp.Body.Close()
			if resp.StatusCode == http.StatusOK {
				return true
			}
		}
	}

	return false
}

// CheckPlaneUIHealth tests the reachability of a running Plane web UI interface.
func CheckPlaneUIHealth(ctx context.Context, endpoint string, timeout time.Duration) bool {
	if endpoint == "" {
		endpoint = "http://localhost:8091"
	}
	trimmed := strings.TrimRight(endpoint, "/")

	client := &http.Client{
		Timeout: timeout,
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, trimmed, nil)
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
