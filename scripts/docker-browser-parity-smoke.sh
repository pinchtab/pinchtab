#!/usr/bin/env bash
# docker-browser-parity-smoke.sh — Browser-provider parity smoke harness.
#
# Runs the same fixture-backed endpoint assertions and API basic E2E suite
# against both providers, sequentially, with full teardown between legs:
#
#   chrome  — standard pinchtab:local image (Chromium already bundled).
#   cloak   — pinchtab-cloakbrowser:test image with the CloakBrowser binary
#             baked in via tests/tools/docker/cloakbrowser-smoke.Dockerfile.
#
# Provider-specific assertions:
#   chrome  → /stealth/status.provider == "chrome"
#   cloak   → /stealth/status.provider == "cloak" && .native == true
#             && .pinchtabOverlaysDisabled == true && .fingerprintSeed == "42069"
#
# This smoke is opt-in and NOT part of default CI / ./dev all.
#
# Usage:
#   ./dev smoke cloakbrowser                       # both legs (default)
#   ./dev smoke cloakbrowser --provider=chrome     # Chrome leg only
#   ./dev smoke cloakbrowser --provider=cloak      # Cloak leg only
#   ./dev smoke cloakbrowser --provider=all
#   ./dev smoke cloakbrowser --multi-target        # one-container multi-target leg
#   ./dev smoke cloakbrowser --provider=all --multi-target  # per-leg + multi-target
#   ./dev smoke cloakbrowser --profile-persistence # opt-in profile-persistence leg
#                                                  # (Docker named volume + UUID round-trip;
#                                                  #  defaults to --provider=cloak,
#                                                  #  honors an explicit --provider=chrome)
#   ./dev smoke cloakbrowser --profile-lock-recovery
#                                                  # opt-in lock-recovery leg (P3b):
#                                                  # seeds the P3a marker, plants a stale
#                                                  # Chrome SingletonLock referencing a
#                                                  # probably-unused PID, restarts the
#                                                  # container, and asserts PinchTab
#                                                  # recovers cleanly without losing
#                                                  # profile state. Mutually exclusive with
#                                                  # --multi-target; defaults to
#                                                  # --provider=cloak.
#
# Env overrides:
#   PINCHTAB_PARITY_CHROME_IMAGE     Chrome leg image (default: pinchtab-local:test)
#   PINCHTAB_PARITY_CLOAK_IMAGE      Cloak leg image  (default: pinchtab-cloakbrowser:test)
#   PINCHTAB_CLOAKBROWSER_RUNNER_IMAGE  E2E runner image (default: pinchtab-e2e-runner-api:cloak-smoke)
#   PINCHTAB_PARITY_E2E_SCENARIOS    Override scenario list (space-separated)
#   PINCHTAB_CLOAKBROWSER_DOCKERFILE Path to Cloak smoke Dockerfile
#   PINCHTAB_CLOAKBROWSER_RUNNER_DOCKERFILE  Runner Dockerfile path
#   SKIP_BUILD=1                     Reuse existing provider image
#   SKIP_RUNNER_BUILD=1              Reuse existing E2E runner image
#
# Backwards compat:
#   PINCHTAB_CLOAKBROWSER_E2E_SCENARIOS  honored on the cloak leg only.
#   First positional arg can be a Cloak image name (legacy).

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
ROOT="$(cd "$SCRIPT_DIR/.." && pwd)"
LIB_DIR="$SCRIPT_DIR/lib"

# shellcheck source=lib/smoke-common.sh
source "$LIB_DIR/smoke-common.sh"
# shellcheck source=lib/smoke-config.sh
source "$LIB_DIR/smoke-config.sh"
# shellcheck source=lib/smoke-container.sh
source "$LIB_DIR/smoke-container.sh"
# shellcheck source=lib/smoke-health.sh
source "$LIB_DIR/smoke-health.sh"
# shellcheck source=lib/smoke-fixtures.sh
source "$LIB_DIR/smoke-fixtures.sh"
# shellcheck source=lib/smoke-endpoints.sh
source "$LIB_DIR/smoke-endpoints.sh"
# shellcheck source=lib/smoke-scenarios.sh
source "$LIB_DIR/smoke-scenarios.sh"
# shellcheck source=lib/smoke-diagnostics.sh
source "$LIB_DIR/smoke-diagnostics.sh"
# shellcheck source=lib/smoke-assertions.sh
source "$LIB_DIR/smoke-assertions.sh"
# shellcheck source=lib/smoke-multi-target.sh
source "$LIB_DIR/smoke-multi-target.sh"
# shellcheck source=lib/smoke-persistence.sh
source "$LIB_DIR/smoke-persistence.sh"

# ── CLI parsing ──────────────────────────────────────────────────────────────

