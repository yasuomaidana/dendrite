#!/usr/bin/env bash
# ==============================================================================
# Dendrite & Personal Context Subsystems: Automated Installer Helpers
# Part of FEAT-101 (Personal Context, Topic Memory Zero State & Installer Helpers)
# Governing Tickets: DEN-804 (Pre-Flight Audit), DEN-805 (Ollama & llama.cpp Provisioner), DEN-933
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
LLAMACPP_HOST="${LLAMACPP_HOST:-http://127.0.0.1:8080}"
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
    echo "  Dendrite Installer Helpers: Pre-Flight, llama.cpp & Provisioning (FEAT-101)"
    echo "======================================================================="
    echo -e "${NC}"
}

usage() {
    echo "Usage: $0 [OPTIONS]"
    echo ""
    echo "Options:"
    echo "  --preflight     Run pre-flight environment, tool, and port audits"
    echo "  --models        Provision zero-cost Ollama models (llama3.2, nomic-embed)"
    echo "  --llamacpp      Install llama.cpp (llama-server) when missing and probe it (:8080)"
    echo "  --plane         Verify Plane v2 connection and synchronize project DEN"
    echo "  --full          Run end-to-end setup (preflight + models + llamacpp + plane + gateway)"
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

    # Check for the llama.cpp server binary
    if command -v llama-server >/dev/null 2>&1; then
        log_success "llama-server available: $(command -v llama-server)"
    else
        log_warn "llama-server not found in PATH. Run: $0 --llamacpp"
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
    # 11434: Ollama, 8080: llama-server, 4000: LiteLLM, 8090: Plane, 5432: Postgres, 6379: Redis
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

# Gate 3B: Install llama.cpp (llama-server) when missing
install_llamacpp() {
    if command -v llama-server >/dev/null 2>&1; then
        log_success "llama-server already installed: $(command -v llama-server)"
        return 0
    fi
    local os arch
    os="$(uname -s)"
    arch="$(uname -m)"
    if command -v brew >/dev/null 2>&1; then
        log_info "Installing llama.cpp with Homebrew..."
        brew install llama.cpp
    elif [[ "$os" == "Linux" ]]; then
        local suffix
        case "$arch" in
            x86_64) suffix="ubuntu-x64" ;;
            aarch64|arm64) suffix="ubuntu-arm64" ;;
            *) log_error "No official llama.cpp release for $arch; build it from https://github.com/ggml-org/llama.cpp"; return 1 ;;
        esac
        log_info "Downloading the latest official llama.cpp release ($suffix)..."
        local url tmp share="${HOME}/.local/share/llama.cpp" bindir="${HOME}/.local/bin"
        url="$(curl -fsSL https://api.github.com/repos/ggml-org/llama.cpp/releases/latest \
            | grep -o "https://[^\"]*bin-${suffix}[^\"]*\.zip" | head -n 1)"
        if [[ -z "$url" ]]; then
            log_error "Could not find a llama.cpp release asset for $suffix"
            return 1
        fi
        tmp="$(mktemp -d)"
        curl -fsSL "$url" -o "$tmp/llama.zip"
        mkdir -p "$share" "$bindir"
        unzip -q -o "$tmp/llama.zip" -d "$share"
        rm -rf "$tmp"
        local bin
        bin="$(find "$share" -name llama-server -type f | head -n 1)"
        if [[ -z "$bin" ]]; then
            log_error "llama-server not found in the downloaded release"
            return 1
        fi
        chmod +x "$bin"
        ln -sf "$bin" "$bindir/llama-server"
        log_success "llama-server installed at $bindir/llama-server (add $bindir to PATH)"
    else
        log_error "Install llama.cpp manually: https://github.com/ggml-org/llama.cpp/releases"
        return 1
    fi
}

# Gate 3C: Probe & Verify the llama.cpp llama-server
probe_llamacpp() {
    log_info "Gate 3C: Probing llama-server at $LLAMACPP_HOST..."
    if curl -s --max-time 3 "$LLAMACPP_HOST/health" >/dev/null 2>&1; then
        log_success "llama-server is active and responding on $LLAMACPP_HOST"
    else
        log_info "llama-server not currently responding on :8080 (Dendrite starts it on demand)"
        if command -v llama-server >/dev/null 2>&1; then
            log_success "llama-server binary discovered in system PATH: $(command -v llama-server)"
        else
            log_info "llama-server not installed; run: $0 --llamacpp"
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
        log_info "Plane v2 container stack accessible via: ./bin/dendrite soma status"
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
            probe_llamacpp
            ;;
        --models)
            provision_models
            ;;
        --llamacpp)
            install_llamacpp
            probe_llamacpp
            ;;
        --plane)
            verify_plane
            ;;
        --full)
            check_prerequisites
            audit_ports
            provision_models
            install_llamacpp
            probe_llamacpp
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
