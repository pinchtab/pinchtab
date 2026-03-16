#!/bin/bash
# 20-auto-https.sh — Test auto-https prefix for CLI URL arguments
# Verifies that CLI commands automatically prepend https:// to URLs without protocol

source "$(dirname "$0")/common.sh"

# ─────────────────────────────────────────────────────────────────
start_test "auto-https: goto without protocol adds https://"

# Navigate to fixture hostname without protocol
# Since fixtures are HTTP-only, https:// should fail with connection error
# This proves the CLI added https:// prefix
pt goto "fixtures:80/index.html"

# Should see an error about https/SSL/TLS since fixture doesn't support it
if echo "$PT_ERR" | grep -qiE "https|ssl|tls|certificate|refused|failed"; then
  echo -e "  ${GREEN}✓${NC} CLI added https:// prefix (got expected SSL/connection error)"
  ((ASSERTIONS_PASSED++)) || true
else
  echo -e "  ${RED}✗${NC} Expected https error, got: $PT_ERR"
  ((ASSERTIONS_FAILED++)) || true
fi

end_test

# ─────────────────────────────────────────────────────────────────
start_test "auto-https: explicit http:// is preserved"

# Navigate with explicit http:// - should work
pt_ok goto "http://fixtures:80/index.html"

end_test

# ─────────────────────────────────────────────────────────────────
start_test "auto-https: explicit https:// is preserved"

# Navigate with explicit https:// to http-only fixture - should fail with SSL error
pt goto "https://fixtures:80/index.html"

if echo "$PT_ERR" | grep -qiE "https|ssl|tls|certificate|refused|failed"; then
  echo -e "  ${GREEN}✓${NC} Explicit https:// preserved (got expected SSL error)"
  ((ASSERTIONS_PASSED++)) || true
else
  echo -e "  ${RED}✗${NC} Expected https error, got: $PT_ERR"
  ((ASSERTIONS_FAILED++)) || true
fi

end_test
