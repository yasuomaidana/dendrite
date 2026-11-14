#!/usr/bin/env bash
# Control of the isolated Plane test stack (tests/e2e/compose.plane-test.yaml), always as
# `docker compose -p dendrite-test`, so it shares nothing with the Plane you work in.
#
#   tests/e2e/plane-test.sh up         start the stack and migrate its database (the images are the live stack's)
#   tests/e2e/plane-test.sh bootstrap  create a workspace and an API token; writes .state/env
#   tests/e2e/plane-test.sh env        print the variables to run Dendrite against this stack
#   tests/e2e/plane-test.sh status     show the containers
#   tests/e2e/plane-test.sh down       stop and remove the containers, keep the data
#   tests/e2e/plane-test.sh reset      stop and remove the containers AND this stack's volumes
#
# It refuses to run when a port or PLANE_BASE_URL points at the live stack (8090, 8091, 5434, 6380).
set -euo pipefail

HERE="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PROJECT="dendrite-test"
COMPOSE=(docker compose -p "$PROJECT" -f "$HERE/compose.plane-test.yaml")
STATE="${E2E_STATE:-$HERE/.state}"
API_PORT="${E2E_PLANE_API_PORT:-8190}"
UI_PORT="${E2E_PLANE_UI_PORT:-8191}"
DB_PORT="${E2E_PLANE_DB_PORT:-5444}"
REDIS_PORT="${E2E_PLANE_REDIS_PORT:-6390}"
WORKSPACE="dendrite-ws"
LIVE_PORTS=(8090 8091 5434 6380)

die() { echo "plane-test: $*" >&2; exit 1; }

guard() {
  local p
  for p in "$API_PORT" "$UI_PORT" "$DB_PORT" "$REDIS_PORT"; do
    for live in "${LIVE_PORTS[@]}"; do
      [[ "$p" == "$live" ]] && die "port $p is the live Plane's; this stack must use others"
    done
  done
  if [[ -n "${PLANE_BASE_URL:-}" && "$PLANE_BASE_URL" =~ :(8090|8091)(/|$) ]]; then
    die "PLANE_BASE_URL ($PLANE_BASE_URL) points at the live Plane; unset it"
  fi
  [[ "$PROJECT" != "stacks" ]] || die "the project name must not be the live one"
}

# wait_api waits until the API answers; with "any" any HTTP answer counts (a fresh database answers 500 until it
# is migrated), otherwise it must be a 200.
wait_api() {
  local mode="${1:-ok}" code i
  for ((i = 0; i < 90; i++)); do
    code="$(curl -s -m 5 -o /dev/null -w '%{http_code}' "http://localhost:$API_PORT/api/instances/" || true)"
    if [[ "$code" == "200" || ( "$mode" == "any" && "$code" != "000" ) ]]; then return 0; fi
    sleep 2
  done
  die "the test Plane API did not answer on :$API_PORT in 3 minutes (docker compose -p $PROJECT logs plane-api)"
}

migrate() {
  echo "migrating the test database..."
  "${COMPOSE[@]}" exec -T plane-api python manage.py migrate --noinput >/dev/null
}

cmd_up() {
  guard
  "${COMPOSE[@]}" up -d
  wait_api any
  migrate
  wait_api
  echo "test Plane is up: API http://localhost:$API_PORT, UI http://localhost:$UI_PORT (project $PROJECT)"
}

cmd_bootstrap() {
  guard
  mkdir -p "$STATE/home"
  local token
  token="$("${COMPOSE[@]}" exec -T plane-api python manage.py shell -c "
from plane.db.models import User, Workspace, WorkspaceMember, APIToken
user, _ = User.objects.get_or_create(email='e2e@example.com', defaults={'username': 'e2e', 'is_active': True, 'is_staff': True, 'is_superuser': True})
user.set_password('e2e-password-only-for-the-test-stack'); user.save()
ws, _ = Workspace.objects.get_or_create(slug='$WORKSPACE', defaults={'name': 'Dendrite test', 'owner': user})
WorkspaceMember.objects.get_or_create(workspace=ws, member=user, defaults={'role': 20})
tok = APIToken.objects.filter(user=user, label='e2e').first() or APIToken.objects.create(user=user, label='e2e', workspace=ws, user_type=0)
print('TOKEN=' + tok.token)
" | sed -n 's/^TOKEN=//p' | tr -d '\r')"
  [[ -n "$token" ]] || die "could not create an API token (docker compose -p $PROJECT logs plane-api)"
  cat >"$STATE/env" <<ENV
export PLANE_BASE_URL=http://localhost:$API_PORT
export PLANE_API_KEY=$token
export PLANE_WORKSPACE_SLUG=$WORKSPACE
export DENDRITE_HOME=$STATE/home
export DENDRITE_STATE_DIR=$STATE/home/state
ENV
  echo "workspace '$WORKSPACE' and an API token created; variables in $STATE/env"
}

cmd_env() {
  [[ -f "$STATE/env" ]] || die "no $STATE/env yet: run '$0 up' then '$0 bootstrap'"
  cat "$STATE/env"
}

case "${1:-}" in
  up) cmd_up ;;
  bootstrap) cmd_bootstrap ;;
  env) cmd_env ;;
  status) guard; "${COMPOSE[@]}" ps ;;
  down) guard; "${COMPOSE[@]}" down ;;
  reset) guard; "${COMPOSE[@]}" down -v; rm -rf "$STATE" ;;
  *) sed -n '2,13p' "${BASH_SOURCE[0]}" | sed 's/^# \{0,1\}//'; exit 2 ;;
esac
