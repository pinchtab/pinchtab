#!/usr/bin/env bash
# Provision one clean container per onboarding persona, each with its starting
# condition. Then drive each with a blind agent using subagent-context.md.
set -euo pipefail

REPO="$(git -C "$(dirname "${BASH_SOURCE[0]}")" rev-parse --show-toplevel)"
HERE="$REPO/tests/install-ux"
cd "$REPO"

case "$(uname -m)" in
  aarch64|arm64) GOARCH=arm64 ;;
  x86_64|amd64)  GOARCH=amd64 ;;
  *) echo "unsupported arch $(uname -m)"; exit 1 ;;
esac

echo "[ux] building linux/$GOARCH pinchtab binary..."
mkdir -p "$HERE/.bin"
CGO_ENABLED=0 GOOS=linux GOARCH="$GOARCH" go build -ldflags="-s -w" -o "$HERE/.bin/pinchtab" ./cmd/pinchtab

echo "[ux] building images..."
docker build -q -f "$REPO/tests/soak/Dockerfile" -t pinchtab-ux-browser "$REPO/tests/soak" >/dev/null  # chromium + curl
docker build -q -f "$HERE/Dockerfile.base" -t pinchtab-ux-base "$HERE" >/dev/null                       # no browser

mk(){ # name image
  docker rm -f "$1" >/dev/null 2>&1 || true
  docker run -d --name "$1" --shm-size=512m "$2" sleep infinity >/dev/null
  docker cp "$HERE/.bin/pinchtab" "$1:/usr/local/bin/pinchtab" >/dev/null
  docker exec "$1" chmod +x /usr/local/bin/pinchtab
}

mk ptux-sa pinchtab-ux-browser   # impatient first-timer, browser present
mk ptux-sb pinchtab-ux-base      # backend dev, NO browser
mk ptux-sc pinchtab-ux-browser   # privacy researcher, browser present
mk ptux-sd pinchtab-ux-base      # devops/CI, minimal, NO browser
mk ptux-se pinchtab-ux-browser   # mis-set browser.binary
mk ptux-sf pinchtab-ux-browser   # API integrator (curl present)

# S-E: pre-set a bogus browser.binary in the DEFAULT config the agent will read.
docker exec ptux-se sh -c 'pinchtab config init >/dev/null 2>&1 && pinchtab config set browser.binary /opt/not-a-real-chrome >/dev/null 2>&1'

echo "[ux] persona containers ready:"
for c in ptux-sa ptux-sb ptux-sc ptux-sd ptux-se ptux-sf; do
  br=$(docker exec "$c" sh -c 'command -v chromium >/dev/null && echo chromium || echo NONE')
  printf "  %-9s browser=%s\n" "$c" "$br"
done
echo "[ux] now launch a blind agent per persona using tests/install-ux/subagent-context.md"
