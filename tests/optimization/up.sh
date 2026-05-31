#!/usr/bin/env bash
# tests/optimization/up.sh <provider>
#
# Bring up a PinchTab instance for the optimization benchmark with the
# requested browser provider (chrome | cloak | ghost-chrome).
#
# - chrome       : Uses the existing tests/tools/docker-compose.yml (regular image).
# - cloak        : Starts the benchmark fixtures + a standalone container using the
#                  cloakbrowser smoke image (pinchtab-cloakbrowser:test).
# - ghost-chrome : Same Chrome image as `chrome` but with a config that sets
#                  `browsers.default = "ghost-chrome"` (static-first routing).
#                  Runs as a standalone container against the shared fixtures.
#
# The script ensures the internal "fixtures" hostname works for all paths
# so that group files and subagent-context.md continue to use http://fixtures/
# without changes.
#
# After success it prints READY with the container name, token, etc.
# Call down.sh when finished.
#
# Environment:
#   REBUILD=1   Force rebuild of the chosen image.
set -euo pipefail

PROVIDER="${1:?provider required (chrome|cloak|ghost-chrome)}"
case "$PROVIDER" in chrome|cloak|ghost-chrome) ;; *) echo "invalid provider: $PROVIDER"; exit 2 ;; esac

ROOT="$(git rev-parse --show-toplevel)"
TOOLS_DIR="$ROOT/tests/tools"
OPT_DIR="$ROOT/tests/optimization"
NAME="optimization-pinchtab"
TOKEN="benchmark-token"

# shellcheck source=/dev/null
source "$ROOT/scripts/lib/smoke-common.sh"
# shellcheck source=/dev/null
source "$ROOT/scripts/lib/smoke-config.sh"

cd "$TOOLS_DIR"

ensure_fixtures() {
  # Make sure the nginx fixtures service (and its network) exists.
  docker compose -f docker-compose.yml up -d --no-recreate fixtures >/dev/null 2>&1 || true
}

get_fixtures_ip() {
  # Return the IP of the fixtures container on the benchmark network.
  docker inspect -f '{{range .NetworkSettings.Networks}}{{.IPAddress}}{{end}}' tools-fixtures-1 2>/dev/null || echo ""
}

ensure_image() {
  local image="$1"
  local dockerfile="$2"

  if [ "${REBUILD:-}" = "1" ] || ! docker image inspect "$image" >/dev/null 2>&1; then
    echo "Building $image..." >&2
    if [ -n "$dockerfile" ]; then
      docker build -f "$ROOT/$dockerfile" -t "$image" "$ROOT" >&2
    else
      docker build -t "$image" "$ROOT" >&2
    fi
  else
    echo "Reusing $image (set REBUILD=1 to rebuild)" >&2
  fi
}

case "$PROVIDER" in
  chrome)
    echo "Starting optimization benchmark with chrome provider (compose)..." >&2
    docker compose -f docker-compose.yml down --remove-orphans 2>/dev/null || true
    docker compose -f docker-compose.yml up -d --build pinchtab fixtures
    # Give it a moment
    sleep 8
    if ! curl -sf -H "Authorization: Bearer $TOKEN" http://127.0.0.1:9867/health >/dev/null 2>&1; then
      echo "ERROR: chrome pinchtab did not become healthy" >&2
      docker compose logs --tail 50 pinchtab || true
      exit 1
    fi
    # The compose service name inside docker is tools-pinchtab-1 for the default project
    CONTAINER_NAME="tools-pinchtab-1"
    CLOAK_BIN=""
    ;;

  cloak)
    echo "Starting optimization benchmark with cloak provider..." >&2
    ensure_fixtures

    FIXTURES_IP="$(get_fixtures_ip)"
    if [ -z "$FIXTURES_IP" ]; then
      echo "ERROR: could not determine fixtures container IP" >&2
      exit 1
    fi

    ensure_image "pinchtab-cloakbrowser:test" "tests/tools/docker/cloakbrowser-smoke.Dockerfile"

    CONFIG_DIR="$OPT_DIR/.tmp"
    CONFIG_PATH="$CONFIG_DIR/pinchtab-cloak.json"
    mkdir -p "$CONFIG_DIR"

    # Use "fixtures" as the host (we'll add it via --add-host on the container)
    write_provider_config "cloak" "$CONFIG_PATH" "$TOKEN" "fixtures" "" "/opt/cloakbrowser/chrome"

    # Allow the benchmark fixtures (already in the config) plus localhost for convenience
    python3 - "$CONFIG_PATH" <<'PY'
import json, sys
p = sys.argv[1]
c = json.load(open(p))
c.setdefault("security", {}).setdefault("allowedDomains", [])
if "fixtures" not in c["security"]["allowedDomains"]:
    c["security"]["allowedDomains"].append("fixtures")
