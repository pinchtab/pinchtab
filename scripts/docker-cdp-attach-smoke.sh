#!/usr/bin/env bash
# docker-cdp-attach-smoke.sh — Opt-in smoke for the CDP attach bridge path.
#
# Two legs:
#   1. Chrome CDP attach — starts a Chromium headless container with remote
#      debugging enabled, runs the PinchTab server on the host, POSTs to
#      /instances/attach with provider=chrome, and asserts the bridge wraps it.
#   2. CloakBrowser CDP attach — same as above but uses the local
#      pinchtab-cloakbrowser:test image. Skipped if the image is unavailable.
#
# This smoke is opt-in and is NOT part of default CI.
#
# Usage:
#   ./dev smoke cdp-attach              # both legs
#   ./dev smoke cdp-attach chrome       # Chrome leg only
#   ./dev smoke cdp-attach cloak        # CloakBrowser leg only
#
# Env overrides:
#   PINCHTAB_BINARY        Path to the pinchtab binary (default: ./dist/pinchtab or `go run`)
#   PINCHTAB_CDP_PORT      Host port for the external CDP browser (default: 19222)
#   PINCHTAB_SERVER_PORT   Host port for PinchTab server (default: 19867)
#   PINCHTAB_CDP_SMOKE_CHROME_IMAGE   Chrome image (default: chromedp/headless-shell:stable)
#   PINCHTAB_CDP_SMOKE_CLOAK_IMAGE    CloakBrowser image (default: pinchtab-cloakbrowser:test)

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
ROOT="$(cd "$SCRIPT_DIR/.." && pwd)"

LEG="${1:-all}"

CHROME_IMAGE="${PINCHTAB_CDP_SMOKE_CHROME_IMAGE:-chromedp/headless-shell:stable}"
CLOAK_IMAGE="${PINCHTAB_CDP_SMOKE_CLOAK_IMAGE:-pinchtab-cloakbrowser:test}"
CDP_PORT="${PINCHTAB_CDP_PORT:-19222}"
SERVER_PORT="${PINCHTAB_SERVER_PORT:-19867}"
CLOAK_DEVTOOLS_PORT="9222"
CLOAK_FORWARD_PORT="9223"

TOKEN="cdp-attach-smoke-${RANDOM}${RANDOM}"
TMP_DIR="$(mktemp -d "${TMPDIR:-/tmp}/pinchtab-cdp-attach.XXXXXX")"
CHROME_CONTAINER="pinchtab-cdp-smoke-chrome-${RANDOM}"
CLOAK_CONTAINER="pinchtab-cdp-smoke-cloak-${RANDOM}"
PINCHTAB_PID=""
FAILED=0
ATTACHED_INSTANCE_ID=""

cleanup() {
  set +e
  if [ -n "${PINCHTAB_PID}" ] && kill -0 "${PINCHTAB_PID}" 2>/dev/null; then
    kill "${PINCHTAB_PID}" 2>/dev/null || true
    wait "${PINCHTAB_PID}" 2>/dev/null || true
  fi
  docker rm -f "${CHROME_CONTAINER}" >/dev/null 2>&1 || true
  docker rm -f "${CLOAK_CONTAINER}" >/dev/null 2>&1 || true
  if [ "${FAILED}" -ne 0 ]; then
    echo ""
    echo "Artifacts kept at: ${TMP_DIR}"
  else
    rm -rf "${TMP_DIR}"
  fi
}
trap cleanup EXIT

fail() {
  FAILED=1
  echo "FAIL: $*" >&2
  exit 1
}

skip() {
  echo "SKIP: $*"
  exit 77
}

require_cmd() {
  command -v "$1" >/dev/null 2>&1 || fail "$1 not found in PATH"
}

require_cmd docker
require_cmd curl
require_cmd jq

BIN="${PINCHTAB_BINARY:-}"
if [ -z "${BIN}" ]; then
  if [ -x "${ROOT}/dist/pinchtab" ]; then
    BIN="${ROOT}/dist/pinchtab"
  else
    echo "Building pinchtab binary…"
    (cd "${ROOT}" && go build -o "${TMP_DIR}/pinchtab" ./cmd/pinchtab)
    BIN="${TMP_DIR}/pinchtab"
  fi
fi
[ -x "${BIN}" ] || fail "pinchtab binary not executable at ${BIN}"

CONFIG_FILE="${TMP_DIR}/config.json"
write_config() {
  local provider="$1" binary="${2:-}"
  cat > "${CONFIG_FILE}" <<EOF
{
  "server": { "port": "${SERVER_PORT}", "bind": "127.0.0.1", "token": "${TOKEN}" },
  "browser": { "provider": "${provider}", "binary": "${binary}" },
  "security": {
    "attach": {
      "enabled": true,
      "allowHosts": ["127.0.0.1", "localhost", "::1"],
      "allowSchemes": ["ws", "http"]
    }
  }
}
EOF
}

