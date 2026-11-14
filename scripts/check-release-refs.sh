#!/usr/bin/env bash
# managed by dendrite — regenerate with `dendrite soma prepare`; do not edit by hand.
# Policy: what goes to main refers to a sibling repository (yasuomaidana/dendrite-*, yasuomaidana/embedag) only by a
# release (a vX.Y.Z tag, a crates.io or go.mod version) or by the sibling's main branch; never by a working branch,
# a commit, a pseudo-version or a local path. This script fails on
#   1. a Cargo git dependency on a sibling without tag = "vX.Y.Z" (branch = "main" is accepted), or with a rev or
#      another branch, and a Cargo path dependency or [patch] that reaches a sibling directory,
#   2. a go.mod requirement on a sibling at a pseudo-version (v0.0.0-<time>-<sha>), or a replace that points a
#      sibling module at a local path,
#   3. a workflow step that checks a sibling out at a ref that is neither main nor a vX.Y.Z tag (a repository
#      variable with such a default, ${{ vars.X || 'v0.1.0' }}, is judged by its default), and
#   4. a Markdown link into a sibling's develop or other working branch (blob, tree, raw).
# A deliberate exception goes in .github/dendrite/release-refs.allow: one `path:<prefix>` per line, with a reason
# after a '#'.
set -euo pipefail

root=$(git rev-parse --show-toplevel 2>/dev/null || pwd)
cd "$root"

sib='yasuomaidana/(dendrite-[A-Za-z0-9_-]+|embedag)'
fails=0
allow=""
[ -f .github/dendrite/release-refs.allow ] && allow=$(sed -e 's/[[:space:]]*#.*$//' -e '/^[[:space:]]*$/d' .github/dendrite/release-refs.allow | sed -n 's/^path://p')

files() { { git ls-files "$@" 2>/dev/null || true; } | while read -r f; do
  [ -f "$f" ] || continue
  skip=0
  for p in $allow; do case "$f" in "$p"*) skip=1 ;; esac; done
  [ "$skip" = 1 ] || echo "$f"
done; }

fail() { echo "release-refs: $1" >&2; fails=1; }

for f in $(files '*Cargo.toml'); do
  while IFS= read -r line; do
    n=${line%%:*}; text=${line#*:}
    if echo "$text" | grep -Eq "git[[:space:]]*=[[:space:]]*\"[^\"]*$sib"; then
      if echo "$text" | grep -Eq 'tag[[:space:]]*=[[:space:]]*"v[0-9]+\.[0-9]+\.[0-9]+[^"]*"'; then :
      elif echo "$text" | grep -Eq 'branch[[:space:]]*=[[:space:]]*"main"' && ! echo "$text" | grep -Eq 'rev[[:space:]]*='; then :
      else fail "$f:$n: git dependency on a sibling must name tag = \"vX.Y.Z\" or branch = \"main\": $text"; fi
    fi
    if echo "$text" | grep -Eq "path[[:space:]]*=[[:space:]]*\"(\.\./|/)[^\"]*(dendrite-[A-Za-z0-9_-]+|embedag)"; then
      fail "$f:$n: path dependency reaches a sibling directory: $text"
    fi
  done < <(grep -nE "(git|path)[[:space:]]*=" "$f" || true)
done

for f in $(files 'go.mod' '*/go.mod'); do
  while IFS= read -r line; do
    n=${line%%:*}; text=${line#*:}
    case "$text" in
      *"=>"*) echo "$text" | grep -Eq "github.com/$sib.*=>[[:space:]]*(\.|/)" && fail "$f:$n: replace points a sibling at a local path: $text" ;;
      *) echo "$text" | grep -Eq "v0\.0\.0-[0-9]{14}-[0-9a-f]{12}" && fail "$f:$n: sibling at a pseudo-version, use a release: $text" ;;
    esac
  done < <(grep -nE "github.com/$sib" "$f" || true)
done

for f in $(files '.github/workflows/*.yml' '.github/workflows/*.yaml'); do
  awk -v F="$f" -v SIB="$sib" '
    function check(ref, n) {
      if (ref ~ /\|\|/) { sub(/^.*\|\|[[:space:]]*/, "", ref); sub(/[[:space:]]*\}\}.*$/, "", ref) }
      gsub(/[\047"]/, "", ref); sub(/[[:space:]]+$/, "", ref)
      if (ref != "main" && ref !~ /^v[0-9]+\.[0-9]+\.[0-9]+/) { print F ":" n ": sibling checkout at ref \"" ref "\": use main or a vX.Y.Z tag"; bad = 1 }
    }
    /^[[:space:]]*-[[:space:]]+/ { if (inrepo && !seen) {} inrepo = 0; seen = 0 }
    $0 ~ "repository:[[:space:]]*" SIB "[[:space:]]*$" { inrepo = 1; next }
    inrepo && /^[[:space:]]*ref:/ { r = $0; sub(/^[[:space:]]*ref:[[:space:]]*/, "", r); check(r, NR); seen = 1 }
    END { exit bad }' "$f" >&2 || fails=1
done

for f in $(files '*.md'); do
  case "$f" in CHANGELOG.md | docs/reports/*) continue ;; esac
  while IFS= read -r line; do
    fail "$f:${line%%:*}: link into a sibling's working branch, use main or a tag: $(echo "${line#*:}" | grep -Eo "[^ (]*$sib[^ )]*/(blob|tree|raw)/(develop|dev|feature|agent)[^ )]*" | head -1)"
  done < <(grep -nE "$sib[^ )\"]*/(blob|tree|raw)/(develop|dev|feature/|agent/)|raw.githubusercontent.com/$sib/(develop|dev|feature/|agent/)" "$f" || true)
done

if [ "$fails" = 0 ]; then echo "release-refs: every sibling reference is a release or main"; fi
exit "$fails"