PROVIDERS_ARG=""
MULTI_TARGET=0
PROFILE_PERSISTENCE=0
PROFILE_LOCK_RECOVERY=0
LEGACY_CLOAK_IMAGE=""

for arg in "$@"; do
  case "$arg" in
    --provider=*)
      PROVIDERS_ARG="${arg#--provider=}"
      ;;
    --provider)
      fail "--provider requires =VALUE (e.g. --provider=chrome)"
      ;;
    --multi-target)
      MULTI_TARGET=1
      ;;
    --profile-persistence)
      PROFILE_PERSISTENCE=1
      ;;
    --profile-lock-recovery)
      PROFILE_LOCK_RECOVERY=1
      ;;
    --help|-h)
      sed -n '2,52p' "$0"
      exit 0
      ;;
    --*)
      fail "unknown flag: $arg"
      ;;
    *)
      if [ -z "$LEGACY_CLOAK_IMAGE" ]; then
        LEGACY_CLOAK_IMAGE="$arg"
      else
        fail "unexpected positional arg: $arg"
      fi
      ;;
  esac
done

PROVIDERS=()
RUN_MULTI_TARGET=0
RUN_PROFILE_PERSISTENCE=0
RUN_PROFILE_LOCK_RECOVERY=0
PERSISTENCE_PROVIDER=""
LOCK_RECOVERY_PROVIDER=""
if [ "$PROFILE_LOCK_RECOVERY" -eq 1 ]; then
  if [ "$MULTI_TARGET" -eq 1 ]; then
    fail "--profile-lock-recovery is mutually exclusive with --multi-target"
  fi
  case "${PROVIDERS_ARG:-cloak}" in
    cloak)  LOCK_RECOVERY_PROVIDER="cloak"  ;;
    chrome) LOCK_RECOVERY_PROVIDER="chrome" ;;
    ""|all)
      fail "--profile-lock-recovery requires a single provider (--provider=chrome or --provider=cloak; default cloak)"
      ;;
    *)
      fail "invalid --provider for --profile-lock-recovery: $PROVIDERS_ARG (expected chrome|cloak)"
      ;;
  esac
  RUN_PROFILE_LOCK_RECOVERY=1
  # Lock-recovery internally invokes assert_persistence_round_trip, so
  # --profile-persistence becomes a no-op subset here.
  PROFILE_PERSISTENCE=0
  RUN_PROFILE_PERSISTENCE=0
  PROVIDERS=()
elif [ "$PROFILE_PERSISTENCE" -eq 1 ]; then
  if [ "$MULTI_TARGET" -eq 1 ]; then
    fail "--profile-persistence is mutually exclusive with --multi-target"
  fi
  case "${PROVIDERS_ARG:-cloak}" in
    cloak)  PERSISTENCE_PROVIDER="cloak"  ;;
    chrome) PERSISTENCE_PROVIDER="chrome" ;;
    ""|all)
      fail "--profile-persistence requires a single provider (--provider=chrome or --provider=cloak; default cloak)"
      ;;
    *)
      fail "invalid --provider for --profile-persistence: $PROVIDERS_ARG (expected chrome|cloak)"
      ;;
  esac
  RUN_PROFILE_PERSISTENCE=1
  PROVIDERS=()
elif [ "$MULTI_TARGET" -eq 1 ]; then
  case "$PROVIDERS_ARG" in
    ""|all)
      RUN_MULTI_TARGET=1
      if [ "$PROVIDERS_ARG" = "all" ]; then
        PROVIDERS=(chrome cloak)
      fi
      ;;
    chrome|cloak)
      fail "--multi-target is mutually exclusive with --provider=${PROVIDERS_ARG} (use --provider=all to run both)"
      ;;
    *)
      fail "invalid --provider: $PROVIDERS_ARG (expected chrome|cloak|all)"
      ;;
  esac
else
  case "${PROVIDERS_ARG:-all}" in
    all)    PROVIDERS=(chrome cloak) ;;
    chrome) PROVIDERS=(chrome) ;;
    cloak)  PROVIDERS=(cloak) ;;
    *)      fail "invalid --provider: $PROVIDERS_ARG (expected chrome|cloak|all)" ;;
  esac
fi

