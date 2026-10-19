# Dendrite Documentation & Project Sharing Hub

This directory contains the production-ready **Docusaurus v3** Docs-as-Code website and **Project Sharing Hub** generated automatically by the Dendrite CLI engine (`pkg/docusaurus`).

For complete guidelines on authoring, committing, and publishing, see [**Documentation Management, Versioning & Publishing Guide**](../docs/DOCUMENTATION_GUIDE.md).

---

## Quick Reference

### 1. Generating & Updating Documentation
Never edit files directly inside `website/docs/`. Always update canonical Markdown specifications under `docs/` in the repository root, then regenerate:
```bash
./bin/dendrite codex generate --docs docs --out website
```

### 2. Local Preview
```bash
# Option A: Built-in preview server (zero npm dependencies)
./bin/dendrite codex site serve --site website --port 3000

# Option B: Docusaurus development server
npm install
npm start
```
- Main Portal: [http://localhost:3000](http://localhost:3000)
- Documentation: [http://localhost:3000/docs/overview](http://localhost:3000/docs/overview)
- Project Sharing Hub: [http://localhost:3000/share](http://localhost:3000/share)
- Live Manifest: [http://localhost:3000/api/v1/sharing-manifest](http://localhost:3000/api/v1/sharing-manifest)

### 3. Production Build
```bash
npm run build
```
Static production output will be generated in `website/build/`.

### 4. Exporting Portable Sharing Bundle
```bash
./bin/dendrite codex site share --site website --out website/dendrite-docs.tar.gz
```
Generates `website/dendrite-docs.tar.gz` and updates `website/sharing-manifest.json` with cryptographic SHA-256 hashes of every document and commit provenance.

### 5. Containerized Intranet Deployment
```bash
docker compose -f docker-compose.share.yaml up --build -d
```

### 6. Git Commit Boundaries
- **Commit**: `package.json`, `docusaurus.config.ts`, `sidebars.ts`, `src/`, `static/`, `Dockerfile`, `docker-compose.share.yaml`, `nginx.conf`, `.gitignore`.
- **Do NOT Commit**: `node_modules/`, `build/`, `.docusaurus/`, `dendrite-docs.tar.gz`.
