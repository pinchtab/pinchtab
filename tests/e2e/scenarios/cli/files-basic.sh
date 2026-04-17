#!/bin/bash
# files-basic.sh — CLI happy-path file and capture scenarios.

GROUP_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
source "${GROUP_DIR}/../../helpers/cli.sh"

start_test "pinchtab screenshot"
pt_ok nav "${FIXTURES_URL}/index.html"
pt_ok screenshot -o /tmp/e2e-screenshot-test.jpg
if [ -f /tmp/e2e-screenshot-test.jpg ]; then
  echo -e "  ${GREEN}✓${NC} screenshot file created"
  ((ASSERTIONS_PASSED++)) || true
  rm -f /tmp/e2e-screenshot-test.jpg
else
  echo -e "  ${RED}✗${NC} screenshot file not created"
  ((ASSERTIONS_FAILED++)) || true
fi
end_test

start_test "pinchtab pdf"
pt_ok nav "${FIXTURES_URL}/index.html"
pt_ok pdf -o /tmp/e2e-pdf-test.pdf
if [ -f /tmp/e2e-pdf-test.pdf ]; then
  echo -e "  ${GREEN}✓${NC} PDF file created"
  ((ASSERTIONS_PASSED++)) || true
  rm -f /tmp/e2e-pdf-test.pdf
else
  echo -e "  ${RED}✗${NC} PDF file not created"
  ((ASSERTIONS_FAILED++)) || true
fi
end_test

# ─────────────────────────────────────────────────────────────────
start_test "pinchtab download (rejects non-allowed domain)"

pt_fail download "http://not-on-allowlist.local/index.html"

end_test

# ─────────────────────────────────────────────────────────────────
start_test "pinchtab download (public URL)"

pt_ok download "${FIXTURES_URL}/sample.txt"
assert_output_contains "data" "response contains download data"

end_test

# ─────────────────────────────────────────────────────────────────
start_test "pinchtab download (save to file)"

pt_ok download "${FIXTURES_URL}/sample.txt" -o /tmp/e2e-download-test.txt
if [ -f /tmp/e2e-download-test.txt ]; then
  echo -e "  ${GREEN}✓${NC} file saved"
  ((ASSERTIONS_PASSED++)) || true
else
  echo -e "  ${RED}✗${NC} file not saved"
  ((ASSERTIONS_FAILED++)) || true
fi
rm -f /tmp/e2e-download-test.txt

end_test

# ─────────────────────────────────────────────────────────────────
start_test "pinchtab upload (basic)"

pt_ok nav "${FIXTURES_URL}/upload.html"

echo "test content" > /tmp/e2e-upload-test.txt
pt_ok upload /tmp/e2e-upload-test.txt --selector "#single-file"
assert_output_contains "ok" "upload succeeded"
rm -f /tmp/e2e-upload-test.txt

end_test