CHROME_BASE_IMAGE="${PINCHTAB_PARITY_CHROME_BASE_IMAGE:-pinchtab-local:test}"
CHROME_IMAGE="${PINCHTAB_PARITY_CHROME_IMAGE:-pinchtab-chrome-smoke:test}"
CLOAK_IMAGE="${PINCHTAB_PARITY_CLOAK_IMAGE:-${LEGACY_CLOAK_IMAGE:-pinchtab-cloakbrowser:test}}"
RUNNER_IMAGE="${PINCHTAB_CLOAKBROWSER_RUNNER_IMAGE:-pinchtab-e2e-runner-api:cloak-smoke}"
CLOAK_CONTAINER_BIN="/opt/cloakbrowser/chrome"
CLOAK_IMAGE_CHROME_BIN="${PINCHTAB_PARITY_MULTI_CHROME_BIN:-/usr/bin/chromium}"
CHROME_DOCKERFILE="${PINCHTAB_PARITY_CHROME_DOCKERFILE:-tests/tools/docker/chrome-smoke.Dockerfile}"
CLOAK_DOCKERFILE="${PINCHTAB_CLOAKBROWSER_DOCKERFILE:-tests/tools/docker/cloakbrowser-smoke.Dockerfile}"
RUNNER_DOCKERFILE="${PINCHTAB_CLOAKBROWSER_RUNNER_DOCKERFILE:-tests/e2e/runner-api/Dockerfile}"

# ── Prereqs ──────────────────────────────────────────────────────────────────

FAILED=0
command -v docker >/dev/null 2>&1 || skip "docker is not installed"
require_cmd python3
require_cmd jq
require_cmd curl

# ── Top-level TMP_DIR (legs use a per-leg sub-directory) ─────────────────────

TOP_TMP_DIR="$(mktemp -d "${TMPDIR:-/tmp}/pinchtab-browser-parity.XXXXXX")"

TMP_DIR=""
TOKEN=""
NAME=""
HOST_PORT=""
HOST_FIXTURES_URL=""
FIXTURES_URL=""
FIXTURES_HOST=""
FIXTURES_PORT=""
CONFIG_PATH=""
CONTAINER_STARTED=0
FIXTURES_STARTED=0
API_RESULT=""
API_STATUS=""
CURRENT_PROVIDER=""
LEG_PROFILE_VOLUME=""

declare -a LEG_RESULTS=()

top_cleanup() {
  set +e
  if [ "$FAILED" -ne 0 ]; then
    echo ""
    echo "Artifacts kept at: $TOP_TMP_DIR"
  else
    rm -rf "$TOP_TMP_DIR"
  fi
}
trap top_cleanup EXIT

# ── Image builds ─────────────────────────────────────────────────────────────

ensure_chrome_image() {
  if [ "${SKIP_BUILD:-}" = "1" ]; then
    if docker image inspect "$CHROME_IMAGE" >/dev/null 2>&1; then
      echo "Using existing Chrome smoke image: $CHROME_IMAGE (SKIP_BUILD=1)"
      return
    fi
    skip "Chrome smoke image $CHROME_IMAGE not found and SKIP_BUILD=1 is set"
  fi

  if ! docker image inspect "$CHROME_BASE_IMAGE" >/dev/null 2>&1; then
    echo "Building base PinchTab image: $CHROME_BASE_IMAGE"
    docker build -t "$CHROME_BASE_IMAGE" "$ROOT"
  else
    echo "Reusing existing base PinchTab image: $CHROME_BASE_IMAGE"
  fi

  echo "Building Chrome smoke overlay image: $CHROME_IMAGE (BASE_IMAGE=$CHROME_BASE_IMAGE)"
  docker build \
    -f "$ROOT/$CHROME_DOCKERFILE" \
    --build-arg "BASE_IMAGE=$CHROME_BASE_IMAGE" \
    -t "$CHROME_IMAGE" \
    "$ROOT"
}

ensure_cloak_image() {
  if [ "${SKIP_BUILD:-}" = "1" ]; then
    if docker image inspect "$CLOAK_IMAGE" >/dev/null 2>&1; then
      echo "Using existing Cloak image: $CLOAK_IMAGE (SKIP_BUILD=1)"
    else
      skip "Cloak image $CLOAK_IMAGE not found and SKIP_BUILD=1 is set"
    fi
  else
    echo "Building PinchTab CloakBrowser Docker image: $CLOAK_IMAGE"
    docker build -f "$ROOT/$CLOAK_DOCKERFILE" -t "$CLOAK_IMAGE" "$ROOT"
  fi
  docker run --rm --entrypoint /bin/sh "$CLOAK_IMAGE" -lc "test -x '$CLOAK_CONTAINER_BIN'" \
    || fail "CloakBrowser binary not found in image at $CLOAK_CONTAINER_BIN"
}

ensure_runner_image() {
  if [ "${SKIP_RUNNER_BUILD:-}" = "1" ]; then
    if docker image inspect "$RUNNER_IMAGE" >/dev/null 2>&1; then
      echo "Using existing E2E runner image: $RUNNER_IMAGE (SKIP_RUNNER_BUILD=1)"
      return
    fi
    skip "E2E runner image $RUNNER_IMAGE not found and SKIP_RUNNER_BUILD=1 is set"
  fi
  echo "Building API E2E runner image: $RUNNER_IMAGE"
  docker build -f "$ROOT/$RUNNER_DOCKERFILE" -t "$RUNNER_IMAGE" "$ROOT/tests/e2e/runner-api"
}

