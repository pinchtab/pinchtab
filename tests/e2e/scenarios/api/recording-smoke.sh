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

pt_post /record/stop -d '{}'
assert_ok "record stop"
assert_json_eq "$RESULT" ".status" "encoding" "stop returns encoding status"

# Poll /record/status until encoding completes (state transitions to "finished")
ENCODE_OK=false
for i in $(seq 1 60); do
  pt_get /record/status >/dev/null 2>&1
  STATE=$(echo "$RESULT" | jq -r '.state // empty')
  if [ "$STATE" = "finished" ]; then
    ENCODE_OK=true
    break
  fi
  if [ "$STATE" = "idle" ]; then
    ENCODE_OK=true
    break
  fi
  sleep 1
done

if [ "$ENCODE_OK" = "true" ]; then
  pass_assert "encoding completed (state=$STATE)"
else
  fail_assert "encoding did not complete within timeout (state=$STATE)"
fi

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

sleep 2

pt_post /record/stop -d '{}'
assert_ok "first stop"

pt_post /record/stop -d '{}'
assert_http_status 400 "second stop returns error"

# Wait for background encoding from the first stop to finish before next test
for i in $(seq 1 60); do
  pt_get /record/status >/dev/null 2>&1
  STATE=$(echo "$RESULT" | jq -r '.state // empty')
  if [ "$STATE" = "finished" ] || [ "$STATE" = "idle" ]; then break; fi
  sleep 1
done

end_test

# ─────────────────────────────────────────────────────────────────
start_test "record: discard drops frames without encoding"

pt_post /navigate -d "{\"url\":\"${FIXTURES_URL}/index.html\"}"
assert_ok "navigate for discard"

pt_post /record/start -d '{"format":"gif","fps":2,"quality":60}'
assert_ok "start for discard"

sleep 2

pt_post /record/stop -d '{"discard":true}'
assert_ok "discard stop"
assert_json_eq "$RESULT" ".status" "discarded" "discard returns discarded status"

pt_get /record/status
assert_ok "status after discard"
assert_json_eq "$RESULT" ".state" "idle" "idle after discard"

end_test
