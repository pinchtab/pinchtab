#!/bin/bash
# memory-extended.sh — the memory capability refusal on a server whose config leaves
# security.allowMemory off (pinchtab-secure), and the snapshot size cap on a server
# with allowMemory on and security.memorySnapshotMaxBytes at 64 KiB (pinchtab-medium).
# memory-basic.sh runs the enabled path on the default server; the basic lane has one
# server, so the other configs live here.

GROUP_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
source "${GROUP_DIR}/../../helpers/api.sh"

memory_disabled_tests() {
  # ─────────────────────────────────────────────────────────────────
  start_test "memory: allowMemory off refuses snapshot, summary and compare, usage still answers"

  pt_get /health
  assert_ok "health"
  assert_json_jq "$RESULT" '(.security.enabledSensitiveEndpoints // []) | index("memory") == null' \
    "health does not list memory as enabled" "health lists memory although allowMemory is off"

  pt_post /navigate -d "{\"url\":\"${FIXTURES_URL}/memory-leak.html\"}"
  assert_ok "navigate to memory-leak.html"
  TAB_ID=$(get_tab_id)

  pt_get "/memory?tabId=${TAB_ID}"
  assert_ok "GET /memory is not capability-gated"
  assert_json_jq "$RESULT" '(.usedJSHeapSize | type) == "number" and .usedJSHeapSize > 0' \
    "usage reports a numeric usedJSHeapSize" "usedJSHeapSize missing or not a positive number"

  pt_post /memory/snapshot -d "{\"tabId\":\"${TAB_ID}\"}"
  assert_http_status 403 "snapshot refused"
  assert_json_eq "$RESULT" '.code' 'memory_disabled' "snapshot answers memory_disabled"
  assert_json_eq "$RESULT" '.details.setting' 'security.allowMemory' "refusal names the setting"

  pt_post "/tabs/${TAB_ID}/memory/snapshot" -d '{}'
  assert_http_status 403 "tab-scoped snapshot refused"
  assert_json_eq "$RESULT" '.code' 'memory_disabled' "tab-scoped snapshot answers memory_disabled"

  pt_get "/memory/snapshot/heap_any/summary"
  assert_http_status 403 "summary refused"
  assert_json_eq "$RESULT" '.code' 'memory_disabled' "summary answers memory_disabled"

  pt_get "/memory/compare?base=heap_a&head=heap_b"
  assert_http_status 403 "compare refused"
  assert_json_eq "$RESULT" '.code' 'memory_disabled' "compare answers memory_disabled"

  end_test
}

memory_cap_tests() {
  # ─────────────────────────────────────────────────────────────────
  start_test "memory: a snapshot over security.memorySnapshotMaxBytes is refused with 413"

  pt_get /health
  assert_ok "health"
  assert_json_jq "$RESULT" '(.security.enabledSensitiveEndpoints // []) | index("memory") != null' \
    "health lists memory as enabled" "health does not list memory although allowMemory is on"

  pt_post /navigate -d "{\"url\":\"${FIXTURES_URL}/memory-leak.html\"}"
  assert_ok "navigate to memory-leak.html"
  TAB_ID=$(get_tab_id)

  pt_post /memory/snapshot -d "{\"tabId\":\"${TAB_ID}\"}"
  assert_http_status 413 "snapshot over the 64 KiB cap"
  assert_json_eq "$RESULT" '.code' 'memory_snapshot_too_large' "cap refusal is memory_snapshot_too_large"
  assert_json_eq "$RESULT" '.details.maxBytes' '65536' "refusal names the configured cap"

  pt_get "/memory?tabId=${TAB_ID}"
  assert_ok "tab still answers after the aborted snapshot"

  end_test
}

with_server "$E2E_SECURE_SERVER" memory_disabled_tests
with_server "$E2E_MEDIUM_SERVER" memory_cap_tests
