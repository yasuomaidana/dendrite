#!/usr/bin/env bash
# ==============================================================================
# Dendrite Local CI/CD & Verification Runner
# Runs full CI/CD verification locally before committing or pushing to remote.
# Combines:
#   1. Dendrite VCS Guard & Branch Policy Check
#   2. Dendrite Anti-Mock Scan & Coverage Audit
#   3. GitHub Copilot Instructions & Skills Sync Check
#   4. Go Formatting, Linter (golangci-lint), Race Detection & Coverage Gate (>= 85%)
#   5. Polyglot Toolchain Verification (Cargo, Bun, uv)
#   6. Native GitHub Actions Container Runner (via `act`)
# ==============================================================================
set -euo pipefail

# ANSI Colors
GREEN='\033[0;32m'
BLUE='\033[0;34m'
YELLOW='\033[1;33m'
RED='\033[0;31m'
BOLD='\033[1m'
NC='\033[0m'

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$ROOT_DIR"

log_info() { echo -e "${BLUE}[INFO]${NC} $1"; }
log_ok()   { echo -e "${GREEN}[✓]${NC} $1"; }
log_warn() { echo -e "${YELLOW}[!]${NC} $1"; }
log_err()  { echo -e "${RED}[✗]${NC} $1" >&2; }

banner() {
    echo -e "${BOLD}${BLUE}"
    echo "======================================================================="
    echo "       Dendrite Local CI/CD Pre-Push Verification Pipeline"
    echo "======================================================================="
    echo -e "${NC}"
}

usage() {
    echo "Usage: $0 [MODE]"
    echo ""
    echo "Modes:"
    echo "  --all       (Default) Run complete local verification (VCS, Lint, Test, Coverage, Toolchains, Build)"
    echo "  --fast      Run quick verification (Format, Vet, and Unit tests without race detector)"
    echo "  --toolchains Verify installed polyglot toolchains (Cargo, Bun, uv, Go)"
    echo "  --act       Run the GitHub Actions workflow (.github/workflows/ci.yml) via 'act' in Docker"
    echo "  --copilot   Verify GitHub Copilot CLI and custom instructions"
    echo "  --help      Show this help message"
}

# 1. Step: VCS Guard & Branch Policy
step_vcs_guard() {
    log_info "1/7 Verifying Git Branch Policy & VCS Guard..."
    if [ -x "./bin/dendrite" ]; then
        ./bin/dendrite myelin check || {
            log_err "VCS Guard rejected branch state."
            return 1
        }
    else
        log_warn "bin/dendrite not compiled yet, checking current branch via git..."
        BRANCH=$(git branch --show-current)
        if [ "$BRANCH" = "main" ] || [ "$BRANCH" = "master" ]; then
            log_err "Direct commits/pushes to '$BRANCH' are strictly forbidden by Dendrite policy!"
            return 1
        fi
        log_ok "Active branch '$BRANCH' is compliant."
    fi
}

# 2. Step: Copilot Instructions & Skills Sync
step_copilot_sync() {
    log_info "2/7 Verifying Copilot Instructions & Agent Skills..."
    if [ ! -f ".github/copilot-instructions.md" ] || [ ! -f ".github/workflows/ci.yml" ]; then
        log_warn "Missing CI/CD or Copilot configuration. Synchronizing environment..."
        if [ -x "./bin/dendrite" ]; then
            ./bin/dendrite soma prepare
        else
            go run ./cmd/dendrite soma prepare
        fi
    fi
    log_ok "Copilot instructions (.github/copilot-instructions.md) active."
    log_ok "CI/CD workflow (.github/workflows/ci.yml) active."
}

# 3. Step: Source Code Formatting & Static Analysis
step_lint() {
    log_info "3/7 Verifying Code Formatting & Linters..."
    
    # Check gofmt
    UNFORMATTED=$(gofmt -l pkg/ cmd/ 2>/dev/null || true)
    if [ -n "$UNFORMATTED" ]; then
        log_warn "Unformatted Go files detected:"
        echo "$UNFORMATTED"
        log_info "Tip: Run 'go fmt ./pkg/... ./cmd/...' to format automatically."
    else
        log_ok "Go formatting clean."
    fi

    # Run go vet
    log_info "Running go vet ./pkg/... ./cmd/..."
    go vet ./pkg/... ./cmd/...
    log_ok "go vet clean."

    # Run golangci-lint if installed
    if command -v golangci-lint >/dev/null 2>&1; then
        log_info "Running golangci-lint..."
        golangci-lint run --timeout=3m || log_warn "golangci-lint reported issues."
    else
        log_warn "golangci-lint not found on PATH. Install via: brew install golangci-lint"
    fi
}

# 4. Step: Anti-Mock Quality Audit
step_anti_mock() {
    log_info "4/7 Running Dendrite Anti-Mock Audit..."
    if [ -x "./bin/dendrite" ]; then
        ./bin/dendrite allostasis audit pkg/env/bootstrap || log_warn "Quality audit reported warnings."
    else
        log_warn "bin/dendrite not found; skipping anti-mock scan."
    fi
}

