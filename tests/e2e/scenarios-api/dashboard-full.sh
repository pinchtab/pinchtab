#!/bin/bash
# dashboard-full.sh — API full dashboard scenarios.

GROUP_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
source "${GROUP_DIR}/../helpers/api.sh"

if [ -z "${E2E_FULL_SERVER:-}" ]; then
  echo "  E2E_FULL_SERVER not set, skipping dashboard full scenarios"
  return 0 2>/dev/null || exit 0
fi

DASHBOARD_OLD_SERVER=""
DASHBOARD_WS_HEADERS_FILE=""
DASHBOARD_WS_BODY_FILE=""
DASHBOARD_WS_PID=""
DASHBOARD_WS_STATUS=""
DASHBOARD_FRAME_BYTES=0
DASHBOARD_TAB_ID=""
DASHBOARD_INST_ID=""

dashboard_use_full_server() {
  DASHBOARD_OLD_SERVER="$E2E_SERVER"
  E2E_SERVER="${E2E_FULL_SERVER:-$E2E_SERVER}"
}

dashboard_restore_server() {
  if [ -n "${DASHBOARD_OLD_SERVER}" ]; then
    E2E_SERVER="${DASHBOARD_OLD_SERVER}"
    DASHBOARD_OLD_SERVER=""
  fi
}

dashboard_cleanup_screencast() {
  if [ -n "${DASHBOARD_WS_PID}" ]; then
    kill "${DASHBOARD_WS_PID}" 2>/dev/null || true
    wait "${DASHBOARD_WS_PID}" 2>/dev/null || true
    DASHBOARD_WS_PID=""
  fi
  rm -f "${DASHBOARD_WS_HEADERS_FILE}" "${DASHBOARD_WS_BODY_FILE}"
  DASHBOARD_WS_HEADERS_FILE=""
  DASHBOARD_WS_BODY_FILE=""
}

dashboard_assert_instance_found() {
  pt_get /instances
  DASHBOARD_INST_ID=$(echo "$RESULT" | jq -r '.[0].id // empty')
  if [ -n "${DASHBOARD_INST_ID}" ]; then
    echo -e "  ${GREEN}✓${NC} found instance for screencast proxy: ${DASHBOARD_INST_ID}"
    ((ASSERTIONS_PASSED++)) || true
  else
    echo -e "  ${RED}✗${NC} could not find instance for screencast proxy"
    ((ASSERTIONS_FAILED++)) || true
  fi
}

dashboard_start_screencast_stream() {
  DASHBOARD_WS_HEADERS_FILE=$(mktemp)
  DASHBOARD_WS_BODY_FILE=$(mktemp)
  (
    e2e_curl -s --http1.1 \
      -X GET \
      "${E2E_SERVER}/instances/${DASHBOARD_INST_ID}/proxy/screencast?tabId=${DASHBOARD_TAB_ID}&fps=5&everyNthFrame=1&quality=20&maxWidth=320" \
      -D "${DASHBOARD_WS_HEADERS_FILE}" \
      -o "${DASHBOARD_WS_BODY_FILE}" \
      -H "Origin: ${E2E_SERVER}" \
      -H "Connection: Upgrade" \
      -H "Upgrade: websocket" \
      -H "Sec-WebSocket-Version: 13" \
      -H "Sec-WebSocket-Key: dGhlIHNhbXBsZSBub25jZQ==" \
      --max-time 5 >/dev/null 2>&1 || true
  ) &
  DASHBOARD_WS_PID=$!

  DASHBOARD_WS_STATUS=""
  for _ in $(seq 1 20); do
    DASHBOARD_WS_STATUS=$(grep '^HTTP/' "${DASHBOARD_WS_HEADERS_FILE}" | tail -n 1 | awk '{print $2}')
    if [ -n "${DASHBOARD_WS_STATUS}" ]; then
      break
    fi
    sleep 0.2
  done
}

