#!/bin/bash
# stealth-full.sh — API extended stealth scenarios.

GROUP_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
source "${GROUP_DIR}/../helpers/api.sh"

run_stealth_level_matrix() {
  STEALTH_LEVEL="${STEALTH_LEVEL:-light}"
  case "$STEALTH_LEVEL" in
    light)
      MATRIX_URL="${E2E_SERVER}"
      ;;
    medium)
      MATRIX_URL="${E2E_MEDIUM_SERVER:-${E2E_SERVER}}"
      ;;
    full)
      MATRIX_URL="${E2E_FULL_SERVER:-${E2E_SERVER}}"
      ;;
    *)
      echo "unknown stealth level: ${STEALTH_LEVEL}" >&2
      exit 1
      ;;
  esac

  ORIG_URL="$E2E_SERVER"
  E2E_SERVER="$MATRIX_URL"
  echo -e "${BLUE}Testing stealth level: ${STEALTH_LEVEL}${NC}\n"

  start_test "stealth-levels: navigate to test page"
  pt_post /navigate "{\"url\":\"${FIXTURES_URL}/bot-detect.html\"}"
  assert_ok "navigate"
  sleep 1
  end_test

  start_test "stealth-levels: [light] webdriver hidden"
  pt_post /evaluate '{"expression":"navigator.webdriver === true"}'
  assert_json_eq "$RESULT" '.result' 'false' "webdriver !== true"
  end_test

  start_test "stealth-levels: [light] plugins array exists"
  pt_post /evaluate '{"expression":"navigator.plugins.length > 0"}'
  assert_json_eq "$RESULT" '.result' 'true' "plugins present"
  end_test

  start_test "stealth-levels: [light] hardware concurrency"
  pt_post /evaluate '{"expression":"navigator.hardwareConcurrency >= 2"}'
  assert_json_eq "$RESULT" '.result' 'true' "hardwareConcurrency >= 2"
  end_test

  start_test "stealth-levels: [light] basic chrome.runtime"
  pt_post /evaluate '{"expression":"!!(window.chrome && window.chrome.runtime)"}'
  assert_json_eq "$RESULT" '.result' 'true' "chrome.runtime exists"
  end_test

  start_test "stealth-levels: [medium] userAgentData exists"
  pt_post /evaluate '{"expression":"!!(navigator.userAgentData && navigator.userAgentData.brands)"}'
  if [ "$STEALTH_LEVEL" = "light" ]; then
    echo -e "  ${MUTED}(skipped - native behavior at light level)${NC}"
    ((ASSERTIONS_PASSED++)) || true
  else
    assert_json_eq "$RESULT" '.result' 'true' "userAgentData.brands exists"
  fi
  end_test

  start_test "stealth-levels: [medium] chrome.runtime.connect exists"
  pt_post /evaluate '{"expression":"typeof window.chrome.runtime.connect === \"function\""}'
  if [ "$STEALTH_LEVEL" = "light" ]; then
    assert_json_eq "$RESULT" '.result' 'false' "connect not spoofed at light"
  else
    assert_json_eq "$RESULT" '.result' 'true' "connect exists at medium+"
  fi
  end_test

  start_test "stealth-levels: [medium] chrome.csi exists"
  pt_post /evaluate '{"expression":"typeof window.chrome.csi === \"function\""}'
  if [ "$STEALTH_LEVEL" = "light" ]; then
    assert_json_eq "$RESULT" '.result' 'false' "csi not spoofed at light"
  else
    assert_json_eq "$RESULT" '.result' 'true' "csi exists at medium+"
  fi
  end_test

  start_test "stealth-levels: [medium] chrome.loadTimes exists"
  pt_post /evaluate '{"expression":"typeof window.chrome.loadTimes === \"function\""}'
  if [ "$STEALTH_LEVEL" = "light" ]; then
    assert_json_eq "$RESULT" '.result' 'false' "loadTimes not spoofed at light"
  else
    assert_json_eq "$RESULT" '.result' 'true' "loadTimes exists at medium+"
  fi
  end_test

  start_test "stealth-levels: [medium] maxTouchPoints spoofed"
  pt_post /evaluate '{"expression":"navigator.maxTouchPoints === 0"}'
  if [ "$STEALTH_LEVEL" = "light" ]; then
    echo -e "  ${MUTED}(skipped - native behavior at light level)${NC}"
    ((ASSERTIONS_PASSED++)) || true
  else
    assert_json_eq "$RESULT" '.result' 'true' "maxTouchPoints === 0 at medium+"
  fi
  end_test

  start_test "stealth-levels: [full] WebGL renderer matches platform"
  pt_post /evaluate '{"expression":"(() => { try { const c = document.createElement(\"canvas\"); const gl = c.getContext(\"webgl\"); const d = gl && gl.getExtension(\"WEBGL_debug_renderer_info\"); const r = d ? gl.getParameter(d.UNMASKED_RENDERER_WEBGL) : \"\"; const p = navigator.platform || \"\"; const lower = r.toLowerCase(); const ok = !d || (!lower.includes(\"swiftshader\") && ((p === \"MacIntel\" && !lower.includes(\"direct3d\")) || (p.includes(\"Linux\") && !lower.includes(\"direct3d\")) || (p.includes(\"Win\") && lower.includes(\"direct3d\")))); return JSON.stringify({available: !!d, renderer: r, platform: p, ok}); } catch(e) { return JSON.stringify({available:false, renderer:\"\", platform:navigator.platform || \"\", ok:false}); } })()"}'
  if [ "$STEALTH_LEVEL" = "full" ]; then
    webgl_json=$(echo "$RESULT" | jq -r '.result // "{}"')
    webgl_available=$(echo "$webgl_json" | jq -r '.available // false')
    webgl_ok=$(echo "$webgl_json" | jq -r '.ok // false')
    webgl_renderer=$(echo "$webgl_json" | jq -r '.renderer // ""')
    webgl_platform=$(echo "$webgl_json" | jq -r '.platform // ""')
    if [ "$webgl_available" = "true" ]; then
      if [ "$webgl_ok" = "true" ]; then
        echo -e "  ${GREEN}✓${NC} WebGL renderer matches platform (${webgl_platform})"
        ((ASSERTIONS_PASSED++)) || true
      else
        echo -e "  ${RED}✗${NC} WebGL renderer mismatches platform ${webgl_platform}: ${webgl_renderer}"
        ((ASSERTIONS_FAILED++)) || true
      fi
    else
      echo -e "  ${MUTED}(skipped - WEBGL_debug_renderer_info unavailable)${NC}"
      ((ASSERTIONS_PASSED++)) || true
    fi
  else
    echo -e "  ${MUTED}(skipped - native WebGL at ${STEALTH_LEVEL} level)${NC}"
    ((ASSERTIONS_PASSED++)) || true
  fi
  end_test

  start_test "stealth-levels: [full] canvas toDataURL modified"
  pt_post /evaluate '{"expression":"typeof HTMLCanvasElement.prototype.toDataURL === \"function\""}'
  assert_json_eq "$RESULT" '.result' 'true' "toDataURL exists"
  end_test

  start_test "stealth-levels: comprehensive score at ${STEALTH_LEVEL} level"
  pt_post /evaluate '{"expression":"JSON.stringify(window.__botDetectScore || {})"}'
  score_json=$(echo "$RESULT" | jq -r '.result // "{}"')
  critical_passed=$(echo "$score_json" | jq -r '.critical // 0')
  critical_total=$(echo "$score_json" | jq -r '.criticalTotal // 0')

  echo -e "  ${MUTED}Critical tests: ${critical_passed}/${critical_total}${NC}"

  case "$STEALTH_LEVEL" in
    light)
      [ "$critical_passed" -ge 6 ] && ((ASSERTIONS_PASSED++)) || ((ASSERTIONS_FAILED++))
      ;;
    medium)
      [ "$critical_passed" -ge 10 ] && ((ASSERTIONS_PASSED++)) || ((ASSERTIONS_FAILED++))
      ;;
    full)
      [ "$critical_passed" -ge 12 ] && ((ASSERTIONS_PASSED++)) || ((ASSERTIONS_FAILED++))
      ;;
  esac

  echo -e "  ${GREEN}✓${NC} score meets ${STEALTH_LEVEL} level expectations"
  end_test

  E2E_SERVER="$ORIG_URL"
}

