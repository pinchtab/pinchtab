#!/usr/bin/env bash
# docker-live-detection-smoke.sh — opt-in live detection-site probe (P6).
#
# Spins up a single PinchTab container (Chrome by default, --browser=cloak for
# CloakBrowser), drives the managed instance through a small list of public bot
# detection demo sites, and for each site captures:
#
#   - a PNG screenshot (GET /tabs/<id>/screenshot)
#   - a summary string (GET /tabs/<id>/text?selector=…) or full-page text
#
# All artifacts are written under a per-run output directory plus a top-level
# report.md summarizing what was visited and any observed detection signal.
#
# Strict semantics:
#   * Opt-in — NOT wired into ./dev all or CI; external network calls only
#     happen when this smoke is explicitly invoked.
#   * Output is advisory. The smoke exits 0 if every site was *attempted*
#     successfully (container healthy, request returned). It exits 1 ONLY on
#     infrastructure failure: container fails to start, PinchTab unreachable,
#     or the image is missing while SKIP_BUILD=1.
#   * A site reporting "you are a bot" is NOT a failure — that is the whole
#     point of the advisory capture.
#
# Usage:
#   ./dev smoke live-detection                          # Chrome leg
#   ./dev smoke live-detection --browser=chrome
#   ./dev smoke live-detection --browser=cloak
#
# Env overrides:
#   PINCHTAB_PARITY_CHROME_IMAGE     Chrome leg image (default: pinchtab-chrome-smoke:test)
#   PINCHTAB_PARITY_CHROME_BASE_IMAGE  Base PinchTab image (default: pinchtab-local:test)
#   PINCHTAB_PARITY_CLOAK_IMAGE      Cloak leg image  (default: pinchtab-cloakbrowser:test)
#   PINCHTAB_LIVE_DETECTION_SITES    Override TSV path (default: scripts/lib/live-detection-sites.tsv)
#   PINCHTAB_LIVE_DETECTION_OUTDIR   Override output directory (default: tests/e2e/results/live-detection/<provider>-<ts>)
#   PINCHTAB_LIVE_DETECTION_NAV_TIMEOUT_MS  Per-site navigate timeout (default: 25000)
#   PINCHTAB_LIVE_DETECTION_ONLY     Space-separated list of site names to include
#   SKIP_BUILD=1                     Reuse existing provider image; do not docker build

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

# ── CLI parsing ──────────────────────────────────────────────────────────────

PROVIDER="chrome"
for arg in "$@"; do
  case "$arg" in
    --browser=chrome|--browser=cloak)
      PROVIDER="${arg#--browser=}"
      ;;
    --browser=*)
      fail "invalid --browser: ${arg#--browser=} (expected chrome|cloak)"
      ;;
    --browser)
      fail "--browser requires =VALUE (e.g. --browser=chrome)"
      ;;
    --help|-h)
      sed -n '2,38p' "$0"
      exit 0
      ;;
    *)
      fail "unknown arg: $arg"
      ;;
  esac
done

CHROME_BASE_IMAGE="${PINCHTAB_PARITY_CHROME_BASE_IMAGE:-pinchtab-local:test}"
CHROME_IMAGE="${PINCHTAB_PARITY_CHROME_IMAGE:-pinchtab-chrome-smoke:test}"
CLOAK_IMAGE="${PINCHTAB_PARITY_CLOAK_IMAGE:-pinchtab-cloakbrowser:test}"
CLOAK_CONTAINER_BIN="/opt/cloakbrowser/chrome"
CHROME_DOCKERFILE="${PINCHTAB_PARITY_CHROME_DOCKERFILE:-tests/tools/docker/chrome-smoke.Dockerfile}"
CLOAK_DOCKERFILE="${PINCHTAB_CLOAKBROWSER_DOCKERFILE:-tests/tools/docker/cloakbrowser-smoke.Dockerfile}"

SITES_FILE="${PINCHTAB_LIVE_DETECTION_SITES:-$LIB_DIR/live-detection-sites.tsv}"
NAV_TIMEOUT_MS="${PINCHTAB_LIVE_DETECTION_NAV_TIMEOUT_MS:-25000}"

[ -r "$SITES_FILE" ] || fail "site list not readable: $SITES_FILE"

# ── Prereqs ──────────────────────────────────────────────────────────────────

