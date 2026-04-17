#!/bin/bash
# sessions-extended.sh — Agent session CRUD and auth scenarios.

GROUP_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
source "${GROUP_DIR}/../../helpers/api.sh"

# ─────────────────────────────────────────────────────────────────
start_test "POST /sessions creates a session"

pt_post /sessions '{"agentId":"e2e-agent","label":"e2e-run"}'
assert_ok "session created"
assert_result_exists ".id" "has id"
assert_result_exists ".sessionToken" "has sessionToken"
assert_result_eq ".agentId" "e2e-agent" "agentId matches"
assert_result_eq ".label" "e2e-run" "label matches"
assert_result_eq ".status" "active" "status is active"

SESSION_ID=$(echo "$RESULT" | jq -r '.id')
SESSION_TOKEN=$(echo "$RESULT" | jq -r '.sessionToken')

end_test

# ─────────────────────────────────────────────────────────────────
start_test "POST /sessions rejects missing agentId"

pt_post /sessions '{}'
assert_http_status 400 "missing agentId rejected"

end_test

# ─────────────────────────────────────────────────────────────────
start_test "GET /sessions lists sessions"

pt_get /sessions
assert_ok "list ok"
SESSION_COUNT=$(echo "$RESULT" | jq 'length')
if [ "$SESSION_COUNT" -ge 1 ]; then
  echo -e "  ${GREEN}✓${NC} session list has $SESSION_COUNT entry(s)"
  ((ASSERTIONS_PASSED++)) || true
else
  echo -e "  ${RED}✗${NC} expected at least 1 session, got $SESSION_COUNT"
  ((ASSERTIONS_FAILED++)) || true
fi

end_test

# ─────────────────────────────────────────────────────────────────
start_test "GET /sessions/:id retrieves session by ID"

pt_get "/sessions/${SESSION_ID}"
assert_ok "get by id ok"
assert_result_eq ".id" "$SESSION_ID" "id matches"
assert_result_eq ".agentId" "e2e-agent" "agentId matches"

end_test

# ─────────────────────────────────────────────────────────────────
start_test "GET /sessions/:id returns 404 for unknown id"

pt_get "/sessions/ses_doesnotexist000000000000000"
assert_http_status 404 "unknown id returns 404"

end_test

# ─────────────────────────────────────────────────────────────────
start_test "GET /sessions/me returns session for session token"

RESULT=$(e2e_curl --token "" -s \
  -H "Authorization: Session ${SESSION_TOKEN}" \
  "${E2E_SERVER}/sessions/me")
HTTP_STATUS=$(e2e_curl --token "" -s -o /dev/null -w "%{http_code}" \
  -H "Authorization: Session ${SESSION_TOKEN}" \
  "${E2E_SERVER}/sessions/me")

assert_http_status 200 "/sessions/me with valid session token"
assert_result_eq ".agentId" "e2e-agent" "agentId matches"

end_test

# ─────────────────────────────────────────────────────────────────
start_test "GET /sessions/me rejects bearer token"

pt_get /sessions/me
assert_http_status 401 "/sessions/me rejects bearer token"

end_test

# ─────────────────────────────────────────────────────────────────
start_test "GET /sessions/me rejects invalid session token"

RESULT=$(e2e_curl --token "" -s \
  -H "Authorization: Session ses_invalidtoken000000000000" \
  "${E2E_SERVER}/sessions/me")
HTTP_STATUS=$(e2e_curl --token "" -s -o /dev/null -w "%{http_code}" \
  -H "Authorization: Session ses_invalidtoken000000000000" \
  "${E2E_SERVER}/sessions/me")

assert_http_status 401 "/sessions/me rejects invalid token"

end_test

# ─────────────────────────────────────────────────────────────────
start_test "session token cannot list sessions"

RESULT=$(e2e_curl --token "" -s \
  -H "Authorization: Session ${SESSION_TOKEN}" \
  "${E2E_SERVER}/sessions")
HTTP_STATUS=$(e2e_curl --token "" -s -o /dev/null -w "%{http_code}" \
  -H "Authorization: Session ${SESSION_TOKEN}" \
  "${E2E_SERVER}/sessions")

assert_http_status 403 "session token blocked from listing sessions"

end_test

# ─────────────────────────────────────────────────────────────────
start_test "session token cannot access dashboard admin config"

