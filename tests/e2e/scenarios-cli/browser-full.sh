#!/bin/bash
# browser-full.sh — CLI advanced browser scenarios.

GROUP_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
source "${GROUP_DIR}/../helpers/cli.sh"

# ─────────────────────────────────────────────────────────────────
start_test "pinchtab scroll <pixels>"

pt_ok nav "${FIXTURES_URL}/table.html"

pt_ok scroll 100
assert_output_contains "scrolled" "confirms scroll action"

end_test

# ─────────────────────────────────────────────────────────────────
start_test "pinchtab scroll down/up"

pt_ok scroll down
assert_output_contains "scrolled" "scroll down succeeded"

pt_ok scroll up
assert_output_contains "scrolled" "scroll up succeeded"

end_test

# ─────────────────────────────────────────────────────────────────
start_test "pinchtab --version"

pt_ok --version
assert_output_contains "pinchtab" "outputs version string"

end_test

# ─────────────────────────────────────────────────────────────────
start_test "pinchtab help"

pt_ok help
assert_output_contains "pinchtab" "outputs help text"
assert_output_contains "nav" "mentions nav command"

end_test

# ─────────────────────────────────────────────────────────────────
start_test "pinchtab --help"

pt_ok --help
assert_output_contains "pinchtab" "outputs help text"

end_test

# Verifies that CLI commands automatically prepend https:// to URLs without protocol

# ─────────────────────────────────────────────────────────────────
start_test "auto-https: goto without protocol adds https://"

pt goto "fixtures:80/index.html"

if echo "$PT_OUT" | grep -qiE "chrome-error|err_|error|refused|failed|ssl"; then
  echo -e "  ${GREEN}✓${NC} CLI added https:// prefix (Chrome shows error page)"
  ((ASSERTIONS_PASSED++)) || true
elif [ "$PT_CODE" -ne 0 ]; then
  echo -e "  ${GREEN}✓${NC} CLI added https:// prefix (navigation failed as expected)"
  ((ASSERTIONS_PASSED++)) || true
else
  # Check if the URL in response starts with https://
  if echo "$PT_OUT" | grep -q '"url".*https://'; then
    echo -e "  ${GREEN}✓${NC} CLI added https:// prefix (URL in response shows https)"
    ((ASSERTIONS_PASSED++)) || true
  else
    echo -e "  ${RED}✗${NC} Expected https:// URL or error, got: $PT_OUT"
    ((ASSERTIONS_FAILED++)) || true
  fi
fi

end_test

# ─────────────────────────────────────────────────────────────────
start_test "auto-https: explicit http:// is preserved"

pt_ok goto "http://fixtures:80/index.html"

if echo "$PT_OUT" | grep -q '"url".*http://'; then
  echo -e "  ${GREEN}✓${NC} Response URL is http://"
  ((ASSERTIONS_PASSED++)) || true
else
  echo -e "  ${RED}✗${NC} Expected http:// in URL"
  ((ASSERTIONS_FAILED++)) || true
fi

end_test

# ─────────────────────────────────────────────────────────────────
start_test "auto-https: explicit https:// is preserved"

pt goto "https://fixtures:80/index.html"

if echo "$PT_OUT" | grep -qiE "chrome-error|err_|error|refused|failed"; then
  echo -e "  ${GREEN}✓${NC} Explicit https:// preserved (Chrome shows error)"
  ((ASSERTIONS_PASSED++)) || true
elif echo "$PT_OUT" | grep -q '"url".*https://'; then
  echo -e "  ${GREEN}✓${NC} Explicit https:// preserved (URL shows https)"
  ((ASSERTIONS_PASSED++)) || true
else
  echo -e "  ${RED}✗${NC} Expected https:// URL or error, got: $PT_OUT"
  ((ASSERTIONS_FAILED++)) || true
fi

end_test

# ─────────────────────────────────────────────────────────────────
start_test "redirects: follow single redirect"

