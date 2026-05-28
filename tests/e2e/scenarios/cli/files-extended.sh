#!/bin/bash
# files-extended.sh — CLI advanced file and capture scenarios.

GROUP_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
source "${GROUP_DIR}/../../helpers/cli.sh"

open_capture_tab() {
  CAPTURE_TAB_ID=""
  pt_ok nav --new-tab "${FIXTURES_URL}/index.html"
  CAPTURE_TAB_ID=$(echo "$PT_OUT" | tr -d '[:space:]')
  if [ -n "$CAPTURE_TAB_ID" ]; then
    pt_ok tab "$CAPTURE_TAB_ID"
    pt_ok wait "body" --tab "$CAPTURE_TAB_ID" --timeout 5000
  else
    fail_assert "capture tab id available"
  fi
}

close_capture_tab() {
  if [ -n "${CAPTURE_TAB_ID:-}" ]; then
    pt tab close "$CAPTURE_TAB_ID" > /dev/null 2>&1 || true
    CAPTURE_TAB_ID=""
  fi
}

# ─────────────────────────────────────────────────────────────────
start_test "pinchtab screenshot -o custom.jpg"

rm -f /tmp/e2e-custom-screenshot.jpg
open_capture_tab
if [ -n "${CAPTURE_TAB_ID:-}" ]; then
  pt_ok screenshot --tab "$CAPTURE_TAB_ID" -o /tmp/e2e-custom-screenshot.jpg
fi

if [ -f /tmp/e2e-custom-screenshot.jpg ]; then
  echo -e "  ${GREEN}✓${NC} file created"
  ((ASSERTIONS_PASSED++)) || true
  rm -f /tmp/e2e-custom-screenshot.jpg
else
  echo -e "  ${RED}✗${NC} file not created"
  ((ASSERTIONS_FAILED++)) || true
fi
close_capture_tab

end_test

# ─────────────────────────────────────────────────────────────────
start_test "pinchtab screenshot -q 10"

rm -f /tmp/e2e-lowq.jpg
open_capture_tab
if [ -n "${CAPTURE_TAB_ID:-}" ]; then
  pt_ok screenshot --tab "$CAPTURE_TAB_ID" -q 10 -o /tmp/e2e-lowq.jpg
fi
assert_file_exists /tmp/e2e-lowq.jpg "low quality screenshot file created"
rm -f /tmp/e2e-lowq.jpg
close_capture_tab

end_test

# ─────────────────────────────────────────────────────────────────
start_test "pinchtab screenshot --beyond-viewport"

# Captures the full scrollable document. The fixture (tall.html) is
# deliberately ~4000px tall so the file is unambiguously larger than the
# viewport-only capture from the previous tests.
rm -f /tmp/e2e-beyond.jpg
pt_ok nav "${FIXTURES_URL}/tall.html"
pt_ok screenshot --beyond-viewport -o /tmp/e2e-beyond.jpg
assert_file_exists /tmp/e2e-beyond.jpg "beyond-viewport screenshot file created"
rm -f /tmp/e2e-beyond.jpg

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
