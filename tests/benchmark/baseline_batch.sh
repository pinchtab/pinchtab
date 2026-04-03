#!/bin/bash
TOKEN="benchmark-token"

# Group 1: Navigation & Content Extraction (continued)

# 1.2 Verify home content via snapshot
SNAP=$(curl -sf "http://localhost:9867/snapshot?format=compact&maxTokens=1000" -H "Authorization: Bearer $TOKEN")
echo "$SNAP" | grep -q "VERIFY_HOME_LOADED_12345" && ./record-step.sh 1 2 pass 200 300 "Home content verified" || ./record-step.sh 1 2 fail 200 300 "VERIFY_HOME_LOADED_12345 not found"

# 1.3 Navigate to wiki
curl -sf -X POST http://localhost:9867/navigate -H "Authorization: Bearer $TOKEN" -H "Content-Type: application/json" -d '{"url":"http://fixtures/wiki.html"}' > /dev/null
./record-step.sh 1 3 pass 150 200 "Navigate to wiki index"

# 1.4 Verify wiki content and counts
SNAP=$(curl -sf "http://localhost:9867/snapshot?format=compact&maxTokens=1500" -H "Authorization: Bearer $TOKEN")
if echo "$SNAP" | grep -q "VERIFY_WIKI_INDEX_55555" && echo "$SNAP" | grep -q "COUNT_LANGUAGES_12" && echo "$SNAP" | grep -q "COUNT_TOOLS_15"; then
  ./record-step.sh 1 4 pass 250 350 "Wiki content verified with counts"
else
  ./record-step.sh 1 4 fail 250 350 "Wiki verification strings not found"
fi

# 1.5 Click through to Go article
RESULT=$(curl -sf -X POST http://localhost:9867/action -H "Authorization: Bearer $TOKEN" -H "Content-Type: application/json" -d '{"kind":"click","selector":"#link-go","waitNav":true}')
echo "$RESULT" | grep -q "success" && ./record-step.sh 1 5 pass 150 200 "Clicked Go article link with waitNav" || ./record-step.sh 1 5 fail 150 200 "Click failed"

# 1.6 Verify Go article loaded
SNAP=$(curl -sf "http://localhost:9867/snapshot?format=compact&maxTokens=2000" -H "Authorization: Bearer $TOKEN")
if echo "$SNAP" | grep -q "VERIFY_WIKI_GO_LANG_88888" && echo "$SNAP" | grep -q "Robert Griesemer" && echo "$SNAP" | grep -q "2009" && echo "$SNAP" | grep -q "FEATURE_COUNT_6"; then
  ./record-step.sh 1 6 pass 300 400 "Go article verified"
else
  ./record-step.sh 1 6 fail 300 400 "Go article verification failed"
fi

# 1.7 Extract table data (text endpoint)
TEXT=$(curl -sf "http://localhost:9867/text" -H "Authorization: Bearer $TOKEN")
if echo "$TEXT" | grep -q "Google LLC" && echo "$TEXT" | grep -q "Ken Thompson"; then
  ./record-step.sh 1 7 pass 200 300 "Table data extracted"
else
  ./record-step.sh 1 7 fail 200 300 "Missing Google LLC or Ken Thompson"
fi

# 1.8 Navigate to articles page
curl -sf -X POST http://localhost:9867/navigate -H "Authorization: Bearer $TOKEN" -H "Content-Type: application/json" -d '{"url":"http://fixtures/articles.html"}' > /dev/null
SNAP=$(curl -sf "http://localhost:9867/snapshot?format=compact&maxTokens=2000" -H "Authorization: Bearer $TOKEN")
if echo "$SNAP" | grep -q "The Future of Artificial Intelligence" && echo "$SNAP" | grep -q "Climate Action in 2026" && echo "$SNAP" | grep -q "Mars Colony"; then
  ./record-step.sh 1 8 pass 250 350 "All article titles found"
else
  ./record-step.sh 1 8 fail 250 350 "Not all article titles found"
fi

echo "Group 1 complete"