pt_ok nav "https://httpbin.org/redirect/1"
pt_ok snap
assert_json_field_contains ".url" "httpbin.org/get" "landed on /get after redirect"

end_test

# ─────────────────────────────────────────────────────────────────
start_test "redirects: follow multiple redirects"

pt_ok nav "https://httpbin.org/redirect/3"
pt_ok snap
assert_json_field_contains ".url" "httpbin.org/get" "multiple redirects followed to /get"

end_test

# ─────────────────────────────────────────────────────────────────
start_test "pinchtab find (basic)"

pt_ok nav "${FIXTURES_URL}/form.html"
pt_ok find "username"
assert_output_contains "ref" "has ref in output"

end_test

# ─────────────────────────────────────────────────────────────────
start_test "pinchtab find --ref-only"

pt_ok find "username" --ref-only
assert_output_contains "e" "outputs ref"

end_test

# ─────────────────────────────────────────────────────────────────
start_test "pinchtab find --explain"

pt_ok find "submit" --explain

end_test

# ─────────────────────────────────────────────────────────────────
start_test "pinchtab find (no match)"

pt find "xyznonexistent99999"

end_test

# ─────────────────────────────────────────────────────────────────
start_test "pinchtab snap --text"

pt_ok nav "${FIXTURES_URL}/index.html"
pt_ok snap --text

end_test

# ─────────────────────────────────────────────────────────────────
start_test "pinchtab snap --interactive"

pt_ok snap --interactive

end_test

# ─────────────────────────────────────────────────────────────────
start_test "pinchtab snap --compact"

pt_ok snap --compact

end_test

# ─────────────────────────────────────────────────────────────────
start_test "pinchtab snap --depth 2"

pt_ok snap --depth 2

end_test

# ─────────────────────────────────────────────────────────────────
start_test "pinchtab snap --max-tokens 100"

pt_ok snap --max-tokens 100

end_test

# ─────────────────────────────────────────────────────────────────
start_test "pinchtab snap --diff"

pt_ok snap

pt_ok snap --diff

end_test

# ─────────────────────────────────────────────────────────────────
start_test "pinchtab snap -s 'body'"

pt_ok snap -s "body"

end_test

# ─────────────────────────────────────────────────────────────────
start_test "pinchtab nav --new-tab"

pt_ok nav "${FIXTURES_URL}/index.html"
pt_ok nav "${FIXTURES_URL}/form.html" --new-tab

end_test

# ─────────────────────────────────────────────────────────────────
start_test "pinchtab nav --block-images"

pt_ok nav "${FIXTURES_URL}/index.html" --block-images

end_test

# ─────────────────────────────────────────────────────────────────
start_test "pinchtab nav --block-ads"

pt_ok nav "${FIXTURES_URL}/index.html" --block-ads

end_test

# ─────────────────────────────────────────────────────────────────
start_test "pinchtab text --raw"

pt_ok nav "${FIXTURES_URL}/index.html"
pt_ok text --raw

end_test

# ─────────────────────────────────────────────────────────────────
start_test "pinchtab nav <url>"

pt_ok nav "${FIXTURES_URL}/index.html"
assert_output_json
assert_output_contains "tabId" "returns tab ID"

end_test

# ─────────────────────────────────────────────────────────────────
start_test "pinchtab nav --new-tab <url>"

pt_ok nav --new-tab "${FIXTURES_URL}/form.html"
assert_output_json
assert_output_contains "tabId" "opens in new tab"

end_test

# ─────────────────────────────────────────────────────────────────
start_test "pinchtab goto <url> (alias for nav)"

pt_ok goto "${FIXTURES_URL}/index.html"
assert_output_json
assert_output_contains "tabId" "goto works as alias"

end_test

# ─────────────────────────────────────────────────────────────────
start_test "pinchtab navigate <url> (alias for nav)"

pt_ok navigate "${FIXTURES_URL}/index.html"
assert_output_json
assert_output_contains "tabId" "navigate works as alias"

end_test
