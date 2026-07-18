#!/bin/bash
# audit-auth-basic.sh — cookie injection and profile auth on audit runs.

GROUP_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
source "${GROUP_DIR}/../../helpers/cli.sh"

COOKIE_ECHO="http://fixtures/audit-site/cookie-echo.html"

report_has_cookie_value() {
  echo "$PT_OUT" | jq -e '[.pages[].browser.consoleLogs[]?.message // empty] | any(contains("e2evalue"))' >/dev/null 2>&1
}

# ─────────────────────────────────────────────────────────────────
start_test "--cookie injects a cookie visible to the audited page"

pt_ok audit "$COOKIE_ECHO" --cookie test=e2evalue --screenshot=false --json

if report_has_cookie_value; then
  pass_assert "report captures e2evalue from the page"
else
  fail_assert "report captures e2evalue (console logs: $(echo "$PT_OUT" | jq -c '[.pages[].browser.consoleLogs[]?.message]'))"
fi

end_test

# ─────────────────────────────────────────────────────────────────
start_test "cookies do not leak into a later run (isolation)"

pt_ok audit "$COOKIE_ECHO" --screenshot=false --json

if report_has_cookie_value; then
  fail_assert "no e2evalue without the flag (leaked cookie)"
else
  pass_assert "no e2evalue without the flag"
fi

end_test

# ─────────────────────────────────────────────────────────────────
start_test "--profile default routes the run"

pt_ok audit "$COOKIE_ECHO" --profile default --screenshot=false --json

PAGE_COUNT=$(echo "$PT_OUT" | jq '.pages | length' 2>/dev/null)
if [ "${PAGE_COUNT:-0}" -eq 1 ] 2>/dev/null; then
  pass_assert "audit ran against the default profile instance"
else
  fail_assert "audit ran against the default profile instance (pages: $PAGE_COUNT)"
fi

end_test

# ─────────────────────────────────────────────────────────────────
start_test "bad cookies file fails with a clear error"

BAD_FILE=$(mktemp /tmp/bad-cookies.XXXXXX.json)
echo '{definitely not valid json' > "$BAD_FILE"

pt_fail audit "$COOKIE_ECHO" --cookies-file "$BAD_FILE"
if echo "$PT_ERR" | grep -q "cookies file"; then
  pass_assert "error message mentions the cookies file"
else
  fail_assert "error message mentions the cookies file (stderr: $PT_ERR)"
fi

rm -f "$BAD_FILE"

end_test
