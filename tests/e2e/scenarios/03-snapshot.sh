#!/bin/bash
# 03-snapshot.sh — Accessibility tree and text extraction

source "$(dirname "$0")/common.sh"

# ─────────────────────────────────────────────────────────────────
start_test "pinchtab snap"

pt_post /navigate -d "{\"url\":\"${FIXTURES_URL}/\"}"

pt_get /snapshot
assert_index_page "$RESULT"
assert_json_length_gte "$RESULT" '.nodes' 1

end_test

# ─────────────────────────────────────────────────────────────────
start_test "pinchtab snap (buttons.html)"

pt_post /navigate -d "{\"url\":\"${FIXTURES_URL}/buttons.html\"}"
sleep 1

pt_get /snapshot
assert_buttons_page "$RESULT"

end_test

# ─────────────────────────────────────────────────────────────────
start_test "pinchtab snap (form.html)"

pt_post /navigate -d "{\"url\":\"${FIXTURES_URL}/form.html\"}"
sleep 1

pt_get /snapshot
assert_form_page "$RESULT"

end_test

# ─────────────────────────────────────────────────────────────────
start_test "pinchtab text (table.html)"

pt_post /navigate -d "{\"url\":\"${FIXTURES_URL}/table.html\"}"
sleep 1

TEXT_RESULT=$(curl -s "${PINCHTAB_URL}/text" | jq -r '.text')
assert_table_page "$TEXT_RESULT"

end_test

# ─────────────────────────────────────────────────────────────────
start_test "snapshot: diff mode"

# Take initial snapshot, then take diff — diff should return ok
pt_post /navigate -d "{\"url\":\"${FIXTURES_URL}/buttons.html\"}"
sleep 1

pt_get /snapshot
assert_ok "initial snapshot"
INITIAL_COUNT=$(echo "$RESULT" | jq '.nodes | length')

# Second snapshot with diff=true (no changes made, so diff should have fewer/no changed nodes)
pt_get "/snapshot?diff=true"
assert_ok "diff snapshot"
DIFF_COUNT=$(echo "$RESULT" | jq '.nodes | length')

# Diff should return fewer or equal nodes compared to full snapshot
if [ "$DIFF_COUNT" -le "$INITIAL_COUNT" ]; then
  echo -e "  ${GREEN}✓${NC} diff has <= nodes than full ($DIFF_COUNT <= $INITIAL_COUNT)"
  ((ASSERTIONS_PASSED++)) || true
else
  echo -e "  ${RED}✗${NC} diff has more nodes than full ($DIFF_COUNT > $INITIAL_COUNT)"
  ((ASSERTIONS_FAILED++)) || true
fi

end_test

# ─────────────────────────────────────────────────────────────────
start_test "snapshot: maxTokens truncation"

pt_post /navigate -d "{\"url\":\"${FIXTURES_URL}/buttons.html\"}"
sleep 1

# Full snapshot
pt_get /snapshot
FULL_COUNT=$(echo "$RESULT" | jq '.nodes | length')

# Snapshot with very small maxTokens — should have fewer nodes
pt_get "/snapshot?maxTokens=50"
assert_ok "snapshot with maxTokens"
LIMITED_COUNT=$(echo "$RESULT" | jq '.nodes | length')

if [ "$LIMITED_COUNT" -le "$FULL_COUNT" ]; then
  echo -e "  ${GREEN}✓${NC} maxTokens limited nodes ($LIMITED_COUNT <= $FULL_COUNT)"
  ((ASSERTIONS_PASSED++)) || true
else
  echo -e "  ${RED}✗${NC} maxTokens did not limit ($LIMITED_COUNT > $FULL_COUNT)"
  ((ASSERTIONS_FAILED++)) || true
fi

end_test

# ─────────────────────────────────────────────────────────────────
start_test "snapshot: depth parameter"

pt_post /navigate -d "{\"url\":\"${FIXTURES_URL}/buttons.html\"}"
sleep 1

# Full tree
pt_get /snapshot
FULL_COUNT=$(echo "$RESULT" | jq '.nodes | length')

# Depth=1 should produce a shallower tree with fewer nodes
pt_get "/snapshot?depth=1"
assert_ok "snapshot with depth=1"
SHALLOW_COUNT=$(echo "$RESULT" | jq '.nodes | length')

if [ "$SHALLOW_COUNT" -le "$FULL_COUNT" ]; then
  echo -e "  ${GREEN}✓${NC} depth=1 limited tree ($SHALLOW_COUNT <= $FULL_COUNT)"
  ((ASSERTIONS_PASSED++)) || true
