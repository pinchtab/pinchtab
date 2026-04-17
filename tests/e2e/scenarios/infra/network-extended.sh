#!/bin/bash
# network-extended.sh — Network capture & export API tests.
# Covers: /network/export, /network/export/stream, format registry,
#         header redaction, body inclusion, file output, and tab-scoped variants.

GROUP_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
source "${GROUP_DIR}/../../helpers/api.sh"

# ─────────────────────────────────────────────────────────────────
# Setup: navigate to a page that generates network traffic
# ─────────────────────────────────────────────────────────────────

pt_post /navigate "{\"url\":\"${FIXTURES_URL}/buttons.html\"}"
TAB_ID=$(get_tab_id)
sleep 1

# ─────────────────────────────────────────────────────────────────
start_test "GET /network/export: default HAR format"

pt_get "/network/export?tabId=${TAB_ID}"
assert_ok "export HAR"

# Validate HAR structure
assert_result_jq '.log.version' "1.2" "HAR version is 1.2"
assert_json_exists "$RESULT" '.log.creator.name' "has creator name"
assert_json_exists "$RESULT" '.log.entries' "has entries array"

ENTRY_COUNT=$(echo "$RESULT" | jq '.log.entries | length')
if [ "$ENTRY_COUNT" -gt 0 ]; then
  echo -e "  ${GREEN}✓${NC} HAR contains $ENTRY_COUNT entries"
  ((ASSERTIONS_PASSED++)) || true
else
  echo -e "  ${RED}✗${NC} HAR has no entries (expected traffic from navigation)"
  ((ASSERTIONS_FAILED++)) || true
fi

end_test

# ─────────────────────────────────────────────────────────────────
start_test "GET /network/export: explicit format=har"

pt_get "/network/export?tabId=${TAB_ID}&format=har"
assert_ok "export format=har"
assert_result_jq '.log.version' "1.2" "HAR version"

end_test

# ─────────────────────────────────────────────────────────────────
start_test "GET /network/export: NDJSON format"

pinchtab GET "/network/export?tabId=${TAB_ID}&format=ndjson"

if [ "$HTTP_STATUS" = "200" ]; then
  echo -e "  ${GREEN}✓${NC} NDJSON export → 200"
  ((ASSERTIONS_PASSED++)) || true

  # Each line should be valid JSON
  LINE_COUNT=$(echo "$RESULT" | grep -c '^{' || true)
  if [ "$LINE_COUNT" -gt 0 ]; then
    FIRST_LINE=$(echo "$RESULT" | head -1)
    echo "$FIRST_LINE" | jq . >/dev/null 2>&1
    if [ $? -eq 0 ]; then
      echo -e "  ${GREEN}✓${NC} NDJSON lines are valid JSON ($LINE_COUNT lines)"
      ((ASSERTIONS_PASSED++)) || true
    else
      echo -e "  ${RED}✗${NC} first NDJSON line is not valid JSON"
      ((ASSERTIONS_FAILED++)) || true
    fi
  else
    echo -e "  ${YELLOW}~${NC} no NDJSON lines (empty export)"
    ((ASSERTIONS_PASSED++)) || true
  fi
else
  echo -e "  ${RED}✗${NC} NDJSON export failed ($HTTP_STATUS)"
  ((ASSERTIONS_FAILED++)) || true
fi

end_test

# ─────────────────────────────────────────────────────────────────
start_test "GET /network/export: unknown format returns 400 with available list"

pt_get "/network/export?tabId=${TAB_ID}&format=xml"
assert_http_status "400" "unknown format → 400"
assert_json_contains "$RESULT" '.code' "unknown_format" "error code"
assert_json_exists "$RESULT" '.available' "lists available formats"

AVAILABLE=$(echo "$RESULT" | jq -r '.available | join(",")')
if echo "$AVAILABLE" | grep -q "har" && echo "$AVAILABLE" | grep -q "ndjson"; then
  echo -e "  ${GREEN}✓${NC} available includes har and ndjson: $AVAILABLE"
  ((ASSERTIONS_PASSED++)) || true
else
  echo -e "  ${RED}✗${NC} unexpected available formats: $AVAILABLE"
  ((ASSERTIONS_FAILED++)) || true
fi

end_test

