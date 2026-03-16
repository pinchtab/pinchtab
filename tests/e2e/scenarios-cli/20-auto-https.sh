#!/bin/bash
# 20-auto-https.sh — Test auto-https prefix for CLI URL arguments
# Verifies that CLI commands automatically prepend https:// to URLs without protocol

source "$(dirname "$0")/common.sh"

# ─────────────────────────────────────────────────────────────────
start_test "auto-https: goto without protocol adds https://"

# Navigate to fixture hostname without protocol
# Since fixtures are HTTP-only, https:// should fail with connection error
# This proves the CLI added https:// prefix
OUTPUT=$(pinchtab goto "fixtures:80/index.html" 2>&1 || true)

# Should see an error about https/SSL/TLS since fixture doesn't support it
if echo "$OUTPUT" | grep -qiE "https|ssl|tls|certificate|connection refused"; then
  echo -e "  ${GREEN}✓${NC} CLI added https:// prefix (got expected SSL/connection error)"
  ((ASSERTIONS_PASSED++)) || true
else
  # If it somehow succeeded or got a different error, check what happened
  echo -e "  ${RED}✗${NC} Expected https error, got: $OUTPUT"
  ((ASSERTIONS_FAILED++)) || true
fi

end_test

# ─────────────────────────────────────────────────────────────────
start_test "auto-https: explicit http:// is preserved"

# Navigate with explicit http:// - should work
pinchtab goto "http://fixtures:80/index.html"
if [ $? -eq 0 ]; then
  echo -e "  ${GREEN}✓${NC} Explicit http:// preserved and worked"
  ((ASSERTIONS_PASSED++)) || true
else
  echo -e "  ${RED}✗${NC} Navigation with explicit http:// failed"
  ((ASSERTIONS_FAILED++)) || true
fi

end_test

# ─────────────────────────────────────────────────────────────────
start_test "auto-https: explicit https:// is preserved"

# Navigate with explicit https:// to http-only fixture - should fail with SSL error
OUTPUT=$(pinchtab goto "https://fixtures:80/index.html" 2>&1 || true)

if echo "$OUTPUT" | grep -qiE "https|ssl|tls|certificate|connection refused"; then
  echo -e "  ${GREEN}✓${NC} Explicit https:// preserved (got expected SSL error)"
  ((ASSERTIONS_PASSED++)) || true
else
  echo -e "  ${RED}✗${NC} Expected https error, got: $OUTPUT"
  ((ASSERTIONS_FAILED++)) || true
fi

end_test
