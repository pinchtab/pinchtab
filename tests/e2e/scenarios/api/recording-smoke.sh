#!/bin/bash
# recording-smoke.sh — Recording smoke tests (API).

GROUP_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
source "${GROUP_DIR}/../../helpers/api.sh"

# ─────────────────────────────────────────────────────────────────
start_test "record: status shows inactive"

pt_get /record/status
assert_ok "record status"
assert_json_eq "$RESULT" ".active" "false" "no active recording"

end_test

# ─────────────────────────────────────────────────────────────────
start_test "record: start → status → stop (gif)"

pt_post /navigate -d "{\"url\":\"${FIXTURES_URL}/index.html\"}"
assert_ok "navigate"

pt_post /record/start -d '{"format":"gif","fps":2,"quality":60}'
assert_ok "record start"
assert_json_eq "$RESULT" ".status" "recording" "recording started"
assert_json_eq "$RESULT" ".format" "gif" "format is gif"

sleep 2

pt_get /record/status
assert_ok "record status"
assert_json_eq "$RESULT" ".active" "true" "recording active"

OUTFILE="/tmp/e2e-recording-test.gif"
e2e_curl -s -X POST "${E2E_SERVER}/record/stop" -o "$OUTFILE" \
  -H "Content-Type: application/json" -d '{}'
FILESIZE=$(wc -c < "$OUTFILE" 2>/dev/null | tr -d ' ')

if [ -f "$OUTFILE" ] && [ "$FILESIZE" -gt 0 ]; then
  pass_assert "recording file created ($FILESIZE bytes)"
else
  fail_assert "recording file missing or empty"
fi
rm -f "$OUTFILE"

pt_get /record/status
assert_ok "record status after stop"
assert_json_eq "$RESULT" ".active" "false" "recording inactive after stop"

end_test

# ─────────────────────────────────────────────────────────────────
start_test "record: stop without start returns 400"

pt_post /record/stop -d '{}'
assert_http_status 400 "stop without active recording"

end_test

# ─────────────────────────────────────────────────────────────────
start_test "record: double stop returns error"

pt_post /navigate -d "{\"url\":\"${FIXTURES_URL}/index.html\"}"
assert_ok "navigate for double stop"

pt_post /record/start -d '{"format":"gif","fps":2,"quality":60}'
assert_ok "start for double stop"

e2e_curl -s -X POST "${E2E_SERVER}/record/stop" -o /dev/null \
  -H "Content-Type: application/json" -d '{}'

pt_post /record/stop -d '{}'
assert_http_status 400 "second stop returns error"

end_test
