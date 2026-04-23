#!/bin/bash
# tabs-extended.sh — API advanced tab scenarios.

GROUP_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
source "${GROUP_DIR}/../../helpers/api.sh"

start_test "tab-specific upload: POST /tabs/{id}/upload"

pt_post /navigate -d "{\"url\":\"${FIXTURES_URL}/upload.html\"}"
TAB_ID=$(get_tab_id)
show_tab "created" "$TAB_ID"

pt_post "/tabs/${TAB_ID}/upload" -d '{"selector":"#single-file","files":["data:text/plain;base64,dGVzdCBmaWxl"]}'
assert_ok "upload to tab"
assert_json_exists "$RESULT" ".files" "upload response has files count"

end_test

# ─────────────────────────────────────────────────────────────────
start_test "tab-specific upload: multiple files"

pt_post "/tabs/${TAB_ID}/upload" -d '{"selector":"#multi-file","files":["data:text/plain;base64,ZmlsZTE=","data:text/plain;base64,ZmlsZTI="]}'
assert_ok "upload multiple files"
assert_json_contains "$RESULT" ".files" "2" "uploaded 2 files"

end_test

# ─────────────────────────────────────────────────────────────────
start_test "tab-specific upload: locked tab rejects wrong owner"

pt_post /navigate -d "{\"url\":\"${FIXTURES_URL}/upload.html\"}"
LOCKED_UPLOAD_TAB_ID=$(get_tab_id)
show_tab "created" "$LOCKED_UPLOAD_TAB_ID"

pt_post "/tabs/${LOCKED_UPLOAD_TAB_ID}/lock" -d '{"owner":"agent-a"}'
assert_ok "lock upload tab"

pinchtab POST "/tabs/${LOCKED_UPLOAD_TAB_ID}/upload" \
  -H "X-Owner: intruder" \
  -d '{"selector":"#single-file","files":["data:text/plain;base64,dGVzdCBmaWxl"]}'
_echo_truncated
assert_http_status 423 "wrong owner blocked on upload"
assert_contains "$RESULT" "tab_locked" "locked tab error returned for upload"

pinchtab POST "/tabs/${LOCKED_UPLOAD_TAB_ID}/upload" \
  -H "X-Owner: agent-a" \
  -d '{"selector":"#single-file","files":["data:text/plain;base64,dGVzdCBmaWxl"]}'
_echo_truncated
assert_ok "correct owner can upload to locked tab"

pt_post "/tabs/${LOCKED_UPLOAD_TAB_ID}/unlock" -d '{"owner":"agent-a"}'
assert_ok "unlock upload tab"

end_test

# ─────────────────────────────────────────────────────────────────
start_test "tab-specific download: GET /tabs/{id}/download"

pt_post /navigate -d "{\"url\":\"${FIXTURES_URL}/index.html\"}"
TAB_ID2=$(get_tab_id)
show_tab "created" "$TAB_ID2"

pt_get "/tabs/${TAB_ID2}/download?url=${FIXTURES_URL}/sample.txt"
assert_ok "download from tab"

end_test

# ─────────────────────────────────────────────────────────────────
start_test "tab-specific download: verify content returned"

if [ -n "$RESULT" ]; then
  echo -e "  ${GREEN}✓${NC} download returned content"
  ((ASSERTIONS_PASSED++)) || true
else
  echo -e "  ${RED}✗${NC} download returned empty content"
  ((ASSERTIONS_FAILED++)) || true
fi

end_test

# The secure pinchtab instance is configured with maxTabs=2 and close_lru.
# Tests that opening a 3rd managed tab evicts the least recently used one.
# Note: Chrome keeps an initial about:blank target that is unmanaged.
# Eviction is based on managed tab count, not Chrome target count.

# ─────────────────────────────────────────────────────────────────
start_test "tab-specific download: locked tab rejects wrong owner"

pt_post /navigate -d "{\"url\":\"${FIXTURES_URL}/index.html\"}"
LOCKED_DOWNLOAD_TAB_ID=$(get_tab_id)
show_tab "created" "$LOCKED_DOWNLOAD_TAB_ID"

pt_post "/tabs/${LOCKED_DOWNLOAD_TAB_ID}/lock" -d '{"owner":"agent-a"}'
assert_ok "lock download tab"

pinchtab GET "/tabs/${LOCKED_DOWNLOAD_TAB_ID}/download?url=${FIXTURES_URL}/sample.txt" \
  -H "X-Owner: intruder"
