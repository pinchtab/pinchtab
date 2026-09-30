#!/usr/bin/env bash
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
# shellcheck source=lib/smoke-common.sh
. "$SCRIPT_DIR/lib/smoke-common.sh"
# shellcheck source=lib/smoke-container.sh
. "$SCRIPT_DIR/lib/smoke-container.sh"
# shellcheck source=lib/smoke-health.sh
. "$SCRIPT_DIR/lib/smoke-health.sh"

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

smoke_container_trap "$NAME"
ensure_cft_image "$IMAGE"

# No host port mapping needed — all checks run via docker exec inside the container.
docker run -d \
  --platform linux/amd64 \
  --name "$NAME" \
  --shm-size=1g \
  "$IMAGE" \
  bash -lc "nc -lk 127.0.0.1 9868 >/dev/null 2>&1 & sleep 1; exec pinchtab server" >/dev/null

echo "Waiting for dashboard health..."
wait_for_exec_health "$NAME" "$TOKEN" 30

NC_PID="$(docker exec "$NAME" sh -lc "ps -eo pid,args | awk '/nc -lk 127.0.0.1 9868/ && !/awk/ {print \$1; exit}'" | tr -d '\r' | xargs)"
if [ -z "$NC_PID" ]; then
  fail "failed to locate the synthetic conflicting listener"
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
  cat "$RESPONSE_BODY" || true
  fail "expected HTTP 409 for explicit port conflict, got $HTTP_CODE"
fi

if ! grep -Fq "instance port 9868 is already in use by pid " "$RESPONSE_BODY"; then
  cat "$RESPONSE_BODY" || true
  fail "detailed port conflict message missing from response"
fi

if ! grep -Fq "for example: kill " "$RESPONSE_BODY"; then
  cat "$RESPONSE_BODY" || true
  fail "kill suggestion missing from response"
fi

rm -f "$RESPONSE_BODY"

echo "Port conflict smoke test passed."
