#!/usr/bin/env bash
set -euo pipefail

IMAGE="${1:-pinchtab-cloakbrowser:test}"
RUNNER_IMAGE="${PINCHTAB_CLOAKBROWSER_RUNNER_IMAGE:-pinchtab-e2e-runner-api:cloak-smoke}"
CLOAK_CONTAINER_BIN="/opt/cloakbrowser/chrome"
TOKEN="pinchtab-docker-cloak-smoke-${RANDOM}${RANDOM}"
NAME="pinchtab-docker-cloak-smoke-${RANDOM}${RANDOM}"
TMP_DIR="$(mktemp -d "${TMPDIR:-/tmp}/pinchtab-docker-cloak.XXXXXX")"
FIXTURES_PORT="${PINCHTAB_CLOAKBROWSER_FIXTURES_PORT:-}"
FIXTURES_HOST="${PINCHTAB_CLOAKBROWSER_FIXTURES_HOST:-cloak-fixtures.local}"
HOST_FIXTURES_URL=""
FIXTURES_URL=""
FAILED=0
CONTAINER_STARTED=0
FIXTURES_STARTED=0
HOST_PORT=""
API_RESULT=""
API_STATUS=""
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
ROOT="$(cd "$SCRIPT_DIR/.." && pwd)"
CLOAK_DOCKERFILE="${PINCHTAB_CLOAKBROWSER_DOCKERFILE:-tests/tools/docker/cloakbrowser-smoke.Dockerfile}"
RUNNER_DOCKERFILE="${PINCHTAB_CLOAKBROWSER_RUNNER_DOCKERFILE:-tests/e2e/runner-api/Dockerfile}"

cleanup() {
  set +e
  if [ "$CONTAINER_STARTED" -eq 1 ] && docker ps -a --format '{{.Names}}' | grep -Fxq "$NAME"; then
    if [ "$FAILED" -ne 0 ]; then
      echo ""
      echo "Container logs:"
      docker logs "$NAME" || true
      echo ""
      echo "Process list:"
      docker exec "$NAME" ps -eo pid,user,args || true
      if [ "$FIXTURES_STARTED" -eq 1 ]; then
        echo ""
        echo "Fixture server log:"
        docker exec "$NAME" sh -lc 'cat /tmp/pinchtab-fixtures.log 2>/dev/null || true' || true
      fi
      mkdir -p "$TMP_DIR/data"
      docker cp "$NAME:/data/." "$TMP_DIR/data/" >/dev/null 2>&1 || true
      if [ -n "${HOST_PORT:-}" ]; then
        echo ""
        echo "HTTP diagnostics:"
        echo "/autorestart/status:"
        curl -sS -H "Authorization: Bearer ${TOKEN}" "http://127.0.0.1:${HOST_PORT}/autorestart/status" || true
        echo ""
        echo "/instances:"
        instances_diag="$(curl -sS -H "Authorization: Bearer ${TOKEN}" "http://127.0.0.1:${HOST_PORT}/instances" || true)"
        echo "$instances_diag"
        echo "$instances_diag" | jq -r '.[].id? // empty' 2>/dev/null | while read -r inst_id; do
          [ -n "$inst_id" ] || continue
          echo ""
          echo "/instances/${inst_id}/logs:"
          curl -sS -H "Authorization: Bearer ${TOKEN}" "http://127.0.0.1:${HOST_PORT}/instances/${inst_id}/logs" || true
          echo ""
        done
      fi
    fi
    docker rm -f "$NAME" >/dev/null 2>&1 || true
  fi
  if [ "$FAILED" -ne 0 ]; then
    echo "Artifacts kept at: $TMP_DIR"
  else
    rm -rf "$TMP_DIR"
  fi
}
trap cleanup EXIT

skip() {
  echo "SKIP: $*"
  exit 77
}

fail() {
  FAILED=1
  echo "FAIL: $*" >&2
  exit 1
}

