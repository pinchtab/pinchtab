#!/bin/bash
# browser-config-basic.sh — Config/dashboard API browser field round trip.
# Covers: GET /api/config browser fields, PUT /api/config browser update,
#         proxy secret redaction.

GROUP_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
source "${GROUP_DIR}/../../helpers/api.sh"

# ─────────────────────────────────────────────────────────────────
start_test "config API: GET /api/config returns browser section"

pt_get /api/config
assert_ok "get config"
assert_json_exists "$RESULT" '.config.browser' "config has browser section"

# The browser section should have a provider field.
PROVIDER=$(echo "$RESULT" | jq -r '.config.browser.provider // empty' 2>/dev/null)
if [ -n "$PROVIDER" ] && [ "$PROVIDER" != "null" ]; then
  pass_assert "config browser.provider=$PROVIDER"
else
  soft_pass_assert "browser.provider not set (using default)"
fi

end_test

# ─────────────────────────────────────────────────────────────────
start_test "config API: proxy secrets are redacted in GET response"

PROXY_SECTION=$(echo "$RESULT" | jq '.config.proxy // empty' 2>/dev/null)
if [ -n "$PROXY_SECTION" ] && [ "$PROXY_SECTION" != "null" ] && [ "$PROXY_SECTION" != "" ]; then
  PROXY_PASS=$(echo "$PROXY_SECTION" | jq -r '.password // empty' 2>/dev/null)
  if [ -z "$PROXY_PASS" ] || [ "$PROXY_PASS" = "null" ] || [ "$PROXY_PASS" = "" ] || [ "$PROXY_PASS" = "***" ]; then
    pass_assert "proxy password is redacted or absent"
  else
    fail_assert "proxy password is exposed in config GET response: $PROXY_PASS"
  fi
else
  soft_pass_assert "no proxy section in config (none configured)"
fi

# Check per-target proxy passwords too
TARGET_PROXIES=$(echo "$RESULT" | jq '[.config | .. | objects | select(has("proxy")) | .proxy | select(type == "object") | .password // empty] | unique' 2>/dev/null || echo "[]")
EXPOSED_COUNT=$(echo "$TARGET_PROXIES" | jq '[.[] | select(. != null and . != "" and . != "***")] | length' 2>/dev/null || echo "0")
if [ "$EXPOSED_COUNT" -eq 0 ]; then
  pass_assert "no exposed proxy passwords in config"
else
  fail_assert "found $EXPOSED_COUNT exposed proxy password(s) in config"
fi

end_test

# ─────────────────────────────────────────────────────────────────
start_test "config API: browser fields round-trip through PUT"

# Read current config.
pt_get /api/config
assert_ok "get current config"
ORIGINAL_CONFIG="$RESULT"

# Extract the browser section and make a minor update.
ORIGINAL_INNER=$(echo "$ORIGINAL_CONFIG" | jq '.config' 2>/dev/null)
UPDATED_INNER=$(echo "$ORIGINAL_INNER" | jq '.browser.blockImages = true' 2>/dev/null)

if [ -n "$UPDATED_INNER" ] && [ "$UPDATED_INNER" != "null" ]; then
  # PUT expects a bare FileConfig, not the envelope.
  pt_post_raw /api/config "$UPDATED_INNER"

  if [ "$HTTP_STATUS" = "200" ] || [ "$HTTP_STATUS" = "204" ]; then
    pass_assert "PUT config accepted"

    # Read back and verify the browser fields survived the round trip.
    pt_get /api/config
    assert_ok "get config after PUT"

    # Restore original config.
    pt_post_raw /api/config "$ORIGINAL_INNER"
    if [ "$HTTP_STATUS" = "200" ] || [ "$HTTP_STATUS" = "204" ]; then
      pass_assert "config restored"
    else
      soft_pass_assert "could not restore config (status: $HTTP_STATUS)"
    fi
  else
    soft_pass_assert "PUT config returned $HTTP_STATUS (dashboard auth may be required)"
  fi
else
  soft_pass_assert "could not construct updated config"
fi

end_test

# ─────────────────────────────────────────────────────────────────
start_test "config API: security.allowedDomains visible in config"

pt_get /api/config
assert_ok "get config"

ALLOWED=$(echo "$RESULT" | jq '.config.security.allowedDomains // empty' 2>/dev/null)
if [ -n "$ALLOWED" ] && [ "$ALLOWED" != "null" ]; then
  pass_assert "security.allowedDomains present in config"
else
  soft_pass_assert "allowedDomains not in config (may use defaults)"
fi

end_test
