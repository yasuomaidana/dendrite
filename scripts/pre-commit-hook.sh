#!/usr/bin/env bash
# ==============================================================================
# Dendrite Git Pre-Commit Hook
# 1. Zero-mock policy (FEAT-044): staged source files must not add mocks/stubs.
# 2. Documentation (DEN-985): every CLI command and MCP tool keeps an up-to-date
#    reference page in docs/cli and docs/mcp.
# Break-glass only: git commit --no-verify
# ==============================================================================
set -eo pipefail

ROOT_DIR="$(git rev-parse --show-toplevel)"
cd "$ROOT_DIR"

STAGED=$(git diff --cached --name-only --diff-filter=ACM)

# 1. Zero-mock policy on staged source files
STAGED_SRC=$(echo "$STAGED" | grep -E '\.(go|rs|ts|py)$' || true)
if [ -n "$STAGED_SRC" ]; then
    BANNED_PATTERN='(gomock|testify/mock|t\.Skip\(|type[[:space:]]+[Mm]ock|type[[:space:]]+[Ff]ake|NewMock)'
    VIOLATIONS=$(echo "$STAGED_SRC" | xargs git grep --cached -n -E "$BANNED_PATTERN" -- 2>/dev/null || true)
    if [ -n "$VIOLATIONS" ]; then
        echo "================================================================================"
        echo "[-] PRE-COMMIT REJECTION: ZERO-MOCK POLICY VIOLATION (FEAT-044)"
        echo "================================================================================"
        echo "$VIOLATIONS"
        echo ""
        echo "Use real implementations, httptest servers, temp dirs or Testcontainers."
        exit 1
    fi
fi

# 2. Reference documentation audit when CLI/MCP code or docs change.
# Always audit the current source with go run, never a possibly stale ./bin/dendrite.
if echo "$STAGED" | grep -qE '^(cmd/|pkg/|docs/(cli|mcp)/|README\.md$)'; then
    if ! go run ./cmd/dendrite codex audit --strict; then
        echo "[-] PRE-COMMIT REJECTION: reference documentation is missing or stale (DEN-985)."
        echo "    Run: go run ./cmd/dendrite codex generate --reference && git add docs/"
        exit 1
    fi
fi

exit 0
