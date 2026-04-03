#!/bin/bash
# files-full.sh — CLI advanced file and capture scenarios.

GROUP_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
source "${GROUP_DIR}/../helpers/cli.sh"

# ─────────────────────────────────────────────────────────────────
start_test "pinchtab screenshot -o custom.jpg"

pt_ok nav "${FIXTURES_URL}/index.html"
pt_ok screenshot -o /tmp/e2e-custom-screenshot.jpg

if [ -f /tmp/e2e-custom-screenshot.jpg ]; then
  echo -e "  ${GREEN}✓${NC} file created"
  ((ASSERTIONS_PASSED++)) || true
  rm -f /tmp/e2e-custom-screenshot.jpg
else
  echo -e "  ${RED}✗${NC} file not created"
  ((ASSERTIONS_FAILED++)) || true
fi

end_test

# ─────────────────────────────────────────────────────────────────
start_test "pinchtab screenshot -q 10"

pt_ok screenshot -q 10 -o /tmp/e2e-lowq.jpg
rm -f /tmp/e2e-lowq.jpg

end_test

# ─────────────────────────────────────────────────────────────────
start_test "pinchtab pdf -o custom.pdf"

pt_ok nav "${FIXTURES_URL}/index.html"
pt_ok pdf -o /tmp/e2e-custom.pdf

if [ -f /tmp/e2e-custom.pdf ]; then
  echo -e "  ${GREEN}✓${NC} file created"
  ((ASSERTIONS_PASSED++)) || true
  rm -f /tmp/e2e-custom.pdf
else
  echo -e "  ${RED}✗${NC} file not created"
  ((ASSERTIONS_FAILED++)) || true
fi

end_test

# ─────────────────────────────────────────────────────────────────
start_test "pinchtab pdf --landscape"

pt_ok pdf --landscape -o /tmp/e2e-landscape.pdf
rm -f /tmp/e2e-landscape.pdf

end_test

# ─────────────────────────────────────────────────────────────────
start_test "pinchtab pdf --scale 0.5"

pt_ok pdf --scale 0.5 -o /tmp/e2e-scaled.pdf
rm -f /tmp/e2e-scaled.pdf

end_test

# ─────────────────────────────────────────────────────────────────
start_test "pinchtab download .gz file (gzip fallback)"

# Serve the fixture .gz file from a temporary public-facing server.
# In CI the FIXTURES_URL is blocked by SSRF guards, so we use a
# Python one-liner to serve the file on an ephemeral port.
GZ_PORT=0
GZ_PID=""
GZ_FIXTURE="${GROUP_DIR}/../fixtures/sitemap.xml.gz"

if [ -f "$GZ_FIXTURE" ]; then
  # Start a minimal HTTP server for the .gz file
  GZ_DIR=$(mktemp -d)
  cp "$GZ_FIXTURE" "$GZ_DIR/sitemap.xml.gz"

  python3 -m http.server 0 --directory "$GZ_DIR" --bind 0.0.0.0 &>/tmp/e2e-gz-server.log &
  GZ_PID=$!

  # Poll for server startup with timeout (handles slow CI)
  GZ_PORT=""
  for i in {1..10}; do
    GZ_PORT=$(grep -oE 'port [0-9]+' /tmp/e2e-gz-server.log 2>/dev/null | head -1 | grep -oE '[0-9]+' || true)
    if [ -n "$GZ_PORT" ] && [ "$GZ_PORT" != "0" ]; then
      break
    fi
    sleep 0.5
  done

  if [ -n "$GZ_PORT" ] && [ "$GZ_PORT" != "0" ]; then
    # The server binds 0.0.0.0, access via the host the pinchtab instance can reach.
    # In Docker, this is typically the host gateway or the container hostname.
    GZ_HOST="${E2E_GZ_HOST:-host.docker.internal}"
    GZ_URL="http://${GZ_HOST}:${GZ_PORT}/sitemap.xml.gz"

    pt_ok download "$GZ_URL" -o /tmp/e2e-download-gz.xml
    if [ -f /tmp/e2e-download-gz.xml ]; then
      if grep -q "example.com" /tmp/e2e-download-gz.xml; then
        echo -e "  ${GREEN}✓${NC} .gz file downloaded and decompressed"
        ((ASSERTIONS_PASSED++)) || true
      else
        echo -e "  ${RED}✗${NC} file downloaded but content not decompressed"
        ((ASSERTIONS_FAILED++)) || true
      fi
      rm -f /tmp/e2e-download-gz.xml
    else
      echo -e "  ${RED}✗${NC} .gz download file not created"
      ((ASSERTIONS_FAILED++)) || true
    fi
  else
    echo -e "  ${YELLOW}⚠${NC} could not start gz fixture server, skipping"
  fi

  [ -n "$GZ_PID" ] && kill "$GZ_PID" 2>/dev/null
  rm -rf "$GZ_DIR" /tmp/e2e-gz-server.log
else
  echo -e "  ${YELLOW}⚠${NC} gz fixture not found, skipping"
fi

end_test
