# shellcheck shell=bash
# Provider-agnostic fixture-backed endpoint smoke. Requires globals
# HOST_PORT, TOKEN, FIXTURES_URL, TMP_DIR.

run_fixture_endpoint_smoke() {
  echo "Running fixture-backed endpoint smoke (provider-agnostic)..."

  api_post /navigate "{\"url\":\"${FIXTURES_URL}/index.html\",\"waitFor\":\"selector\",\"waitSelector\":\"#welcome\"}"
  assert_api_jq '.title | contains("Home")' "fixture home navigation title"
  assert_api_jq '.url | contains("index.html")' "fixture home navigation URL"
  local tab_id
  tab_id="$(echo "$API_RESULT" | jq -r '.tabId // empty')"
  [ -n "$tab_id" ] || fail "navigate response did not include tabId: $API_RESULT"

  api_get /tabs
  echo "$API_RESULT" | jq -e --arg tab "$tab_id" '.tabs | map(.id) | index($tab) != null' >/dev/null \
    || fail "tabs includes fixture tab failed: $API_RESULT"

  api_get "/snapshot?tabId=${tab_id}&filter=interactive"
  assert_api_jq '.nodes | length > 0' "snapshot returns nodes"

  api_get "/tabs/${tab_id}/text"
  assert_api_jq '.text | contains("Welcome to the E2E test fixtures")' "tab text includes fixture content"

  api_get "/tabs/${tab_id}/html?selector=%23welcome"
  assert_api_jq '.html | contains("Welcome to the E2E test fixtures")' "selected html includes fixture content"

  api_get "/tabs/${tab_id}/styles?selector=%23welcome&prop=display"
  assert_api_jq '.styles.display == "block"' "selected styles include display"

  assert_screenshot_png "/tabs/${tab_id}/screenshot?format=png&raw=true"

  api_post "/tabs/${tab_id}/navigate" "{\"url\":\"${FIXTURES_URL}/buttons.html\",\"waitFor\":\"selector\",\"waitSelector\":\"#increment\"}"
  assert_api_jq '.title | contains("Buttons")' "buttons navigation title"

  api_post "/tabs/${tab_id}/action" '{"kind":"click","selector":"#increment"}'
  assert_api_jq '.success == true' "click action succeeded"

  api_post "/tabs/${tab_id}/evaluate" '{"expression":"document.querySelector(\"#count\").textContent"}'
  assert_api_jq '.result == "1"' "evaluate sees click result"

  api_post "/tabs/${tab_id}/navigate" "{\"url\":\"${FIXTURES_URL}/form.html\",\"waitFor\":\"selector\",\"waitSelector\":\"#username\"}"
  assert_api_jq '.title | contains("Form")' "form navigation title"

  api_post "/tabs/${tab_id}/actions" '{"stopOnError":true,"actions":[{"kind":"fill","selector":"#username","text":"parity_user"},{"kind":"check","selector":"#terms"}]}'
  assert_api_jq '.successful == 2 and .failed == 0' "batch actions succeeded"

  api_post "/tabs/${tab_id}/evaluate" '{"expression":"({username: document.querySelector(\"#username\").value, terms: document.querySelector(\"#terms\").checked})"}'
  assert_api_jq '.result.username == "parity_user" and .result.terms == true' "evaluate sees batch action results"
}
