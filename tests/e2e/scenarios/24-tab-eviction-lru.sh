#!/bin/bash
# 24-tab-eviction-lru.sh — LRU tab eviction (maxTabs=2 on secure instance)
#
# The secure pinchtab instance is configured with maxTabs=2 and close_lru.
# Tests that opening a 3rd managed tab evicts the least recently used one.
#
# Note: Chrome keeps an initial about:blank target that is unmanaged.
# Eviction is based on managed tab count, not Chrome target count.

source "$(dirname "$0")/common.sh"

# Use the secure instance (maxTabs=2)
E2E_SERVER="$E2E_SECURE_SERVER"

# ─────────────────────────────────────────────────────────────────
start_test "LRU eviction: open 2 tabs (at limit)"

# Tab 1
pt_post /navigate -d "{\"url\":\"${FIXTURES_URL}/index.html\"}"
TAB1=$(echo "$RESULT" | jq -r '.tabId')
assert_ok "open tab 1 (index)"
echo -e "  ${MUTED}tab1: ${TAB1:0:12}...${NC}"

sleep 1

# Tab 2
pt_post /navigate -d "{\"url\":\"${FIXTURES_URL}/form.html\"}"
TAB2=$(echo "$RESULT" | jq -r '.tabId')
assert_ok "open tab 2 (form)"
echo -e "  ${MUTED}tab2: ${TAB2:0:12}...${NC}"

# Both tabs should be accessible
pt_get "/tabs/$TAB1/snapshot" > /dev/null
assert_ok "tab1 accessible"
pt_get "/tabs/$TAB2/snapshot" > /dev/null
assert_ok "tab2 accessible"

end_test

# ─────────────────────────────────────────────────────────────────
start_test "LRU eviction: 3rd tab evicts least recently used"

# Touch tab2 to make it recently used (clear time separation)
sleep 1
pt_get "/tabs/$TAB2/snapshot" > /dev/null
sleep 1

# Tab 3 — should evict tab1 (LRU, not touched since creation)
pt_post /navigate -d "{\"url\":\"${FIXTURES_URL}/buttons.html\"}"
TAB3=$(echo "$RESULT" | jq -r '.tabId')
assert_ok "open tab 3 (triggers eviction)"
echo -e "  ${MUTED}tab3: ${TAB3:0:12}...${NC}"

# Tab1 should be gone (evicted as LRU)
pt_get "/tabs/$TAB1/snapshot"
assert_http_error 404 "tab1 evicted (LRU)"

# Tab2 should still be accessible (recently used)
pt_get "/tabs/$TAB2/snapshot" > /dev/null
assert_ok "tab2 survived (recently used)"

# Tab3 should be accessible (just created)
pt_get "/tabs/$TAB3/snapshot" > /dev/null
assert_ok "tab3 accessible"

end_test

# ─────────────────────────────────────────────────────────────────
start_test "LRU eviction: continuous eviction works"

# Touch tab3 so tab2 becomes LRU
sleep 1
pt_get "/tabs/$TAB3/snapshot" > /dev/null
sleep 1

# Tab 4 — should evict tab2 (LRU)
pt_post /navigate -d "{\"url\":\"${FIXTURES_URL}/table.html\"}"
TAB4=$(echo "$RESULT" | jq -r '.tabId')
assert_ok "open tab 4 (triggers second eviction)"

# Tab2 should be gone now
pt_get "/tabs/$TAB2/snapshot"
assert_http_error 404 "tab2 evicted (LRU)"

# Tab3 and tab4 should be accessible
pt_get "/tabs/$TAB3/snapshot" > /dev/null
assert_ok "tab3 survived"
pt_get "/tabs/$TAB4/snapshot" > /dev/null
assert_ok "tab4 accessible"

end_test