dashboard_assert_screencast_streaming() {
  if [ "${DASHBOARD_WS_STATUS}" = "101" ]; then
    echo -e "  ${GREEN}✓${NC} screencast websocket upgraded"
    ((ASSERTIONS_PASSED++)) || true
  else
    echo -e "  ${RED}✗${NC} screencast websocket upgrade failed (status: ${DASHBOARD_WS_STATUS:-none})"
    ((ASSERTIONS_FAILED++)) || true
  fi

  DASHBOARD_FRAME_BYTES=0
  if [ "${DASHBOARD_WS_STATUS}" = "101" ]; then
    for _ in $(seq 1 20); do
      DASHBOARD_FRAME_BYTES=$(wc -c < "${DASHBOARD_WS_BODY_FILE}" | tr -d '[:space:]')
      if [ "${DASHBOARD_FRAME_BYTES}" -gt 0 ]; then
        break
      fi
      sleep 0.2
    done
  fi

  if [ "${DASHBOARD_FRAME_BYTES}" -gt 0 ]; then
    echo -e "  ${GREEN}✓${NC} screencast streamed bytes (${DASHBOARD_FRAME_BYTES})"
    ((ASSERTIONS_PASSED++)) || true
  else
    echo -e "  ${RED}✗${NC} screencast did not stream any bytes"
    ((ASSERTIONS_FAILED++)) || true
  fi
}

dashboard_assert_screencast_continues() {
  local label="$1"
  local before_bytes="${DASHBOARD_FRAME_BYTES:-0}"
  local current_bytes="$before_bytes"

  if [ "${DASHBOARD_WS_STATUS}" = "101" ]; then
    for _ in $(seq 1 20); do
      current_bytes=$(wc -c < "${DASHBOARD_WS_BODY_FILE}" | tr -d '[:space:]')
      if [ "${current_bytes}" -gt "${before_bytes}" ]; then
        break
      fi
      sleep 0.2
    done
  fi

  if [ "${current_bytes}" -gt "${before_bytes}" ]; then
    DASHBOARD_FRAME_BYTES="${current_bytes}"
    echo -e "  ${GREEN}✓${NC} screencast kept streaming ${label} (${current_bytes} bytes)"
    ((ASSERTIONS_PASSED++)) || true
  else
    echo -e "  ${RED}✗${NC} screencast stopped streaming ${label}"
    ((ASSERTIONS_FAILED++)) || true
  fi
}

dashboard_assert_screencast_hidden() {
  local label="$1"
  pt_post /evaluate -d '{"expression":"({ hasGlobal: Object.prototype.hasOwnProperty.call(globalThis, \"__pinchtabScreencastRepaint\"), hasIdentifiableElement: !!document.querySelector(\"[id*=pinchtab],[class*=pinchtab],[data-pinchtab]\") })"}'
  assert_ok "evaluate main world ${label}"
  assert_result_eq ".result.hasGlobal" "false" "main world cannot see screencast global ${label}"
  assert_result_eq ".result.hasIdentifiableElement" "false" "no pinchtab-identifiable element leaks into DOM ${label}"
}

# ─────────────────────────────────────────────────────────────────
start_test "dashboard: screencast injection stays isolated from main world"

dashboard_use_full_server

pt_post /navigate -d "{\"url\":\"${FIXTURES_URL}/evaluate.html\"}"
assert_ok "navigate for screencast isolation"
DASHBOARD_TAB_ID=$(get_tab_id)
sleep 1

dashboard_assert_instance_found
dashboard_start_screencast_stream
dashboard_assert_screencast_streaming
dashboard_assert_screencast_hidden "during screencast"

pt_post /action -d '{"kind":"scroll","scrollY":100}'
assert_ok "scroll while screencast active"
sleep 1

dashboard_assert_screencast_continues "after scroll"
dashboard_assert_screencast_hidden "after scroll"

dashboard_cleanup_screencast
dashboard_restore_server

end_test
