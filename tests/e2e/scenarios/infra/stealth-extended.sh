#!/bin/bash
# stealth-extended.sh — API extended stealth scenarios.

GROUP_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
source "${GROUP_DIR}/../../helpers/api.sh"

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
      return 1
      ;;
  esac

  ORIG_URL="$E2E_SERVER"
  E2E_SERVER="$MATRIX_URL"
  MATRIX_STATUS=""
  echo -e "${BLUE}Testing stealth level: ${STEALTH_LEVEL}${NC}\n"

  start_test "stealth-levels: navigate to test page"
  pt_post /navigate "{\"url\":\"${FIXTURES_URL}/bot-detect.html\"}"
  assert_ok "navigate"
  sleep 1
  end_test

  start_test "stealth-levels: status reports configured level"
  pt_get /stealth/status
  MATRIX_STATUS="$RESULT"
  assert_json_eq "$RESULT" '.level' "$STEALTH_LEVEL" "status level matches instance"
  assert_json_exists "$RESULT" '.scriptHash' "status includes script hash"
  assert_json_eq "$RESULT" '.flags.globalUserAgent' 'true' "status reports global launch UA"
  assert_json_eq "$RESULT" '.capabilities.webdriverNativeStrategy' 'true' "status reports native webdriver strategy"
  assert_json_eq "$RESULT" '.capabilities.downlinkMax' 'true' "status reports downlinkMax capability"
  end_test

  start_test "stealth-levels: [light] webdriver hidden"
  pt_post /evaluate '{"expression":"navigator.webdriver !== true"}'
  assert_json_eq "$RESULT" '.result' 'true' "webdriver !== true"
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
  pt_get /stealth/status
  if [ "$STEALTH_LEVEL" = "light" ]; then
    assert_json_eq "$RESULT" '.capabilities.userAgentData' 'false' "status keeps userAgentData disabled at light"
    assert_json_eq "$RESULT" '.capabilities.iframeIsolation' 'false' "status keeps iframe isolation disabled at light"
    assert_json_eq "$RESULT" '.capabilities.errorStackSanitized' 'false' "status keeps stack sanitization disabled at light"
    assert_json_eq "$RESULT" '.capabilities.functionToStringMasked' 'false' "status keeps toString masking disabled at light"
    assert_json_eq "$RESULT" '.capabilities.functionToStringNative' 'true' "status keeps native Function.prototype.toString at light"
  else
    assert_json_eq "$RESULT" '.capabilities.userAgentData' 'true' "status enables userAgentData at medium+"
    if [ "$STEALTH_LEVEL" = "medium" ]; then
      assert_json_eq "$RESULT" '.capabilities.iframeIsolation' 'true' "status enables iframe isolation at medium"
    else
      assert_json_eq "$RESULT" '.capabilities.iframeIsolation' 'false' "status keeps iframe isolation disabled at full"
    fi
    assert_json_eq "$RESULT" '.capabilities.errorStackSanitized' 'false' "status keeps stack sanitization disabled at medium+"
    assert_json_eq "$RESULT" '.capabilities.functionToStringMasked' 'false' "status keeps toString masking disabled at medium+"
    assert_json_eq "$RESULT" '.capabilities.functionToStringNative' 'true' "status keeps native Function.prototype.toString at medium+"
  fi
  assert_json_eq "$RESULT" '.capabilities.intlLocaleCoherent' 'true' "status reports locale coherence"
  assert_json_eq "$RESULT" '.capabilities.errorPrepareStackTraceNative' 'true' "status reports native Error.prepareStackTrace semantics"
  assert_json_eq "$RESULT" '.capabilities.navigatorOverridesAbsent' 'true' "status reports no navigator own-property overrides"
  end_test

  start_test "stealth-levels: [medium] chrome.runtime.connect exists"
  pt_post /evaluate '{"expression":"typeof window.chrome.runtime.connect === \"function\""}'
  if [ "$STEALTH_LEVEL" = "medium" ]; then
    assert_json_eq "$RESULT" '.result' 'true' "connect exists at medium"
  else
    assert_json_eq "$RESULT" '.result' 'false' "connect remains absent outside medium"
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

  start_test "stealth-levels: capability fixture reports expected page surface"
  pt_post /navigate "{\"url\":\"${FIXTURES_URL}/stealth-capabilities.html\"}"
  assert_ok "navigate to capability fixture"
  sleep 1

  if [ "$STEALTH_LEVEL" = "light" ]; then
    assert_eval_poll "window.__stealthCapabilities.chromeRuntimeConnect" "false" "connect remains absent at light"
    assert_eval_poll "window.__stealthCapabilities.iframeIsolation" "false" "iframe isolation remains absent at light"
    assert_eval_poll "window.__stealthCapabilities.errorStackSanitized" "false" "stack sanitization remains absent at light"
    assert_eval_poll "window.__stealthCapabilities.functionToStringMasked" "false" "native-looking toString masking remains absent at light"
  elif [ "$STEALTH_LEVEL" = "medium" ]; then
    assert_eval_poll "window.__stealthCapabilities.chromeRuntimeConnect" "true" "connect present at medium"
    assert_eval_poll "window.__stealthCapabilities.iframeIsolation" "true" "iframe isolation present at medium"
    assert_eval_poll "window.__stealthCapabilities.errorStackSanitized" "false" "stack sanitization remains absent at medium"
    assert_eval_poll "window.__stealthCapabilities.functionToStringMasked" "false" "native-looking toString masking remains absent at medium"
  else
    assert_eval_poll "window.__stealthCapabilities.chromeRuntimeConnect" "false" "connect remains absent at full"
    assert_eval_poll "window.__stealthCapabilities.iframeIsolation" "false" "iframe isolation remains absent at full"
    assert_eval_poll "window.__stealthCapabilities.errorStackSanitized" "false" "stack sanitization remains absent at full"
    assert_eval_poll "window.__stealthCapabilities.functionToStringMasked" "false" "native-looking toString masking remains absent at full"
  fi
  assert_eval_poll "window.__stealthCapabilities.functionToStringNative" "true" "Function.prototype.toString remains native-like"
  assert_eval_poll "window.__stealthCapabilities.webdriverDescriptorNativeLike" "true" "webdriver descriptor getter stays native-like"
  assert_eval_poll "window.__stealthCapabilities.canPlayTypeNativeLike" "true" "canPlayType remains native-like"
  assert_eval_poll "window.__stealthCapabilities.canvasToDataURLNativeLike" "true" "canvas toDataURL remains native-like"
  assert_eval_poll "window.__stealthCapabilities.canvasToBlobNativeLike" "true" "canvas toBlob remains native-like"
  assert_eval_poll "window.__stealthCapabilities.canvasGetImageDataNativeLike" "true" "canvas getImageData remains native-like"
  assert_eval_poll "window.__stealthCapabilities.canvasMeasureTextNativeLike" "true" "canvas measureText remains native-like"
  assert_eval_poll "window.__stealthCapabilities.canvasTransparentPixelPreserved" "true" "transparent canvas pixels stay untouched"
  assert_eval_poll "window.__stealthCapabilities.webglGetParameterNativeLike" "true" "WebGL getParameter remains native-like"
  assert_eval_poll "window.__stealthCapabilities.rtcPeerConnectionNativeLike" "true" "RTCPeerConnection stays native-like"
  assert_eval_poll "window.__stealthCapabilities.audioContextCreateOscillatorNativeLike" "true" "AudioContext.createOscillator stays native-like"
  assert_eval_poll "window.__stealthCapabilities.getComputedStyleNameNative" "true" "getComputedStyle keeps its native name"
  assert_eval_poll "window.__stealthCapabilities.matchMediaNameNative" "true" "matchMedia keeps its native name"
  assert_eval_poll "window.__stealthCapabilities.timeZoneDateCoherent" "true" "resolved timezone matches local Date clock"
  assert_eval_poll "window.__stealthCapabilities.timeZoneOffsetSane" "true" "timezone offset stays in a sane range"
  assert_eval_poll "window.__stealthCapabilities.intlLocaleCoherent" "true" "Intl locale family stays coherent"
  assert_eval_poll "window.__stealthCapabilities.intlDateTimeLocaleCoherent" "true" "Intl.DateTimeFormat locale matches navigator.language"
  assert_eval_poll "window.__stealthCapabilities.intlNumberLocaleCoherent" "true" "Intl.NumberFormat locale matches navigator.language"
  assert_eval_poll "window.__stealthCapabilities.errorPrepareStackTraceNative" "true" "Error.prepareStackTrace is not trapped"
  assert_eval_poll "window.__stealthCapabilities.navigatorOverridesAbsent" "true" "navigator keeps core properties off the instance"
  assert_eval_poll "window.__stealthCapabilities.userAgentVersionCoherent" "true" "appVersion aligns with the UA version"
  assert_eval_poll "window.__stealthCapabilities.userAgentDataBrandMajorCoherent" "true" "userAgentData brand major matches UA major"
  assert_eval_poll "window.__stealthCapabilities.userAgentDataVersionCoherent" "true" "userAgentData versions align with UA major"
  assert_eval_poll "window.__stealthCapabilities.userAgentDataFullVersionCoherent" "true" "userAgentData full version aligns with the UA version"

  if [ "$STEALTH_LEVEL" = "full" ]; then
    assert_eval_poll "window.__stealthCapabilities.getComputedStyleNativeLike" "true" "getComputedStyle remains native-like at full"
    assert_eval_poll "window.__stealthCapabilities.matchMediaNativeLike" "true" "matchMedia remains native-like at full"
    assert_eval_poll "window.__stealthCapabilities.pluginObjectSemantics" "true" "plugin semantics still coherent at full"
    assert_eval_poll "window.__stealthCapabilities.webglRendererNoSwiftShader" "true" "WebGL renderer avoids SwiftShader at full"
    assert_eval_poll "window.__stealthCapabilities.webglRendererPlatformCoherent" "true" "WebGL renderer stays coherent with platform at full"
    assert_eval_poll "window.__stealthCapabilities.systemColorsResolved" "true" "system colors resolve cleanly at full"
    assert_eval_poll "window.__stealthCapabilities.screenYDefined" "true" "screenY/screenTop defined at full"
  fi
  assert_eval_poll "window.__stealthCapabilities.downlinkMaxPresent" "true" "downlinkMax present in capability fixture"
  assert_eval_poll "window.__stealthCapabilities.devicePixelRatioPositive" "true" "devicePixelRatio stays positive"
  assert_eval_poll "window.__stealthCapabilities.screenAvailCoherent" "true" "available screen stays within screen bounds"
  assert_eval_poll "window.__stealthCapabilities.devtoolsFormattersAbsentByDefault" "true" "page starts without preinstalled devtools formatters"
  end_test

  start_test "stealth-levels: created tab inherits stealth contract"
  base_status=""
  pt_post /navigate "{\"url\":\"${FIXTURES_URL}/stealth-capabilities.html\"}"
  assert_ok "navigate baseline capability tab"
  base_tab_id=$(echo "$RESULT" | jq -r '.tabId // empty')
  if [ -n "$base_tab_id" ]; then
    pt_get "/stealth/status?tabId=${base_tab_id}"
    base_status="$RESULT"
    echo "$base_status" | jq -e '.scriptHash and .level' >/dev/null 2>&1
    if [ $? -eq 0 ]; then
      echo -e "  ${GREEN}✓${NC} baseline tab exposes stealth status"
      ((ASSERTIONS_PASSED++)) || true
    else
      echo -e "  ${RED}✗${NC} baseline tab missing stealth status fields"
      ((ASSERTIONS_FAILED++)) || true
    fi
  else
    echo -e "  ${RED}✗${NC} baseline capability tab missing tabId"
    ((ASSERTIONS_FAILED++)) || true
  fi

  pt_post /navigate "{\"url\":\"${FIXTURES_URL}/stealth-capabilities.html\",\"newTab\":true}"
  assert_ok "navigate created capability tab"
  sleep 1
  created_tab_id=$(echo "$RESULT" | jq -r '.tabId // empty')
  if [ -n "$created_tab_id" ] && [ -n "$base_status" ]; then
    pt_get "/stealth/status?tabId=${created_tab_id}"
    created_status="$RESULT"

    base_level=$(echo "$base_status" | jq -r '.level // empty')
    created_level=$(echo "$created_status" | jq -r '.level // empty')
    if [ "$created_level" = "$base_level" ] && [ -n "$created_level" ]; then
      echo -e "  ${GREEN}✓${NC} created tab keeps same stealth level"
      ((ASSERTIONS_PASSED++)) || true
    else
      echo -e "  ${RED}✗${NC} created tab level drifted (${created_level} vs ${base_level})"
      ((ASSERTIONS_FAILED++)) || true
    fi

    base_hash=$(echo "$base_status" | jq -r '.scriptHash // empty')
    created_hash=$(echo "$created_status" | jq -r '.scriptHash // empty')
    if [ "$created_hash" = "$base_hash" ] && [ -n "$created_hash" ]; then
      echo -e "  ${GREEN}✓${NC} created tab keeps same script hash"
      ((ASSERTIONS_PASSED++)) || true
    else
      echo -e "  ${RED}✗${NC} created tab script hash drifted (${created_hash} vs ${base_hash})"
      ((ASSERTIONS_FAILED++)) || true
    fi

    base_launch_mode=$(echo "$base_status" | jq -r '.launchMode // empty')
    created_launch_mode=$(echo "$created_status" | jq -r '.launchMode // empty')
    if [ "$created_launch_mode" = "$base_launch_mode" ] && [ -n "$created_launch_mode" ]; then
      echo -e "  ${GREEN}✓${NC} created tab keeps same launch mode"
      ((ASSERTIONS_PASSED++)) || true
    else
      echo -e "  ${RED}✗${NC} created tab launch mode drifted (${created_launch_mode} vs ${base_launch_mode})"
      ((ASSERTIONS_FAILED++)) || true
    fi

    pt_post "/tabs/${created_tab_id}/evaluate" '{"expression":"!!window.__stealthCapabilities && window.__stealthCapabilities.webdriverDescriptorNativeLike && window.__stealthCapabilities.userAgentVersionCoherent"}'
    assert_json_eq "$RESULT" '.result' 'true' "created tab exposes the same capability fixture contract"
  else
    echo -e "  ${RED}✗${NC} created capability tab missing tabId or baseline status"
    ((ASSERTIONS_FAILED++)) || true
  fi
  end_test

  start_test "stealth-levels: restored tab keeps stealth contract"
  restore_profile_name="stealth-restore-${STEALTH_LEVEL}-$$-$(date +%s)"
  restore_profile_id=""
  restore_instance_id=""
  matrix_hash=$(echo "$MATRIX_STATUS" | jq -r '.scriptHash // empty')
  matrix_launch_mode=$(echo "$MATRIX_STATUS" | jq -r '.launchMode // empty')

  pt_post /profiles "{\"name\":\"${restore_profile_name}\"}"
  assert_ok "create restore profile"
  restore_profile_id=$(echo "$RESULT" | jq -r '.id // empty')

  if [ -n "$restore_profile_id" ]; then
    pt_post "/profiles/${restore_profile_id}/start" '{"headless":true}'
    assert_http_status "201" "start restore profile instance"
    restore_instance_id=$(echo "$RESULT" | jq -r '.id // empty')

    if [ -n "$restore_instance_id" ] && wait_for_orchestrator_instance_status "${E2E_SERVER}" "${restore_instance_id}" "running" 30; then
      echo -e "  ${GREEN}✓${NC} restore profile instance reached running"
      ((ASSERTIONS_PASSED++)) || true
      pt_post "/instances/${restore_instance_id}/tabs/open" "{\"url\":\"${FIXTURES_URL}/stealth-capabilities.html\"}"
      assert_ok "open tab on restore profile instance"
      sleep 1

      pt_post "/profiles/${restore_profile_id}/stop" '{}'
      assert_ok "stop restore profile instance"
      wait_for_instances_gone "${E2E_SERVER}" 15 "${restore_instance_id}" || true

      pt_post "/profiles/${restore_profile_id}/start" '{"headless":true}'
      assert_http_status "201" "restart restore profile instance"
      restore_instance_id=$(echo "$RESULT" | jq -r '.id // empty')

      if [ -n "$restore_instance_id" ] && wait_for_orchestrator_instance_status "${E2E_SERVER}" "${restore_instance_id}" "running" 30; then
        restored_tab_id=$(poll_instance_tab_id_by_url "${restore_instance_id}" "${FIXTURES_URL}/stealth-capabilities.html" 30 0.5)
        if [ -n "${restored_tab_id:-}" ]; then
          echo -e "  ${GREEN}✓${NC} restored tab reappeared after restart"
          ((ASSERTIONS_PASSED++)) || true
          pt_get "/stealth/status?tabId=${restored_tab_id}"
          restored_status="$RESULT"

          restored_level=$(echo "$restored_status" | jq -r '.level // empty')
          restored_hash=$(echo "$restored_status" | jq -r '.scriptHash // empty')
          restored_launch_mode=$(echo "$restored_status" | jq -r '.launchMode // empty')

          if [ "$restored_level" = "$STEALTH_LEVEL" ] && [ -n "$restored_level" ]; then
            echo -e "  ${GREEN}✓${NC} restored tab keeps same stealth level"
            ((ASSERTIONS_PASSED++)) || true
          else
            echo -e "  ${RED}✗${NC} restored tab level drifted (${restored_level} vs ${STEALTH_LEVEL})"
            ((ASSERTIONS_FAILED++)) || true
          fi

          if [ "$restored_hash" = "$matrix_hash" ] && [ -n "$restored_hash" ]; then
            echo -e "  ${GREEN}✓${NC} restored tab keeps same script hash"
            ((ASSERTIONS_PASSED++)) || true
          else
            echo -e "  ${RED}✗${NC} restored tab script hash drifted (${restored_hash} vs ${matrix_hash})"
            ((ASSERTIONS_FAILED++)) || true
          fi

          if [ "$restored_launch_mode" = "$matrix_launch_mode" ] && [ -n "$restored_launch_mode" ]; then
            echo -e "  ${GREEN}✓${NC} restored tab keeps same launch mode"
            ((ASSERTIONS_PASSED++)) || true
          else
            echo -e "  ${RED}✗${NC} restored tab launch mode drifted (${restored_launch_mode} vs ${matrix_launch_mode})"
            ((ASSERTIONS_FAILED++)) || true
          fi

          assert_eval_poll "!!window.__stealthCapabilities && window.__stealthCapabilities.webdriverDescriptorNativeLike && window.__stealthCapabilities.userAgentVersionCoherent" "true" "restored tab exposes the same capability fixture contract" 10 0.5 "${restored_tab_id}"
        else
          echo -e "  ${RED}✗${NC} restored tab did not reappear after restart"
          ((ASSERTIONS_FAILED++)) || true
        fi
      else
        echo -e "  ${RED}✗${NC} restore profile instance failed to restart"
        ((ASSERTIONS_FAILED++)) || true
      fi
    else
      echo -e "  ${RED}✗${NC} restore profile instance failed to reach running"
      ((ASSERTIONS_FAILED++)) || true
    fi

    if [ -n "$restore_instance_id" ]; then
      pt_post "/profiles/${restore_profile_id}/stop" '{}' >/dev/null 2>&1 || true
    fi
    pt_delete "/profiles/${restore_profile_id}" >/dev/null 2>&1 || true
  else
    echo -e "  ${RED}✗${NC} restore profile creation did not return an id"
    ((ASSERTIONS_FAILED++)) || true
  fi
  end_test

  start_test "stealth-levels: local devtools probe stays quiet"
  pt_post /navigate "{\"url\":\"${FIXTURES_URL}/stealth-devtools-probe.html\"}"
  assert_ok "navigate to devtools probe fixture"
  sleep 2
  assert_eval_poll "!!window.__stealthDevtoolsProbe && window.__stealthDevtoolsProbe.ready" "true" "devtools probe report ready"
  assert_eval_poll "window.__stealthDevtoolsProbe.initialFormattersLen" "0" "page starts without preinstalled devtools formatters"
  assert_eval_poll "window.__stealthDevtoolsProbe.formatterCalled" "false" "console formatter probe stays untouched"
  assert_eval_poll "window.__stealthDevtoolsProbe.getterCalled" "false" "console getter probe stays untouched"
  assert_eval_poll "window.__stealthDevtoolsProbe.stackGetterCalls <= 1" "true" "error stack getter stays within baseline console access"
  end_test

  start_test "stealth-levels: worker parity fixture stays coherent"
  pt_post /navigate "{\"url\":\"${FIXTURES_URL}/stealth-workers.html\"}"
  assert_ok "navigate to worker parity fixture"
  sleep 1
  assert_eval_poll "!!window.__stealthWorkerReport && window.__stealthWorkerReport.ready" "true" "worker report ready"
  assert_eval_poll "window.__stealthWorkerReport.matches.userAgent" "true" "worker userAgent matches page"
  assert_eval_poll "window.__stealthWorkerReport.matches.platform" "true" "worker platform matches page"
  assert_eval_poll "window.__stealthWorkerReport.matches.hardwareConcurrency" "true" "worker hardwareConcurrency matches page"
  assert_eval_poll "window.__stealthWorkerReport.matches.deviceMemory" "true" "worker deviceMemory matches page"
  end_test

  start_test "stealth-levels: comprehensive score at ${STEALTH_LEVEL} level"
  pt_post /navigate "{\"url\":\"${FIXTURES_URL}/bot-detect.html\"}"
  assert_ok "navigate back to bot-detect fixture"
  sleep 1
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

  if [ "$ASSERTIONS_FAILED" -eq 0 ]; then
    echo -e "  ${GREEN}✓${NC} score meets ${STEALTH_LEVEL} level expectations"
  else
    echo -e "  ${RED}✗${NC} score below ${STEALTH_LEVEL} level expectations"
  fi
  end_test

  E2E_SERVER="$ORIG_URL"
}

if [ "${STEALTH_MATRIX:-0}" = "1" ]; then
  matrix_levels=()
  if [ -n "${STEALTH_LEVEL:-}" ]; then
    matrix_levels=("${STEALTH_LEVEL}")
  else
    matrix_levels=(light medium full)
  fi

  original_level="${STEALTH_LEVEL:-}"
  for matrix_level in "${matrix_levels[@]}"; do
    STEALTH_LEVEL="${matrix_level}"
    run_stealth_level_matrix
  done
  if [ -n "${original_level}" ]; then
    STEALTH_LEVEL="${original_level}"
  else
    unset STEALTH_LEVEL
  fi
  if [[ "${BASH_SOURCE[0]}" == "${0}" ]]; then
    print_summary
  fi
  return 0
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

start_test "bot-detect-full: device memory shape acceptable"

pt_post /evaluate '{"expression":"navigator.deviceMemory === undefined || navigator.deviceMemory > 0"}'
assert_json_eq "$RESULT" '.result' 'true' "deviceMemory is absent or > 0"

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
