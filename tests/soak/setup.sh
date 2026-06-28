#!/usr/bin/env bash
# Provision a clean PinchTab soak container: cross-compile the binary for the
# container arch, build the image, start the container under resource limits, and
# bring up a fixture server + a guards-down PinchTab server.
#
# Env (all optional):
#   CONTAINER   container name           (default: pinchtab-soak)
#   MEM_CAP     docker --memory          (default: 2g)
#   SHM         docker --shm-size        (default: 1g)
#   CPUS        docker --cpus            (default: 2)
#   MAX_TABS    instanceDefaults.maxTabs (default: 20 — raise to find the breaking point)
#   FIX_PORT    fixture http port        (default: 8088)
#   HEAVY_MB    approx size of heavy.html in MB (default: 5)
set -euo pipefail

CONTAINER="${CONTAINER:-pinchtab-soak}"
MEM_CAP="${MEM_CAP:-2g}"
SHM="${SHM:-1g}"
CPUS="${CPUS:-2}"
MAX_TABS="${MAX_TABS:-20}"
FIX_PORT="${FIX_PORT:-8088}"
HEAVY_MB="${HEAVY_MB:-5}"

REPO="$(git -C "$(dirname "${BASH_SOURCE[0]}")" rev-parse --show-toplevel)"
HERE="$REPO/tests/soak"
cd "$REPO"

case "$(uname -m)" in
  aarch64|arm64) GOARCH=arm64 ;;
  x86_64|amd64)  GOARCH=amd64 ;;
  *) echo "unsupported arch $(uname -m)"; exit 1 ;;
esac

echo "[soak] building linux/$GOARCH pinchtab binary..."
mkdir -p "$HERE/.bin"
CGO_ENABLED=0 GOOS=linux GOARCH="$GOARCH" go build -ldflags="-s -w" -o "$HERE/.bin/pinchtab" ./cmd/pinchtab

echo "[soak] building image..."
docker build -q -f "$HERE/Dockerfile" -t pinchtab-soak-img "$HERE" >/dev/null

echo "[soak] starting container '$CONTAINER' (mem=$MEM_CAP shm=$SHM cpus=$CPUS maxTabs=$MAX_TABS)..."
docker rm -f "$CONTAINER" >/dev/null 2>&1 || true
docker run -d --name "$CONTAINER" --shm-size="$SHM" --memory="$MEM_CAP" --cpus="$CPUS" \
  pinchtab-soak-img sleep infinity >/dev/null

docker cp "$HERE/.bin/pinchtab" "$CONTAINER:/usr/local/bin/pinchtab" >/dev/null
docker exec "$CONTAINER" chmod +x /usr/local/bin/pinchtab
docker cp "$REPO/tests/tools/fixtures" "$CONTAINER:/fixtures" >/dev/null

echo "[soak] generating heavy.html (~${HEAVY_MB}MB)..."
docker exec "$CONTAINER" python3 -c "
rows=int(${HEAVY_MB}*8000)
open('/fixtures/heavy.html','w').write('<html><body><table>'+''.join(f'<tr><td>{i}</td><td>lorem ipsum dolor sit amet {i}</td></tr>' for i in range(rows))+'</table>'+'<div>filler</div>'*int(${HEAVY_MB}*4000)+'</body></html>')
import os; print('  heavy.html', round(os.path.getsize('/fixtures/heavy.html')/1e6,1),'MB')
"

docker exec -d "$CONTAINER" sh -c "cd /fixtures && python3 -m http.server $FIX_PORT --bind 127.0.0.1"
sleep 1

# Guards-down so public + heavy pages load without the IDPI allowlist getting in the way.
docker exec "$CONTAINER" sh -c "
  pinchtab config init >/dev/null 2>&1
  pinchtab security down >/dev/null 2>&1
  pinchtab config set instanceDefaults.maxTabs $MAX_TABS >/dev/null 2>&1
  pinchtab config set security.allowedDomains 'localhost,127.0.0.1,::1,en.wikipedia.org,example.com,news.ycombinator.com' >/dev/null 2>&1
"
docker exec -d "$CONTAINER" sh -c 'pinchtab server -b >/tmp/srv.log 2>&1'
sleep 3

docker exec "$CONTAINER" sh -c "
  echo '[soak] ready:'
  pinchtab nav http://localhost:$FIX_PORT/heavy.html >/dev/null 2>&1 && echo '  heavy nav OK'
  echo \"  instance: \$(pinchtab instances 2>&1 | head -1)\"
  echo \"  maxTabs=\$(pinchtab config get instanceDefaults.maxTabs)  fixtures=http://localhost:$FIX_PORT\"
"
echo "[soak] container '$CONTAINER' ready. Run:  CONTAINER=$CONTAINER FIX_PORT=$FIX_PORT tests/soak/soak.sh 3600"
