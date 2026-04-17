#!/usr/bin/env bash
set -euo pipefail

IMAGE="${1:-pinchtab-local:test}"
SMOKE_TOKEN="pinchtab-smoke-token-${RANDOM}${RANDOM}"

NAME="pinchtab-smoke-${RANDOM}${RANDOM}"
FAILED=0

cleanup() {
  if docker ps -a --format '{{.Names}}' | grep -Fxq "$NAME"; then
    if [ "$FAILED" -ne 0 ]; then
      echo ""
      echo "Container logs:"
      docker logs "$NAME" || true
    fi
    docker rm -f "$NAME" >/dev/null 2>&1 || true
  fi
}
trap cleanup EXIT

docker run -d --name "$NAME" -e PINCHTAB_TOKEN="$SMOKE_TOKEN" -p 127.0.0.1::9867 "$IMAGE" >/dev/null

HOST_PORT="$(docker port "$NAME" 9867/tcp | head -1 | awk -F: '{print $NF}')"
if [ -z "$HOST_PORT" ]; then
  FAILED=1
  echo "failed to determine published host port"
  exit 1
fi

health_check() {
  printf 'fail\nsilent\nshow-error\nheader = "Authorization: Bearer %s"\nurl = "http://127.0.0.1:%s/health"\n' "$SMOKE_TOKEN" "$HOST_PORT" \
    | curl --config - >/dev/null 2>&1
}

echo "Waiting for PinchTab to become healthy on port $HOST_PORT..."
for _ in $(seq 1 60); do
  if health_check; then
    break
  fi
  sleep 1
done

if ! health_check; then
  FAILED=1
  echo "health check did not pass"
  exit 1
fi

bind_addr="$(docker exec "$NAME" pinchtab config get server.bind | tr -d '\r')"
if [ "$bind_addr" != "0.0.0.0" ]; then
  FAILED=1
  echo "unexpected server.bind: $bind_addr"
  exit 1
fi

config_path="$(docker exec "$NAME" pinchtab config path | tr -d '\r')"
if [ -z "$config_path" ]; then
  FAILED=1
  echo "failed to determine container config path"
  exit 1
fi

docker exec "$NAME" test -f "$config_path"

echo "Docker smoke test passed."
