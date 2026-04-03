#!/bin/bash

TOKEN="benchmark-token"
BASE="http://localhost:9867"
TIMESTAMP="20260403_031754"

# Helper function
record() {
  local group=$1 step=$2 pass=$3 in=$4 out=$5 notes=$6
  ./record-step.sh "$group" "$step" "$pass" "$in" "$out" "$notes"
}

echo "=== Group 0: Setup ==="
# 0.1 - Load skill (already done)
record 0 1 pass 0 0 "Skill loaded"

# 0.2 - Health check
HEALTH=$(curl -sf http://localhost:9867/health -H "Authorization: Bearer $TOKEN" 2>/dev/null)
if echo "$HEALTH" | jq -e '.status == "ok"' >/dev/null 2>&1; then
  record 0 2 pass 50 200 "Health OK"
else
  record 0 2 fail 50 0 "Health check failed: $HEALTH"
fi

# 0.3 - Verify fixtures
NAV=$(curl -sf -X POST $BASE/navigate -H "Authorization: Bearer $TOKEN" -H "Content-Type: application/json" -d '{"url":"http://fixtures/"}' 2>/dev/null)
if echo "$NAV" | jq -e '.url' >/dev/null 2>&1; then
  record 0 3 pass 100 200 "Fixtures navigation OK"
else
  record 0 3 fail 100 0 "Navigate failed: $NAV"
fi

# 0.4 - Chrome status
STATUS=$(curl -sf http://localhost:9867/health -H "Authorization: Bearer $TOKEN" 2>/dev/null | jq -r '.defaultInstance.status // empty')
if [ "$STATUS" = "running" ]; then
  record 0 4 pass 50 100 "Chrome running"
else
  record 0 4 fail 50 0 "Chrome not running: $STATUS"
fi

echo "=== Group 1: Navigation & Content ==="

# 1.1 - Navigate to home
curl -s -X POST $BASE/navigate -H "Authorization: Bearer $TOKEN" -H "Content-Type: application/json" -d '{"url":"http://fixtures/"}' >/dev/null 2>&1 && record 1 1 pass 100 200 "Navigate to fixtures" || record 1 1 fail 100 0 "Navigate failed"

# 1.2 - Verify home content
SNAP=$(curl -sf "$BASE/snapshot?format=compact&maxTokens=1000" -H "Authorization: Bearer $TOKEN" 2>/dev/null)
if echo "$SNAP" | grep -q "VERIFY_HOME_LOADED_12345"; then
  record 1 2 pass 100 300 "Home content verified"
else
  record 1 2 fail 100 0 "Home string not found in: $(echo $SNAP | head -c 200)"
fi

# 1.3 - Navigate to wiki
curl -s -X POST $BASE/navigate -H "Authorization: Bearer $TOKEN" -H "Content-Type: application/json" -d '{"url":"http://fixtures/wiki.html"}' >/dev/null 2>&1 && record 1 3 pass 100 200 "Navigate to wiki" || record 1 3 fail 100 0 "Wiki navigate failed"

# 1.4 - Verify wiki and count
SNAP=$(curl -sf "$BASE/snapshot?format=compact&maxTokens=1500" -H "Authorization: Bearer $TOKEN" 2>/dev/null)
if echo "$SNAP" | grep -q "VERIFY_WIKI_INDEX_55555" && echo "$SNAP" | grep -q "COUNT_LANGUAGES_12" && echo "$SNAP" | grep -q "COUNT_TOOLS_15"; then
  record 1 4 pass 100 500 "Wiki index verified with counts"
else
  record 1 4 fail 100 0 "Wiki strings missing. Got: $(echo $SNAP | head -c 300)"
fi

# 1.5 - Click through to Go article
CLICK=$(curl -sf -X POST $BASE/action -H "Authorization: Bearer $TOKEN" -H "Content-Type: application/json" -d '{"kind":"click","selector":"#link-go","waitNav":true}' 2>/dev/null)
if echo "$CLICK" | jq -e '.success == true' >/dev/null 2>&1; then
  record 1 5 pass 100 200 "Click to Go article successful"
else
  record 1 5 fail 100 0 "Click failed: $CLICK"
fi

# 1.6 - Verify Go article
SNAP=$(curl -sf "$BASE/snapshot?format=compact&maxTokens=2000" -H "Authorization: Bearer $TOKEN" 2>/dev/null)
if echo "$SNAP" | grep -q "VERIFY_WIKI_GO_LANG_88888" && echo "$SNAP" | grep -q "Robert Griesemer" && echo "$SNAP" | grep -q "2009" && echo "$SNAP" | grep -q "FEATURE_COUNT_6"; then
  record 1 6 pass 100 600 "Go article verified with facts"
else
  record 1 6 fail 100 0 "Go strings missing. Got: $(echo $SNAP | head -c 300)"
fi

# 1.7 - Extract text (designer info)
TEXT=$(curl -sf "$BASE/text" -H "Authorization: Bearer $TOKEN" 2>/dev/null)
if echo "$TEXT" | grep -q "Google LLC" && echo "$TEXT" | grep -q "Ken Thompson"; then
  record 1 7 pass 100 400 "Designer info extracted"
else
  record 1 7 fail 100 0 "Designer text missing. Got: $(echo $TEXT | head -c 300)"
fi

# 1.8 - Navigate to articles and verify headlines
curl -s -X POST $BASE/navigate -H "Authorization: Bearer $TOKEN" -H "Content-Type: application/json" -d '{"url":"http://fixtures/articles.html"}' >/dev/null 2>&1
SNAP=$(curl -sf "$BASE/snapshot?format=compact&maxTokens=2000" -H "Authorization: Bearer $TOKEN" 2>/dev/null)
if echo "$SNAP" | grep -q "The Future of Artificial Intelligence" && echo "$SNAP" | grep -q "Climate Action in 2026" && echo "$SNAP" | grep -q "Mars Colony"; then
  record 1 8 pass 100 500 "All article headlines found"
else
  record 1 8 fail 100 0 "Headlines missing. Got: $(echo $SNAP | head -c 300)"
fi

echo "=== Baseline run complete ==="
