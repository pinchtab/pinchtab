# shellcheck shell=bash
# Provider-specific /stealth/status assertions.

assert_stealth_status() {
  local provider="$1"
  local status
  if ! status="$(curl -fsS -H "Authorization: Bearer ${TOKEN}" "http://127.0.0.1:${HOST_PORT}/stealth/status" 2>&1)"; then
    fail "stealth status request failed: $status"
  fi
  echo "  /stealth/status: $status"

  case "$provider" in
    chrome)
      echo "$status" | jq -e '.provider == "chrome"' >/dev/null \
        || fail "expected /stealth/status.provider=chrome: $status"
      ;;
    cloak)
      echo "$status" | jq -e '
        .provider == "cloak" and
        .native == true and
        .pinchtabOverlaysDisabled == true and
        .fingerprintSeed == "42069"
      ' >/dev/null || fail "unexpected cloak stealth status: $status"
      ;;
    *)
      fail "assert_stealth_status: unknown provider: $provider"
      ;;
  esac
}
