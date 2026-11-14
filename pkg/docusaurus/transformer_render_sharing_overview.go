package docusaurus

func renderSharingOverviewDoc() string {
	return `---
id: project-sharing-overview
title: "Project Sharing & Distribution Architecture"
sidebar_label: "Overview"
sidebar_position: 1
description: "Comprehensive guide to sharing Dendrite documentation and architectural knowledge across teams."
---

# Project Sharing & Distribution Architecture

## 1. Principles of Project Sharing

Dendrite treats documentation not merely as static files, but as a **first-class shareable asset**:
- **Portability**: Standalone export archives with cryptographic SHA-256 manifests.
- **Interoperability**: Clear linkages between user/organization repositories and open-source ecosystems.
- **Containerization**: 1-click Docker and Nginx distribution for private cloud and intranet environments.
- **Live Tool Bridge**: Deep integration with JetBrains IDEs and Plane v2 project management.
`
}

func renderFederatedEcosystemDoc() string {
	return `---
id: federated-ecosystem
title: "Distributed Codebase Federation"
sidebar_label: "Federated Ecosystem"
sidebar_position: 2
description: "Cross-repository federation, capability routing, and shared specifications."
---

# Distributed Codebase Federation

The Federated Ecosystem connects multi-repo teams through unified documentation taxonomies, shared type definitions, and synchronized ticket lifecycles.
`
}

func renderExportAndDeploymentDoc() string {
	return `---
id: export-and-deployment
title: "Export & Containerized Deployment"
sidebar_label: "Deployment & Bundling"
sidebar_position: 3
description: "How to bundle and deploy Docusaurus documentation."
---

# Export & Containerized Deployment

Deploying the documentation site locally or across internal clusters:

### 1. Build and Run via Docker Compose
` + "```bash\n" + `docker compose -f docker-compose.share.yaml up --build -d
` + "```\n\n" + `### 2. Export Standalone Archive
` + "```bash\n" + `dendrite codex site share --out dendrite-docs.tar.gz
` + "```\n"
}