# ── Per-leg runner ───────────────────────────────────────────────────────────

leg_cleanup() {
  local rc=$?
  set +e
  if [ "$rc" -ne 0 ] && [ -n "$CURRENT_PROVIDER" ]; then
    if [ "$CONTAINER_STARTED" -eq 1 ] && docker ps -a --format '{{.Names}}' | grep -Fxq "$NAME"; then
      dump_diagnostics "$CURRENT_PROVIDER" "$NAME" "$HOST_PORT" "$TOKEN" "$CONFIG_PATH" "$FIXTURES_STARTED" || true
    fi
  fi
  if [ -n "$NAME" ]; then
    teardown_container "$NAME"
  fi
  if [ -n "${LEG_PROFILE_VOLUME:-}" ]; then
    cleanup_profile_volume "$LEG_PROFILE_VOLUME"
    LEG_PROFILE_VOLUME=""
  fi
  return $rc
}

run_leg() {
  local provider="$1"
  local image="$2"

  echo ""
  echo "=============================================="
  echo "=== Browser parity leg: provider=${provider}"
  echo "=============================================="

  trap leg_cleanup EXIT

  CURRENT_PROVIDER="$provider"
  TMP_DIR="$TOP_TMP_DIR/$provider"
  mkdir -p "$TMP_DIR"
  TOKEN="pinchtab-parity-${provider}-${RANDOM}${RANDOM}"
  NAME="pinchtab-parity-${provider}-${RANDOM}${RANDOM}"
  HOST_PORT=""
  CONTAINER_STARTED=0
  FIXTURES_STARTED=0
  API_RESULT=""
  API_STATUS=""
  FIXTURES_HOST="parity-${provider}-fixtures.local"
  FIXTURES_PORT="$(choose_free_port)"
  HOST_FIXTURES_URL="http://127.0.0.1:${FIXTURES_PORT}"
  FIXTURES_URL="http://${FIXTURES_HOST}:${FIXTURES_PORT}"
  CONFIG_PATH="$TMP_DIR/pinchtab-${provider}.json"

  if [ "$provider" = "cloak" ]; then
    write_provider_config "$provider" "$CONFIG_PATH" "$TOKEN" "$FIXTURES_HOST" "" "$CLOAK_CONTAINER_BIN"
  else
    write_provider_config "$provider" "$CONFIG_PATH" "$TOKEN" "$FIXTURES_HOST" "" ""
  fi

  start_provider_container "$NAME" "$image" "$CONFIG_PATH" "$FIXTURES_HOST" "$FIXTURES_PORT" "$ROOT"
  HOST_PORT="$(resolve_host_port "$NAME")"
  echo "Container $NAME up — PinchTab on host port $HOST_PORT"

  start_fixture_server "$NAME" "$FIXTURES_PORT"
  wait_fixtures_ready "$NAME" "$FIXTURES_URL" "$HOST_FIXTURES_URL"

  echo "Waiting for PinchTab health on :${HOST_PORT}..."
  wait_for_health "$HOST_PORT" "$TOKEN"

  local uid
  uid="$(docker exec "$NAME" id -u | tr -d '\r')"
  [ "$uid" != "0" ] || fail "container is running as root; expected non-root user"

  local shm_mb
  shm_mb="$(docker exec "$NAME" sh -lc "df -Pm /dev/shm | awk 'NR==2 {print \$2}'" | tr -d '\r')"
  if [ "${shm_mb:-0}" -lt 1024 ]; then
    fail "/dev/shm too small: ${shm_mb:-unknown} MB"
  fi

  docker exec "$NAME" sh -lc \
    'if command -v fc-list >/dev/null 2>&1; then fc-list | grep -q .; else find /usr/share/fonts -type f 2>/dev/null | head -1 | grep -q .; fi' \
    || fail "no fonts found in container"

  echo "Waiting for managed ${provider} instance..."
  wait_for_instance_running "$HOST_PORT" "$TOKEN"

  assert_stealth_status "$provider"
  run_fixture_endpoint_smoke
  run_e2e_scenarios "$provider" "$NAME"

  echo "Leg passed: provider=${provider}"
}

# ── Multi-target leg ─────────────────────────────────────────────────────────

ensure_multi_target_image_binaries() {
  # Fail-fast: verify both browser binaries are still present in the shared
  # Cloak smoke image before launching the multi-target container.
  docker run --rm --entrypoint /bin/sh "$CLOAK_IMAGE" -lc \
    "test -x '$CLOAK_CONTAINER_BIN' && test -x '$CLOAK_IMAGE_CHROME_BIN'" \
    || fail "multi-target leg: image $CLOAK_IMAGE must contain both $CLOAK_CONTAINER_BIN and $CLOAK_IMAGE_CHROME_BIN"
}