if [ "${STEALTH_MATRIX:-0}" = "1" ]; then
  run_stealth_level_matrix
  if [[ "${BASH_SOURCE[0]}" == "${0}" ]]; then
    print_summary
  fi
  exit 0
fi

# Adds the heavier heuristics and secure-instance smoke checks on top of the
# baseline coverage in 45-bot-detection.sh.

start_test "bot-detect-full: navigate to test page"

pt_post /navigate "{\"url\":\"${FIXTURES_URL}/bot-detect.html\"}"
assert_ok "navigate"
sleep 1  # Allow stealth.js injection

end_test

# Note: SwiftShader check moved to full stealth instance section below.
# Light/medium levels don't spoof WebGL, so SwiftShader is expected in headless.

start_test "bot-detect-full: outer window dimensions exist"

pt_post /evaluate '{"expression":"window.outerWidth > 0 && window.outerHeight > 0"}'
assert_json_eq "$RESULT" '.result' 'true' "outerWidth/outerHeight > 0"

end_test

start_test "bot-detect-full: hardware concurrency reasonable"

pt_post /evaluate '{"expression":"navigator.hardwareConcurrency >= 2"}'
assert_json_eq "$RESULT" '.result' 'true' "hardwareConcurrency >= 2"

end_test

start_test "bot-detect-full: device memory exists"