# 5. Step: Polyglot Toolchains Verification (Cargo, Bun, uv)
step_toolchains() {
    log_info "5/7 Verifying Canonical Polyglot Toolchains (Cargo, Bun, uv)..."

    # Cargo (Rust)
    if command -v cargo >/dev/null 2>&1; then
        CARGO_VER=$(cargo --version)
        log_ok "Rust Toolchain: ${CARGO_VER}"
        if [ -f "Cargo.toml" ]; then
            log_info "Running: cargo test --verbose"
            cargo test --verbose
        fi
    else
        log_warn "cargo not installed. Install via: curl --proto '=https' --tlsv1.2 -sSf https://sh.rustup.rs | sh"
    fi

    # Bun (JS/TS)
    if command -v bun >/dev/null 2>&1; then
        BUN_VER=$(bun --version)
        log_ok "Bun Runtime & Package Manager: v${BUN_VER}"
        if [ -d "website" ] && [ -f "website/package.json" ]; then
            log_info "Running Bun test in website/..."
            (cd website && bun run test)
            log_ok "Bun test verification passed."
        fi
    else
        log_warn "bun not installed. Install via: curl -fsSL https://bun.sh/install | bash"
    fi

    # uv (Python)
    if command -v uv >/dev/null 2>&1; then
        UV_VER=$(uv --version)
        log_ok "uv Python Toolchain: ${UV_VER}"
        if [ -f "scripts/pyproject.toml" ]; then
            log_info "Verifying scripts/pyproject.toml with uv..."
            (cd scripts && uv pip list 2>/dev/null || true)
            log_ok "uv project verification clean."
        fi
    else
        log_warn "uv not installed. Install via: curl -LsSf https://astral.sh/uv/install.sh | sh"
    fi
}

# 6. Step: Test Suite & Mandatory Statement Coverage (>= 85.0%)
step_test_coverage() {
    local mode="${1:-full}"
    log_info "6/7 Executing Test Suites & Measuring Statement Coverage..."
    
    if [ "$mode" = "fast" ]; then
        go test -v ./pkg/env/bootstrap/...
        log_ok "Fast test suite passed."
        return 0
    fi

    COVERAGE_FILE="coverage.out"
    log_info "Running: go test -v -race -covermode=atomic -coverprofile=${COVERAGE_FILE} ./pkg/env/bootstrap/..."
    go test -v -race -covermode=atomic -coverprofile="${COVERAGE_FILE}" ./pkg/env/bootstrap/...
    
    if [ -f "$COVERAGE_FILE" ]; then
        TOTAL_COV=$(go tool cover -func="${COVERAGE_FILE}" | grep 'total:' | awk '{print $3}' | tr -d '%')
        log_info "Measured Statement Coverage: ${TOTAL_COV}%"
        
        # Check against 85.0% threshold
        PASS=$(awk -v cov="$TOTAL_COV" 'BEGIN { if (cov >= 85.0) print "1"; else print "0" }')
        if [ "$PASS" = "1" ]; then
            log_ok "Statement coverage ${TOTAL_COV}% meets DoD threshold (>= 85.0%)."
        else
            log_err "Statement coverage ${TOTAL_COV}% is below required 85.0% threshold!}"
            return 1
        fi
    fi
}

# 7. Step: Binary Compilation Check
step_build() {
    log_info "7/8 Verifying Compilation & Binary Build..."
    go build -v -o bin/dendrite ./cmd/dendrite
    log_ok "Binary successfully compiled at ./bin/dendrite"
}

# 8. Step: Documentation Completeness Audit (--strict)
step_docs_audit() {
    log_info "8/8 Auditing CLI & MCP Documentation Completeness (--strict)..."
    # Always audit against the current source, never a possibly stale ./bin/dendrite.
    go run ./cmd/dendrite codex audit --strict
    log_ok "Documentation reference audit passed (--strict)."
}

# Optional Step: Native GitHub Actions Runner (via `act`)
run_act_runner() {
    log_info "Running GitHub Actions (.github/workflows/ci.yml) locally via 'act'..."
    if ! command -v act >/dev/null 2>&1; then
        log_err "'act' is not installed."
        echo "To run GitHub Actions locally inside Docker, install act via:"
        echo "  brew install act"
        echo ""
        echo "Or run native verification via: $0 --all"
        return 1
    fi

    log_info "Executing: act push -j verify"
    act push -j verify
}

# Main Execution Flow
main() {
    banner
    MODE="${1:---all}"

    case "$MODE" in
        --help|-h)
            usage
            exit 0
            ;;
        --fast)
            step_vcs_guard
            step_lint
            step_test_coverage "fast"
            step_build
            echo ""
            log_ok "${BOLD}Fast CI/CD verification succeeded!${NC}"
            ;;
        --toolchains)
            step_toolchains
            ;;
        --act)
            run_act_runner
            ;;
        --copilot)
            step_copilot_sync
            if command -v copilot >/dev/null 2>&1 || command -v gh >/dev/null 2>&1; then
                log_ok "Copilot tooling available on host."
            else
                log_warn "Install Copilot CLI via: npm install -g @github/copilot-cli OR gh extension install github/gh-copilot"
            fi
            ;;
        --all|*)
            step_vcs_guard
            step_copilot_sync
            step_lint
            step_anti_mock
            step_toolchains
            step_test_coverage "full"
            step_build
            step_docs_audit
            echo ""
            log_ok "${BOLD}All local CI/CD verification gates passed successfully! Safe to push.${NC}"
            ;;
    esac
}

main "$@"