run_multi_target_leg() {
  echo ""
  echo "=============================================="
  echo "=== Browser parity leg: multi-target"
  echo "=============================================="

  trap leg_cleanup EXIT

  CURRENT_PROVIDER="multi-target"
  TMP_DIR="$TOP_TMP_DIR/multi-target"
  mkdir -p "$TMP_DIR"
  TOKEN="pinchtab-parity-multi-${RANDOM}${RANDOM}"
  NAME="pinchtab-parity-multi-${RANDOM}${RANDOM}"
  HOST_PORT=""
  CONTAINER_STARTED=0
  FIXTURES_STARTED=0
  API_RESULT=""
  API_STATUS=""
  FIXTURES_HOST="parity-multi-fixtures.local"
  FIXTURES_PORT="$(choose_free_port)"
  HOST_FIXTURES_URL="http://127.0.0.1:${FIXTURES_PORT}"
  FIXTURES_URL="http://${FIXTURES_HOST}:${FIXTURES_PORT}"
  CONFIG_PATH="$TMP_DIR/pinchtab-multi.json"

  # Stub lives on the host so the non-root container user can exec it via bind-mount.
  local broken_host="$TMP_DIR/broken-chrome.sh"
  setup_broken_binary "$broken_host" >/dev/null
  local broken_container="/opt/broken-chrome.sh"

  write_multi_target_config \
    "$CONFIG_PATH" "$TOKEN" "$FIXTURES_HOST" \
    "$CLOAK_IMAGE_CHROME_BIN" "$CLOAK_CONTAINER_BIN"
  jq -e '.browser.targets.chrome and .browser.targets.cloak and .browser.defaultTarget == "chrome" and (.browser.fallbackOrder | index("chrome") != null) and (.browser.fallbackOrder | index("cloak") != null)' \
    "$CONFIG_PATH" >/dev/null \
    || fail "generated multi-target config is malformed: $(cat "$CONFIG_PATH")"

  docker run -d \
    --name "$NAME" \
    --user 1000:1000 \
    --shm-size=2g \
    --tmpfs /data:rw,size=1024m,uid=1000,gid=1000,mode=0755 \
    --tmpfs /tmp:rw,size=512m,mode=1777 \
    --add-host "${FIXTURES_HOST}:127.0.0.1" \
    -e "PINCHTAB_CONFIG=/config/pinchtab.json" \
    -v "${CONFIG_PATH}:/config/pinchtab.json:ro" \
    -v "${broken_host}:${broken_container}:ro" \
    -v "${ROOT}/tests/e2e/fixtures:/fixtures:ro" \
    -v "${ROOT}/tests/tools/fixtures/static-server.pl:/usr/local/bin/fixture-server.pl:ro" \
    --publish "127.0.0.1::9867" \
    --publish "127.0.0.1:${FIXTURES_PORT}:${FIXTURES_PORT}" \
    "$CLOAK_IMAGE" >/dev/null
  CONTAINER_STARTED=1

  HOST_PORT="$(resolve_host_port "$NAME")"
  echo "Container $NAME up — PinchTab on host port $HOST_PORT (multi-target)"

  start_fixture_server "$NAME" "$FIXTURES_PORT"
  wait_fixtures_ready "$NAME" "$FIXTURES_URL" "$HOST_FIXTURES_URL"

  echo "Waiting for PinchTab health on :${HOST_PORT}..."
  wait_for_health "$HOST_PORT" "$TOKEN"

  echo "→ Phase A: per-request target selection"
  assert_target_selection "$HOST_PORT" "$TOKEN" "chrome" "chrome"
  assert_target_selection "$HOST_PORT" "$TOKEN" "cloak" "cloak"

  echo "→ Phase B: default target resolution"
  assert_default_target "$HOST_PORT" "$TOKEN" "chrome"

  # Phase C uses a second config that points the chrome target at the broken
  # stub, then restarts the container so PinchTab loads it.
  local broken_cfg="$TMP_DIR/pinchtab-multi-broken.json"
  write_multi_target_config \
    "$broken_cfg" "$TOKEN" "$FIXTURES_HOST" \
    "$CLOAK_IMAGE_CHROME_BIN" "$CLOAK_CONTAINER_BIN" \
    "$broken_container"

  echo "→ Phase C: fallback (restarting container with broken chrome primary)"
  teardown_container "$NAME"
  CONTAINER_STARTED=0
  FIXTURES_STARTED=0

  docker run -d \
    --name "$NAME" \
    --user 1000:1000 \
    --shm-size=2g \
    --tmpfs /data:rw,size=1024m,uid=1000,gid=1000,mode=0755 \
    --tmpfs /tmp:rw,size=512m,mode=1777 \
    --add-host "${FIXTURES_HOST}:127.0.0.1" \
    -e "PINCHTAB_CONFIG=/config/pinchtab.json" \
    -v "${broken_cfg}:/config/pinchtab.json:ro" \
    -v "${broken_host}:${broken_container}:ro" \
    -v "${ROOT}/tests/e2e/fixtures:/fixtures:ro" \
    -v "${ROOT}/tests/tools/fixtures/static-server.pl:/usr/local/bin/fixture-server.pl:ro" \
    --publish "127.0.0.1::9867" \
    --publish "127.0.0.1:${FIXTURES_PORT}:${FIXTURES_PORT}" \
    "$CLOAK_IMAGE" >/dev/null
  CONTAINER_STARTED=1
  HOST_PORT="$(resolve_host_port "$NAME")"
  start_fixture_server "$NAME" "$FIXTURES_PORT"
  wait_fixtures_ready "$NAME" "$FIXTURES_URL" "$HOST_FIXTURES_URL"
  wait_for_health "$HOST_PORT" "$TOKEN"
  CONFIG_PATH="$broken_cfg"

  assert_fallback "$HOST_PORT" "$TOKEN" "chrome" "cloak" "cloak"

  echo "Leg passed: multi-target"
}