wait_http() {
  local url="$1" tries="${2:-60}"
  for _ in $(seq 1 "${tries}"); do
    if curl -fsS "${url}" >/dev/null 2>&1; then return 0; fi
    sleep 0.5
  done
  return 1
}

wait_api_http() {
  local url="$1" tries="${2:-60}"
  for _ in $(seq 1 "${tries}"); do
    if curl -fsS -H "Authorization: Bearer ${TOKEN}" "${url}" >/dev/null 2>&1; then return 0; fi
    sleep 0.5
  done
  return 1
}

api_post() {
  local path="$1" body="$2"
  curl -sS -X POST -H "Authorization: Bearer ${TOKEN}" -H "Content-Type: application/json" \
    -d "${body}" "http://127.0.0.1:${SERVER_PORT}${path}"
}

api_get() {
  local path="$1"
  curl -sS -H "Authorization: Bearer ${TOKEN}" "http://127.0.0.1:${SERVER_PORT}${path}"
}

start_pinchtab() {
  echo ">>> Starting PinchTab server on :${SERVER_PORT}"
  PINCHTAB_TOKEN="${TOKEN}" PINCHTAB_CONFIG="${CONFIG_FILE}" \
    "${BIN}" server >"${TMP_DIR}/pinchtab.log" 2>&1 &
  PINCHTAB_PID=$!
  if ! wait_api_http "http://127.0.0.1:${SERVER_PORT}/health" 60; then
    cat "${TMP_DIR}/pinchtab.log" || true
    fail "PinchTab server did not become healthy"
  fi
  echo "    pinchtab pid=${PINCHTAB_PID}"
}

stop_pinchtab() {
  if [ -n "${PINCHTAB_PID}" ] && kill -0 "${PINCHTAB_PID}" 2>/dev/null; then
    kill "${PINCHTAB_PID}" 2>/dev/null || true
    wait "${PINCHTAB_PID}" 2>/dev/null || true
  fi
  PINCHTAB_PID=""
}

assert_attached_instance() {
  local name="$1" provider="$2"
  local resp http
  resp="$(api_post /instances/attach "$(jq -nc \
    --arg name "${name}" \
    --arg url "http://127.0.0.1:${CDP_PORT}" \
    --arg provider "${provider}" \
    '{name:$name, cdpUrl:$url, provider:$provider}')")"
  echo "    /instances/attach response: ${resp}" >&2
  echo "${resp}" | jq -e '.attached == true' >/dev/null || fail "attached != true"
  echo "${resp}" | jq -e '.attachType == "cdp-bridge"' >/dev/null || fail "attachType != cdp-bridge"
  echo "${resp}" | jq -e '.url | startswith("http://")' >/dev/null || fail "url is not http:// bridge URL"
  local inst_id
  inst_id="$(echo "${resp}" | jq -r '.id')"
  [ -n "${inst_id}" ] && [ "${inst_id}" != "null" ] || fail "missing instance id"
  ATTACHED_INSTANCE_ID="${inst_id}"

  echo ">>> Asserting /instances lists attached instance" >&2
  api_get /instances | jq -e --arg id "${inst_id}" '.[] | select(.id == $id)' >/dev/null \
    || fail "/instances does not list the attached instance"

  echo ">>> Asserting /stealth/status reports launchMode=remote-cdp" >&2
  local status
  status="$(api_get /stealth/status || true)"
  echo "    /stealth/status: ${status}" >&2
  echo "${status}" | jq -e --arg p "${provider}" '.provider == $p' >/dev/null \
    || echo "    (warning) provider mismatch in /stealth/status" >&2
}

stop_attached_instance() {
  local inst_id="$1"
  local resp
  resp="$(api_post "/instances/${inst_id}/stop" '')"
  echo "    /instances/${inst_id}/stop response: ${resp}"
  echo "${resp}" | jq -e '.status == "stopped"' >/dev/null || fail "attached instance stop did not report stopped"
}