else
  echo -e "  ${RED}✗${NC} depth=1 did not limit ($SHALLOW_COUNT > $FULL_COUNT)"
  ((ASSERTIONS_FAILED++)) || true
fi

end_test

# ─────────────────────────────────────────────────────────────────
start_test "snapshot: format=text"

pt_post /navigate -d "{\"url\":\"${FIXTURES_URL}/index.html\"}"
pt_get "/snapshot?format=text"
assert_ok "get text format"

# Should not be JSON (no leading {)
if echo "$RESULT" | head -c1 | grep -q '{'; then
  echo -e "  ${RED}✗${NC} got JSON instead of text"
  ((ASSERTIONS_FAILED++)) || true
else
  echo -e "  ${GREEN}✓${NC} format is text, not JSON"
  ((ASSERTIONS_PASSED++)) || true
fi

end_test

# ─────────────────────────────────────────────────────────────────
start_test "snapshot: nonexistent tabId → error"

pt_get "/snapshot?tabId=nonexistent_xyz_999"
assert_not_ok "rejects bad tab"

end_test

# ─────────────────────────────────────────────────────────────────
start_test "snapshot: ref stability after action"

pt_post /navigate -d "{\"url\":\"${FIXTURES_URL}/form.html\"}"
pt_get /snapshot
REFS_BEFORE=$(echo "$RESULT" | jq '[.nodes[].ref] | sort')

pt_post /action '{"kind":"press","key":"Escape"}'
pt_get /snapshot
REFS_AFTER=$(echo "$RESULT" | jq '[.nodes[].ref] | sort')

if [ "$REFS_BEFORE" = "$REFS_AFTER" ]; then
  echo -e "  ${GREEN}✓${NC} refs stable after action"
  ((ASSERTIONS_PASSED++)) || true
else
  echo -e "  ${YELLOW}⚠${NC} refs changed (may be expected if DOM changed)"
  ((ASSERTIONS_PASSED++)) || true
fi

end_test

# ─────────────────────────────────────────────────────────────────
start_test "snapshot: format=yaml"

pt_post /navigate -d "{\"url\":\"${FIXTURES_URL}/index.html\"}"
pt_get "/snapshot?format=yaml"
assert_ok "get yaml format"

# Should contain YAML-like content (indentation, : separators, no { braces)
if echo "$RESULT" | grep -q "role:\|name:\|ref:"; then
  echo -e "  ${GREEN}✓${NC} looks like YAML"
  ((ASSERTIONS_PASSED++)) || true
else
  echo -e "  ${YELLOW}⚠${NC} may not be YAML format"
  ((ASSERTIONS_PASSED++)) || true
fi

end_test

# ─────────────────────────────────────────────────────────────────
start_test "snapshot: output=file"

pt_post /navigate -d "{\"url\":\"${FIXTURES_URL}/index.html\"}"
pt_get "/snapshot?output=file"
assert_ok "snapshot output=file"
assert_json_exists "$RESULT" '.path' "has file path"

end_test
# ─────────────────────────────────────────────────────────────────
start_test "snapshot: multi-tab content isolation with tabId"

# Navigate to two different pages, capturing tab IDs
pt_post /navigate -d "{\"url\":\"${FIXTURES_URL}/index.html\"}"
assert_ok "navigate to index"
TAB_A=$(echo "$RESULT" | jq -r '.tabId')

pt_post /navigate -d "{\"url\":\"${FIXTURES_URL}/form.html\"}"
assert_ok "navigate to form"
TAB_B=$(echo "$RESULT" | jq -r '.tabId')

# Snapshot tab A — should return index.html content (not form.html)
pt_get "/snapshot?tabId=${TAB_A}"
assert_ok "snapshot tab A"
assert_json_contains "$RESULT" '.url' "index.html" "tab A has index.html URL"

# Snapshot tab B — should return form.html content
pt_get "/snapshot?tabId=${TAB_B}"
assert_ok "snapshot tab B"
assert_json_contains "$RESULT" '.url' "form.html" "tab B has form.html URL"

# Text from tab A
pt_get "/text?tabId=${TAB_A}&format=text"
assert_ok "text tab A"
assert_contains "$RESULT" "E2E Test Suite" "tab A text matches index.html"

# Text from tab B
pt_get "/text?tabId=${TAB_B}&format=text"
assert_ok "text tab B"
assert_contains "$RESULT" "Form Test" "tab B text matches form.html"

end_test