# Profile-persistence leg (P3a): single provider against a Docker named
# volume; navigates a marker fixture, restarts container, asserts marker
# survives. Opt-in only.

run_profile_persistence_leg() {
  local provider="$1"
  local image="$2"

  echo ""
  echo "=============================================="
  echo "=== Browser parity leg: profile-persistence (provider=${provider})"
  echo "=============================================="

  trap leg_cleanup EXIT

  CURRENT_PROVIDER="persistence-${provider}"
  TMP_DIR="$TOP_TMP_DIR/persistence-${provider}"
  mkdir -p "$TMP_DIR"
  TOKEN="pinchtab-persistence-${provider}-${RANDOM}${RANDOM}"
  NAME="pinchtab-persistence-${provider}-${RANDOM}${RANDOM}"
  HOST_PORT=""
  CONTAINER_STARTED=0
  FIXTURES_STARTED=0
  API_RESULT=""
  API_STATUS=""
  FIXTURES_HOST="persistence-${provider}-fixtures.local"
  FIXTURES_PORT="$(choose_free_port)"
  HOST_FIXTURES_URL="http://127.0.0.1:${FIXTURES_PORT}"
  FIXTURES_URL="http://${FIXTURES_HOST}:${FIXTURES_PORT}"
  CONFIG_PATH="$TMP_DIR/pinchtab-${provider}.json"
  LEG_PROFILE_VOLUME="pinchtab-persistence-${provider}-$$-${RANDOM}"

  # Identical config bytes are reused on restart.
  if [ "$provider" = "cloak" ]; then
    write_provider_config "$provider" "$CONFIG_PATH" "$TOKEN" "$FIXTURES_HOST" "" "$CLOAK_CONTAINER_BIN"
  else
    write_provider_config "$provider" "$CONFIG_PATH" "$TOKEN" "$FIXTURES_HOST" "" ""
  fi
  jq -e '.server.stateDir == "/data"' "$CONFIG_PATH" >/dev/null \
    || fail "persistence leg: generated config did not set server.stateDir=/data"

  setup_profile_volume "$LEG_PROFILE_VOLUME" >/dev/null
  echo "Profile volume ready: $LEG_PROFILE_VOLUME"

  PROFILE_VOLUME="$LEG_PROFILE_VOLUME" start_provider_container \
    "$NAME" "$image" "$CONFIG_PATH" "$FIXTURES_HOST" "$FIXTURES_PORT" "$ROOT"
  HOST_PORT="$(resolve_host_port "$NAME")"
  echo "Container $NAME up — PinchTab on host port $HOST_PORT (volume=$LEG_PROFILE_VOLUME)"

  start_fixture_server "$NAME" "$FIXTURES_PORT"
  wait_fixtures_ready "$NAME" "$FIXTURES_URL" "$HOST_FIXTURES_URL"

  echo "Waiting for PinchTab health on :${HOST_PORT}..."
  wait_for_health "$HOST_PORT" "$TOKEN"
  echo "Waiting for managed ${provider} instance..."
  wait_for_instance_running "$HOST_PORT" "$TOKEN"

  assert_persistence_round_trip \
    "$provider" "$image" "$NAME" "$CONFIG_PATH" \
    "$FIXTURES_HOST" "$FIXTURES_PORT" \
    "$FIXTURES_URL" "$HOST_FIXTURES_URL" \
    "$TOKEN" "$ROOT" "$LEG_PROFILE_VOLUME"

  echo "Leg passed: profile-persistence (provider=${provider})"
}