FAILED=0
command -v docker >/dev/null 2>&1 || skip "docker is not installed"
require_cmd python3
require_cmd jq
require_cmd curl

# ── Output directory ─────────────────────────────────────────────────────────

if [ -n "${PINCHTAB_LIVE_DETECTION_OUTDIR:-}" ]; then
  ARTIFACT_DIR="$PINCHTAB_LIVE_DETECTION_OUTDIR"
  mkdir -p "$ARTIFACT_DIR"
else
  ts="$(date -u +%Y%m%dT%H%M%SZ)"
  ARTIFACT_DIR="$ROOT/tests/e2e/results/live-detection/${PROVIDER}-${ts}"
  mkdir -p "$ARTIFACT_DIR"
fi
REPORT="$ARTIFACT_DIR/report.md"

TMP_DIR="$(mktemp -d "${TMPDIR:-/tmp}/pinchtab-live-detection.XXXXXX")"
TOKEN="pinchtab-live-detection-${PROVIDER}-${RANDOM}${RANDOM}"
NAME="pinchtab-live-detection-${PROVIDER}-${RANDOM}${RANDOM}"
HOST_PORT=""
CONTAINER_STARTED=0
API_RESULT=""
API_STATUS=""
# Unused at runtime but required by write_provider_config.
FIXTURES_HOST="live-detection-fixtures.local"
FIXTURES_PORT="$(choose_free_port)"
CONFIG_PATH="$TMP_DIR/pinchtab-${PROVIDER}.json"

cleanup() {
  set +e
  if [ -n "$NAME" ]; then
    teardown_container "$NAME"
  fi
  rm -rf "$TMP_DIR"
}
trap cleanup EXIT

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
  if ! docker image inspect "$CHROME_IMAGE" >/dev/null 2>&1; then
    echo "Building Chrome smoke overlay image: $CHROME_IMAGE"
    docker build \
      -f "$ROOT/$CHROME_DOCKERFILE" \
      --build-arg "BASE_IMAGE=$CHROME_BASE_IMAGE" \
      -t "$CHROME_IMAGE" \
      "$ROOT"
  else
    echo "Reusing existing Chrome smoke image: $CHROME_IMAGE"
  fi
}

ensure_cloak_image() {
  if [ "${SKIP_BUILD:-}" = "1" ]; then
    if docker image inspect "$CLOAK_IMAGE" >/dev/null 2>&1; then
      echo "Using existing Cloak image: $CLOAK_IMAGE (SKIP_BUILD=1)"
    else
      skip "Cloak image $CLOAK_IMAGE not found and SKIP_BUILD=1 is set"
    fi
  else
    if ! docker image inspect "$CLOAK_IMAGE" >/dev/null 2>&1; then
      echo "Building PinchTab CloakBrowser Docker image: $CLOAK_IMAGE"
      docker build -f "$ROOT/$CLOAK_DOCKERFILE" -t "$CLOAK_IMAGE" "$ROOT"
    else
      echo "Reusing existing Cloak image: $CLOAK_IMAGE"
    fi
  fi
  docker run --rm --entrypoint /bin/sh "$CLOAK_IMAGE" -lc "test -x '$CLOAK_CONTAINER_BIN'" \
    || fail "CloakBrowser binary not found in image at $CLOAK_CONTAINER_BIN"
}

case "$PROVIDER" in
  chrome) ensure_chrome_image ; IMAGE="$CHROME_IMAGE" ;;
  cloak)  ensure_cloak_image  ; IMAGE="$CLOAK_IMAGE"  ;;
esac

# ── Config + container ───────────────────────────────────────────────────────

if [ "$PROVIDER" = "cloak" ]; then
  write_provider_config "$PROVIDER" "$CONFIG_PATH" "$TOKEN" "$FIXTURES_HOST" "" "$CLOAK_CONTAINER_BIN"
else
  write_provider_config "$PROVIDER" "$CONFIG_PATH" "$TOKEN" "$FIXTURES_HOST" "" ""
fi

# Clear allowedDomains: this smoke navigates the open internet, so the
# implicit IDPI allowlist must not constrain it.
python3 - "$CONFIG_PATH" <<'PY'
import json, sys
p = sys.argv[1]
with open(p) as fh:
    cfg = json.load(fh)