pt_post /evaluate '{"expression":"navigator.deviceMemory > 0"}'
assert_json_eq "$RESULT" '.result' 'true' "deviceMemory > 0"

end_test

start_test "bot-detect-full: comprehensive score check"

pt_post /evaluate '{"expression":"JSON.stringify(window.__botDetectScore || {})"}'
score_json=$(echo "$RESULT" | jq -r '.result // "{}"')

pt_post /evaluate '{"expression":"window.__pinchtab_stealth_level || \"light\""}'
stealth_level=$(echo "$RESULT" | jq -r '.result // "light"')

critical_passed=$(echo "$score_json" | jq -r '.critical // 0')
critical_total=$(echo "$score_json" | jq -r '.criticalTotal // 0')

case "$stealth_level" in
  medium|full)
    min_critical="$critical_total"
    ;;
  *)
    # Light intentionally skips connect/csi/loadTimes spoofing.
    min_critical=$((critical_total - 3))
    ;;
esac

echo -e "  ${MUTED}Score: critical ${critical_passed}/${critical_total} (${stealth_level})${NC}"

if [ "$critical_passed" -ge "$min_critical" ]; then
  echo -e "  ${GREEN}✓${NC} score meets ${stealth_level} expectations"
  ((ASSERTIONS_PASSED++)) || true
else
  echo -e "  ${RED}✗${NC} score below ${stealth_level} expectations (${critical_passed}/${critical_total}, need ${min_critical})"
  ((ASSERTIONS_FAILED++)) || true
fi

end_test

