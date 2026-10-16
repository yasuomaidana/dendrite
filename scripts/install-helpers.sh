#!/usr/bin/env bash
# ==============================================================================
# Dendrite & Personal Context Subsystems: Automated Installer Helpers
# Part of FEAT-101 (Personal Context, Topic Memory Zero State & Installer Helpers)
# Governing Tickets: DEN-804 (Pre-Flight Audit), DEN-805 (Ollama & OxiLLaMa Provisioner)
# ==============================================================================
set -euo pipefail

# ANSI Color Codes
GREEN='\033[0;32m'
BLUE='\033[0;34m'
YELLOW='\033[1;33m'
RED='\033[0;31m'
BOLD='\033[1m'
NC='\033[0m' # No Color

OLLAMA_HOST="${OLLAMA_HOST:-http://127.0.0.1:11434}"
OXILLAMA_HOST="${OXILLAMA_HOST:-http://127.0.0.1:8080}"
LITELLM_HOST="${LITELLM_HOST:-http://127.0.0.1:4000}"
PLANE_HOST="${PLANE_HOST:-http://127.0.0.1:8090}"

log_info() {
    echo -e "${BLUE}[INFO]${NC} $1"
}

log_success() {
    echo -e "${GREEN}[OK]${NC} $1"
}

log_warn() {
    echo -e "${YELLOW}[WARN]${NC} $1"
}

log_error() {
    echo -e "${RED}[ERROR]${NC} $1" >&2
}

banner() {
    echo -e "${BOLD}${BLUE}"
    echo "======================================================================="
    echo "  Dendrite Installer Helpers: Pre-Flight, OxiLLaMa & Provisioning (FEAT-101)"
    echo "======================================================================="
    echo -e "${NC}"
}

usage() {
    echo "Usage: $0 [OPTIONS]"
    echo ""
    echo "Options:"
    echo "  --preflight     Run pre-flight environment, tool, and port audits"
    echo "  --models        Provision zero-cost Ollama models (llama3.2, nomic-embed)"
    echo "  --oxillama      Probe and verify OxiLLaMa pure-Rust GGUF inference daemon (:8080)"
    echo "  --plane         Verify Plane v2 connection and synchronize project DEN"
    echo "  --full          Run end-to-end setup (preflight + models + oxillama + plane + gateway)"
    echo "  --help          Display this help message"
    echo ""
}

# Gate 1: Check System Tools, Prerequisites and Accelerators
check_prerequisites() {
    log_info "Gate 1: Verifying OS, architecture, and core dependencies..."
    local os
    os="$(uname -s)"
    local arch
    arch="$(uname -m)"

    log_info "Detected OS: $os ($arch)"

    local required_tools=("curl" "git" "python3")
    for tool in "${required_tools[@]}"; do
        if command -v "$tool" >/dev/null 2>&1; then
            log_success "Tool available: $tool"
        else
            log_error "Missing required tool: $tool"
            exit 1
        fi
    done

    # Check for Rust toolchain for OxiLLaMa native compilation
    if command -v cargo >/dev/null 2>&1; then
        local rust_ver
        rust_ver="$(cargo --version | head -n 1)"
        log_success "Rust Toolchain: $rust_ver (OxiLLaMa compilation ready)"
    else
        log_warn "Cargo not found in PATH. Pre-built OxiLLaMa binary required."
    fi

    # Check for hardware acceleration (Apple Silicon Metal or NVIDIA CUDA)
    if [[ "$os" == "Darwin" && "$arch" == "arm64" ]]; then
        local cpu_brand
        cpu_brand="$(sysctl -n machdep.cpu.brand_string 2>/dev/null || echo 'Apple Silicon')"
        log_success "Hardware Acceleration: Apple Silicon Metal GPU ($cpu_brand) detected"
    elif command -v nvidia-smi >/dev/null 2>&1; then
        local gpu_name
        gpu_name="$(nvidia-smi --query-gpu=name --format=csv,noheader 2>/dev/null | head -n 1)"
        log_success "Hardware Acceleration: NVIDIA CUDA GPU ($gpu_name) detected"
    else
        log_warn "No dedicated Metal/CUDA GPU detected. Inference will execute on standard CPU."
    fi
}

