---
id: local-cicd-verification
title: "Local CI/CD Verification & Pre-Push Quality Pipeline"
sidebar_label: "Local CI/CD Verification"
sidebar_position: 11
tags: [cicd, verification, quality-gate, pre-push, copilot, github-actions, act]
---

# Local CI/CD Verification & Pre-Push Quality Pipeline

This guide outlines how to execute the complete CI/CD verification pipeline locally on your development workstation before committing or pushing changes to remote Git branches.

---

## 1. Automated Verification Script (`scripts/verify-cicd-local.sh`)

Dendrite provides a unified pre-push verification script that runs all gates in sequence:

```bash
./scripts/verify-cicd-local.sh
```

### Available Execution Modes

| Command | Purpose |
| :--- | :--- |
| `./scripts/verify-cicd-local.sh` | **Full CI/CD Pipeline**: VCS Guard $\to$ Copilot/Skills Sync $\to$ Linting $\to$ Anti-Mock Audit $\to$ Race-Detected Tests $\to$ $\ge 85\%$ Statement Coverage Gate $\to$ Compilation |
| `./scripts/verify-cicd-local.sh --fast` | **Fast Verification**: Format check, `go vet`, and unit tests without race detector overhead ($\approx 3\text{s}$) |
| `./scripts/verify-cicd-local.sh --act` | **GitHub Actions Local Container Runner**: Runs `.github/workflows/ci.yml` locally inside Docker using `act` |
| `./scripts/verify-cicd-local.sh --copilot` | **Copilot Verification**: Audits host `copilot` / `gh copilot` installation and validates `.github/copilot-instructions.md` |

---

## 2. Running Individual Verification Steps Manually

You can also execute each verification stage individually using native Go and Dendrite commands:

### Step 1: VCS Guard & Branch Policy
Ensures you are on a permitted branch (`develop` or feature branch) and forbids direct mutation of `main`:
```bash
./bin/dendrite vcs check
```

### Step 2: Synchronize Agent Rules & Copilot Instructions
Verifies that `.github/copilot-instructions.md`, `.github/workflows/ci.yml`, `.agents/rules.md`, and all skills are synchronized:
```bash
./bin/dendrite env sync --dry-run
```

### Step 3: Anti-Mock Quality Audit
Verifies zero mock usage for databases, caches, and protocol layers:
```bash
./bin/dendrite quality audit pkg/env/bootstrap
```

### Step 4: Formatting & Static Analysis
```bash
# Check formatting
gofmt -l pkg/ cmd/

# Static analysis
go vet ./pkg/... ./cmd/...

# Run golangci-lint (if installed)
golangci-lint run
```

### Step 5: Test Execution & Statement Coverage Gate ($\ge 85.0\%$)
```bash
go test -v -race -covermode=atomic -coverprofile=coverage.out ./pkg/env/bootstrap/...
go tool cover -func=coverage.out | grep total:
```

### Step 6: Binary Build Verification
```bash
go build -v -o bin/dendrite ./cmd/dendrite
```

---

## 3. Running GitHub Actions Workflows Locally via `act`

To execute the exact `.github/workflows/ci.yml` pipeline locally in throwaway Docker containers identical to GitHub Actions:

```bash
# 1. Install act (if not already installed)
brew install act

# 2. Run the CI job locally
act push -j verify
```

---

## 4. Git Pre-Push Hook Setup (Optional Automation)

To guarantee you never push failing code, install `scripts/verify-cicd-local.sh` as a Git `pre-push` hook:

```bash
mkdir -p .git/hooks
cat << 'EOF' > .git/hooks/pre-push
#!/usr/bin/env bash
./scripts/verify-cicd-local.sh
EOF
chmod +x .git/hooks/pre-push
```
When configured, running `git push` automatically runs all verification gates, rejecting the push if any test or coverage gate fails.
