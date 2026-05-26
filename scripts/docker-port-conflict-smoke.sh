#!/usr/bin/env bash
set -euo pipefail

IMAGE="${1:-pinchtab-chrome-cft-smoke:${RANDOM}${RANDOM}}"
NAME="pinchtab-port-conflict-smoke-${RANDOM}${RANDOM}"
TOKEN="chrome-cft-smoke-token"
FAILED=0

# Chrome for Testing (linux/amd64) does not run reliably under Rosetta on Apple Silicon.
# Skip on non-x86_64 hosts; CI (linux/amd64) runs the full test.
ARCH="$(uname -m)"
if [ "$ARCH" != "x86_64" ]; then
  echo "Skipping port conflict smoke test on $ARCH host (requires x86_64)."
  exit 0
fi

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
trap 'rc=$?; [ "$rc" -ne 0 ] && FAILED=1; cleanup' EXIT

if docker image inspect "$IMAGE" >/dev/null 2>&1; then
  echo "Using existing Ubuntu + Chrome for Testing smoke image: $IMAGE"
else
  echo "Building Ubuntu + Chrome for Testing smoke image..."
  docker build \
    --platform linux/amd64 \
    -f tests/tools/docker/chrome-cft-smoke.Dockerfile \
    -t "$IMAGE" \
    .
fi

# No host port mapping needed — all checks run via docker exec inside the container.
docker run -d \
  --platform linux/amd64 \
  --name "$NAME" \
  --shm-size=1g \
  "$IMAGE" \
  bash -lc "nc -lk 127.0.0.1 9868 >/dev/null 2>&1 & sleep 1; exec pinchtab server" >/dev/null

health_check() {
  docker exec "$NAME" sh -c \
    "curl -sf -H 'Authorization: Bearer ${TOKEN}' http://127.0.0.1:9867/health" \
    >/dev/null 2>&1
}

echo "Waiting for dashboard health..."
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

HTTP_CODE="$(docker exec "$NAME" sh -c \
  "curl -sS -o /tmp/conflict_response -w '%{http_code}' \
    -H 'Authorization: Bearer ${TOKEN}' \
    -H 'Content-Type: application/json' \
    -X POST \
    -d '{\"port\":\"9868\"}' \
    http://127.0.0.1:9867/instances/start" | tr -d '\r')"
RESPONSE_BODY="$(mktemp)"
docker cp "$NAME":/tmp/conflict_response "$RESPONSE_BODY" 2>/dev/null || true

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

FAILED=0
echo "Port conflict smoke test passed."
