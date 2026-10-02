#!/bin/bash
# a11y-axe-basic.sh — GET /a11y/audit?engine=axe: violations with impact/tags/
# helpUrl and ref-mapped nodes, iframe auditing, isolated-world tamper-proofing,
# tag filtering, actuation of a ref, and the engine=bogus refusal.

GROUP_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
source "${GROUP_DIR}/../../helpers/api.sh"

# ─────────────────────────────────────────────────────────────────
start_test "a11y axe: violations carry id, impact, tags, helpUrl and refs"

pt_post /navigate -d "{\"url\":\"${FIXTURES_URL}/a11y-violations.html\"}"
assert_ok "navigate to a11y-violations.html"

pt_get "/a11y/audit?engine=axe"
assert_ok "axe audit"
assert_json_eq "$RESULT" '.engine' 'axe' "engine echoes axe"
assert_json_eq "$RESULT" '.version' '4.13.0' "axe version echoed"

IDS=$(echo "$RESULT" | jq -r '[.violations[].id] | sort | join(",")')
for want in image-alt label color-contrast; do
  if echo "$IDS" | grep -q "$want"; then
    pass_assert "violation $want present"
  else
    fail_assert "violation $want missing (got: $IDS)"
  fi
done

assert_json_exists "$RESULT" '.violations[] | select(.id=="image-alt") | select(.impact!=null)' "image-alt has impact"
assert_json_exists "$RESULT" '.violations[] | select(.id=="image-alt") | select(.helpUrl!="")' "image-alt has helpUrl"
assert_json_exists "$RESULT" '.violations[].nodes[] | select(.target!=null and .html!="")' "nodes carry target and html"

end_test

# ─────────────────────────────────────────────────────────────────
# The fixture tampers in the main world (fake window.axe, poisoned getAttribute).
# The audit still finds the violations, which proves it ran in the isolated world.
start_test "a11y axe: isolated world defeats page tampering"

VIOLATION_COUNT=$(echo "$RESULT" | jq '.violations | length')
if [ "$VIOLATION_COUNT" -ge 3 ]; then
  pass_assert "$VIOLATION_COUNT violations despite a fake window.axe returning none"
else
  fail_assert "only $VIOLATION_COUNT violations; page tampering leaked into the run"
fi

end_test

# ─────────────────────────────────────────────────────────────────
# The audit re-epochs the tab's ref cache and mints a fresh vocabulary token, so
# acting on a returned ref must echo THAT token. An agent that kept a pre-audit
# token (the nav -> audit -> click flow the card sells) would be refused 409, so
# the scenario captures the audit's token and proves both directions.
start_test "a11y axe: a ref-mapped node is actionable with the audit's vocab token"

REF=$(echo "$RESULT" | jq -r 'first(.violations[] | select(.id=="label") | .nodes[] | select(.ref!=null and .ref!="") | .ref)')
if [ -n "$REF" ] && [ "$REF" != "null" ]; then
  pass_assert "label violation carries ref $REF"
else
  fail_assert "label violation carries no ref"
fi

VOCAB=$(echo "$RESULT" | jq -r '.vocabularyToken')
if [ -n "$VOCAB" ] && [ "$VOCAB" != "null" ]; then
  pass_assert "audit published a vocabulary token $VOCAB"
else
  fail_assert "audit published no vocabulary token, so an agent cannot act on its refs"
fi

pt_post /action -d "{\"kind\":\"focus\",\"ref\":\"${REF}\",\"vocab\":\"${VOCAB}\"}"
assert_ok "focus the failing element echoing the audit's own token"

pt_post /action -d "{\"kind\":\"focus\",\"ref\":\"${REF}\",\"vocab\":\"stale-pre-audit-token\"}"
assert_http_status 409 "a pre-audit token is refused vocab_superseded"

end_test

# ─────────────────────────────────────────────────────────────────
start_test "a11y axe: iframe content is audited"

pt_get "/a11y/audit?engine=axe"
assert_ok "axe audit (iframe check)"
IFRAME_NODE=$(echo "$RESULT" | jq '[.violations[].nodes[] | select((.target | length) > 1)] | length')
if [ "$IFRAME_NODE" -ge 1 ]; then
  pass_assert "an iframe-nested violation node is reported"
else
  fail_assert "no iframe-nested violation reported"
fi

end_test

# ─────────────────────────────────────────────────────────────────
start_test "a11y axe: tags=wcag2a drops the wcag2aa-only color-contrast rule"

pt_get "/a11y/audit?engine=axe&tags=wcag2a"
assert_ok "axe audit tags=wcag2a"
assert_json_exists "$RESULT" '.violations[] | select(.id=="image-alt")' "wcag2a keeps image-alt"
CC=$(echo "$RESULT" | jq '[.violations[] | select(.id=="color-contrast")] | length')
if [ "$CC" -eq 0 ]; then
  pass_assert "color-contrast (wcag2aa) absent under tags=wcag2a"
else
  fail_assert "color-contrast present under tags=wcag2a"
fi

end_test

# ─────────────────────────────────────────────────────────────────
start_test "a11y axe: unknown engine is a 400"

pt_get "/a11y/audit?engine=bogus"
assert_http_status 400 "engine=bogus refused"

end_test

# ─────────────────────────────────────────────────────────────────
start_test "a11y native: engine omitted keeps the native report"

pt_get "/a11y/audit"
assert_ok "native audit"
assert_json_exists "$RESULT" '.findings' "native report has findings"
assert_json_exists "$RESULT" 'select(.score!=null)' "native report has score"
assert_json_eq "$RESULT" '.engine' 'null' "native report has no engine field"

end_test