RESULT=$(e2e_curl --token "" -s \
  -H "Authorization: Session ${SESSION_TOKEN}" \
  "${E2E_SERVER}/api/config")
HTTP_STATUS=$(e2e_curl --token "" -s -o /dev/null -w "%{http_code}" \
  -H "Authorization: Session ${SESSION_TOKEN}" \
  "${E2E_SERVER}/api/config")

assert_http_status 403 "session token blocked from /api/config"

end_test

# ─────────────────────────────────────────────────────────────────
start_test "POST /sessions creates a second session for scope checks"

pt_post /sessions '{"agentId":"e2e-agent-2","label":"scope-check"}'
assert_ok "second session created"
assert_result_exists ".id" "has second session id"
assert_result_exists ".sessionToken" "has second session token"

SESSION_ID_2=$(echo "$RESULT" | jq -r '.id')
SESSION_TOKEN_2=$(echo "$RESULT" | jq -r '.sessionToken')

end_test

# ─────────────────────────────────────────────────────────────────
start_test "session token cannot revoke another session"

RESULT=$(e2e_curl --token "" -s -X POST \
  -H "Authorization: Session ${SESSION_TOKEN}" \
  "${E2E_SERVER}/sessions/${SESSION_ID_2}/revoke")
HTTP_STATUS=$(e2e_curl --token "" -s -o /dev/null -w "%{http_code}" -X POST \
  -H "Authorization: Session ${SESSION_TOKEN}" \
  "${E2E_SERVER}/sessions/${SESSION_ID_2}/revoke")

assert_http_status 403 "session token blocked from revoking another session"

OTHER_STATUS=$(e2e_curl --token "" -s -o /dev/null -w "%{http_code}" \
  -H "Authorization: Session ${SESSION_TOKEN_2}" \
  "${E2E_SERVER}/sessions/me")
if [ "$OTHER_STATUS" = "200" ]; then
  echo -e "  ${GREEN}✓${NC} other session remains active after forbidden revoke"
  ((ASSERTIONS_PASSED++)) || true
else
  echo -e "  ${RED}✗${NC} other session should remain active (got $OTHER_STATUS)"
  ((ASSERTIONS_FAILED++)) || true
fi

end_test

# ─────────────────────────────────────────────────────────────────
start_test "session token can revoke its own session"

response=$(e2e_curl --token "" -s -w "\n%{http_code}" -X POST \
  -H "Authorization: Session ${SESSION_TOKEN}" \
  "${E2E_SERVER}/sessions/${SESSION_ID}/revoke")
split_pinchtab_response "$response"

assert_http_status 200 "self-revoke ok"

REVOKED_STATUS=$(e2e_curl --token "" -s -o /dev/null -w "%{http_code}" \
  -H "Authorization: Session ${SESSION_TOKEN}" \
  "${E2E_SERVER}/sessions/me")
if [ "$REVOKED_STATUS" = "401" ]; then
  echo -e "  ${GREEN}✓${NC} token rejected after revoke (401)"
  ((ASSERTIONS_PASSED++)) || true
else
  echo -e "  ${RED}✗${NC} token should be invalid after revoke (got $REVOKED_STATUS)"
  ((ASSERTIONS_FAILED++)) || true
fi

end_test

# ─────────────────────────────────────────────────────────────────
start_test "POST /sessions/:id/revoke returns 404 for unknown id"

pt_post "/sessions/ses_doesnotexist000000000000000/revoke" '{}'
assert_http_status 404 "revoke unknown id returns 404"

end_test

# ─────────────────────────────────────────────────────────────────
start_test "session token authenticates browser actions"

pt_post /sessions '{"agentId":"e2e-action-agent"}'
assert_ok "session created for action test"
ACTION_TOKEN=$(echo "$RESULT" | jq -r '.sessionToken')

ACTION_STATUS=$(e2e_curl --token "" -s -o /dev/null -w "%{http_code}" \
  -H "Authorization: Session ${ACTION_TOKEN}" \
  "${E2E_SERVER}/health")
if [ "$ACTION_STATUS" = "200" ]; then
  echo -e "  ${GREEN}✓${NC} session token authenticates /health"
  ((ASSERTIONS_PASSED++)) || true
else
  echo -e "  ${RED}✗${NC} session token should authenticate /health (got $ACTION_STATUS)"
  ((ASSERTIONS_FAILED++)) || true
fi

end_test
