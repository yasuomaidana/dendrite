package docusaurus

import (
	"fmt"
)

func renderDockerCompose(cfg *SiteConfig) string {
	return fmt.Sprintf(`version: '3.8'

services:
  docs-share:
    build:
      context: .
      dockerfile: Dockerfile
    container_name: %s-docs-portal
    ports:
      - "%d:80"
    restart: unless-stopped
    healthcheck:
      test: ["CMD", "wget", "-q", "--spider", "http://localhost:80/"]
      interval: 10s
      timeout: 3s
      retries: 3
`, cfg.ProjectName, cfg.Port)
}

func renderNginxConf() string {
	return `server {
    listen 80;
    server_name localhost;

    location / {
        root /usr/share/nginx/html;
        index index.html index.htm;
        try_files $uri $uri/ /index.html;
    }

    gzip on;
    gzip_types text/plain text/css application/json application/javascript text/xml application/xml application/xml+rss text/javascript;
}
`
}

func renderLogoSVG() string {
	return `<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 100 100" width="100" height="100">
  <circle cx="50" cy="50" r="45" fill="#0f172a" stroke="#10b981" stroke-width="4"/>
  <circle cx="50" cy="50" r="12" fill="#10b981"/>
  <circle cx="25" cy="35" r="7" fill="#34d399"/>
  <circle cx="75" cy="35" r="7" fill="#34d399"/>
  <circle cx="30" cy="70" r="7" fill="#34d399"/>
  <circle cx="70" cy="70" r="7" fill="#34d399"/>
  <line x1="50" y1="50" x2="25" y2="35" stroke="#10b981" stroke-width="3"/>
  <line x1="50" y1="50" x2="75" y2="35" stroke="#10b981" stroke-width="3"/>
  <line x1="50" y1="50" x2="30" y2="70" stroke="#10b981" stroke-width="3"/>
  <line x1="50" y1="50" x2="70" y2="70" stroke="#10b981" stroke-width="3"/>
</svg>
`
}

func renderGitignore() string {
	return `node_modules/
build/
.docusaurus/
.cache-loader/
*.log
.DS_Store
dendrite-docs.tar.gz
`
}
