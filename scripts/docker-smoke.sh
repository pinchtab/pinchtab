#!/usr/bin/env bash
set -euo pipefail

IMAGE="${1:-pinchtab-local:test}"
SMOKE_TOKEN="pinchtab-smoke-token-${RANDOM}${RANDOM}"

NAME="pinchtab-smoke-${RANDOM}${RANDOM}"
FAILED=0

fail() {
  FAILED=1
  echo "$1"
  exit 1
}

cleanup() {
  if docker ps -a --format '{{.Names}}' | grep -Fxq "$NAME"; then
    if [ "$FAILED" -ne 0 ]; then
      echo ""
      echo "Container logs:"
      docker logs "$NAME" 2>&1 | tail -50 || true
    fi
    docker rm -f "$NAME" >/dev/null 2>&1 || true
  fi
}
trap 'rc=$?; [ "$rc" -ne 0 ] && FAILED=1; cleanup' EXIT

# No host port mapping needed — all checks run via docker exec inside the container.
docker run -d --name "$NAME" -e PINCHTAB_TOKEN="$SMOKE_TOKEN" "$IMAGE" >/dev/null

health_check() {
  docker exec "$NAME" sh -c \
    "wget -qO- --header='Authorization: Bearer $SMOKE_TOKEN' http://127.0.0.1:9867/health" \
    >/dev/null 2>&1
}

echo "Waiting for PinchTab to become healthy..."
for _ in $(seq 1 60); do
  if health_check; then
    break
  fi
  sleep 1
done

if ! health_check; then
  fail "health check did not pass within 60s"
fi

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

FAILED=0
echo "Docker smoke test passed."
