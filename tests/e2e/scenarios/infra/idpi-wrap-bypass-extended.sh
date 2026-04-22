#!/bin/bash
# idpi-wrap-bypass-extended.sh — Regression test for GHSA-r4f2-qghj-v4hf fix
#
# Tests that WrapContent() sanitizes injected delimiters to prevent
# trust boundary bypass via </untrusted_web_content> in page content.

GROUP_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
source "${GROUP_DIR}/../../helpers/api.sh"

# Use the secure server which has wrapContent enabled
_ORIG_E2E_SERVER="$E2E_SERVER"
E2E_SERVER="$E2E_SECURE_SERVER"

# ═══════════════════════════════════════════════════════════════════
# GHSA-r4f2-qghj-v4hf: Trust boundary bypass via unsanitized delimiter
# ═══════════════════════════════════════════════════════════════════

start_test "ghsa-r4f2-qghj-v4hf: WrapContent delimiter injection"

# Navigate to the malicious page
pt_post /navigate -d "{\"url\":\"${FIXTURES_URL}/idpi-wrap-bypass.html\"}"
assert_ok "navigate to wrap bypass page"
sleep 1  # Wait for JavaScript to inject the payload

# Get wrapped text content (JSON format to see full wrapped output)
pt_get "/text?mode=raw"
assert_ok "text extraction succeeds"

# Extract the text field from JSON for analysis
TEXT_CONTENT=$(echo "$RESULT" | jq -r '.text // empty')
if [ -z "$TEXT_CONTENT" ]; then
  TEXT_CONTENT="$RESULT"
fi

echo "  Raw text content (first 500 chars):"
echo "$TEXT_CONTENT" | head -c 500
echo ""

# Count occurrences of the closing delimiter
# If vulnerable: 2 close tags (one injected, one from WrapContent)
# If fixed: 1 close tag (injected one is sanitized)
CLOSE_TAG_COUNT=$(echo "$TEXT_CONTENT" | grep -o '</untrusted_web_content>' | wc -l | tr -d ' ')

echo "  Close tag count: $CLOSE_TAG_COUNT"

if [ "$CLOSE_TAG_COUNT" -gt 1 ]; then
  echo -e "  ${RED}✗${NC} VULNERABLE: Found $CLOSE_TAG_COUNT close tags (expected 1)"
  echo -e "  ${RED}  Attacker can inject content outside trust boundary${NC}"
  ((ASSERTIONS_FAILED++)) || true

  # Show the injection point
  echo ""
  echo "  Evidence of bypass:"
  echo "$TEXT_CONTENT" | grep -A2 -B2 "IMPORTANT SYSTEM INSTRUCTION" | head -10
else
  if [ "$CLOSE_TAG_COUNT" -eq 0 ]; then
    echo -e "  ${RED}✗${NC} WARNING: No close tags found - wrapping may not be applied"
    ((ASSERTIONS_FAILED++)) || true
  else
    echo -e "  ${GREEN}✓${NC} FIXED: Only $CLOSE_TAG_COUNT close tag (delimiter sanitized)"
    ((ASSERTIONS_PASSED++)) || true
  fi
fi

# Additional check: verify the injected instruction appears between delimiters
# (i.e., outside the trust boundary)
if echo "$TEXT_CONTENT" | grep -q "IMPORTANT SYSTEM INSTRUCTION"; then
  # Check if it appears AFTER a close tag but BEFORE another close tag
  # This would indicate successful boundary escape
  BETWEEN_TAGS=$(echo "$TEXT_CONTENT" | awk '
    /<\/untrusted_web_content>/ { found_first=1; next }
    found_first && /IMPORTANT SYSTEM INSTRUCTION/ { print "ESCAPED"; exit }
  ')

  if [ "$BETWEEN_TAGS" = "ESCAPED" ]; then
    echo -e "  ${RED}✗${NC} Injected instruction appears OUTSIDE trust boundary"
    ((ASSERTIONS_FAILED++)) || true
  else
    echo -e "  ${GREEN}✓${NC} Injected instruction contained within trust boundary"
    ((ASSERTIONS_PASSED++)) || true
  fi
fi

end_test

# Restore original E2E_SERVER
E2E_SERVER="$_ORIG_E2E_SERVER"