cfg.setdefault("security", {})
cfg["security"]["allowedDomains"] = []
cfg["security"]["downloadAllowedDomains"] = []
with open(p, "w") as fh:
    json.dump(cfg, fh, indent=2)
PY

echo "Starting PinchTab container (provider=${PROVIDER}, image=${IMAGE})..."
start_provider_container "$NAME" "$IMAGE" "$CONFIG_PATH" "$FIXTURES_HOST" "$FIXTURES_PORT" "$ROOT"
HOST_PORT="$(resolve_host_port "$NAME")"
echo "Container $NAME up — PinchTab on host port $HOST_PORT"

echo "Waiting for PinchTab health on :${HOST_PORT}..."
wait_for_health "$HOST_PORT" "$TOKEN"
echo "Waiting for managed ${PROVIDER} instance..."
wait_for_instance_running "$HOST_PORT" "$TOKEN"

# ── Report header ────────────────────────────────────────────────────────────

{
  echo "# PinchTab live detection probe — ${PROVIDER}"
  echo
  echo "Generated: $(date -u +%Y-%m-%dT%H:%M:%SZ)"
  echo "Provider:  \`${PROVIDER}\`"
  echo "Image:     \`${IMAGE}\`"
  echo "Site list: \`${SITES_FILE#$ROOT/}\`"
  echo
  echo "> Output is **advisory only**. Detection verdicts here are not"
  echo "> deterministic pass/fail release gates."
  echo
  echo "## Sites visited"
  echo
} > "$REPORT"

# ── Per-site probe ───────────────────────────────────────────────────────────

