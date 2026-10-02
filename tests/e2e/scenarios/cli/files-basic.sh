#!/bin/bash
# files-basic.sh — CLI happy-path file and capture scenarios.

GROUP_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
source "${GROUP_DIR}/../../helpers/cli.sh"

start_test "pinchtab screenshot"

FIRST_SCREENSHOT=/tmp/e2e-screenshot-first.png
SECOND_SCREENSHOT=/tmp/e2e-screenshot-second.png
rm -f "$FIRST_SCREENSHOT" "$SECOND_SCREENSHOT"

pt_ok nav "${FIXTURES_URL}/index.html"
pt_ok screenshot -o "$FIRST_SCREENSHOT"
pt_ok nav "${FIXTURES_URL}/buttons.html"
pt_ok screenshot -o "$SECOND_SCREENSHOT"

assert_file_exists "$FIRST_SCREENSHOT" "first screenshot file created"
assert_file_exists "$SECOND_SCREENSHOT" "second screenshot file created"

PNG_MAGIC=""
if [ -f "$FIRST_SCREENSHOT" ]; then
  PNG_MAGIC=$(od -An -tx1 -N8 "$FIRST_SCREENSHOT" | tr -d ' \n')
fi
if [ "$PNG_MAGIC" = "89504e470d0a1a0a" ]; then
  pass_assert ".png output contains PNG bytes"
else
  fail_assert ".png output contains PNG bytes"
fi

if [ ! -f "$FIRST_SCREENSHOT" ] || [ ! -f "$SECOND_SCREENSHOT" ]; then
  fail_assert "different pages produce different viewport screenshots"
elif cmp -s "$FIRST_SCREENSHOT" "$SECOND_SCREENSHOT"; then
  fail_assert "different pages produce different viewport screenshots"
else
  pass_assert "different pages produce different viewport screenshots"
fi

rm -f "$FIRST_SCREENSHOT" "$SECOND_SCREENSHOT"
end_test

start_test "pinchtab pdf"
pt_ok nav "${FIXTURES_URL}/index.html"
pt_ok pdf -o /tmp/e2e-pdf-test.pdf
if [ -f /tmp/e2e-pdf-test.pdf ]; then
  pass_assert "PDF file created"
  rm -f /tmp/e2e-pdf-test.pdf
else
  fail_assert "PDF file not created"
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
  pass_assert "file saved"
else
  fail_assert "file not saved"
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