api_request() {
  local method="$1"
  local path="$2"
  local body="${3:-}"
  local url="http://127.0.0.1:${HOST_PORT}${path}"
  local -a args=(
    -sS
    -w $'\n%{http_code}'
    -X "$method"
    -H "Authorization: Bearer ${TOKEN}"
  )
  if [ "$method" = "POST" ]; then
    args+=(-H "Content-Type: application/json" -d "$body")
  fi

  local response
  if ! response="$(curl "${args[@]}" "$url" 2>&1)"; then
    fail "$method $path failed: $response"
  fi
  API_STATUS="${response##*$'\n'}"
  API_RESULT="${response%$'\n'*}"
  if [ "$API_STATUS" -lt 200 ] || [ "$API_STATUS" -ge 300 ]; then
    fail "$method $path failed with HTTP $API_STATUS: $API_RESULT"
  fi
}

api_get() {
  api_request GET "$1"
}

api_post() {
  api_request POST "$1" "$2"
}

assert_api_jq() {
  local expr="$1"
  local label="$2"
  echo "$API_RESULT" | jq -e "$expr" >/dev/null || fail "$label failed: $API_RESULT"
}

assert_file_min_bytes() {
  local path="$1"
  local min_bytes="$2"
  local label="$3"
  local bytes
  bytes="$(wc -c < "$path" | tr -d '[:space:]')"
  if [ "${bytes:-0}" -lt "$min_bytes" ]; then
    fail "$label too small: ${bytes:-0} bytes"
  fi
}

assert_screenshot_png() {
  local path="$1"
  local out="$TMP_DIR/screenshot.png"
  local headers="$TMP_DIR/screenshot.headers"
  curl -fsS \
    -D "$headers" \
    -o "$out" \
    -H "Authorization: Bearer ${TOKEN}" \
    "http://127.0.0.1:${HOST_PORT}${path}" || fail "GET $path failed"
  grep -qi '^content-type: image/png' "$headers" || fail "GET $path did not return image/png"
  assert_file_min_bytes "$out" 500 "screenshot"
}

choose_free_port() {
  python3 - <<'PY'
import socket

sock = socket.socket()
sock.bind(("127.0.0.1", 0))
print(sock.getsockname()[1])
sock.close()
PY
}

run_fixture_endpoint_smoke() {
  echo "Running fixture-backed CloakBrowser endpoint smoke..."

  api_post /navigate "{\"url\":\"${FIXTURES_URL}/index.html\",\"waitFor\":\"selector\",\"waitSelector\":\"#welcome\"}"
  assert_api_jq '.title | contains("Home")' "fixture home navigation title"
  assert_api_jq '.url | contains("index.html")' "fixture home navigation URL"
  local tab_id
  tab_id="$(echo "$API_RESULT" | jq -r '.tabId // empty')"
  [ -n "$tab_id" ] || fail "navigate response did not include tabId: $API_RESULT"

  api_get /tabs
  echo "$API_RESULT" | jq -e --arg tab "$tab_id" '.tabs | map(.id) | index($tab) != null' >/dev/null \
    || fail "tabs includes fixture tab failed: $API_RESULT"

  api_get "/snapshot?tabId=${tab_id}&filter=interactive"
  assert_api_jq '.nodes | length > 0' "snapshot returns nodes"

  api_get "/tabs/${tab_id}/text"
  assert_api_jq '.text | contains("Welcome to the E2E test fixtures")' "tab text includes fixture content"

  api_get "/tabs/${tab_id}/html?selector=%23welcome"
  assert_api_jq '.html | contains("Welcome to the E2E test fixtures")' "selected html includes fixture content"

  api_get "/tabs/${tab_id}/styles?selector=%23welcome&prop=display"
  assert_api_jq '.styles.display == "block"' "selected styles include display"

  assert_screenshot_png "/tabs/${tab_id}/screenshot?format=png&raw=true"

  api_post "/tabs/${tab_id}/navigate" "{\"url\":\"${FIXTURES_URL}/buttons.html\",\"waitFor\":\"selector\",\"waitSelector\":\"#increment\"}"
  assert_api_jq '.title | contains("Buttons")' "buttons navigation title"

  api_post "/tabs/${tab_id}/action" '{"kind":"click","selector":"#increment"}'
  assert_api_jq '.success == true' "click action succeeded"

  api_post "/tabs/${tab_id}/evaluate" '{"expression":"document.querySelector(\"#count\").textContent"}'
  assert_api_jq '.result == "1"' "evaluate sees click result"

  api_post "/tabs/${tab_id}/navigate" "{\"url\":\"${FIXTURES_URL}/form.html\",\"waitFor\":\"selector\",\"waitSelector\":\"#username\"}"
  assert_api_jq '.title | contains("Form")' "form navigation title"

  api_post "/tabs/${tab_id}/actions" '{"stopOnError":true,"actions":[{"kind":"fill","selector":"#username","text":"cloak_user"},{"kind":"check","selector":"#terms"}]}'
  assert_api_jq '.successful == 2 and .failed == 0' "batch actions succeeded"

  api_post "/tabs/${tab_id}/evaluate" '{"expression":"({username: document.querySelector(\"#username\").value, terms: document.querySelector(\"#terms\").checked})"}'
  assert_api_jq '.result.username == "cloak_user" and .result.terms == true' "evaluate sees batch action results"
}

