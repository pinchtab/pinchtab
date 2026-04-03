#!/bin/bash

TOKEN="benchmark-token"
BASE="http://localhost:9867"

echo "=== Diagnosing Step 2.1 Failure (Wiki Search) ==="

# Navigate to wiki
echo "1. Navigate to wiki.html"
curl -s -X POST $BASE/navigate -H "Authorization: Bearer $TOKEN" -H "Content-Type: application/json" -d '{"url":"http://fixtures/wiki.html"}' | jq '.' 2>/dev/null | head -10

# Get snapshot to find search element
echo -e "\n2. Get snapshot with interactive filter"
SNAP=$(curl -s "$BASE/snapshot?filter=interactive&format=compact" -H "Authorization: Bearer $TOKEN" 2>/dev/null)
echo "$SNAP" | head -50

# Try filling search 
echo -e "\n3. Try to fill #wiki-search-input"
FILL=$(curl -s -X POST $BASE/action -H "Authorization: Bearer $TOKEN" -H "Content-Type: application/json" -d '{"kind":"fill","selector":"#wiki-search-input","text":"golang"}' 2>/dev/null)
echo "$FILL" | jq '.' 2>/dev/null || echo "$FILL"

# Try press
echo -e "\n4. Try to press Enter"
PRESS=$(curl -s -X POST $BASE/action -H "Authorization: Bearer $TOKEN" -H "Content-Type: application/json" -d '{"kind":"press","selector":"#wiki-search-input","key":"Enter"}' 2>/dev/null)
echo "$PRESS" | jq '.' 2>/dev/null || echo "$PRESS"

# Check where we are
echo -e "\n5. Check current page"
SNAP=$(curl -s "$BASE/snapshot?format=compact&maxTokens=500" -H "Authorization: Bearer $TOKEN" 2>/dev/null)
echo "$SNAP" | head -20

echo -e "\n=== Diagnosing Step 8.2 (Missing Element) ==="
# Try clicking non-existent element
CLICK=$(curl -s -X POST $BASE/action -H "Authorization: Bearer $TOKEN" -H "Content-Type: application/json" -d '{"kind":"click","selector":"#fake-button-that-does-not-exist"}' 2>/dev/null)
echo "Response to missing element click:"
echo "$CLICK" | jq '.' 2>/dev/null || echo "$CLICK"
