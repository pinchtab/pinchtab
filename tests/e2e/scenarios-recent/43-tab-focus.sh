#!/bin/bash
# Test: Tab focus
# Tests for `POST /tab {"action":"focus","tabId":"..."}`
source "$(dirname "$0")/common.sh"

# --- T1: Focus tab by ID ---
start_test "Focus tab by ID"

# Navigate to page 1 to get first tab ID
pt_post /navigate "{\"url\":\"${FIXTURES_URL}/index.html\"}"
assert_ok "navigate to index.html"
TAB_ID_A=$(echo "$RESULT" | jq -r '.tabId // empty')
assert_tab_id "tab A"
show_tab "TAB_ID_A" "$TAB_ID_A"

# Navigate to page 2 to get second tab ID
pt_post /navigate "{\"url\":\"${FIXTURES_URL}/form.html\"}"
assert_ok "navigate to form.html"
TAB_ID_B=$(echo "$RESULT" | jq -r '.tabId // empty')
assert_tab_id "tab B"
show_tab "TAB_ID_B" "$TAB_ID_B"

# Focus on tab A
pt_post /tab "{\"action\":\"focus\",\"tabId\":\"${TAB_ID_A}\"}"
assert_ok "focus on tab A"
assert_json_contains "$RESULT" ".focused" "true" "response contains focused=true"

end_test

# --- T2: Focus tab switches active tab ---
start_test "Focus tab switches active tab"

# Get snapshot of tab A (should have index.html content)
pt_get "/snapshot?tabId=${TAB_ID_A}"
assert_ok "snapshot of tab A"
assert_result_exists ".title" "snapshot has title"

# Verify we can get tab A's text content (index.html)
pt_get "/text?tabId=${TAB_ID_A}&format=text"
assert_ok "text of tab A"
assert_contains "$RESULT" "E2E" "tab A contains E2E Test Suite content"

# Now focus on tab B
pt_post /tab "{\"action\":\"focus\",\"tabId\":\"${TAB_ID_B}\"}"
assert_ok "focus on tab B"

# Verify tab B has form content
pt_get "/text?tabId=${TAB_ID_B}&format=text"
assert_ok "text of tab B"
assert_contains "$RESULT" "Form" "tab B contains Form content"

end_test

# --- T3: Focus missing tabId returns 400 ---
start_test "Focus missing tabId returns 400"

pt_post /tab "{\"action\":\"focus\"}"
assert_http_status 400 "missing tabId returns 400"
assert_json_contains "$RESULT" ".error" "tabId" "error message mentions tabId"

end_test

# --- T4: Focus nonexistent tab returns 404 ---
start_test "Focus nonexistent tab returns 404"

pt_post /tab "{\"action\":\"focus\",\"tabId\":\"nonexistent-tab-id-12345\"}"
assert_http_status 404 "nonexistent tabId returns 404"
assert_json_contains "$RESULT" ".error" "not found" "error message indicates tab not found"

end_test

# --- T5: Focus invalid action returns 400 ---
start_test "Focus invalid action returns 400"

pt_post /tab "{\"action\":\"invalid\",\"tabId\":\"${TAB_ID_A}\"}"
assert_http_status 400 "invalid action returns 400"
assert_json_contains "$RESULT" ".error" "action" "error message mentions action"

end_test
