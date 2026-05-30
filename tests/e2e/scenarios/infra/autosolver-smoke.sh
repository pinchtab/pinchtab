#!/bin/bash

GROUP_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
source "${GROUP_DIR}/../../helpers/api.sh"
source "${GROUP_DIR}/../../helpers/autosolver.sh"

autosolver_use_medium_server

if [ "${PINCHTAB_E2E_BROWSER:-chrome}" = "cloak" ]; then
  autosolver_run_score_case_allowing_failures \
    "autosolver: bot-detect baseline" \
    "bot-detect" \
    "bot-detect.html" \
    "JSON.stringify(window.__botDetectScore || null)" \
    "JSON.stringify(window.__botDetectResults || {})" \
    "chrome_runtime,useragentdata_exists,useragentdata_highentropy" \
    "webdriver_value" \
    "plugins_present" \
    "chrome_runtime" \
    "useragentdata_exists" \
    "useragentdata_highentropy" \
    "no_cdp_traces" \
    "ua_not_headless"
else
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
fi

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
  finish_suite
fi
