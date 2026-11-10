#!/usr/bin/env bash
# managed by dendrite — regenerate with `dendrite soma prepare`; do not edit by hand.
# Policy: Rust code reaches a database only through Diesel. This script fails when
#   1. a Cargo manifest declares a database driver or ORM other than the Diesel stack
#      (sqlx, rusqlite, sea-orm, mysql, mysql_async, ...), or a bare postgres, tokio-postgres or libsql
#      driver in a project that declares no diesel crate, or
#   2. Rust code calls diesel::sql_query outside a module named `raw` (raw.rs or raw/) or test code
#      (tests/, tests.rs, *_test.rs).
# A dependency that is not an SQL database access path (an embedded key-value store, say) is not matched.
# A deliberate exception goes in .github/dendrite/db-access.allow, with a reason after a '#': a line `<crate>`
# allows a dependency, a line `path:<prefix>` allows raw SQL under that path (a code generator, say).
set -euo pipefail

root=$(git rev-parse --show-toplevel 2>/dev/null || pwd)
cd "$root"

list() { git ls-files "$1" 2>/dev/null || find . -name "$1" -not -path './target/*' -not -path './.git/*' | sed 's#^\./##'; }

manifests=$(list '*Cargo.toml')
if [ -z "$manifests" ]; then
  echo "db-access: no Cargo manifests, nothing to check"
  exit 0
fi

# Dependency names of every manifest, one "manifest:name" per line, from the [dependencies],
# [dev-dependencies], [build-dependencies], [workspace.dependencies] and [target.*.dependencies] tables.
deps=$(for f in $manifests; do
  awk -v F="$f" '
    /^[[:space:]]*\[/ {
      h = $0; sub(/^[[:space:]]*\[+/, "", h); sub(/\].*$/, "", h); insec = 0
      if (h ~ /(^|\.)(dev-|build-)?dependencies$/) { insec = 1 }
      else if (h ~ /(^|\.)(dev-|build-)?dependencies\.[A-Za-z0-9_-]+$/) { n = h; sub(/^.*dependencies\./, "", n); print F ":" n }
      next
    }
    insec && /^[[:space:]]*[A-Za-z0-9_-]+[[:space:]]*[=.]/ {
      n = $0; sub(/^[[:space:]]*/, "", n); sub(/[[:space:]]*[=.].*$/, "", n); print F ":" n
    }' "$f"
done)

allowed=""
raw_paths=""
if [ -f .github/dendrite/db-access.allow ]; then
  entries=$(sed 's/#.*//' .github/dendrite/db-access.allow | tr -d ' \t' | grep -v '^$' || true)
  allowed=$(printf '%s\n' "$entries" | grep -v '^path:' || true)
  raw_paths=$(printf '%s\n' "$entries" | grep '^path:' | sed 's/^path://' || true)
fi

fail=0
violation() { echo "[-] db-access: $1"; fail=1; }

forbidden='^(sqlx(-[a-z0-9-]+)?|rusqlite|tokio-rusqlite|r2d2_sqlite|sea-orm(-[a-z0-9-]+)?|sea_orm|mysql|mysql_async|mysql-async|sqlite|deadpool-postgres|bb8-postgres)$'
bare='^(postgres|tokio-postgres|libsql)$'
has_diesel=$(printf '%s\n' "$deps" | sed 's/^[^:]*://' | grep -Ec '^diesel([-_].*)?$' || true)

while IFS= read -r line; do
  [ -n "$line" ] || continue
  file=${line%%:*}
  name=${line#*:}
  if [ -n "$allowed" ] && printf '%s\n' "$allowed" | grep -qx "$name"; then
    continue
  fi
  if printf '%s\n' "$name" | grep -Eq "$forbidden"; then
    violation "$file declares '$name'; reach databases through Diesel (diesel, diesel-async, diesel-libsql), or list it in .github/dendrite/db-access.allow with a reason"
  elif printf '%s\n' "$name" | grep -Eq "$bare" && [ "$has_diesel" -eq 0 ]; then
    violation "$file declares the '$name' driver but the project declares no diesel crate; use Diesel for database access"
  fi
done <<EOF
$deps
EOF

rust=$(list '*.rs')
if [ -n "$rust" ]; then
  # shellcheck disable=SC2086
  scan=$(printf '%s\n' $rust | grep -Ev '(^|/)raw(/|\.rs$)|(^|/)tests?(/|\.rs$)|_tests?\.rs$' || true)
  for p in $raw_paths; do scan=$(printf '%s\n' "$scan" | grep -Fv "$p" || true); done
  hits=""
  if [ -n "$scan" ]; then
    hits=$(printf '%s\n' "$scan" | xargs grep -nE '\bsql_query[[:space:]]*\(' 2>/dev/null || true)
  fi
  if [ -n "$hits" ]; then
    while IFS= read -r h; do
      violation "raw SQL outside a 'raw' module: $h"
    done <<EOF
$hits
EOF
  fi
fi

if [ "$fail" -ne 0 ]; then
  echo "db-access: Rust code reaches databases only through Diesel; raw SQL lives in a module named 'raw'."
  exit 1
fi
echo "db-access: ok (diesel only)"
