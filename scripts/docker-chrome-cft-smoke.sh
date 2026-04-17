#!/usr/bin/env bash
set -euo pipefail

IMAGE="${1:-pinchtab-chrome-cft-smoke:${RANDOM}${RANDOM}}"
NAME="pinchtab-chrome-cft-smoke-${RANDOM}${RANDOM}"
TOKEN="chrome-cft-smoke-token"
FAILED=0

cleanup() {
  if docker ps -a --format '{{.Names}}' | grep -Fxq "$NAME"; then
    if [ "$FAILED" -ne 0 ]; then
      echo ""
      echo "Container logs:"
      docker logs "$NAME" || true
      echo ""
      echo "Chrome processes:"
      docker exec "$NAME" ps -eo pid,args || true
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
  "$IMAGE" >/dev/null

HOST_PORT="$(docker port "$NAME" 9867/tcp | head -1 | awk -F: '{print $NF}')"
if [ -z "$HOST_PORT" ]; then
  FAILED=1
  echo "failed to determine published host port"
  exit 1
fi

if docker exec "$NAME" sh -lc 'test -z "${DISPLAY:-}"'; then
  echo "Confirmed DISPLAY is unset inside the container."
else
  FAILED=1
  echo "expected DISPLAY to be unset inside the container"
  exit 1
fi

HEALTH_BODY=""
HEALTH_CODE=""

fetch_health() {
  HEALTH_BODY="$(mktemp)"
  HEALTH_CODE="$(curl -sS -o "$HEALTH_BODY" -w '%{http_code}' -H "Authorization: Bearer ${TOKEN}" "http://127.0.0.1:${HOST_PORT}/health" || true)"
}

echo "Waiting for PinchTab to report healthy with Chrome for Testing on port $HOST_PORT..."
for _ in $(seq 1 90); do
  fetch_health
  if [ "$HEALTH_CODE" = "200" ] && grep -q '"status":"ok"' "$HEALTH_BODY"; then
    break
  fi
  rm -f "$HEALTH_BODY"
  sleep 1
done

fetch_health
if [ "$HEALTH_CODE" != "200" ] || ! grep -q '"status":"ok"' "$HEALTH_BODY"; then
  FAILED=1
  echo "health check did not pass"
  echo "HTTP $HEALTH_CODE"
  cat "$HEALTH_BODY" || true
  rm -f "$HEALTH_BODY"
  exit 1
fi
rm -f "$HEALTH_BODY"

echo "Ubuntu Chrome for Testing smoke test passed."
