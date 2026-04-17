#!/bin/bash
# files-extended.sh — API advanced file and capture scenarios.

GROUP_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
source "${GROUP_DIR}/../../helpers/api.sh"

# Migrated from: tests/integration/upload_test.go (UP6-UP9, UP11)

pt_post /navigate "{\"url\":\"${FIXTURES_URL}/upload.html\"}"
assert_ok "navigate"
sleep 1

# ─────────────────────────────────────────────────────────────────
start_test "upload: default selector"

FILE_CONTENT="data:text/plain;base64,SGVsbG8="
pt_post /upload "{\"files\":[\"${FILE_CONTENT}\"]}"
assert_ok "upload with default selector"

end_test

# ─────────────────────────────────────────────────────────────────
start_test "upload: invalid selector → error"

pt_post /upload '{"selector":"#nonexistent","files":["data:text/plain;base64,SGVsbG8="]}'
assert_not_ok "rejects invalid selector"

end_test

# ─────────────────────────────────────────────────────────────────
start_test "upload: missing files → error"

pt_post /upload '{"selector":"#single-file"}'
assert_not_ok "rejects missing files"

end_test

# ─────────────────────────────────────────────────────────────────
start_test "upload: bad JSON → error"

pt_post_raw /upload "{broken"
assert_http_status "400" "rejects bad JSON"

end_test

# ─────────────────────────────────────────────────────────────────
start_test "upload: nonexistent file path → error"

pt_post /upload '{"selector":"#single-file","paths":["/tmp/nonexistent_file_xyz_12345.jpg"]}'
assert_not_ok "rejects missing file"

end_test

# ─────────────────────────────────────────────────────────────────
start_test "upload: too many files → error"

pt_post /upload '{"selector":"#multi-file","files":["data:text/plain;base64,QQ==","data:text/plain;base64,QQ==","data:text/plain;base64,QQ==","data:text/plain;base64,QQ==","data:text/plain;base64,QQ==","data:text/plain;base64,QQ==","data:text/plain;base64,QQ==","data:text/plain;base64,QQ==","data:text/plain;base64,QQ=="]}'
assert_http_status 400 "rejects too many files"
assert_contains "$RESULT" "too many files" "too many files message returned"

end_test

# Migrated from: tests/integration/pdf_test.go (PD1-PD12)

pt_post /navigate "{\"url\":\"${FIXTURES_URL}/table.html\"}"
assert_ok "navigate"
TAB_ID=$(get_tab_id)

# ─────────────────────────────────────────────────────────────────
start_test "pdf: base64 output"

pt_get "/tabs/${TAB_ID}/pdf"
assert_ok "pdf base64"
assert_json_exists "$RESULT" '.base64' "has base64 field"

end_test

# ─────────────────────────────────────────────────────────────────
start_test "pdf: raw output"

pinchtab GET "/tabs/${TAB_ID}/pdf?raw=true"
assert_ok "pdf raw"

end_test

# ─────────────────────────────────────────────────────────────────
start_test "pdf: landscape"

pt_get "/tabs/${TAB_ID}/pdf?landscape=true"
assert_ok "pdf landscape"

end_test

# ─────────────────────────────────────────────────────────────────
start_test "pdf: custom scale"

pt_get "/tabs/${TAB_ID}/pdf?scale=0.5"
assert_ok "pdf scale 0.5"

end_test

# ─────────────────────────────────────────────────────────────────
start_test "pdf: custom paper size"

pt_get "/tabs/${TAB_ID}/pdf?paperWidth=7&paperHeight=9"
assert_ok "pdf custom paper"

end_test

# ─────────────────────────────────────────────────────────────────
start_test "pdf: custom margins"

pt_get "/tabs/${TAB_ID}/pdf?marginTop=0.75&marginLeft=0.75&marginRight=0.75&marginBottom=0.75"
assert_ok "pdf custom margins"

end_test

# ─────────────────────────────────────────────────────────────────
start_test "pdf: page ranges"

pt_get "/tabs/${TAB_ID}/pdf?pageRanges=1"
assert_ok "pdf page range"

end_test

# ─────────────────────────────────────────────────────────────────
start_test "pdf: header/footer"

pt_get "/tabs/${TAB_ID}/pdf?displayHeaderFooter=true"
assert_ok "pdf header/footer"

end_test

# ─────────────────────────────────────────────────────────────────
start_test "pdf: accessible (tagged + outline)"

pt_get "/tabs/${TAB_ID}/pdf?generateTaggedPDF=true&generateDocumentOutline=true"
assert_ok "pdf accessible"

end_test

# ─────────────────────────────────────────────────────────────────
start_test "pdf: prefer CSS page size"

pt_get "/tabs/${TAB_ID}/pdf?preferCSSPageSize=true"
assert_ok "pdf CSS page size"

end_test

# ─────────────────────────────────────────────────────────────────
start_test "pdf: output=file saves to disk"

pt_post /navigate '{"url":"'"${FIXTURES_URL}"'/index.html"}'
pt_get "/pdf?output=file"
assert_ok "pdf output=file"
assert_json_exists "$RESULT" '.path' "has file path"

end_test

# Covers:
# - direct internal/private targets rejected at initial validation
# - redirected internal targets rejected during browser-side navigation

build_download_redirect_url() {
  # The target arg is intentionally ignored: the SSRF test only needs *some*
  # redirect to *some* private/internal target. The fixtures nginx exposes a
  # dedicated /redirect-to-internal endpoint with the target hardcoded to
  # http://127.0.0.1:9999/health, which sidesteps stock nginx's inability to
  # URL-decode query args (and the resulting Location-header relative-path
  # bug that breaks parameterized /redirect-to?url= for encoded targets).
  local _ignored_target="$1"
  local attacker_url="${FIXTURES_URL}/redirect-to-internal"
  jq -rn --arg u "$attacker_url" '$u|@uri'
}

start_test "download security: non-allowed domain blocked"

pt_get "/download?url=http://not-on-allowlist.local/sample.txt"
assert_http_status 400 "non-allowed domain blocked"
assert_contains "$RESULT" "not allowed\|blocked" "domain rejection error message"

end_test

# ─────────────────────────────────────────────────────────────────
start_test "download security: redirected internal target blocked"

ATTACKER_URL=$(build_download_redirect_url "http://127.0.0.1:9999/health")
pt_get "/download?url=${ATTACKER_URL}"
assert_http_status 400 "redirected internal target blocked"
assert_contains "$RESULT" "unsafe browser request\|blocked\|private" "redirect SSRF error message"

end_test

# ─────────────────────────────────────────────────────────────────
start_test "download fallback: gzip download path still succeeds"

pt_get "/download?url=${FIXTURES_URL}/sitemap.xml.gz"
assert_ok "gzip fallback download"
assert_json_eq "$RESULT" '.contentType' 'application/xml' "gzip fallback reports decompressed XML content type"
assert_json_jq "$RESULT" '.size > 0' "gzip fallback returns non-empty body" "gzip fallback body was empty"

end_test