_echo_truncated
assert_http_status 423 "wrong owner blocked on download"
assert_contains "$RESULT" "tab_locked" "locked tab error returned for download"

pinchtab GET "/tabs/${LOCKED_DOWNLOAD_TAB_ID}/download?url=${FIXTURES_URL}/sample.txt" \
  -H "X-Owner: agent-a"
_echo_truncated
assert_ok "correct owner can download from locked tab"

pt_post "/tabs/${LOCKED_DOWNLOAD_TAB_ID}/unlock" -d '{"owner":"agent-a"}'
assert_ok "unlock download tab"

end_test

ORIG_URL="$E2E_SERVER"
E2E_SERVER="$E2E_SECURE_SERVER"

# ─────────────────────────────────────────────────────────────────
start_test "LRU eviction: open 2 tabs (at limit)"

pt_post /navigate -d "{\"url\":\"${FIXTURES_URL}/index.html\"}"
TAB1=$(echo "$RESULT" | jq -r '.tabId')
assert_ok "open tab 1 (index)"
echo -e "  ${MUTED}tab1: ${TAB1:0:12}...${NC}"

sleep 1

pt_post /navigate -d "{\"url\":\"${FIXTURES_URL}/form.html\"}"
TAB2=$(echo "$RESULT" | jq -r '.tabId')
assert_ok "open tab 2 (form)"
echo -e "  ${MUTED}tab2: ${TAB2:0:12}...${NC}"

pt_get "/tabs/$TAB1/snapshot" > /dev/null
assert_ok "tab1 accessible"
pt_get "/tabs/$TAB2/snapshot" > /dev/null
assert_ok "tab2 accessible"

end_test

# ─────────────────────────────────────────────────────────────────
start_test "LRU eviction: 3rd tab evicts least recently used"

sleep 1
pt_get "/tabs/$TAB2/snapshot" > /dev/null
sleep 1

pt_post /navigate -d "{\"url\":\"${FIXTURES_URL}/buttons.html\"}"
TAB3=$(echo "$RESULT" | jq -r '.tabId')
assert_ok "open tab 3 (triggers eviction)"
echo -e "  ${MUTED}tab3: ${TAB3:0:12}...${NC}"

pt_get "/tabs/$TAB1/snapshot"
assert_http_error 404 "tab1 evicted (LRU)"

pt_get "/tabs/$TAB2/snapshot" > /dev/null
assert_ok "tab2 survived (recently used)"

pt_get "/tabs/$TAB3/snapshot" > /dev/null
assert_ok "tab3 accessible"

end_test

# ─────────────────────────────────────────────────────────────────
start_test "LRU eviction: continuous eviction works"

sleep 1
pt_get "/tabs/$TAB3/snapshot" > /dev/null
sleep 1

pt_post /navigate -d "{\"url\":\"${FIXTURES_URL}/table.html\"}"
TAB4=$(echo "$RESULT" | jq -r '.tabId')
assert_ok "open tab 4 (triggers second eviction)"

pt_get "/tabs/$TAB2/snapshot"
assert_http_error 404 "tab2 evicted (LRU)"

pt_get "/tabs/$TAB3/snapshot" > /dev/null
assert_ok "tab3 survived"
pt_get "/tabs/$TAB4/snapshot" > /dev/null
assert_ok "tab4 accessible"

end_test

# ─────────────────────────────────────────────────────────────────
start_test "tabs: list returns array"

pt_get /tabs
assert_ok "list tabs"
assert_json_exists "$RESULT" '.tabs'
assert_json_length_gte "$RESULT" '.tabs' '1' "at least 1 tab"

end_test

# ─────────────────────────────────────────────────────────────────
start_test "tabs: new + close roundtrip"

pt_post /tab "{\"action\":\"new\",\"url\":\"${FIXTURES_URL}/index.html\"}"
assert_ok "new tab"
NEW_TAB=$(echo "$RESULT" | jq -r '.tabId')

pt_post /tab "{\"action\":\"close\",\"tabId\":\"${NEW_TAB}\"}"
assert_ok "close tab"

end_test

# ─────────────────────────────────────────────────────────────────
start_test "tabs: close without tabId → 400"

pt_post /tab '{"action":"close"}'
assert_http_status "400" "rejects close without tabId"

end_test

# ─────────────────────────────────────────────────────────────────
start_test "tabs: bad action → 400"

pt_post /tab '{"action":"explode"}'
assert_http_status "400" "rejects bad action"

