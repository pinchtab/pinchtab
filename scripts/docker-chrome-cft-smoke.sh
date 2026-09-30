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

smoke_container_trap "$NAME"
ensure_cft_image "$IMAGE"

# No host port mapping needed — all checks run via docker exec inside the container.
docker run -d \
  --platform linux/amd64 \
  --name "$NAME" \
  --shm-size=1g \
  "$IMAGE" >/dev/null

if docker exec "$NAME" sh -lc 'test -z "${DISPLAY:-}"'; then
  echo "Confirmed DISPLAY is unset inside the container."
else
  fail "expected DISPLAY to be unset inside the container"
fi

echo "Waiting for PinchTab to report healthy with Chrome for Testing..."
wait_for_exec_health "$NAME" "$TOKEN" 90

echo "Ubuntu Chrome for Testing smoke test passed."
