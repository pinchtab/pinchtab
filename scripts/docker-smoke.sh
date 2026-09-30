#!/usr/bin/env bash
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
# shellcheck source=lib/smoke-common.sh
. "$SCRIPT_DIR/lib/smoke-common.sh"
# shellcheck source=lib/smoke-container.sh
. "$SCRIPT_DIR/lib/smoke-container.sh"
# shellcheck source=lib/smoke-health.sh
. "$SCRIPT_DIR/lib/smoke-health.sh"

IMAGE="${1:-pinchtab-local:test}"
SMOKE_TOKEN="pinchtab-smoke-token-${RANDOM}${RANDOM}"
NAME="pinchtab-smoke-${RANDOM}${RANDOM}"
FAILED=0

smoke_container_trap "$NAME"

# No host port mapping needed — all checks run via docker exec inside the container.
docker run -d --name "$NAME" -e PINCHTAB_TOKEN="$SMOKE_TOKEN" "$IMAGE" >/dev/null

echo "Waiting for PinchTab to become healthy..."
wait_for_exec_health "$NAME" "$SMOKE_TOKEN" 60

bind_addr="$(docker exec "$NAME" pinchtab config get server.bind | tr -d '\r')"
if [ "$bind_addr" != "0.0.0.0" ]; then
  fail "unexpected server.bind: $bind_addr"
fi

config_path="$(docker exec "$NAME" pinchtab config path | tr -d '\r')"
if [ -z "$config_path" ]; then
  fail "failed to determine container config path"
fi

if ! docker exec "$NAME" test -f "$config_path"; then
  fail "config file not found at reported path: $config_path"
fi

echo "Docker smoke test passed."