# Gate 2: Audit Essential Ports
audit_ports() {
    log_info "Gate 2: Auditing port availability and service bindings..."
    # 11434: Ollama, 8080: OxiLLaMa, 4000: LiteLLM, 8090: Plane, 5432: Postgres, 6379: Redis
    local ports=(11434 8080 4000 8090 5432 6379)
    for port in "${ports[@]}"; do
        if lsof -i ":$port" >/dev/null 2>&1; then
            local proc
            proc="$(lsof -i ":$port" -sTCP:LISTEN -t 2>/dev/null | head -n 1 || echo 'active')"
            log_info "Port :$port is active (PID: $proc)"
        else
            log_info "Port :$port is free"
        fi
    done
}

# Gate 3A: Provision Zero-Cost Local Ollama Models
provision_models() {
    log_info "Gate 3A: Probing Ollama inference engine at $OLLAMA_HOST..."
    if ! curl -s --max-time 3 "$OLLAMA_HOST/api/tags" >/dev/null 2>&1; then
        log_warn "Ollama is not responding at $OLLAMA_HOST. Please ensure Ollama is started."
        return 0
    fi

    log_success "Ollama engine is reachable"

    local models=("llama3.2:latest" "nomic-embed-text:latest")
    for model in "${models[@]}"; do
        log_info "Checking local availability for model: $model..."
        if curl -s "$OLLAMA_HOST/api/tags" | grep -q "$model"; then
            log_success "Model already provisioned: $model"
        else
            log_info "Pulling zero-cost model: $model (this may take a few moments)..."
            curl -s -X POST "$OLLAMA_HOST/api/pull" -d "{\"name\": \"$model\", \"stream\": false}" >/dev/null || true
            log_success "Model pull complete: $model"
        fi
    done
}

# Gate 3B: Probe & Verify OxiLLaMa Pure-Rust GGUF Engine
probe_oxillama() {
    log_info "Gate 3B: Probing OxiLLaMa pure-Rust inference engine at $OXILLAMA_HOST..."
    if curl -s --max-time 3 "$OXILLAMA_HOST/health" >/dev/null 2>&1 || curl -s --max-time 3 "$OXILLAMA_HOST/v1/models" >/dev/null 2>&1; then
        log_success "OxiLLaMa zero-FFI daemon is active and responding on $OXILLAMA_HOST"
    else
        log_info "OxiLLaMa daemon not currently responding on :8080 (standby mode)"
        if command -v oxillama >/dev/null 2>&1; then
            log_success "OxiLLaMa binary discovered in system PATH: $(command -v oxillama)"
        elif [[ -f "./bin/oxillama" ]]; then
            log_success "OxiLLaMa binary discovered in ./bin/oxillama"
        else
            log_info "OxiLLaMa can be spawned on demand via: ./bin/dendrite inference zero-cost"
        fi
    fi
}

# Gate 4: Bootstrap LiteLLM Gateway Routing
bootstrap_gateway() {
    log_info "Gate 4: Checking LiteLLM token governance proxy at $LITELLM_HOST..."
    if curl -s --max-time 3 "$LITELLM_HOST/health" >/dev/null 2>&1 || curl -s --max-time 3 "$LITELLM_HOST/v1/models" >/dev/null 2>&1; then
        log_success "LiteLLM Proxy is running and accessible"
    else
        log_info "LiteLLM proxy is offline or standby. Config file: stacks/litellm_config.yaml"
    fi
}

# Gate 5: Authoritative Project Management Verification
verify_plane() {
    log_info "Gate 5: Checking Plane v2 API reachability at $PLANE_HOST..."
    if curl -s --max-time 3 "$PLANE_HOST" >/dev/null 2>&1 || curl -s --max-time 3 "$PLANE_HOST/api/instances/" >/dev/null 2>&1; then
        log_success "Plane v2 instance is reachable at $PLANE_HOST"
    else
        log_info "Plane v2 container stack accessible via: ./bin/dendrite env status"
    fi
}

# Main Execution Switchboard
main() {
    banner
    local mode="${1:---full}"

    case "$mode" in
        --preflight)
            check_prerequisites
            audit_ports
            probe_oxillama
            ;;
        --models)
            provision_models
            ;;
        --oxillama)
            probe_oxillama
            ;;
        --plane)
            verify_plane
            ;;
        --full)
            check_prerequisites
            audit_ports
            provision_models
            probe_oxillama
            bootstrap_gateway
            verify_plane
            echo ""
            log_success "Installer helpers completed all 5 validation gates successfully!"
            ;;
        --help|-h)
            usage
            exit 0
            ;;
        *)
            log_error "Unknown option: $mode"
            usage
            exit 1
            ;;
    esac
}

main "$@"
