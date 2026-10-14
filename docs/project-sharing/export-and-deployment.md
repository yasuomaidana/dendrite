---
id: export-and-deployment
title: "Export & Containerized Deployment"
sidebar_label: "Deployment & Bundling"
sidebar_position: 3
description: "How to bundle and deploy Docusaurus documentation."
---

# Export & Containerized Deployment

Deploying the documentation site locally or across internal clusters:

### 1. Build and Run via Docker Compose
```bash
docker compose -f docker-compose.share.yaml up --build -d
```

### 2. Export Standalone Archive
```bash
dendrite doc docusaurus share --out dendrite-docs.tar.gz
```
