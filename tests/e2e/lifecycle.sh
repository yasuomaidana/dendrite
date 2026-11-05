#!/usr/bin/env bash
# End-to-end check of the ticket lifecycle against the isolated test Plane (tests/e2e/plane-test.sh): create,
# claim, comment, decision, plan, the edit lock, finish (blocked and done) and the gate, all through the real
# `dendrite` binary, a real dendrite-plane sidecar and a throw-away DENDRITE_HOME. Nothing here reaches the Plane
# or the ~/.dendrite you work in.
#
#   tests/e2e/plane-test.sh up && tests/e2e/plane-test.sh bootstrap && tests/e2e/lifecycle.sh
set -euo pipefail

HERE="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
ROOT="$(cd "$HERE/../.." && pwd)"
STATE="${E2E_STATE:-$HERE/.state}"

[[ -f "$STATE/env" ]] || { echo "lifecycle: run plane-test.sh up and bootstrap first" >&2; exit 2; }
# shellcheck disable=SC1091
source "$STATE/env"
case "$PLANE_BASE_URL" in *:8090* | *:8091*) echo "lifecycle: refusing the live Plane ($PLANE_BASE_URL)" >&2; exit 2 ;; esac
case "$DENDRITE_HOME" in "$HOME/.dendrite" | "$HOME/.dendrite/"*) echo "lifecycle: refusing the real ~/.dendrite" >&2; exit 2 ;; esac
unset LITELLM_PROXY_URL; export LITELLM_PROXY_URL="http://127.0.0.1:9" # no model is ever asked in this script

BIN="${DENDRITE_BIN:-$STATE/dendrite}"
[[ -n "${DENDRITE_BIN:-}" ]] || (cd "$ROOT" && go build -o "$BIN" ./cmd/dendrite)
mkdir -p "$DENDRITE_STATE_DIR"

pass=0 fail=0
check() { # check "what" <command...>
  local what="$1"; shift
  if "$@" >/dev/null 2>&1; then echo "  ok   $what"; pass=$((pass + 1)); else echo "  FAIL $what"; fail=$((fail + 1)); fi
}
dend() { (cd "$REPO" && "$BIN" "$@" 2>&1 | grep -v embedder); }
contains() { grep -q -- "$1" <<<"$2"; }

new_repo() { # new_repo <dir> <module|plain>
  REPO="$1"; mkdir -p "$REPO"
  git -C "$REPO" init -q -b develop
  git -C "$REPO" config user.name e2e; git -C "$REPO" config user.email e2e@example.com
  if [[ "$2" == module ]]; then
    printf 'module e2e\n\ngo 1.22\n' >"$REPO/go.mod"
    printf 'package e2e\n\nfunc Add(a, b int) int { return a + b }\n' >"$REPO/e2e.go"
    printf 'package e2e\n\nimport "testing"\n\nfunc TestAdd(t *testing.T) {\n\tif Add(1, 2) != 3 {\n\t\tt.Fatal("add")\n\t}\n}\n' >"$REPO/e2e_test.go"
  else
    echo "notes" >"$REPO/README.md"
  fi
  git -C "$REPO" add -A; git -C "$REPO" commit -q -m base
}

WORK="$(mktemp -d "${TMPDIR:-/tmp}/dendrite-e2e.XXXXXX")"
trap 'rm -rf "$WORK"' EXIT

echo "== a project and its first ticket"
new_repo "$WORK/plain" plain
out="$(dend chronos project ensure --identifier E2E)"; check "project ensure creates the Plane project" contains "E2E" "$out"
out="$(dend chronos create --name "First ticket" --desc "Goal.
- [AC-1] it exists" --estimate 1)"
T="$(grep -o 'E2E-[0-9]*' <<<"$out" | head -1)"; check "create returns a ticket id ($T)" test -n "$T"

echo "== claim, the write lock and the comments"
git -C "$REPO" checkout -q -b "agent/$(tr 'A-Z' 'a-z' <<<"$T")"
out="$(dend chronos claim "$T")"; check "claim moves it to In Progress" contains "In Progress" "$out"
out="$(dend chronos claim "$T" 2>&1)"; check "claiming it again from the same branch is fine" contains "Claimed" "$out"
event="{\"session_id\":\"s\",\"cwd\":\"$REPO\",\"tool_name\":\"Write\",\"tool_input\":{\"file_path\":\"$REPO/new.txt\"}}"
check "the lock allows an edit in a claimed copy" bash -c "cd '$REPO' && echo '$event' | '$BIN' hook pre-edit"
git -C "$REPO" checkout -q develop
check "the lock refuses an edit on develop of the main checkout" bash -c "! (cd '$REPO' && echo '$event' | '$BIN' hook pre-edit)"
git -C "$REPO" checkout -q "agent/$(tr 'A-Z' 'a-z' <<<"$T")"
dend chronos comment "$T" --kind progress --body "halfway" >/dev/null
dend chronos decision "$T" --question "Which?" --answer "A" --option A --option B >/dev/null
out="$(dend chronos comments "$T")"
check "progress comment is on the ticket" contains "\[dendrite:progress\] halfway" "$out"
check "decision comment is on the ticket" contains "A: A" "$out"
printf -- '- [x] design\n- [ ] build it\n' >"$WORK/plan.md"
dend chronos plan push "$T" --file "$WORK/plan.md" >/dev/null
out="$(dend chronos plan get "$T")"; check "the plan comes back with its next step" contains "build it" "$out"

echo "== finish: no toolchain means the gate cannot run, so the ticket is blocked with a handoff"
dend chronos finish "$T" >/dev/null || true
out="$(dend chronos comments "$T" --kind handoff --last 1)"
check "a handoff is written" contains "Challenges:" "$out"
check "the ticket is Blocked" contains "Blocked" "$(dend chronos context "$T")"

echo "== a Go module whose tests pass: finish sets Done"
new_repo "$WORK/module" module
dend chronos project ensure --identifier E2G >/dev/null
out="$(dend chronos create --name "Add the sum" --desc "A plain goal, no criteria." --estimate 1)"
T2="$(grep -o 'E2G-[0-9]*' <<<"$out" | head -1)"
git -C "$REPO" checkout -q -b "agent/$(tr 'A-Z' 'a-z' <<<"$T2")"
dend chronos claim "$T2" >/dev/null
printf 'package e2e\n\nfunc Sub(a, b int) int { return a - b }\n' >"$REPO/sub.go"
printf 'package e2e\n\nimport "testing"\n\nfunc TestSub(t *testing.T) {\n\tif Sub(3, 1) != 2 {\n\t\tt.Fatal("sub")\n\t}\n}\n' >"$REPO/sub_test.go"
git -C "$REPO" add -A; git -C "$REPO" commit -q -m "[$T2] add Sub"
out="$(dend chronos finish "$T2" || true)"
check "finish reports Done when the tests and the gate pass" contains "is now Done" "$out"
out="$(dend chronos comments "$T2" --last 1)"
check "the report names the quality gate" contains "Quality gate (changes)" "$out"

echo
echo "$pass passed, $fail failed"
[[ "$fail" -eq 0 ]]