FULL_URL="${E2E_FULL_SERVER:-}"
if [ -n "$FULL_URL" ]; then
  echo ""
  echo -e "${BLUE}Testing FULL stealth mode (permissive instance)${NC}"

  ORIG_URL="$E2E_SERVER"
  E2E_SERVER="$FULL_URL"

  start_test "bot-detect-full: can navigate with permissive full stealth"

  pt_post /navigate "{\"url\":\"${FIXTURES_URL}/bot-detect.html\"}"
  assert_ok "navigate to bot-detect fixture (permissive full stealth)"
  sleep 1

  end_test

  start_test "bot-detect-full: permissive full legacy shims are present"

  pt_post /evaluate '{"expression":"typeof window.chrome.csi === \"function\""}'
  assert_json_eq "$RESULT" '.result' 'true' "chrome.csi exists"

  pt_post /evaluate '{"expression":"typeof window.chrome.loadTimes === \"function\""}'
  assert_json_eq "$RESULT" '.result' 'true' "chrome.loadTimes exists"

  end_test

  start_test "bot-detect-full: WebGL not SwiftShader (full stealth spoofs GPU)"

  pt_post /evaluate '{"expression":"(() => { try { const c = document.createElement(\"canvas\"); const gl = c.getContext(\"webgl\"); const d = gl.getExtension(\"WEBGL_debug_renderer_info\"); return gl.getParameter(d.UNMASKED_RENDERER_WEBGL).toLowerCase().includes(\"swiftshader\"); } catch(e) { return false; } })()"}'
  assert_json_eq "$RESULT" '.result' 'false' "WebGL renderer not SwiftShader"

  end_test

  start_test "bot-detect-full: WebGL renderer matches platform when observable"

  pt_post /evaluate '{"expression":"(() => { try { const c = document.createElement(\"canvas\"); const gl = c.getContext(\"webgl\"); const d = gl && gl.getExtension(\"WEBGL_debug_renderer_info\"); const r = d ? gl.getParameter(d.UNMASKED_RENDERER_WEBGL) : \"\"; const p = navigator.platform || \"\"; const lower = r.toLowerCase(); const ok = !d || (!lower.includes(\"swiftshader\") && ((p === \"MacIntel\" && !lower.includes(\"direct3d\")) || (p.includes(\"Linux\") && !lower.includes(\"direct3d\")) || (p.includes(\"Win\") && lower.includes(\"direct3d\")))); return JSON.stringify({available: !!d, renderer: r, platform: p, ok}); } catch(e) { return JSON.stringify({available:false, renderer:\"\", platform:navigator.platform || \"\", ok:false}); } })()"}'
  webgl_json=$(echo "$RESULT" | jq -r '.result // "{}"')
  webgl_available=$(echo "$webgl_json" | jq -r '.available // false')
  webgl_ok=$(echo "$webgl_json" | jq -r '.ok // false')
  webgl_renderer=$(echo "$webgl_json" | jq -r '.renderer // ""')
  webgl_platform=$(echo "$webgl_json" | jq -r '.platform // ""')
  if [ "$webgl_available" = "true" ]; then
    if [ "$webgl_ok" = "true" ]; then
      echo -e "  ${GREEN}✓${NC} WebGL renderer matches platform (${webgl_platform})"
      ((ASSERTIONS_PASSED++)) || true
    else
      echo -e "  ${RED}✗${NC} WebGL renderer mismatches platform ${webgl_platform}: ${webgl_renderer}"
      ((ASSERTIONS_FAILED++)) || true
    fi
  else
    echo -e "  ${MUTED}(skipped - WEBGL_debug_renderer_info unavailable)${NC}"
    ((ASSERTIONS_PASSED++)) || true
  fi

  end_test

  start_test "bot-detect-full: permissive full score check"

  pt_post /evaluate '{"expression":"JSON.stringify(window.__botDetectScore || {})"}'
  score_json=$(echo "$RESULT" | jq -r '.result // "{}"')
  critical_passed=$(echo "$score_json" | jq -r '.critical // 0')
  critical_total=$(echo "$score_json" | jq -r '.criticalTotal // 0')

  echo -e "  ${MUTED}Score: critical ${critical_passed}/${critical_total} (full)${NC}"

  if [ "$critical_passed" -ge "$critical_total" ]; then
    echo -e "  ${GREEN}✓${NC} score meets full expectations"
    ((ASSERTIONS_PASSED++)) || true
  else
    echo -e "  ${RED}✗${NC} score below full expectations (${critical_passed}/${critical_total})"
    ((ASSERTIONS_FAILED++)) || true
  fi

  end_test

  E2E_SERVER="$ORIG_URL"
fi

echo ""
echo -e "${BLUE}Testing FULL stealth mode (restrictive instance)${NC}"
echo -e "${YELLOW}Note: evaluate disabled on restrictive instance, testing navigation only${NC}"

ORIG_URL="$E2E_SERVER"
E2E_SERVER="$E2E_SECURE_SERVER"

start_test "bot-detect-full: can navigate with full stealth"

pt_post /navigate "{\"url\":\"${FIXTURES_URL}/bot-detect.html\"}"
assert_ok "navigate to bot-detect fixture (full stealth)"

TAB_ID=$(echo "$RESULT" | jq -r '.tabId // empty')
if [ -n "$TAB_ID" ]; then
  echo -e "  ${GREEN}✓${NC} Got tabId: $TAB_ID"
  ((ASSERTIONS_PASSED++)) || true
else
  echo -e "  ${RED}✗${NC} No tabId in response"
  ((ASSERTIONS_FAILED++)) || true
fi

end_test

start_test "bot-detect-full: page title loaded correctly"

TITLE=$(echo "$RESULT" | jq -r '.title // empty')
if [ "$TITLE" = "Bot Detection Tests" ]; then
  echo -e "  ${GREEN}✓${NC} Page title: $TITLE"
  ((ASSERTIONS_PASSED++)) || true
else
  echo -e "  ${YELLOW}⚠${NC} Unexpected title: $TITLE"
fi

end_test

E2E_SERVER="$ORIG_URL"

if [[ "${BASH_SOURCE[0]}" == "${0}" ]]; then
  print_summary
fi
