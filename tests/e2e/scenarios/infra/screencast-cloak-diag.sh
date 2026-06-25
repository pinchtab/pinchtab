#!/bin/bash
# screencast-cloak-diag.sh — Diagnose screencast polling issues.
#
# Exercises the screencast pipeline step-by-step to isolate where frames
# are being lost in polling mode (Cloak / headless). Tests:
#
#   1. Direct /screenshot — does CaptureScreenshot work at all?
#   2. Direct /screencast WS — does the polling goroutine produce frames?
#   3. Dashboard proxy /screencast — does the two-hop proxy deliver frames?
#
# Run against a Cloak instance to reproduce the "no bytes in 4s" failure,
# or against Chrome to confirm the pipeline works end-to-end.

GROUP_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
source "${GROUP_DIR}/../../helpers/api.sh"

# ---------------------------------------------------------------------------
# 1. Direct screenshot — prove CaptureScreenshot itself works
# ---------------------------------------------------------------------------

start_test "screencast-diag: direct screenshot latency"

pt_post /navigate -d "{\"url\":\"${FIXTURES_URL}/evaluate.html\"}"
assert_ok "navigate to evaluate.html"
sleep 1

SCREENSHOT_START=$(get_time_ms)
SCREENSHOT_RAW=$(e2e_curl -s -o /dev/null -w "%{http_code} %{size_download} %{time_total}" \
  "${E2E_SERVER}/screenshot?format=jpeg&quality=30&maxWidth=320")
SCREENSHOT_END=$(get_time_ms)

SCREENSHOT_HTTP=$(echo "$SCREENSHOT_RAW" | awk '{print $1}')
SCREENSHOT_BYTES=$(echo "$SCREENSHOT_RAW" | awk '{print $2}')
SCREENSHOT_TIME=$(echo "$SCREENSHOT_RAW" | awk '{print $3}')
SCREENSHOT_LATENCY=$((SCREENSHOT_END - SCREENSHOT_START))

if [ "$SCREENSHOT_HTTP" = "200" ]; then
  pass_assert "screenshot returned 200 (${SCREENSHOT_BYTES} bytes, ${SCREENSHOT_TIME}s curl, ${SCREENSHOT_LATENCY}ms wall)"
else
  fail_assert "screenshot failed (HTTP ${SCREENSHOT_HTTP})"
fi

if [ "${SCREENSHOT_BYTES}" -gt 0 ] 2>/dev/null; then
  pass_assert "screenshot has content (${SCREENSHOT_BYTES} bytes)"
else
  fail_assert "screenshot returned 0 bytes"
fi

if [ "${SCREENSHOT_LATENCY}" -lt 3000 ]; then
  pass_assert "screenshot latency under 3s (${SCREENSHOT_LATENCY}ms)"
else
  fail_assert "screenshot latency too high (${SCREENSHOT_LATENCY}ms) — CaptureScreenshot is slow through CDP proxy"
fi

end_test

# ---------------------------------------------------------------------------
# 2. Direct screencast WebSocket — does polling produce frames?
# ---------------------------------------------------------------------------

start_test "screencast-diag: direct screencast WebSocket (polling)"

TAB_ID=$(get_tab_id)
WS_HEADERS=$(mktemp)
WS_BODY=$(mktemp)

(
  e2e_curl -s --http1.1 \
    -X GET \
    "${E2E_SERVER}/screencast?tabId=${TAB_ID}&fps=5&everyNthFrame=1&quality=20&maxWidth=320" \
    -D "${WS_HEADERS}" \
    -o "${WS_BODY}" \
    -H "Origin: ${E2E_SERVER}" \
    -H "Connection: Upgrade" \
    -H "Upgrade: websocket" \
    -H "Sec-WebSocket-Version: 13" \
    -H "Sec-WebSocket-Key: dGhlIHNhbXBsZSBub25jZQ==" \
    --max-time 15 >/dev/null 2>&1 || true
) &
WS_PID=$!

# Wait for upgrade
WS_STATUS=""
for _ in $(seq 1 20); do
  WS_STATUS=$(grep '^HTTP/' "${WS_HEADERS}" | tail -n 1 | awk '{print $2}')
  if [ -n "${WS_STATUS}" ]; then
    break
  fi
  sleep 0.2
done

if [ "${WS_STATUS}" = "101" ]; then
  pass_assert "screencast WS upgraded (101)"
else
  fail_assert "screencast WS upgrade failed (status: ${WS_STATUS:-none})"
fi

# Wait for first frame bytes — the key metric
FRAME_BYTES=0
FIRST_FRAME_START=$(get_time_ms)
for attempt in $(seq 1 40); do
  FRAME_BYTES=$(wc -c < "${WS_BODY}" | tr -d '[:space:]')
  if [ "${FRAME_BYTES}" -gt 0 ]; then
    break
  fi
  sleep 0.2
done
FIRST_FRAME_END=$(get_time_ms)
FIRST_FRAME_LATENCY=$((FIRST_FRAME_END - FIRST_FRAME_START))

