#!/bin/bash
# security-basic.sh — API security baseline scenarios.

GROUP_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
source "${GROUP_DIR}/../helpers/api.sh"

start_test "security: evaluate ALLOWED when enabled"

pt_post /navigate -d '{"url":"about:blank"}'
pt_post /evaluate -d '{"expression":"1+1"}'
assert_ok "evaluate allowed"

end_test

start_test "security: download ALLOWED when enabled"

pt_get "/download?url=https://httpbin.org/robots.txt"
assert_ok "download allowed"

end_test

start_test "security: upload ALLOWED when enabled"

pt_post /navigate -d "{\"url\":\"${FIXTURES_URL}/upload.html\"}"
sleep 1
pt_post /upload -d '{"selector":"#single-file","files":["data:text/plain;base64,dGVzdA=="]}'
assert_ok "upload allowed"

end_test

start_test "security: IDPI allows whitelisted domains"

pt_post /navigate -d "{\"url\":\"${FIXTURES_URL}/index.html\"}"
assert_ok "navigate to allowed domain"

end_test
