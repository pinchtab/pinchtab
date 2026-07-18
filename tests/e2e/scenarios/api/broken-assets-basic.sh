#!/bin/bash
# broken-assets-basic.sh — GET /network?broken=true broken asset detection.

GROUP_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
source "${GROUP_DIR}/../../helpers/api.sh"

# Polls /network?broken=true until at least $1 broken entries appear (the
# page's subresource failures land asynchronously after navigate returns).
wait_for_broken_count() {
  local want="$1"
  local tries=0
  while [ "$tries" -lt 20 ]; do
    pt_get "/network?broken=true"
    local count
    count=$(echo "$RESULT" | jq '.broken | length')
    if [ "$count" -ge "$want" ]; then
      return 0
    fi
    tries=$((tries + 1))
    sleep 0.3
  done
  return 1
}

# ─────────────────────────────────────────────────────────────────
start_test "broken-assets page reports 3 asset 404s and the failed fetch"

pt_post /network/clear ''
pt_post /navigate -d "{\"url\":\"${FIXTURES_URL}/audit-site/broken-assets.html\"}"
assert_ok "navigate to broken-assets.html"

if wait_for_broken_count 4; then
  pass_assert "broken entries appeared"
else
  fail_assert "broken entries did not reach 4 (got: $(echo "$RESULT" | jq '.broken | length'))"
fi

assert_json_length "$RESULT" '.broken' 4 "exactly 4 broken entries"

assert_json_eq "$RESULT" '[.broken[] | select(.url | endswith("assets/missing.css"))][0].resourceType' "stylesheet" "missing.css classified as stylesheet"
assert_json_eq "$RESULT" '[.broken[] | select(.url | endswith("assets/missing.css"))][0].statusCode' "404" "missing.css is 404"
assert_json_eq "$RESULT" '[.broken[] | select(.url | endswith("assets/missing.js"))][0].resourceType' "script" "missing.js classified as script"
assert_json_eq "$RESULT" '[.broken[] | select(.url | endswith("assets/missing.js"))][0].statusCode' "404" "missing.js is 404"
assert_json_eq "$RESULT" '[.broken[] | select(.url | endswith("assets/missing.png"))][0].resourceType' "image" "missing.png classified as image"
assert_json_eq "$RESULT" '[.broken[] | select(.url | endswith("assets/missing.png"))][0].statusCode' "404" "missing.png is 404"

FETCH_TYPE=$(echo "$RESULT" | jq -r '[.broken[] | select(.url | endswith("assets/missing.json"))][0].resourceType')
if [ "$FETCH_TYPE" = "fetch" ] || [ "$FETCH_TYPE" = "xhr" ]; then
  pass_assert "missing.json classified as xhr/fetch (got: $FETCH_TYPE)"
else
  fail_assert "missing.json classified as xhr/fetch (got: $FETCH_TYPE)"
fi
assert_json_eq "$RESULT" '[.broken[] | select(.url | endswith("assets/missing.json"))][0].statusCode' "404" "missing.json is 404"

end_test

# ─────────────────────────────────────────────────────────────────
start_test "clean page reports no broken assets"

pt_post /network/clear ''
pt_post /navigate -d "{\"url\":\"${FIXTURES_URL}/audit-site/clean.html\"}"
assert_ok "navigate to clean.html"

sleep 1
pt_get "/network?broken=true"
assert_ok "get broken assets"
assert_json_length "$RESULT" '.broken' 0 "broken list empty"
assert_json_eq "$RESULT" '.count' "0" "count is 0"

end_test
