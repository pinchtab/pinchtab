#!/bin/bash
set -e

TIMESTAMP="${1:-$(date +%s)}"
cd "$(dirname "$0")"

echo "=== Group 1: Navigation & Content Extraction ==="

# 1.1
curl -X POST http://localhost:9867/navigate \
  -H "Authorization: Bearer benchmark-token" \
  -H "Content-Type: application/json" \
  -d '{"url":"http://fixtures/"}' | jq . && \
./record-step.sh 1 1 pass 0 0 "Navigate to fixtures home"

# 1.2
SNAP=$(curl -s "http://localhost:9867/snapshot?format=compact&maxTokens=1000" \
  -H "Authorization: Bearer benchmark-token")
if echo "$SNAP" | grep -q "VERIFY_HOME_LOADED_12345"; then
  ./record-step.sh 1 2 pass 0 0 "Home content verified"
else
  ./record-step.sh 1 2 fail 0 0 "Expected VERIFY_HOME_LOADED_12345, got: $(echo "$SNAP" | head -c 100)"
fi

# 1.3
curl -X POST http://localhost:9867/navigate \
  -H "Authorization: Bearer benchmark-token" \
  -H "Content-Type: application/json" \
  -d '{"url":"http://fixtures/wiki.html"}' | jq . && \
./record-step.sh 1 3 pass 0 0 "Navigate to wiki index"

# 1.4
SNAP=$(curl -s "http://localhost:9867/snapshot?format=compact&maxTokens=1500" \
  -H "Authorization: Bearer benchmark-token")
if echo "$SNAP" | grep -q "VERIFY_WIKI_INDEX_55555" && \
   echo "$SNAP" | grep -q "COUNT_LANGUAGES_12" && \
   echo "$SNAP" | grep -q "COUNT_TOOLS_15"; then
  ./record-step.sh 1 4 pass 0 0 "Wiki index verified with counts"
else
  ./record-step.sh 1 4 fail 0 0 "Missing verification strings. Snapshot: $(echo "$SNAP" | head -c 200)"
fi

# 1.5
curl -X POST http://localhost:9867/action \
  -H "Authorization: Bearer benchmark-token" \
  -H "Content-Type: application/json" \
  -d '{"kind":"click","selector":"#link-go","waitNav":true}' | jq . && \
./record-step.sh 1 5 pass 0 0 "Click through to Go article"

# 1.6
SNAP=$(curl -s "http://localhost:9867/snapshot?format=compact&maxTokens=2000" \
  -H "Authorization: Bearer benchmark-token")
if echo "$SNAP" | grep -q "VERIFY_WIKI_GO_LANG_88888" && \
   echo "$SNAP" | grep -q "Robert Griesemer" && \
   echo "$SNAP" | grep -q "2009" && \
   echo "$SNAP" | grep -q "FEATURE_COUNT_6"; then
  ./record-step.sh 1 6 pass 0 0 "Go article loaded and verified"
else
  ./record-step.sh 1 6 fail 0 0 "Missing Go article verification. Snapshot: $(echo "$SNAP" | head -c 200)"
fi

# 1.7
TEXT=$(curl -s "http://localhost:9867/text" \
  -H "Authorization: Bearer benchmark-token")
if echo "$TEXT" | grep -q "Google LLC" && echo "$TEXT" | grep -q "Ken Thompson"; then
  ./record-step.sh 1 7 pass 0 0 "Table data extracted"
else
  ./record-step.sh 1 7 fail 0 0 "Missing table data. Text: $(echo "$TEXT" | head -c 100)"
fi

# 1.8
curl -X POST http://localhost:9867/navigate \
  -H "Authorization: Bearer benchmark-token" \
  -H "Content-Type: application/json" \
  -d '{"url":"http://fixtures/articles.html"}' | jq . && \
SNAP=$(curl -s "http://localhost:9867/snapshot?format=compact&maxTokens=2000" \
  -H "Authorization: Bearer benchmark-token")
if echo "$SNAP" | grep -q "The Future of Artificial Intelligence" && \
   echo "$SNAP" | grep -q "Climate Action in 2026" && \
   echo "$SNAP" | grep -q "Mars Colony"; then
  ./record-step.sh 1 8 pass 0 0 "All article headlines found"
else
  ./record-step.sh 1 8 fail 0 0 "Missing article titles. Snapshot: $(echo "$SNAP" | head -c 200)"
fi

echo "Group 1 complete"