run_chrome_leg() {
  echo ""
  echo "=== Chrome CDP attach leg ==="

  echo ">>> Starting Chromium container on :${CDP_PORT}"
  local -a docker_args=(
    docker run -d --rm --name "${CHROME_CONTAINER}"
    -p "127.0.0.1:${CDP_PORT}:9222" \
    "${CHROME_IMAGE}"
  )
  case "${CHROME_IMAGE}" in
    chromedp/headless-shell*)
      # The chromedp image entrypoint starts headless-shell on an internal
      # DevTools port and forwards container :9222 with socat. Passing our own
      # --remote-debugging-port=9222 makes headless-shell race that forwarder.
      ;;
    *)
      docker_args+=(
        --remote-debugging-address=0.0.0.0
        --remote-debugging-port=9222
        --no-sandbox
        --disable-dev-shm-usage
        --headless
      )
      ;;
  esac
  "${docker_args[@]}" >/dev/null

  if ! wait_http "http://127.0.0.1:${CDP_PORT}/json/version" 60; then
    docker logs "${CHROME_CONTAINER}" | tail -50 || true
    fail "Chromium DevTools not reachable on :${CDP_PORT}"
  fi

  write_config "chrome"
  start_pinchtab
  ATTACHED_INSTANCE_ID=""
  assert_attached_instance "smoke-chrome" "chrome"
  local inst_id="${ATTACHED_INSTANCE_ID}"

  echo ">>> Stopping attached PinchTab instance ${inst_id}"
  stop_attached_instance "${inst_id}"

  echo ">>> Verifying external Chromium container is still running"
  if ! docker ps --format '{{.Names}}' | grep -Fxq "${CHROME_CONTAINER}"; then
    fail "external Chromium container was killed by stop (must not happen)"
  fi
  echo "    OK — external browser process preserved."

  stop_pinchtab
  docker rm -f "${CHROME_CONTAINER}" >/dev/null 2>&1 || true
  echo "Chrome leg PASS"
}

run_cloak_leg() {
  echo ""
  echo "=== CloakBrowser CDP attach leg ==="
  if ! docker image inspect "${CLOAK_IMAGE}" >/dev/null 2>&1; then
    skip "CloakBrowser image '${CLOAK_IMAGE}' not present — build with tests/tools/docker/cloakbrowser-smoke.Dockerfile to enable"
  fi
  if ! docker run --rm --entrypoint /bin/sh "${CLOAK_IMAGE}" -lc 'command -v socat >/dev/null 2>&1'; then
    fail "CloakBrowser image '${CLOAK_IMAGE}' is missing socat; rebuild with tests/tools/docker/cloakbrowser-smoke.Dockerfile"
  fi

  echo ">>> Starting CloakBrowser container on :${CDP_PORT}"
  docker run -d --rm --name "${CLOAK_CONTAINER}" \
    -p "127.0.0.1:${CDP_PORT}:${CLOAK_FORWARD_PORT}" \
    -e CLOAK_DEVTOOLS_PORT="${CLOAK_DEVTOOLS_PORT}" \
    -e CLOAK_FORWARD_PORT="${CLOAK_FORWARD_PORT}" \
    "${CLOAK_IMAGE}" \
    /bin/sh -lc '
      set -e
      /opt/cloakbrowser/chrome \
        --remote-debugging-address=127.0.0.1 \
        --remote-debugging-port="${CLOAK_DEVTOOLS_PORT}" \
        --no-sandbox \
        --headless &
      browser_pid=$!
      trap "kill ${browser_pid} 2>/dev/null || true" TERM INT EXIT
      for _ in $(seq 1 120); do
        if curl -fsS "http://127.0.0.1:${CLOAK_DEVTOOLS_PORT}/json/version" >/dev/null 2>&1; then
          exec socat "TCP-LISTEN:${CLOAK_FORWARD_PORT},fork,reuseaddr,bind=0.0.0.0" "TCP:127.0.0.1:${CLOAK_DEVTOOLS_PORT}"
        fi
        if ! kill -0 "${browser_pid}" 2>/dev/null; then
          wait "${browser_pid}"
          exit $?
        fi
        sleep 0.25
      done
      echo "CloakBrowser DevTools did not become ready on 127.0.0.1:${CLOAK_DEVTOOLS_PORT}" >&2
      exit 1
    ' \
    >/dev/null

  if ! wait_http "http://127.0.0.1:${CDP_PORT}/json/version" 90; then
    docker logs "${CLOAK_CONTAINER}" | tail -50 || true
    fail "CloakBrowser DevTools not reachable on :${CDP_PORT}"
  fi

  write_config "cloak" "/opt/cloakbrowser/chrome"
  start_pinchtab
  ATTACHED_INSTANCE_ID=""
  assert_attached_instance "smoke-cloak" "cloak"
  local inst_id="${ATTACHED_INSTANCE_ID}"

  echo ">>> Stopping attached PinchTab instance ${inst_id}"
  stop_attached_instance "${inst_id}"

  echo ">>> Verifying external CloakBrowser container is still running"
  if ! docker ps --format '{{.Names}}' | grep -Fxq "${CLOAK_CONTAINER}"; then
    fail "external CloakBrowser container was killed by stop (must not happen)"
  fi
  echo "    OK — external browser process preserved."

  stop_pinchtab
  docker rm -f "${CLOAK_CONTAINER}" >/dev/null 2>&1 || true
  echo "CloakBrowser leg PASS"
}

case "${LEG}" in
  all)    run_chrome_leg; run_cloak_leg ;;
  chrome) run_chrome_leg ;;
  cloak)  run_cloak_leg ;;
  *)      fail "unknown leg: ${LEG} (expected: all | chrome | cloak)" ;;
esac

echo ""
echo "OK: docker-cdp-attach-smoke"
