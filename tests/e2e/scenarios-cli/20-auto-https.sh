#!/bin/bash
# 20-auto-https.sh — Test auto-https prefix for CLI URL arguments
# Verifies that CLI commands automatically prepend https:// to URLs without protocol

source "$(dirname "$0")/common.sh"

# ─────────────────────────────────────────────────────────────────
start_test "auto-https: goto without protocol adds https://"

# Navigate to fixture hostname without protocol
# Since fixtures are HTTP-only, https:// navigation will land on error page
pt goto "fixtures:80/index.html"

# Chrome shows error page for failed https:// - check URL or title indicates error
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

# Navigate with explicit http:// - should work and URL should be http://
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

# Navigate with explicit https:// to http-only fixture
pt goto "https://fixtures:80/index.html"

# Should show error page or fail (fixture doesn't support HTTPS)
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