json.dump(c, open(p, "w"), indent=2)
PY

    docker rm -f "$NAME" >/dev/null 2>&1 || true

    docker run -d --name "$NAME" \
      --network tools_benchmark-net \
      --add-host "fixtures:${FIXTURES_IP}" \
      -p 127.0.0.1:9867:9867 \
      --shm-size=2g \
      --dns 8.8.8.8 --dns 1.1.1.1 \
      -v "$CONFIG_PATH:/fixture-config/pinchtab.json:ro" \
      -e PINCHTAB_CONFIG=/fixture-config/pinchtab.json \
      pinchtab-cloakbrowser:test >/dev/null

    echo "Waiting for /health on 127.0.0.1:9867 (cloak)..." >&2
    for i in $(seq 1 45); do
      if curl -sf -H "Authorization: Bearer $TOKEN" http://127.0.0.1:9867/health >/dev/null 2>&1; then
        CONTAINER_NAME="$NAME"
        CLOAK_BIN="/opt/cloakbrowser/chrome"
        break
      fi
      sleep 1
    done

    if [ -z "${CONTAINER_NAME:-}" ]; then
      echo "ERROR: cloak pinchtab did not become healthy after 45s" >&2
      docker logs "$NAME" 2>&1 | tail -60 >&2
      exit 1
    fi
    ;;

  ghost-chrome)
    echo "Starting optimization benchmark with ghost-chrome provider..." >&2
    ensure_fixtures

    FIXTURES_IP="$(get_fixtures_ip)"
    if [ -z "$FIXTURES_IP" ]; then
      echo "ERROR: could not determine fixtures container IP" >&2
      exit 1
    fi

    # Ghost-chrome reuses the chrome compose image (`tools-pinchtab:latest`) —
    # the difference is purely config (browsers.default = "ghost-chrome").
    CHROME_IMAGE="tools-pinchtab:latest"
    if [ "${REBUILD:-}" = "1" ] || ! docker image inspect "$CHROME_IMAGE" >/dev/null 2>&1; then
      echo "Building $CHROME_IMAGE via compose..." >&2
      docker compose -f docker-compose.yml build pinchtab >&2
    else
      echo "Reusing $CHROME_IMAGE (set REBUILD=1 to rebuild)" >&2
    fi

    CONFIG_DIR="$OPT_DIR/.tmp"
    CONFIG_PATH="$CONFIG_DIR/pinchtab-ghost-chrome.json"
    mkdir -p "$CONFIG_DIR"

    # Start from a chrome-flavoured config, then flip browsers.default to
    # "ghost-chrome" and pin configVersion so the startup wizard does not
    # rewrite the file. Mirrors writeGhostChromeConfig in
    # tests/tools/runner/internal/e2e/provider.go.
    write_provider_config "chrome" "$CONFIG_PATH" "$TOKEN" "fixtures" "" ""
    python3 - "$CONFIG_PATH" <<'PY'
import json, sys
p = sys.argv[1]
c = json.load(open(p))
c["configVersion"] = "0.8.0"
browsers = c.setdefault("browsers", {})
browsers["default"] = "ghost-chrome"
browsers["available"] = ["ghost-chrome"]
c.setdefault("security", {}).setdefault("allowedDomains", [])
if "fixtures" not in c["security"]["allowedDomains"]:
    c["security"]["allowedDomains"].append("fixtures")
json.dump(c, open(p, "w"), indent=2)
PY

    docker rm -f "$NAME" >/dev/null 2>&1 || true

    docker run -d --name "$NAME" \
      --network tools_benchmark-net \
      --add-host "fixtures:${FIXTURES_IP}" \
      -p 127.0.0.1:9867:9867 \
      --shm-size=2g \
      --dns 8.8.8.8 --dns 1.1.1.1 \
      -v "$CONFIG_PATH:/fixture-config/pinchtab.json:ro" \
      -e PINCHTAB_CONFIG=/fixture-config/pinchtab.json \
      "$CHROME_IMAGE" >/dev/null

    echo "Waiting for /health on 127.0.0.1:9867 (ghost-chrome)..." >&2
    for i in $(seq 1 30); do
      if curl -sf -H "Authorization: Bearer $TOKEN" http://127.0.0.1:9867/health >/dev/null 2>&1; then
        CONTAINER_NAME="$NAME"
        CLOAK_BIN=""
        break
      fi
      sleep 1
    done

    if [ -z "${CONTAINER_NAME:-}" ]; then
      echo "ERROR: ghost-chrome pinchtab did not become healthy after 30s" >&2
      docker logs "$NAME" 2>&1 | tail -60 >&2
      exit 1
    fi
    ;;
esac

cat <<EOF
READY
provider=$PROVIDER
container=$CONTAINER_NAME
token=$TOKEN
port=9867
config=${CONFIG_PATH:-"(compose default)"}
EOF
exit 0