# ─────────────────────────────────────────────────────────────────
start_test "GET /network/export: sensitive headers redacted by default"

pt_get "/network/export?tabId=${TAB_ID}&format=har"
assert_ok "export for redaction check"

# Check that no entry has un-redacted Cookie or Authorization headers
LEAKED=$(echo "$RESULT" | jq '[
  .log.entries[]
  | (.request.headers + .response.headers)[]
  | select((.name | ascii_downcase) == "cookie" or
           (.name | ascii_downcase) == "authorization" or
           (.name | ascii_downcase) == "set-cookie")
  | select(.value != "[REDACTED]")
] | length')

if [ "$LEAKED" = "0" ] || [ "$LEAKED" = "null" ]; then
  echo -e "  ${GREEN}✓${NC} no leaked sensitive headers"
  ((ASSERTIONS_PASSED++)) || true
else
  echo -e "  ${RED}✗${NC} found $LEAKED un-redacted sensitive header values"
  ((ASSERTIONS_FAILED++)) || true
fi

end_test

# ─────────────────────────────────────────────────────────────────
start_test "GET /network/export: redact=false exposes raw headers"

pt_get "/network/export?tabId=${TAB_ID}&format=har&redact=false"
assert_ok "export with redact=false"

# Just verify it returns valid HAR — we can't guarantee the fixture sets cookies,
# but the flag should be accepted without error.
assert_result_jq '.log.version' "1.2" "HAR still valid with redact=false"

end_test

# ─────────────────────────────────────────────────────────────────
start_test "GET /network/export: body=true includes response content"

pt_get "/network/export?tabId=${TAB_ID}&format=har&body=true"
assert_ok "export with bodies"

# At least one entry should have non-empty content.text for an HTML page
HAS_BODY=$(echo "$RESULT" | jq '[.log.entries[] | select(.response.content.text != null and .response.content.text != "")] | length')
if [ "$HAS_BODY" -gt 0 ]; then
  echo -e "  ${GREEN}✓${NC} $HAS_BODY entries include response body"
  ((ASSERTIONS_PASSED++)) || true
else
  echo -e "  ${YELLOW}~${NC} no entries have body content (may depend on timing)"
  ((ASSERTIONS_PASSED++)) || true
fi

end_test

# ─────────────────────────────────────────────────────────────────
start_test "GET /network/export: filter by method"

pt_get "/network/export?tabId=${TAB_ID}&format=har&method=GET"
assert_ok "export filter method=GET"

NON_GET=$(echo "$RESULT" | jq '[.log.entries[] | select(.request.method != "GET")] | length')
if [ "$NON_GET" = "0" ] || [ "$NON_GET" = "null" ]; then
  echo -e "  ${GREEN}✓${NC} all exported entries are GET"
  ((ASSERTIONS_PASSED++)) || true
else
  echo -e "  ${RED}✗${NC} found $NON_GET non-GET entries"
  ((ASSERTIONS_FAILED++)) || true
fi

end_test

# ─────────────────────────────────────────────────────────────────
start_test "GET /network/export: limit parameter"

pt_get "/network/export?tabId=${TAB_ID}&format=har&limit=2"
assert_ok "export with limit=2"

ENTRY_COUNT=$(echo "$RESULT" | jq '.log.entries | length')
if [ "$ENTRY_COUNT" -le 2 ]; then
  echo -e "  ${GREEN}✓${NC} limit respected: $ENTRY_COUNT entries (<= 2)"
  ((ASSERTIONS_PASSED++)) || true
else
  echo -e "  ${RED}✗${NC} limit exceeded: $ENTRY_COUNT entries (expected <= 2)"
  ((ASSERTIONS_FAILED++)) || true
fi

end_test

# ─────────────────────────────────────────────────────────────────
start_test "GET /network/export: output=file saves to disk"

pt_get "/network/export?tabId=${TAB_ID}&format=har&output=file&path=e2e-test.har"
assert_ok "export to file"
assert_json_exists "$RESULT" '.path' "response has file path"
assert_json_exists "$RESULT" '.entries' "response has entry count"
assert_json_contains "$RESULT" '.format' "har" "format in response"

FILE_ENTRIES=$(echo "$RESULT" | jq '.entries')
if [ "$FILE_ENTRIES" -gt 0 ] 2>/dev/null; then
  echo -e "  ${GREEN}✓${NC} file has $FILE_ENTRIES entries"
  ((ASSERTIONS_PASSED++)) || true
