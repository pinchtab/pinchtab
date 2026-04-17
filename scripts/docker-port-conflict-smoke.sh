#!/usr/bin/env bash
set -euo pipefail

IMAGE="${1:-pinchtab-chrome-cft-smoke:${RANDOM}${RANDOM}}"
NAME="pinchtab-port-conflict-smoke-${RANDOM}${RANDOM}"
TOKEN="chrome-cft-smoke-token"
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

echo "Building Ubuntu + Chrome for Testing smoke image..."
docker build \
  --platform linux/amd64 \
  -f tests/docker/chrome-cft-smoke.Dockerfile \
  -t "$IMAGE" \
  .

docker run -d \
  --platform linux/amd64 \
  --name "$NAME" \
  --shm-size=1g \
  -p 127.0.0.1::9867 \
  "$IMAGE" \
  bash -lc "nc -lk 127.0.0.1 9868 >/dev/null 2>&1 & sleep 1; exec pinchtab server" >/dev/null

HOST_PORT="$(docker port "$NAME" 9867/tcp | head -1 | awk -F: '{print $NF}')"
if [ -z "$HOST_PORT" ]; then
  FAILED=1
  echo "failed to determine published host port"
  exit 1
fi

health_check() {
  curl -sS -o /dev/null -H "Authorization: Bearer ${TOKEN}" "http://127.0.0.1:${HOST_PORT}/health"
}

echo "Waiting for dashboard health on port $HOST_PORT..."
for _ in $(seq 1 30); do
  if health_check; then
    break
  fi
  sleep 1
done

if ! health_check; then
  FAILED=1
  echo "dashboard health check did not pass"
  exit 1
fi

NC_PID="$(docker exec "$NAME" sh -lc "ps -eo pid,args | awk '/nc -lk 127.0.0.1 9868/ && !/awk/ {print \$1; exit}'" | tr -d '\r' | xargs)"
if [ -z "$NC_PID" ]; then
  FAILED=1
  echo "failed to locate the synthetic conflicting listener"
  exit 1
fi

RESPONSE_BODY="$(mktemp)"
HTTP_CODE="$(curl -sS -o "$RESPONSE_BODY" -w '%{http_code}' \
  -H "Authorization: Bearer ${TOKEN}" \
  -H "Content-Type: application/json" \
  -X POST \
  -d '{"port":"9868"}' \
  "http://127.0.0.1:${HOST_PORT}/instances/start")"

if [ "$HTTP_CODE" != "409" ]; then
  FAILED=1
  echo "expected HTTP 409 for explicit port conflict, got $HTTP_CODE"
  cat "$RESPONSE_BODY" || true
  exit 1
fi

if ! grep -Fq "instance port 9868 is already in use by pid " "$RESPONSE_BODY"; then
  FAILED=1
  echo "detailed port conflict message missing from response"
  cat "$RESPONSE_BODY" || true
  exit 1
fi

if ! grep -Fq "for example: kill " "$RESPONSE_BODY"; then
  FAILED=1
  echo "kill suggestion missing from response"
  cat "$RESPONSE_BODY" || true
  exit 1
fi

rm -f "$RESPONSE_BODY"

echo "Port conflict smoke test passed."