probe_site() {
  local name="$1" url="$2" wait_for="$3" selector="$4" notes="$5"
  local site_dir="$ARTIFACT_DIR/$name"
  mkdir -p "$site_dir"

  echo ""
  echo "── ${name} — ${url}"

  local nav_payload
  if [ "$wait_for" = "selector" ] && [ -n "$selector" ] && [ "$selector" != "-" ]; then
    nav_payload="$(jq -nc \
      --arg url "$url" \
      --arg sel "$selector" \
      --argjson timeout "$NAV_TIMEOUT_MS" \
      '{url:$url, waitFor:"selector", waitSelector:$sel, timeout:$timeout}')"
  else
    nav_payload="$(jq -nc \
      --arg url "$url" \
      --argjson timeout "$NAV_TIMEOUT_MS" \
      '{url:$url, timeout:$timeout}')"
  fi

  # Tolerant of non-2xx — api_post would call fail() and abort the run.
  local nav_status nav_body tab_id="" nav_error=""
  local response
  if response="$(curl -sS \
    -w $'\n%{http_code}' \
    -X POST \
    -H "Authorization: Bearer ${TOKEN}" \
    -H "Content-Type: application/json" \
    -d "$nav_payload" \
    --max-time $(( NAV_TIMEOUT_MS / 1000 + 10 )) \
    "http://127.0.0.1:${HOST_PORT}/navigate" 2>&1)"; then
    nav_status="${response##*$'\n'}"
    nav_body="${response%$'\n'*}"
  else
    nav_status="000"
    nav_body="$response"
  fi

  if [ "$nav_status" != "200" ]; then
    nav_error="navigate HTTP ${nav_status}: $(echo "$nav_body" | head -c 400)"
    echo "  navigate advisory error: $nav_error"
    printf '%s\n' "$nav_body" > "$site_dir/navigate-error.txt"
  else
    tab_id="$(echo "$nav_body" | jq -r '.tabId // empty' 2>/dev/null || true)"
    if [ -z "$tab_id" ]; then
      nav_error="navigate succeeded but no tabId in response"
      echo "  $nav_error"
      printf '%s\n' "$nav_body" > "$site_dir/navigate-error.txt"
    else
      echo "  navigated; tab=$tab_id"
    fi
  fi

  local screenshot_path="$site_dir/screenshot.png"
  local summary_path="$site_dir/summary.txt"
  local screenshot_status="skipped" summary_status="skipped" summary_kind="-"

  if [ -n "$tab_id" ]; then
    if curl -fsS \
        -o "$screenshot_path" \
        -H "Authorization: Bearer ${TOKEN}" \
        --max-time 30 \
        "http://127.0.0.1:${HOST_PORT}/tabs/${tab_id}/screenshot?format=png&raw=true" 2>/dev/null; then
      local bytes
      bytes="$(wc -c < "$screenshot_path" | tr -d '[:space:]')"
      screenshot_status="ok (${bytes} bytes)"
      echo "  screenshot: ${bytes} bytes"
    else
      screenshot_status="failed"
      rm -f "$screenshot_path"
      echo "  screenshot failed"
    fi

    local text_url
    if [ -n "$selector" ] && [ "$selector" != "-" ]; then
      text_url="http://127.0.0.1:${HOST_PORT}/tabs/${tab_id}/text?selector=$(python3 -c 'import urllib.parse,sys;print(urllib.parse.quote(sys.argv[1]))' "$selector")"
      summary_kind="selector(${selector})"
    else
      text_url="http://127.0.0.1:${HOST_PORT}/tabs/${tab_id}/text"
      summary_kind="full-page"
    fi
    local text_body
    if text_body="$(curl -fsS \
        -H "Authorization: Bearer ${TOKEN}" \
        --max-time 20 \
        "$text_url" 2>&1)"; then
      # Endpoint returns JSON when selector is used, plain text otherwise.
      local extracted
      extracted="$(echo "$text_body" | jq -r '.text // empty' 2>/dev/null)"
      if [ -z "$extracted" ]; then
        extracted="$text_body"
      fi
      printf '%s\n' "$extracted" > "$summary_path"
      summary_status="ok ($(wc -c < "$summary_path" | tr -d '[:space:]') bytes)"
      echo "  summary: ${summary_status}"
    else
      summary_status="failed"
      echo "  summary failed"
    fi
  fi

  local signal="(no hint)"
  if [ -r "$summary_path" ]; then
    if grep -Eiq 'cloudflare|attention required|just a moment' "$summary_path"; then
      signal="cloudflare-challenge"
    elif grep -Eiq 'you are a bot|bot detected|access denied' "$summary_path"; then
      signal="bot-flagged"
    elif grep -Eiq 'pass|human|not a robot|verified' "$summary_path"; then
      signal="possibly-passing"
    fi
  fi

  {
    echo "### ${name}"
    echo
    echo "- URL: <${url}>"
    echo "- Notes: ${notes}"
    echo "- Wait/extract: \`${summary_kind}\`"
    echo "- Navigate: $( [ -z "$nav_error" ] && echo "ok (tab=${tab_id})" || echo "advisory error — ${nav_error}" )"
    echo "- Screenshot: ${screenshot_status} — \`${name}/screenshot.png\`"
    echo "- Summary: ${summary_status} — \`${name}/summary.txt\`"
    echo "- Heuristic signal: \`${signal}\`"
    echo
  } >> "$REPORT"
}

declare -A SITE_FILTER=()
HAS_FILTER=0
if [ -n "${PINCHTAB_LIVE_DETECTION_ONLY:-}" ]; then
  HAS_FILTER=1
  for s in $PINCHTAB_LIVE_DETECTION_ONLY; do
    SITE_FILTER[$s]=1
  done
fi

SITES_VISITED=0
while IFS=$'\t' read -r name url wait_for selector notes; do
  case "$name" in
    ""|"#"*) continue ;;
  esac
  if [ "$HAS_FILTER" -eq 1 ] && [ -z "${SITE_FILTER[$name]:-}" ]; then
    continue
  fi
  probe_site "$name" "$url" "$wait_for" "$selector" "$notes" || true
  SITES_VISITED=$(( SITES_VISITED + 1 ))
done < "$SITES_FILE"

if [ "$SITES_VISITED" -eq 0 ]; then
  fail "no sites matched filter (PINCHTAB_LIVE_DETECTION_ONLY=${PINCHTAB_LIVE_DETECTION_ONLY:-})"
fi

# ── Wrap up ──────────────────────────────────────────────────────────────────

{
  echo "---"
  echo
  echo "Sites attempted: ${SITES_VISITED}"
} >> "$REPORT"

echo ""
echo "=============================================="
echo "Live detection smoke complete"
echo "  provider:    ${PROVIDER}"
echo "  sites:       ${SITES_VISITED}"
echo "  artifacts:   ${ARTIFACT_DIR}"
echo "  report:      ${REPORT}"
echo "=============================================="

exit 0
