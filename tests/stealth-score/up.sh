#!/usr/bin/env bash
# tests/stealth-score/up.sh <provider>
#
# Bring up a Docker container for stealth-score work with the requested
# browser provider (chrome | cloak). The container exposes PinchTab on host
# port 9867 and the agent drives it through the existing ./scripts/pt
# wrapper. When the agent is done, call down.sh to tear it down.
#
# Build behaviour:
#   - Reuses cached images if present (pinchtab-chrome-smoke:test,
#     pinchtab-cloakbrowser:test). Set REBUILD=1 to force fresh builds.
#
# Side effects:
#   - Stops/removes any existing container named "stealth-score-pinchtab".
#   - Writes a temp config to tests/stealth-score/.tmp/.
set -euo pipefail

PROVIDER="${1:?provider required (chrome|cloak)}"
case "$PROVIDER" in chrome|cloak) ;; *) echo "invalid provider: $PROVIDER"; exit 2 ;; esac

ROOT="$(git rev-parse --show-toplevel)"
NAME="stealth-score-pinchtab"
TOKEN="stealth-score-token"
CONFIG_DIR="$ROOT/tests/stealth-score/.tmp"
CONFIG_PATH="$CONFIG_DIR/pinchtab-$PROVIDER.json"
mkdir -p "$CONFIG_DIR"

# shellcheck source=/dev/null
source "$ROOT/scripts/lib/smoke-common.sh"
# shellcheck source=/dev/null
source "$ROOT/scripts/lib/smoke-config.sh"

ensure_image() {
  local image="$1" dockerfile="$2"
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
    ensure_image "pinchtab-local:test" ""
    ensure_image "pinchtab-chrome-smoke:test" "tests/tools/docker/chrome-smoke.Dockerfile"
    IMAGE="pinchtab-chrome-smoke:test"
    CLOAK_BIN=""
    ;;
  cloak)
    ensure_image "pinchtab-cloakbrowser:test" "tests/tools/docker/cloakbrowser-smoke.Dockerfile"
    IMAGE="pinchtab-cloakbrowser:test"
    CLOAK_BIN="/opt/cloakbrowser/chrome"
    ;;
esac

write_provider_config "$PROVIDER" "$CONFIG_PATH" "$TOKEN" "stealth-score-fixtures.local" "" "$CLOAK_BIN"

# Open allowedDomains so the agent can hit the public detection sites.
python3 - "$CONFIG_PATH" <<'PY'
import json, sys
p = sys.argv[1]
c = json.load(open(p))
c.setdefault("security", {})["allowedDomains"] = []
json.dump(c, open(p, "w"), indent=2)
PY

docker rm -f "$NAME" >/dev/null 2>&1 || true

docker run -d --name "$NAME" \
  -p 127.0.0.1:9867:9867 \
  --shm-size=2g \
  --dns 8.8.8.8 --dns 1.1.1.1 \
  -v "$CONFIG_PATH:/fixture-config/pinchtab.json:ro" \
  -e PINCHTAB_CONFIG=/fixture-config/pinchtab.json \
  "$IMAGE" >/dev/null

echo "Waiting for /health on 127.0.0.1:9867..." >&2
for i in $(seq 1 40); do
  if curl -sf -H "Authorization: Bearer $TOKEN" http://127.0.0.1:9867/health >/dev/null 2>&1; then
    cat <<EOF
READY
container=$NAME
provider=$PROVIDER
token=$TOKEN
port=9867
config=$CONFIG_PATH
EOF
    exit 0
  fi
  sleep 1
done

echo "FAILED to become healthy after 40s" >&2
docker logs "$NAME" 2>&1 | tail -40 >&2
exit 1
