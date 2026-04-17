#!/bin/bash
# files-extended.sh — CLI advanced file and capture scenarios.

GROUP_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
source "${GROUP_DIR}/../../helpers/cli.sh"

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

# fixtures domain allowlisted in e2e config for this test
GZ_URL="${FIXTURES_URL}/sitemap.xml.gz"

pt_ok download "$GZ_URL" -o /tmp/e2e-download-gz.xml
assert_file_exists /tmp/e2e-download-gz.xml ".gz download file created"

if [ -f /tmp/e2e-download-gz.xml ]; then
  PT_OUT=$(cat /tmp/e2e-download-gz.xml)
  assert_output_contains "example.com" ".gz file decompressed correctly"
fi
rm -f /tmp/e2e-download-gz.xml

end_test