end_test

# ─────────────────────────────────────────────────────────────────
start_test "tabs: new tab returns tabId"

pt_post /tab '{"action":"new","url":"about:blank"}'
assert_ok "new tab"
assert_tab_id "new tab returns tabId"

pt_post /tab "{\"action\":\"close\",\"tabId\":\"${TAB_ID}\"}"

end_test

# ─────────────────────────────────────────────────────────────────
start_test "tabs: nonexistent tab → 404"

FAKE_TAB="A25658CE1BA82659EBE9C93C46CEE63A"

pt_post "/tabs/${FAKE_TAB}/navigate" "{\"url\":\"${FIXTURES_URL}/index.html\"}"
assert_http_status "404" "navigate on fake tab"

pt_get "/tabs/${FAKE_TAB}/snapshot"
assert_http_status "404" "snapshot on fake tab"

end_test

# ─────────────────────────────────────────────────────────────────
start_test "tab lock: lock and unlock"

pt_post /navigate -d "{\"url\":\"${FIXTURES_URL}/index.html\"}"
TAB_ID=$(get_tab_id)

pt_post /lock -d "{\"tabId\":\"${TAB_ID}\",\"owner\":\"test-agent\"}"
assert_ok "lock tab"
assert_json_eq "$RESULT" '.locked' 'true' "tab is locked"
assert_json_eq "$RESULT" '.owner' 'test-agent' "owner matches"

pt_post /unlock -d "{\"tabId\":\"${TAB_ID}\",\"owner\":\"test-agent\"}"
assert_ok "unlock tab"
assert_json_eq "$RESULT" '.unlocked' 'true' "tab is unlocked"

end_test

# ─────────────────────────────────────────────────────────────────
start_test "tab lock: wrong owner cannot unlock"

pt_post /navigate -d "{\"url\":\"${FIXTURES_URL}/index.html\"}"
TAB_ID=$(get_tab_id)

pt_post /lock -d "{\"tabId\":\"${TAB_ID}\",\"owner\":\"agent-a\"}"
assert_ok "lock tab"

pt_post /unlock -d "{\"tabId\":\"${TAB_ID}\",\"owner\":\"agent-b\"}"
assert_not_ok "wrong owner rejected"

pt_post /unlock -d "{\"tabId\":\"${TAB_ID}\",\"owner\":\"agent-a\"}"

end_test

E2E_SERVER="$ORIG_URL"

# ─────────────────────────────────────────────────────────────────
start_test "tab lock: lock with timeoutSec"

pt_post /navigate -d "{\"url\":\"${FIXTURES_URL}/index.html\"}"
TAB_ID=$(get_tab_id)

pt_post /lock -d "{\"tabId\":\"${TAB_ID}\",\"owner\":\"test-ttl\",\"timeoutSec\":60}"
assert_ok "lock with timeout"
assert_json_exists "$RESULT" '.expiresAt' "has expiration time"

pt_post /unlock -d "{\"tabId\":\"${TAB_ID}\",\"owner\":\"test-ttl\"}"

end_test

# ─────────────────────────────────────────────────────────────────
start_test "tab lock: path-based lock (POST /tabs/{id}/lock)"

pt_post /navigate -d "{\"url\":\"${FIXTURES_URL}/index.html\"}"
TAB_ID=$(get_tab_id)

pt_post "/tabs/${TAB_ID}/lock" -d "{\"owner\":\"path-agent\"}"
assert_ok "path-based lock"
assert_json_eq "$RESULT" '.locked' 'true'

pt_post "/tabs/${TAB_ID}/unlock" -d "{\"owner\":\"path-agent\"}"
assert_ok "path-based unlock"

end_test

# ─────────────────────────────────────────────────────────────────
# Human Handoff Tests
# ─────────────────────────────────────────────────────────────────

start_test "handoff: pause and resume flow"

pt_post /navigate -d "{\"url\":\"${FIXTURES_URL}/buttons.html\"}"
HANDOFF_TAB=$(get_tab_id)
show_tab "created" "$HANDOFF_TAB"

pt_post "/tabs/${HANDOFF_TAB}/handoff" -d '{"reason":"captcha_test"}'
assert_ok "handoff tab"
assert_json_eq "$RESULT" '.status' 'paused_handoff' "status is paused_handoff"
assert_json_eq "$RESULT" '.reason' 'captcha_test' "reason matches"