if [ "${FRAME_BYTES}" -gt 0 ]; then
  pass_assert "first frame arrived (${FRAME_BYTES} bytes in ${FIRST_FRAME_LATENCY}ms)"
else
  fail_assert "no screencast frames received within 8s — polling goroutine likely errored silently"
fi

# Wait a bit longer and check if more frames are arriving
if [ "${FRAME_BYTES}" -gt 0 ]; then
  BEFORE_BYTES="${FRAME_BYTES}"
  sleep 2
  AFTER_BYTES=$(wc -c < "${WS_BODY}" | tr -d '[:space:]')
  DELTA=$((AFTER_BYTES - BEFORE_BYTES))
  if [ "${DELTA}" -gt 0 ]; then
    pass_assert "screencast is streaming (+${DELTA} bytes after 2s, total ${AFTER_BYTES})"
  else
    fail_assert "screencast stalled after first frame (stuck at ${BEFORE_BYTES} bytes)"
  fi
fi

kill "${WS_PID}" 2>/dev/null || true
wait "${WS_PID}" 2>/dev/null || true
rm -f "${WS_HEADERS}" "${WS_BODY}"

end_test

# ---------------------------------------------------------------------------
# 3. Dashboard proxy screencast (if E2E_FULL_SERVER is set)
# ---------------------------------------------------------------------------

if [ -n "${E2E_FULL_SERVER:-}" ]; then

start_test "screencast-diag: dashboard proxy screencast (polling)"

SAVED_SERVER="$E2E_SERVER"
E2E_SERVER="${E2E_FULL_SERVER}"

pt_get "/instances/tabs?fresh=1"
assert_ok "list tabs for proxy screencast"

INST_ID=$(echo "$RESULT" | jq -r '.[] | select((.url // "") | contains("evaluate.html")) | .instanceId // empty' | head -n 1)
PROXY_TAB_ID=$(echo "$RESULT" | jq -r '.[] | select((.url // "") | contains("evaluate.html")) | .id // empty' | head -n 1)

if [ -n "${INST_ID}" ] && [ -n "${PROXY_TAB_ID}" ]; then
  pass_assert "found instance ${INST_ID} with tab ${PROXY_TAB_ID:0:12}..."

  PROXY_WS_HEADERS=$(mktemp)
  PROXY_WS_BODY=$(mktemp)

  (
    e2e_curl -s --http1.1 \
      -X GET \
      "${E2E_SERVER}/instances/${INST_ID}/proxy/screencast?tabId=${PROXY_TAB_ID}&fps=5&everyNthFrame=1&quality=20&maxWidth=320" \
      -D "${PROXY_WS_HEADERS}" \
      -o "${PROXY_WS_BODY}" \
      -H "Origin: ${E2E_SERVER}" \
      -H "Connection: Upgrade" \
      -H "Upgrade: websocket" \
      -H "Sec-WebSocket-Version: 13" \
      -H "Sec-WebSocket-Key: dGhlIHNhbXBsZSBub25jZQ==" \
      --max-time 15 >/dev/null 2>&1 || true
  ) &
  PROXY_WS_PID=$!

  PROXY_WS_STATUS=""
  for _ in $(seq 1 20); do
    PROXY_WS_STATUS=$(grep '^HTTP/' "${PROXY_WS_HEADERS}" | tail -n 1 | awk '{print $2}')
    if [ -n "${PROXY_WS_STATUS}" ]; then
      break
    fi
    sleep 0.2
  done

  if [ "${PROXY_WS_STATUS}" = "101" ]; then
    pass_assert "proxy screencast WS upgraded (101)"
  else
    fail_assert "proxy screencast WS upgrade failed (status: ${PROXY_WS_STATUS:-none})"
  fi

  PROXY_FRAME_BYTES=0
  PROXY_START=$(get_time_ms)
  for _ in $(seq 1 40); do
    PROXY_FRAME_BYTES=$(wc -c < "${PROXY_WS_BODY}" | tr -d '[:space:]')
    if [ "${PROXY_FRAME_BYTES}" -gt 0 ]; then
      break
    fi
    sleep 0.2
  done
  PROXY_END=$(get_time_ms)
  PROXY_LATENCY=$((PROXY_END - PROXY_START))

  if [ "${PROXY_FRAME_BYTES}" -gt 0 ]; then
    pass_assert "proxy first frame arrived (${PROXY_FRAME_BYTES} bytes in ${PROXY_LATENCY}ms)"
  else
    fail_assert "proxy screencast: no frames within 8s — frames lost in proxy hop"
  fi

  kill "${PROXY_WS_PID}" 2>/dev/null || true
  wait "${PROXY_WS_PID}" 2>/dev/null || true
  rm -f "${PROXY_WS_HEADERS}" "${PROXY_WS_BODY}"
else
  skip_assert "no evaluate.html tab found — cannot test proxy screencast"
fi

E2E_SERVER="${SAVED_SERVER}"

end_test

fi
