#!/usr/bin/env bash
set -euo pipefail

IMAGE="${1:-pinchtab-local:test}"

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

docker run -d --name "$NAME" -p 127.0.0.1::9867 "$IMAGE" >/dev/null

HOST_PORT="$(docker port "$NAME" 9867/tcp | head -1 | awk -F: '{print $NF}')"
if [ -z "$HOST_PORT" ]; then
  FAILED=1
  echo "failed to determine published host port"
  exit 1
fi

AUTH_HEADER=()
TOKEN="$(docker exec "$NAME" pinchtab config get server.token | tr -d '\r')"
if [ -n "$TOKEN" ]; then
  AUTH_HEADER=(-H "Authorization: Bearer ${TOKEN}")
fi

echo "Waiting for PinchTab to become healthy on port $HOST_PORT..."
for _ in $(seq 1 60); do
  if curl -fsS "${AUTH_HEADER[@]}" "http://127.0.0.1:${HOST_PORT}/health" >/dev/null 2>&1; then
    break
  fi
  sleep 1
done

if ! curl -fsS "${AUTH_HEADER[@]}" "http://127.0.0.1:${HOST_PORT}/health" >/dev/null 2>&1; then
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

extra_flags="$(docker exec "$NAME" pinchtab config get browser.extraFlags | tr -d '\r')"
if [ "$extra_flags" != "--no-sandbox --disable-gpu" ]; then
  FAILED=1
  echo "unexpected browser.extraFlags: $extra_flags"
  exit 1
fi

docker exec "$NAME" test -f /data/.config/pinchtab/config.json

echo "Docker smoke test passed."