pt_get "/tabs/${HANDOFF_TAB}/handoff"
assert_ok "get handoff status"
assert_json_eq "$RESULT" '.status' 'paused_handoff' "still paused"

pt_post "/tabs/${HANDOFF_TAB}/resume" -d '{"status":"completed"}'
assert_ok "resume tab"

pt_get "/tabs/${HANDOFF_TAB}/handoff"
assert_ok "get handoff status after resume"
assert_json_eq "$RESULT" '.status' 'active' "status is active after resume"

end_test

# ─────────────────────────────────────────────────────────────────
start_test "handoff: actions blocked while paused"

pt_post /navigate -d "{\"url\":\"${FIXTURES_URL}/buttons.html\"}"
BLOCK_TAB=$(get_tab_id)
show_tab "created" "$BLOCK_TAB"

pt_post "/tabs/${BLOCK_TAB}/handoff" -d '{"reason":"manual_intervention"}'
assert_ok "pause tab for handoff"

pt_post /action -d "{\"kind\":\"click\",\"selector\":\"#increment\",\"tabId\":\"${BLOCK_TAB}\"}"
assert_http_status 409 "action blocked during handoff"
assert_contains "$RESULT" "tab_paused_handoff" "error code is tab_paused_handoff"

pt_post "/tabs/${BLOCK_TAB}/resume" -d '{}'
assert_ok "resume tab"

end_test

# ─────────────────────────────────────────────────────────────────
start_test "handoff: actions work after resume"

pt_post "/tabs/${BLOCK_TAB}/navigate" -d "{\"url\":\"${FIXTURES_URL}/buttons.html\"}"
assert_ok "navigate to buttons page"

pt_post /wait -d "{\"tabId\":\"${BLOCK_TAB}\",\"selector\":\"#increment\"}"
assert_ok "wait for button"

pt_post /action -d "{\"kind\":\"click\",\"selector\":\"#increment\",\"tabId\":\"${BLOCK_TAB}\"}"
assert_ok "action succeeds after resume"

end_test

# ─────────────────────────────────────────────────────────────────
start_test "handoff: batch actions blocked while paused"

pt_post /navigate -d "{\"url\":\"${FIXTURES_URL}/buttons.html\"}"
BATCH_TAB=$(get_tab_id)
show_tab "created" "$BATCH_TAB"

pt_post "/tabs/${BATCH_TAB}/handoff" -d '{"reason":"captcha"}'
assert_ok "pause tab"

pt_post /actions -d "{\"tabId\":\"${BATCH_TAB}\",\"actions\":[{\"kind\":\"click\",\"selector\":\"#increment\"}]}"
assert_ok "batch returns 200 with error in results"
assert_json_contains "$RESULT" '.results[0].error' 'paused for human handoff' "error mentions handoff"
assert_json_eq "$RESULT" '.failed' '1' "one action failed"

pt_post "/tabs/${BATCH_TAB}/resume" -d '{}'
assert_ok "resume tab"

end_test

# ─────────────────────────────────────────────────────────────────
start_test "handoff: timeout auto-expires"

pt_post /navigate -d "{\"url\":\"${FIXTURES_URL}/buttons.html\"}"
TIMEOUT_TAB=$(get_tab_id)
show_tab "created" "$TIMEOUT_TAB"

pt_post "/tabs/${TIMEOUT_TAB}/handoff" -d '{"reason":"short_timeout","timeoutMs":1500}'
assert_ok "handoff with timeout"
assert_json_exists "$RESULT" '.expiresAt' "response includes expiresAt"

pt_get "/tabs/${TIMEOUT_TAB}/handoff"
assert_ok "check status before timeout"
assert_json_eq "$RESULT" '.status' 'paused_handoff' "still paused before timeout"

sleep 2

pt_get "/tabs/${TIMEOUT_TAB}/handoff"
assert_ok "check status after timeout"
assert_json_eq "$RESULT" '.status' 'active' "auto-expired to active"

pt_post "/tabs/${TIMEOUT_TAB}/navigate" -d "{\"url\":\"${FIXTURES_URL}/buttons.html\"}"
assert_ok "navigate to buttons page"

pt_post /wait -d "{\"tabId\":\"${TIMEOUT_TAB}\",\"selector\":\"#increment\"}"
assert_ok "wait for button"

pt_post /action -d "{\"kind\":\"click\",\"selector\":\"#increment\",\"tabId\":\"${TIMEOUT_TAB}\"}"
assert_ok "action succeeds after timeout expiry"

end_test