run_e2e_scenarios() {
  local -a default_scenarios=(
    "actions-basic.sh"
    "browser-basic.sh"
    "clipboard-basic.sh"
    "console-basic.sh"
    "emulation-basic.sh"
    "files-basic.sh"
    "inspect-basic.sh"
    "tabs-basic.sh"
  )
  local -a scenarios=()
  local -a scenario_args=()

  if [ -n "${PINCHTAB_CLOAKBROWSER_E2E_SCENARIOS:-}" ]; then
    read -r -a scenarios <<< "${PINCHTAB_CLOAKBROWSER_E2E_SCENARIOS}"
  else
    scenarios=("${default_scenarios[@]}")
  fi

  for scenario in "${scenarios[@]}"; do
    [ -n "$scenario" ] || continue
    scenario_args+=("scenario=$scenario")
  done

  if [ "${#scenario_args[@]}" -eq 0 ]; then
    echo "Skipping API E2E scenarios because PINCHTAB_CLOAKBROWSER_E2E_SCENARIOS is empty."
    return
  fi

  echo "Running API E2E scenarios against CloakBrowser in Docker:"
  printf '  - %s\n' "${scenarios[@]}"

  docker run --rm \
    --network "container:${NAME}" \
    -v "$ROOT/tests/e2e":/e2e:ro \
    -e "FIXTURES_HOST=${FIXTURES_HOST}" \
    -e "E2E_SERVER=http://127.0.0.1:9867" \
    -e "E2E_SERVER_TOKEN=${TOKEN}" \
    -e "FIXTURES_URL=${FIXTURES_URL}" \
    -e "E2E_HELPER=api" \
    -e "E2E_SCENARIO_DIR=scenarios/api" \
    -e "E2E_REQUIRED_COMMANDS=curl jq" \
    -e "E2E_READY_TARGETS=E2E_SERVER|60|E2E_SERVER_TOKEN" \
    -e "E2E_SUMMARY_TITLE=CloakBrowser API E2E scenarios" \
    "$RUNNER_IMAGE" \
    /bin/sh -lc 'printf "127.0.0.1 %s\n" "$FIXTURES_HOST" >> /etc/hosts; exec /bin/bash /e2e/run.sh "$@"' \
    _ "${scenario_args[@]}" \
    || fail "API E2E scenarios failed"
}

command -v docker >/dev/null 2>&1 || skip "docker is not installed"
command -v python3 >/dev/null 2>&1 || fail "python3 is required to write the smoke config"
command -v jq >/dev/null 2>&1 || fail "jq is required to validate smoke responses"
command -v curl >/dev/null 2>&1 || fail "curl is required to run smoke requests"

if [ -z "$FIXTURES_PORT" ]; then
  FIXTURES_PORT="$(choose_free_port)"
fi
HOST_FIXTURES_URL="http://127.0.0.1:${FIXTURES_PORT}"
FIXTURES_URL="http://${FIXTURES_HOST}:${FIXTURES_PORT}"