# Profile-lock-recovery leg (P3b): strict superset of P3a. After the
# persistence round-trip, plants stale SingletonLock/Socket/Cookie referencing
# an unused PID and asserts PinchTab recovers cleanly. Opt-in only.

run_profile_lock_recovery_leg() {
  local provider="$1"
  local image="$2"

  echo ""
  echo "=============================================="
  echo "=== Browser parity leg: profile-lock-recovery (provider=${provider})"
  echo "=============================================="

  trap leg_cleanup EXIT

  CURRENT_PROVIDER="lock-recovery-${provider}"
  TMP_DIR="$TOP_TMP_DIR/lock-recovery-${provider}"
  mkdir -p "$TMP_DIR"
  TOKEN="pinchtab-lockrec-${provider}-${RANDOM}${RANDOM}"
  NAME="pinchtab-lockrec-${provider}-${RANDOM}${RANDOM}"
  HOST_PORT=""
  CONTAINER_STARTED=0
  FIXTURES_STARTED=0
  API_RESULT=""
  API_STATUS=""
  FIXTURES_HOST="lockrec-${provider}-fixtures.local"
  FIXTURES_PORT="$(choose_free_port)"
  HOST_FIXTURES_URL="http://127.0.0.1:${FIXTURES_PORT}"
  FIXTURES_URL="http://${FIXTURES_HOST}:${FIXTURES_PORT}"
  CONFIG_PATH="$TMP_DIR/pinchtab-${provider}.json"
  LEG_PROFILE_VOLUME="pinchtab-lockrec-${provider}-$$-${RANDOM}"

  # Config bytes must be byte-stable across the two restarts in this flow.
  if [ "$provider" = "cloak" ]; then
    write_provider_config "$provider" "$CONFIG_PATH" "$TOKEN" "$FIXTURES_HOST" "" "$CLOAK_CONTAINER_BIN"
  else
    write_provider_config "$provider" "$CONFIG_PATH" "$TOKEN" "$FIXTURES_HOST" "" ""
  fi
  jq -e '.server.stateDir == "/data"' "$CONFIG_PATH" >/dev/null \
    || fail "lock-recovery leg: generated config did not set server.stateDir=/data"

  # The live profile name (queried below) is not always profiles.defaultProfile —
  # the cloak orchestrator currently launches a hard-coded "default" profile.
  local profile_basedir
  profile_basedir="$(jq -r '.profiles.baseDir // empty' "$CONFIG_PATH")"
  [ -n "$profile_basedir" ] || fail "lock-recovery leg: profiles.baseDir missing"
  local profile_basedir_rel="${profile_basedir#/data/}"

  setup_profile_volume "$LEG_PROFILE_VOLUME" >/dev/null
  echo "Profile volume ready: $LEG_PROFILE_VOLUME"

  PROFILE_VOLUME="$LEG_PROFILE_VOLUME" start_provider_container \
    "$NAME" "$image" "$CONFIG_PATH" "$FIXTURES_HOST" "$FIXTURES_PORT" "$ROOT"
  HOST_PORT="$(resolve_host_port "$NAME")"
  echo "Container $NAME up — PinchTab on host port $HOST_PORT (volume=$LEG_PROFILE_VOLUME)"

  start_fixture_server "$NAME" "$FIXTURES_PORT"
  wait_fixtures_ready "$NAME" "$FIXTURES_URL" "$HOST_FIXTURES_URL"

  echo "Waiting for PinchTab health on :${HOST_PORT}..."
  wait_for_health "$HOST_PORT" "$TOKEN"
  echo "Waiting for managed ${provider} instance..."
  wait_for_instance_running "$HOST_PORT" "$TOKEN"

  local live_profile_name
  live_profile_name="$(curl -fsS -H "Authorization: Bearer ${TOKEN}" \
    "http://127.0.0.1:${HOST_PORT}/instances" 2>/dev/null \
    | jq -r '.[]? | select(.status=="running") | .profileName' | head -1)"
  [ -n "$live_profile_name" ] || fail "lock-recovery leg: could not discover live profile name from /instances"
  local profile_subpath="${profile_basedir_rel}/${live_profile_name}"
  echo "Discovered live profile subpath: ${profile_subpath}"

  assert_lock_recovery_round_trip \
    "$provider" "$image" "$NAME" "$CONFIG_PATH" \
    "$FIXTURES_HOST" "$FIXTURES_PORT" \
    "$FIXTURES_URL" "$HOST_FIXTURES_URL" \
    "$TOKEN" "$ROOT" "$LEG_PROFILE_VOLUME" "$profile_subpath"

  echo "Leg passed: profile-lock-recovery (provider=${provider})"
}

