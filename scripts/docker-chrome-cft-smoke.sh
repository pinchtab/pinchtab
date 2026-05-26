#!/usr/bin/env bash
set -euo pipefail

IMAGE="${1:-pinchtab-chrome-cft-smoke:${RANDOM}${RANDOM}}"
NAME="pinchtab-chrome-cft-smoke-${RANDOM}${RANDOM}"
TOKEN="chrome-cft-smoke-token"
FAILED=0

# Chrome for Testing (linux/amd64) does not run reliably under Rosetta on Apple Silicon.
# Skip on non-x86_64 hosts; CI (linux/amd64) runs the full test.
ARCH="$(uname -m)"
if [ "$ARCH" != "x86_64" ]; then
  echo "Skipping Chrome for Testing startup test on $ARCH host (requires x86_64)."
  exit 0
fi

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
  "$IMAGE" >/dev/null

if docker exec "$NAME" sh -lc 'test -z "${DISPLAY:-}"'; then
  echo "Confirmed DISPLAY is unset inside the container."
else
  echo "expected DISPLAY to be unset inside the container"
  exit 1
fi

health_check() {
  docker exec "$NAME" sh -c \
    "curl -sf -H 'Authorization: Bearer ${TOKEN}' http://127.0.0.1:9867/health" \
    >/dev/null 2>&1
}

echo "Waiting for PinchTab to report healthy with Chrome for Testing..."
for _ in $(seq 1 90); do
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

FAILED=0
echo "Ubuntu Chrome for Testing smoke test passed."