if [ "${SKIP_BUILD:-}" = "1" ]; then
  if docker image inspect "$IMAGE" >/dev/null 2>&1; then
    echo "Using existing Docker image: $IMAGE (SKIP_BUILD=1)"
  else
    skip "Docker image $IMAGE not found and SKIP_BUILD=1 is set"
  fi
else
  echo "Building PinchTab CloakBrowser Docker image: $IMAGE"
  docker build -f "$ROOT/$CLOAK_DOCKERFILE" -t "$IMAGE" "$ROOT"
fi

if [ "${SKIP_RUNNER_BUILD:-}" = "1" ]; then
  if docker image inspect "$RUNNER_IMAGE" >/dev/null 2>&1; then
    echo "Using existing E2E runner image: $RUNNER_IMAGE (SKIP_RUNNER_BUILD=1)"
  else
    skip "E2E runner image $RUNNER_IMAGE not found and SKIP_RUNNER_BUILD=1 is set"
  fi
else
  echo "Building API E2E runner image: $RUNNER_IMAGE"
  docker build -f "$ROOT/$RUNNER_DOCKERFILE" -t "$RUNNER_IMAGE" "$ROOT/tests/e2e/runner-api"
fi

docker run --rm --entrypoint /bin/sh "$IMAGE" -lc "test -x '$CLOAK_CONTAINER_BIN'" \
  || fail "CloakBrowser binary not found in image at $CLOAK_CONTAINER_BIN; rebuild the dedicated CloakBrowser smoke image"

CONFIG_PATH="$TMP_DIR/pinchtab-cloak.json"
python3 - "$CONFIG_PATH" "$TOKEN" "$CLOAK_CONTAINER_BIN" "$FIXTURES_HOST" <<'PY'
import json
import sys

path, token, cloak_binary, fixtures_host = sys.argv[1:]
cfg = {
    "server": {
        "bind": "0.0.0.0",
        "port": "9867",
        "token": token,
        "stateDir": "/data",
    },
    "browser": {
        "provider": "cloak",
        "binary": cloak_binary,
        "extensionPaths": [],
        "cloak": {
            "fingerprintSeed": "42069",
            "platform": "linux",
            "locale": "en-US",
            "timezone": "UTC",
            "disableDefaultStealthArgs": True,
        },
    },
    "instanceDefaults": {
        "mode": "headless",
        "humanize": True,
        "maxTabs": 10,
    },
    "security": {
        "allowEvaluate": True,
        "allowDownload": True,
        "allowCookies": True,
        "allowUpload": True,
        "allowClipboard": True,
        "allowStateExport": True,
        "allowedDomains": [fixtures_host, "127.0.0.1", "localhost", "::1"],
        "downloadAllowedDomains": [fixtures_host],
        "trustedResolveCIDRs": ["127.0.0.1/32"],
    },
    "profiles": {
        "baseDir": "/data/profiles",
        "defaultProfile": "cloak-docker-smoke",
    },
}
with open(path, "w", encoding="utf-8") as fh:
    json.dump(cfg, fh, indent=2)
PY

docker run -d \
  --name "$NAME" \
  --user 1000:1000 \
  --shm-size=2g \
  --tmpfs /data:rw,size=1024m,uid=1000,gid=1000,mode=0755 \
  --tmpfs /tmp:rw,size=512m,mode=1777 \
  --add-host "${FIXTURES_HOST}:127.0.0.1" \
  -e PINCHTAB_CONFIG=/config/pinchtab-cloak.json \
  -v "$CONFIG_PATH":/config/pinchtab-cloak.json:ro \
  -v "$ROOT/tests/e2e/fixtures":/fixtures:ro \
  -v "$ROOT/tests/tools/fixtures/static-server.pl":/usr/local/bin/fixture-server.pl:ro \
  --publish 127.0.0.1::9867 \
  --publish "127.0.0.1:${FIXTURES_PORT}:${FIXTURES_PORT}" \
  "$IMAGE" >/dev/null
CONTAINER_STARTED=1