# ── Drive the legs ───────────────────────────────────────────────────────────

for p in "${PROVIDERS[@]}"; do
  case "$p" in
    chrome) ensure_chrome_image ;;
    cloak)  ensure_cloak_image ;;
  esac
done
if [ "$RUN_MULTI_TARGET" -eq 1 ]; then
  ensure_cloak_image
  ensure_multi_target_image_binaries
fi
if [ "$RUN_PROFILE_PERSISTENCE" -eq 1 ]; then
  case "$PERSISTENCE_PROVIDER" in
    cloak)  ensure_cloak_image  ;;
    chrome) ensure_chrome_image ;;
  esac
fi
if [ "$RUN_PROFILE_LOCK_RECOVERY" -eq 1 ]; then
  case "$LOCK_RECOVERY_PROVIDER" in
    cloak)  ensure_cloak_image  ;;
    chrome) ensure_chrome_image ;;
  esac
fi
if [ "${#PROVIDERS[@]}" -gt 0 ]; then
  ensure_runner_image
fi

OVERALL_FAILED=0

for p in "${PROVIDERS[@]}"; do
  case "$p" in
    chrome) leg_image="$CHROME_IMAGE" ;;
    cloak)  leg_image="$CLOAK_IMAGE" ;;
    *)      fail "unsupported provider: $p" ;;
  esac

  # Subshell isolates a leg failure from later legs; its leg_cleanup trap
  # handles diagnostics + teardown.
  if (
    set -e
    FAILED=0
    run_leg "$p" "$leg_image"
  ); then
    LEG_RESULTS+=("$p:PASS")
  else
    leg_status=$?
    LEG_RESULTS+=("$p:FAIL($leg_status)")
    OVERALL_FAILED=1
    echo ""
    echo "Leg failed: provider=${p} (continuing with remaining legs)"
  fi
done

if [ "$RUN_MULTI_TARGET" -eq 1 ]; then
  if (
    set -e
    FAILED=0
    run_multi_target_leg
  ); then
    LEG_RESULTS+=("multi-target:PASS")
  else
    leg_status=$?
    LEG_RESULTS+=("multi-target:FAIL($leg_status)")
    OVERALL_FAILED=1
    echo ""
    echo "Leg failed: multi-target"
  fi
fi

if [ "$RUN_PROFILE_PERSISTENCE" -eq 1 ]; then
  case "$PERSISTENCE_PROVIDER" in
    cloak)  persistence_image="$CLOAK_IMAGE"  ;;
    chrome) persistence_image="$CHROME_IMAGE" ;;
    *)      fail "unsupported persistence provider: $PERSISTENCE_PROVIDER" ;;
  esac
  if (
    set -e
    FAILED=0
    run_profile_persistence_leg "$PERSISTENCE_PROVIDER" "$persistence_image"
  ); then
    LEG_RESULTS+=("profile-persistence(${PERSISTENCE_PROVIDER}):PASS")
  else
    leg_status=$?
    LEG_RESULTS+=("profile-persistence(${PERSISTENCE_PROVIDER}):FAIL($leg_status)")
    OVERALL_FAILED=1
    echo ""
    echo "Leg failed: profile-persistence (${PERSISTENCE_PROVIDER})"
  fi
fi

if [ "$RUN_PROFILE_LOCK_RECOVERY" -eq 1 ]; then
  case "$LOCK_RECOVERY_PROVIDER" in
    cloak)  lock_recovery_image="$CLOAK_IMAGE"  ;;
    chrome) lock_recovery_image="$CHROME_IMAGE" ;;
    *)      fail "unsupported lock-recovery provider: $LOCK_RECOVERY_PROVIDER" ;;
  esac
  if (
    set -e
    FAILED=0
    run_profile_lock_recovery_leg "$LOCK_RECOVERY_PROVIDER" "$lock_recovery_image"
  ); then
    LEG_RESULTS+=("profile-lock-recovery(${LOCK_RECOVERY_PROVIDER}):PASS")
  else
    leg_status=$?
    LEG_RESULTS+=("profile-lock-recovery(${LOCK_RECOVERY_PROVIDER}):FAIL($leg_status)")
    OVERALL_FAILED=1
    echo ""
    echo "Leg failed: profile-lock-recovery (${LOCK_RECOVERY_PROVIDER})"
  fi
fi

echo ""
echo "=============================================="
echo "Browser parity smoke summary"
for r in "${LEG_RESULTS[@]}"; do
  echo "  - $r"
done
echo "=============================================="

if [ "$OVERALL_FAILED" -ne 0 ]; then
  FAILED=1
  echo "FAIL: at least one provider leg failed" >&2
  exit 1
fi

echo "OK: docker-browser-parity-smoke"