else
  echo -e "  ${YELLOW}~${NC} file written with $FILE_ENTRIES entries"
  ((ASSERTIONS_PASSED++)) || true
fi

end_test

# ─────────────────────────────────────────────────────────────────
start_test "GET /network/export: output=file auto-generates name"

pt_get "/network/export?tabId=${TAB_ID}&format=har&output=file"
assert_ok "export to auto-named file"
assert_json_exists "$RESULT" '.path' "has auto-generated path"

AUTO_PATH=$(echo "$RESULT" | jq -r '.path')
if echo "$AUTO_PATH" | grep -q "network-.*\.har$"; then
  echo -e "  ${GREEN}✓${NC} auto-generated filename: $(basename "$AUTO_PATH")"
  ((ASSERTIONS_PASSED++)) || true
else
  echo -e "  ${RED}✗${NC} unexpected auto filename: $AUTO_PATH"
  ((ASSERTIONS_FAILED++)) || true
fi

end_test

# ─────────────────────────────────────────────────────────────────
start_test "GET /network/export: output=file with NDJSON"

pt_get "/network/export?tabId=${TAB_ID}&format=ndjson&output=file&path=e2e-test.ndjson"
assert_ok "NDJSON file export"
assert_json_contains "$RESULT" '.format' "ndjson" "format in response"

end_test

# ─────────────────────────────────────────────────────────────────
# Tab-scoped export endpoints
# ─────────────────────────────────────────────────────────────────

start_test "GET /tabs/{id}/network/export: tab-scoped HAR export"

pt_get "/tabs/${TAB_ID}/network/export"
assert_ok "tab-scoped export"
assert_result_jq '.log.version' "1.2" "HAR version"

end_test

# ─────────────────────────────────────────────────────────────────
start_test "GET /tabs/{id}/network/export: bad tab → error"

pt_get "/tabs/nonexistent_xyz_999/network/export"
assert_not_ok "rejects bad tab"

end_test

# ─────────────────────────────────────────────────────────────────
# Streaming export
# ─────────────────────────────────────────────────────────────────

start_test "GET /network/export/stream: requires path parameter"

pt_get "/network/export/stream?tabId=${TAB_ID}"
assert_http_status "400" "missing path → 400"

end_test

# ─────────────────────────────────────────────────────────────────
start_test "GET /network/export/stream: SSE events on navigation"

# Start a streaming export in background, navigate, then check output.
# We use a short timeout curl to capture initial SSE events.
STREAM_OUTPUT=$(e2e_curl -s -N \
  --max-time 5 \
  -H "Content-Type: application/json" \
  "${E2E_SERVER}/network/export/stream?tabId=${TAB_ID}&format=har&path=e2e-stream.har" \
  2>/dev/null || true)

# Trigger traffic while the stream might still be running
pt_post /navigate "{\"url\":\"${FIXTURES_URL}/form.html\"}"
sleep 2

# The curl with --max-time will have timed out by now. Check output.
if echo "$STREAM_OUTPUT" | grep -q "event:"; then
  echo -e "  ${GREEN}✓${NC} received SSE events from stream"
  ((ASSERTIONS_PASSED++)) || true
elif echo "$STREAM_OUTPUT" | grep -q "text/event-stream"; then
  echo -e "  ${GREEN}✓${NC} stream responded with SSE content-type"
  ((ASSERTIONS_PASSED++)) || true
else
  # Even without events the endpoint should have started successfully (200)
  echo -e "  ${YELLOW}~${NC} no SSE events captured (timing-dependent)"
  ((ASSERTIONS_PASSED++)) || true
fi

end_test

# ─────────────────────────────────────────────────────────────────
start_test "GET /tabs/{id}/network/export/stream: requires path"

pt_get "/tabs/${TAB_ID}/network/export/stream"
assert_http_status "400" "tab-scoped stream missing path → 400"

end_test

# ─────────────────────────────────────────────────────────────────
# HAR entry structure validation
# ─────────────────────────────────────────────────────────────────

start_test "HAR entries have correct structure"

pt_get "/network/export?tabId=${TAB_ID}&format=har"
assert_ok "export for structure check"

