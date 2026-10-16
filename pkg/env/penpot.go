package env

import (
	"context"
	"fmt"
	"net/http"
	"strings"
	"time"
)

// PenpotConfig defines configuration parameters for Penpot design stack.
type PenpotConfig struct {
	FrontendPort  int    `json:"frontend_port"`
	BackendPort   int    `json:"backend_port"`
	DBPort        int    `json:"db_port"`
	RedisPort     int    `json:"redis_port"`
	LocalEndpoint string `json:"local_endpoint"`
	SecretKey     string `json:"secret_key"`
}

// DefaultPenpotConfig returns sensible default options for replicating Penpot.
func DefaultPenpotConfig() PenpotConfig {
	return PenpotConfig{
		FrontendPort:  9000,
		BackendPort:   9001,
		DBPort:        5435,
		RedisPort:     6381,
		LocalEndpoint: "http://localhost:9000",
		SecretKey:     "dendrite_penpot_secret_key_mock_local",
	}
}

// GeneratePenpotCompose renders a valid docker-compose configuration for self-hosted Penpot.
func GeneratePenpotCompose(cfg PenpotConfig) string {
	if cfg.FrontendPort <= 0 {
		cfg.FrontendPort = 9000
	}
	if cfg.BackendPort <= 0 {
		cfg.BackendPort = 9001
	}
	if cfg.DBPort <= 0 {
		cfg.DBPort = 5435
	}
	if cfg.RedisPort <= 0 {
		cfg.RedisPort = 6381
	}
	if cfg.SecretKey == "" {
		cfg.SecretKey = "dendrite_penpot_secret_key_mock_local"
	}

	return fmt.Sprintf(`version: "3.8"

networks:
  dendrite-mesh:
    driver: bridge

volumes:
  penpot-postgres-data:
  penpot-redis-data:
  penpot-assets:

services:
  penpot-postgres:
    image: postgres:16-alpine
    container_name: dendrite-penpot-db
    restart: unless-stopped
    environment:
      POSTGRES_DB: penpot_dev
      POSTGRES_USER: penpot_user
      POSTGRES_PASSWORD: penpot_password
    ports:
      - "%d:5432"
    volumes:
      - penpot-postgres-data:/var/lib/postgresql/data
    networks:
      - dendrite-mesh
    healthcheck:
      test: ["CMD-SHELL", "pg_isready -U penpot_user -d penpot_dev"]
      interval: 5s
      timeout: 3s
      retries: 5

  penpot-redis:
    image: redis:7-alpine
    container_name: dendrite-penpot-redis
    restart: unless-stopped
    ports:
      - "%d:6379"
    volumes:
      - penpot-redis-data:/data
    networks:
      - dendrite-mesh
    healthcheck:
      test: ["CMD", "redis-cli", "ping"]
      interval: 5s
      timeout: 3s
      retries: 5

  penpot-backend:
    image: penpotapp/backend:latest
    container_name: dendrite-penpot-backend
    restart: unless-stopped
    depends_on:
      penpot-postgres:
        condition: service_healthy
      penpot-redis:
        condition: service_healthy
    environment:
      PENPOT_FLAGS: "enable-prepl-server disable-email-verification disable-secure-session-cookies"
      PENPOT_DATABASE_URI: "postgresql://penpot-postgres:5432/penpot_dev"
      PENPOT_DATABASE_USERNAME: "penpot_user"
      PENPOT_DATABASE_PASSWORD: "penpot_password"
      PENPOT_REDIS_URI: "redis://penpot-redis:6379/0"
      PENPOT_SECRET_KEY: "%s"
      PENPOT_PUBLIC_URI: "http://localhost:%d"
    ports:
      - "%d:6060"
    volumes:
      - penpot-assets:/opt/data/assets
    networks:
      - dendrite-mesh

  penpot-frontend:
    image: penpotapp/frontend:latest
    container_name: dendrite-penpot-frontend
    restart: unless-stopped
    depends_on:
      - penpot-backend
    environment:
      PENPOT_FLAGS: "enable-prepl-server disable-email-verification disable-secure-session-cookies"
      PENPOT_PUBLIC_URI: "http://localhost:%d"
      PENPOT_BACKEND_URI: "http://penpot-backend:6060"
      PENPOT_EXPORTER_URI: "http://penpot-backend:6060"
    ports:
      - "%d:8080"
    networks:
      - dendrite-mesh
    healthcheck:
      test: ["CMD", "bash", "-c", "exec 3<>/dev/tcp/127.0.0.1/8080"]
      interval: 10s
      timeout: 5s
      retries: 3
`, cfg.DBPort, cfg.RedisPort, cfg.SecretKey, cfg.FrontendPort, cfg.BackendPort, cfg.FrontendPort, cfg.FrontendPort)
}

// CheckPenpotHealth tests reachability of a running Penpot service instance.
func CheckPenpotHealth(ctx context.Context, endpoint string, timeout time.Duration) bool {
	if endpoint == "" {
		endpoint = "http://localhost:9000"
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
