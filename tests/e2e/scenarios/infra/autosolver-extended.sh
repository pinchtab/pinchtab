#!/bin/bash

GROUP_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
source "${GROUP_DIR}/../../helpers/api.sh"
source "${GROUP_DIR}/../../helpers/autosolver.sh"

autosolver_use_medium_server

autosolver_run_score_case \
  "autosolver: bot-detect baseline" \
  "bot-detect" \
  "bot-detect.html" \
  "JSON.stringify(window.__botDetectScore || null)" \
  "JSON.stringify(window.__botDetectResults || {})" \
  "webdriver_value" \
  "plugins_present" \
  "chrome_runtime" \
  "no_cdp_traces" \
  "ua_not_headless"

autosolver_run_score_case \
  "autosolver: cdp-detect baseline" \
  "cdp-detect" \
  "cdp-detect.html" \
  "JSON.stringify(window.__cdpDetectScore || null)" \
  "JSON.stringify(window.__cdpDetectResults || {})" \
  "no_cdc_properties" \
  "no_selenium_globals" \
  "no_puppeteer_playwright_globals" \
  "no_runtime_evaluate_trace" \
  "webdriver_not_true"

autosolver_run_normal_page_case "autosolver: no-crash on normal page"
autosolver_run_retry_case "autosolver: retry loop — challenge page settles"

autosolver_restore_server

if [[ "${BASH_SOURCE[0]}" == "${0}" ]]; then
  print_summary
fi