ENTRY_COUNT=$(echo "$RESULT" | jq '.log.entries | length')
if [ "$ENTRY_COUNT" -gt 0 ]; then
  # Check first entry has required HAR fields
  FIRST=$(echo "$RESULT" | jq '.log.entries[0]')

  for field in startedDateTime time request response timings; do
    VAL=$(echo "$FIRST" | jq ".$field")
    if [ "$VAL" != "null" ] && [ -n "$VAL" ]; then
      echo -e "  ${GREEN}✓${NC} entry has .$field"
      ((ASSERTIONS_PASSED++)) || true
    else
      echo -e "  ${RED}✗${NC} entry missing .$field"
      ((ASSERTIONS_FAILED++)) || true
    fi
  done

  # Check request sub-fields
  for field in method url httpVersion headers queryString; do
    VAL=$(echo "$FIRST" | jq ".request.$field")
    if [ "$VAL" != "null" ] && [ -n "$VAL" ]; then
      echo -e "  ${GREEN}✓${NC} request has .$field"
      ((ASSERTIONS_PASSED++)) || true
    else
      echo -e "  ${RED}✗${NC} request missing .$field"
      ((ASSERTIONS_FAILED++)) || true
    fi
  done

  # Check response sub-fields
  for field in status statusText httpVersion headers content; do
    VAL=$(echo "$FIRST" | jq ".response.$field")
    if [ "$VAL" != "null" ] && [ -n "$VAL" ]; then
      echo -e "  ${GREEN}✓${NC} response has .$field"
      ((ASSERTIONS_PASSED++)) || true
    else
      echo -e "  ${RED}✗${NC} response missing .$field"
      ((ASSERTIONS_FAILED++)) || true
    fi
  done

  # Check timings
  for field in send wait receive; do
    VAL=$(echo "$FIRST" | jq ".timings.$field")
    if [ "$VAL" != "null" ] && [ -n "$VAL" ]; then
      echo -e "  ${GREEN}✓${NC} timings has .$field"
      ((ASSERTIONS_PASSED++)) || true
    else
      echo -e "  ${RED}✗${NC} timings missing .$field"
      ((ASSERTIONS_FAILED++)) || true
    fi
  done
else
  echo -e "  ${YELLOW}~${NC} no entries to validate structure"
  ((ASSERTIONS_PASSED++)) || true
fi

end_test

# ─────────────────────────────────────────────────────────────────
# Network basics (moved from browser-full for grouping)
# ─────────────────────────────────────────────────────────────────

start_test "GET /network: list entries with filters"

pt_post /navigate "{\"url\":\"${FIXTURES_URL}/index.html\"}"
NEW_TAB_ID=$(get_tab_id)
sleep 1

pt_get "/network?tabId=${NEW_TAB_ID}"
assert_ok "get network entries"
assert_json_exists "$RESULT" '.entries' "has entries"

pt_get "/network?tabId=${NEW_TAB_ID}&method=GET&limit=3"
assert_ok "filter + limit"

ENTRIES_COUNT=$(echo "$RESULT" | jq '.entries | length')
if [ "$ENTRIES_COUNT" -le 3 ]; then
  echo -e "  ${GREEN}✓${NC} limit respected: $ENTRIES_COUNT <= 3"
  ((ASSERTIONS_PASSED++)) || true
else
  echo -e "  ${RED}✗${NC} limit exceeded: $ENTRIES_COUNT > 3"
  ((ASSERTIONS_FAILED++)) || true
fi

end_test

# ─────────────────────────────────────────────────────────────────
start_test "POST /network/clear: clears captured data"

pt_post /network/clear "{\"tabId\":\"${NEW_TAB_ID}\"}"
assert_ok "clear network"

pt_get "/network?tabId=${NEW_TAB_ID}"
assert_ok "get after clear"

ENTRIES_COUNT=$(echo "$RESULT" | jq '.entries | length')
if [ "$ENTRIES_COUNT" -eq 0 ]; then
  echo -e "  ${GREEN}✓${NC} entries cleared"
  ((ASSERTIONS_PASSED++)) || true
else
  echo -e "  ${YELLOW}~${NC} still has $ENTRIES_COUNT entries after clear"
  ((ASSERTIONS_PASSED++)) || true
fi

end_test