HOST_PORT="$(docker port "$NAME" 9867/tcp 2>/dev/null | head -1 | awk -F: '{print $NF}' || true)"
if [ -z "$HOST_PORT" ]; then
  HOST_PORT="$(docker inspect -f '{{with index .NetworkSettings.Ports "9867/tcp"}}{{(index . 0).HostPort}}{{end}}' "$NAME" 2>/dev/null || true)"
fi
if [ -z "$HOST_PORT" ]; then
  fail "failed to determine published host port"
fi

echo "Waiting for PinchTab+CloakBrowser container on port $HOST_PORT..."
docker exec "$NAME" sh -lc "FIXTURES_ROOT=/fixtures FIXTURES_PORT=${FIXTURES_PORT} /usr/bin/perl /usr/local/bin/fixture-server.pl >/tmp/pinchtab-fixtures.log 2>&1 &"
FIXTURES_STARTED=1

fixtures_ready=0
for _ in $(seq 1 30); do
  if curl -fsS "${HOST_FIXTURES_URL}/index.html" >/dev/null 2>&1 &&
    docker exec "$NAME" curl -fsS "${FIXTURES_URL}/index.html" >/dev/null 2>&1; then
    fixtures_ready=1
    break
  fi
  sleep 1
done
if [ "$fixtures_ready" -ne 1 ]; then
  fail "fixture server was not reachable from the PinchTab container at ${FIXTURES_URL}"
fi

health_ready=0
for _ in $(seq 1 90); do
  if curl -fsS -H "Authorization: Bearer ${TOKEN}" "http://127.0.0.1:${HOST_PORT}/health" >/dev/null 2>&1; then
    health_ready=1
    break
  fi
  sleep 1
done
if [ "$health_ready" -ne 1 ]; then
  health_body="$(curl -sS -H "Authorization: Bearer ${TOKEN}" "http://127.0.0.1:${HOST_PORT}/health" 2>&1 || true)"
  fail "health check did not pass: $health_body"
fi

uid="$(docker exec "$NAME" id -u | tr -d '\r')"
if [ "$uid" = "0" ]; then
  fail "container is running as root; expected non-root user"
fi

shm_mb="$(docker exec "$NAME" sh -lc "df -Pm /dev/shm | awk 'NR==2 {print \$2}'" | tr -d '\r')"
if [ "${shm_mb:-0}" -lt 1024 ]; then
  fail "/dev/shm is too small: ${shm_mb:-unknown} MB"
fi

docker exec "$NAME" sh -lc 'if command -v fc-list >/dev/null 2>&1; then fc-list | grep -q .; else find /usr/share/fonts -type f 2>/dev/null | head -1 | grep -q .; fi' || fail "no fonts found in container"

echo "Waiting for managed CloakBrowser instance..."
instance_ready=0
instances_body="[]"
autorestart_body="{}"
for _ in $(seq 1 120); do
  instances_body="$(curl -fsS -H "Authorization: Bearer ${TOKEN}" "http://127.0.0.1:${HOST_PORT}/instances" 2>/dev/null || echo "[]")"
  if echo "$instances_body" | jq -e '.[]? | select(.status == "running")' >/dev/null; then
    instance_ready=1
    break
  fi
  autorestart_body="$(curl -fsS -H "Authorization: Bearer ${TOKEN}" "http://127.0.0.1:${HOST_PORT}/autorestart/status" 2>/dev/null || echo "{}")"
  if echo "$autorestart_body" | jq -e '.status == "crashed"' >/dev/null 2>&1; then
    break
  fi
  sleep 1
done
if [ "$instance_ready" -ne 1 ]; then
  fail "managed CloakBrowser instance did not become running: instances=$instances_body autorestart=$autorestart_body"
fi

if ! status="$(curl -fsS -H "Authorization: Bearer ${TOKEN}" "http://127.0.0.1:${HOST_PORT}/stealth/status" 2>&1)"; then
  fail "stealth status request failed: $status"
fi
echo "$status" | jq -e '
  .provider == "cloak" and
  .native == true and
  .pinchtabOverlaysDisabled == true and
  .fingerprintSeed == "42069"
' >/dev/null || fail "unexpected stealth status: $status"

run_fixture_endpoint_smoke
run_e2e_scenarios

echo "Docker CloakBrowser smoke passed."
