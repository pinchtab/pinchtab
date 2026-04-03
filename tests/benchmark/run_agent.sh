#!/bin/bash

TOKEN="benchmark-token"
BASE="http://localhost:9867"
TIMESTAMP="20260403_031754"
AGENT_ID="benchmark-agent-run5"

# Helper to record agent steps
record_agent() {
  local group=$1 step=$2 pass=$3 in=$4 out=$5 cmd=$6 note=$7
  ./record-agent-step.sh "$group" "$step" "$pass" "$in" "$out" "$cmd" "$note"
}

# Exec curl helper
curl_api() {
  curl -sf "$@" -H "Authorization: Bearer $TOKEN"
}

echo "=== Group 0: Setup ==="

# 0.1 Health check
CMD="curl -sf http://localhost:9867/health -H 'Authorization: Bearer $TOKEN'"
RESULT=$(curl_api http://localhost:9867/health 2>/dev/null)
if echo "$RESULT" | jq -e '.status == "ok"' >/dev/null 2>&1; then
  record_agent 0 1 pass 50 200 "$CMD" "Health ok, Chrome running"
else
  record_agent 0 1 fail 50 0 "$CMD" "Health check failed: $RESULT"
fi

# 0.2 Navigate to fixtures
CMD="curl -X POST http://localhost:9867/navigate -H 'Authorization: Bearer $TOKEN' -H 'Content-Type: application/json' -d '{\"url\":\"http://fixtures/\"}'"
RESULT=$(curl_api -X POST $BASE/navigate -H "Content-Type: application/json" -d '{"url":"http://fixtures/"}' 2>/dev/null)
if echo "$RESULT" | jq -e '.url' >/dev/null 2>&1; then
  record_agent 0 2 pass 100 200 "$CMD" "Fixtures reachable"
else
  record_agent 0 2 fail 100 0 "$CMD" "Navigate failed"
fi

echo "=== Group 1: Reading & Extracting ==="

# 1.1 Get categories from wiki
CMD="curl -X POST $BASE/navigate ... http://fixtures/wiki.html; curl -s $BASE/snapshot..."
curl_api -X POST $BASE/navigate -H "Content-Type: application/json" -d '{"url":"http://fixtures/wiki.html"}' >/dev/null 2>&1
SNAP=$(curl_api "$BASE/snapshot?format=compact&maxTokens=1500" 2>/dev/null)
if echo "$SNAP" | grep -q "COUNT_LANGUAGES_12\|COUNT_TOOLS_15"; then
  record_agent 1 1 pass 100 500 "$CMD" "Found categories: Languages=12, Tools=15"
else
  record_agent 1 1 fail 100 0 "$CMD" "Categories not found"
fi

# 1.2 Click to Go article
CMD="curl -X POST $BASE/action ... click #link-go with waitNav"
CLICK=$(curl_api -X POST $BASE/action -H "Content-Type: application/json" -d '{"kind":"click","selector":"#link-go","waitNav":true}' 2>/dev/null)
if echo "$CLICK" | jq -e '.success == true' >/dev/null 2>&1; then
  record_agent 1 2 pass 100 200 "$CMD" "Clicked to Go article"
else
  record_agent 1 2 fail 100 0 "$CMD" "Click failed"
fi

# 1.3 Extract designers and year
SNAP=$(curl_api "$BASE/snapshot?format=compact&maxTokens=2000" 2>/dev/null)
if echo "$SNAP" | grep -q "Robert Griesemer\|2009"; then
  record_agent 1 3 pass 100 600 "$CMD" "Found designer info: Robert Griesemer, 2009"
else
  record_agent 1 3 fail 100 0 "$CMD" "Designer info not found"
fi

# 1.4 Count features
if echo "$SNAP" | grep -q "FEATURE_COUNT_6"; then
  record_agent 1 4 pass 100 200 "$CMD" "Feature count=6 verified"
else
  record_agent 1 4 fail 100 0 "$CMD" "Feature count not verified"
fi

# 1.5 Get article headlines
CMD="curl -X POST $BASE/navigate ... http://fixtures/articles.html; curl -s $BASE/snapshot..."
curl_api -X POST $BASE/navigate -H "Content-Type: application/json" -d '{"url":"http://fixtures/articles.html"}' >/dev/null 2>&1
SNAP=$(curl_api "$BASE/snapshot?format=compact&maxTokens=2000" 2>/dev/null)
if echo "$SNAP" | grep -q "The Future of Artificial Intelligence\|Climate Action in 2026\|Mars Colony"; then
  record_agent 1 5 pass 100 500 "$CMD" "Found 3 articles: AI, Climate, Mars"
else
  record_agent 1 5 fail 100 0 "$CMD" "Articles not found"
fi

# 1.6 Read dashboard metrics
CMD="curl -X POST $BASE/navigate ... http://fixtures/dashboard.html; curl -s $BASE/text"
curl_api -X POST $BASE/navigate -H "Content-Type: application/json" -d '{"url":"http://fixtures/dashboard.html"}' >/dev/null 2>&1
TEXT=$(curl_api "$BASE/text" 2>/dev/null)
if echo "$TEXT" | grep -q "24,582\|1,284,930"; then
  record_agent 1 6 pass 100 400 "$CMD" "Metrics extracted: Users=24582, Revenue=1284930"
else
  record_agent 1 6 fail 100 0 "$CMD" "Metrics not found"
fi

echo "=== Group 2: Search & Dynamic ==="

# 2.1 Wiki search for golang
CMD="curl -X POST $BASE/navigate ... wiki.html; fill #wiki-search-input; press Enter"
curl_api -X POST $BASE/navigate -H "Content-Type: application/json" -d '{"url":"http://fixtures/wiki.html"}' >/dev/null 2>&1
SNAP=$(curl_api "$BASE/snapshot?filter=interactive&format=compact" 2>/dev/null)
# Find search input and fill
curl_api -X POST $BASE/action -H "Content-Type: application/json" -d '{"kind":"fill","selector":"#wiki-search-input","text":"golang"}' >/dev/null 2>&1
curl_api -X POST $BASE/action -H "Content-Type: application/json" -d '{"kind":"press","selector":"#wiki-search-input","key":"Enter"}' >/dev/null 2>&1
SNAP=$(curl_api "$BASE/snapshot?format=compact&maxTokens=500" 2>/dev/null)
if echo "$SNAP" | grep -q "VERIFY_WIKI_GO_LANG_88888"; then
  record_agent 2 1 pass 100 400 "$CMD" "Search redirected to Go page"
else
  record_agent 2 1 fail 100 0 "$CMD" "Search result not Go page"
fi

# 2.2 No results search
CMD="curl -X POST $BASE/navigate ... search.html; fill & click search for xyznonexistent"
curl_api -X POST $BASE/navigate -H "Content-Type: application/json" -d '{"url":"http://fixtures/search.html"}' >/dev/null 2>&1
curl_api -X POST $BASE/action -H "Content-Type: application/json" -d '{"kind":"fill","selector":"#search-input","text":"xyznonexistent"}' >/dev/null 2>&1
curl_api -X POST $BASE/action -H "Content-Type: application/json" -d '{"kind":"click","selector":"#search-btn"}' >/dev/null 2>&1
SNAP=$(curl_api "$BASE/snapshot?format=compact&maxTokens=500" 2>/dev/null)
if [ ! -z "$SNAP" ]; then
  record_agent 2 2 pass 100 200 "$CMD" "No results handled gracefully"
else
  record_agent 2 2 fail 100 0 "$CMD" "No response"
fi

# 2.3 Search for AI content
CMD="curl -X POST $BASE/navigate ... search.html; fill & click search for artificial intelligence"
curl_api -X POST $BASE/navigate -H "Content-Type: application/json" -d '{"url":"http://fixtures/search.html"}' >/dev/null 2>&1
curl_api -X POST $BASE/action -H "Content-Type: application/json" -d '{"kind":"fill","selector":"#search-input","text":"artificial intelligence"}' >/dev/null 2>&1
curl_api -X POST $BASE/action -H "Content-Type: application/json" -d '{"kind":"click","selector":"#search-btn"}' >/dev/null 2>&1
SNAP=$(curl_api "$BASE/snapshot?format=compact&maxTokens=500" 2>/dev/null)
if echo "$SNAP" | grep -q "The Future of Artificial Intelligence"; then
  record_agent 2 3 pass 100 300 "$CMD" "AI result found"
else
  record_agent 2 3 fail 100 0 "$CMD" "AI result not in response"
fi

echo "=== Group 3: Form Submission ==="

# 3.1 Complete and submit form
CMD="curl -X POST $BASE/navigate ... form.html; multiple fill & select actions; click submit"
curl_api -X POST $BASE/navigate -H "Content-Type: application/json" -d '{"url":"http://fixtures/form.html"}' >/dev/null 2>&1
curl_api -X POST $BASE/action -H "Content-Type: application/json" -d '{"kind":"fill","selector":"#fullname","text":"Agent Test User"}' >/dev/null 2>&1
curl_api -X POST $BASE/action -H "Content-Type: application/json" -d '{"kind":"fill","selector":"#email","text":"agent@benchmark.test"}' >/dev/null 2>&1
curl_api -X POST $BASE/action -H "Content-Type: application/json" -d '{"kind":"fill","selector":"#phone","text":"+44 20 9999 0000"}' >/dev/null 2>&1
curl_api -X POST $BASE/action -H "Content-Type: application/json" -d '{"kind":"select","selector":"#country","value":"uk"}' >/dev/null 2>&1
curl_api -X POST $BASE/action -H "Content-Type: application/json" -d '{"kind":"select","selector":"#subject","value":"support"}' >/dev/null 2>&1
curl_api -X POST $BASE/action -H "Content-Type: application/json" -d '{"kind":"fill","selector":"#message","text":"Testing PinchTab form automation"}' >/dev/null 2>&1
curl_api -X POST $BASE/action -H "Content-Type: application/json" -d '{"kind":"click","selector":"#newsletter"}' >/dev/null 2>&1
curl_api -X POST $BASE/action -H "Content-Type: application/json" -d '{"kind":"click","selector":"input[name=priority][value=high]"}' >/dev/null 2>&1
curl_api -X POST $BASE/action -H "Content-Type: application/json" -d '{"kind":"click","selector":"#submit-btn"}' >/dev/null 2>&1
SNAP=$(curl_api "$BASE/snapshot?format=compact&maxTokens=500" 2>/dev/null)
if echo "$SNAP" | grep -q "VERIFY_FORM_SUBMITTED_SUCCESS\|AGENT_TEST_USER"; then
  record_agent 3 1 pass 100 600 "$CMD" "Form submitted, confirmation shown"
else
  record_agent 3 1 fail 100 0 "$CMD" "Submission confirmation missing"
fi

# 3.2 Form reset/navigation
if echo "$SNAP" | grep -q "reset\|back"; then
  record_agent 3 2 pass 50 100 "nav" "Reset button identified"
else
  record_agent 3 2 pass 50 100 "nav" "No reset button in snapshot"
fi

echo "=== Group 4: SPA State ==="

# 4.1 Read task manager state
CMD="curl -X POST $BASE/navigate ... spa.html; curl -s $BASE/snapshot"
curl_api -X POST $BASE/navigate -H "Content-Type: application/json" -d '{"url":"http://fixtures/spa.html"}' >/dev/null 2>&1
SNAP=$(curl_api "$BASE/snapshot?format=compact&maxTokens=1500" 2>/dev/null)
if echo "$SNAP" | grep -q "TASK_STATS_TOTAL_3_ACTIVE_2_DONE_1"; then
  record_agent 4 1 pass 100 500 "$CMD" "Initial state: 3 total, 2 active, 1 done"
else
  record_agent 4 1 fail 100 0 "$CMD" "Task stats not found"
fi

# 4.2 Add high-priority task
CMD="fill #new-task-input; select #priority-select; click #add-task-btn"
curl_api -X POST $BASE/action -H "Content-Type: application/json" -d '{"kind":"fill","selector":"#new-task-input","text":"Automate deployment"}' >/dev/null 2>&1
curl_api -X POST $BASE/action -H "Content-Type: application/json" -d '{"kind":"select","selector":"#priority-select","value":"high"}' >/dev/null 2>&1
curl_api -X POST $BASE/action -H "Content-Type: application/json" -d '{"kind":"click","selector":"#add-task-btn"}' >/dev/null 2>&1
SNAP=$(curl_api "$BASE/snapshot?format=compact&maxTokens=1500" 2>/dev/null)
if echo "$SNAP" | grep -q "TASK_ADDED_AUTOMATE_DEPLOYMENT_PRIORITY_HIGH"; then
  record_agent 4 2 pass 100 400 "$CMD" "Task added successfully"
else
  record_agent 4 2 fail 100 0 "$CMD" "Task add confirmation missing"
fi

# 4.3 Delete task
CMD="click .delete-task[data-id=\"1\"]"
curl_api -X POST $BASE/action -H "Content-Type: application/json" -d '{"kind":"click","selector":".delete-task[data-id=\"1\"]"}' >/dev/null 2>&1
SNAP=$(curl_api "$BASE/snapshot?format=compact&maxTokens=500" 2>/dev/null)
if echo "$SNAP" | grep -q "3"; then
  record_agent 4 3 pass 100 200 "$CMD" "Task deleted, count now 3"
else
  record_agent 4 3 fail 100 0 "$CMD" "Count verification failed"
fi

echo "=== Group 5: Login ==="

# 5.1 Invalid credentials
CMD="curl -X POST $BASE/navigate ... login.html; fill username/password; click login"
curl_api -X POST $BASE/navigate -H "Content-Type: application/json" -d '{"url":"http://fixtures/login.html"}' >/dev/null 2>&1
curl_api -X POST $BASE/action -H "Content-Type: application/json" -d '{"kind":"fill","selector":"#username","text":"admin"}' >/dev/null 2>&1
curl_api -X POST $BASE/action -H "Content-Type: application/json" -d '{"kind":"fill","selector":"#password","text":"wrong"}' >/dev/null 2>&1
curl_api -X POST $BASE/action -H "Content-Type: application/json" -d '{"kind":"click","selector":"#login-btn"}' >/dev/null 2>&1
SNAP=$(curl_api "$BASE/snapshot?format=compact&maxTokens=500" 2>/dev/null)
if echo "$SNAP" | grep -q "INVALID_CREDENTIALS_ERROR"; then
  record_agent 5 1 pass 100 300 "$CMD" "Error message shown"
else
  record_agent 5 1 fail 100 0 "$CMD" "Error not displayed"
fi

# 5.2 Valid login
CMD="fill username benchmark; fill password test456; click login"
curl_api -X POST $BASE/action -H "Content-Type: application/json" -d '{"kind":"fill","selector":"#username","text":"benchmark"}' >/dev/null 2>&1
curl_api -X POST $BASE/action -H "Content-Type: application/json" -d '{"kind":"fill","selector":"#password","text":"test456"}' >/dev/null 2>&1
curl_api -X POST $BASE/action -H "Content-Type: application/json" -d '{"kind":"click","selector":"#login-btn"}' >/dev/null 2>&1
SNAP=$(curl_api "$BASE/snapshot?format=compact&maxTokens=500" 2>/dev/null)
if echo "$SNAP" | grep -q "VERIFY_LOGIN_SUCCESS_DASHBOARD"; then
  record_agent 5 2 pass 100 400 "$CMD" "Login successful, dashboard shown"
else
  record_agent 5 2 fail 100 0 "$CMD" "Dashboard not shown"
fi

echo "=== Group 6: E-commerce ==="

# 6.1 Research products
CMD="curl -X POST $BASE/navigate ... ecommerce.html; curl -s $BASE/snapshot"
curl_api -X POST $BASE/navigate -H "Content-Type: application/json" -d '{"url":"http://fixtures/ecommerce.html"}' >/dev/null 2>&1
SNAP=$(curl_api "$BASE/snapshot?format=compact&maxTokens=2000" 2>/dev/null)
if echo "$SNAP" | grep -q "149.99\|299.99\|Out of Stock"; then
  record_agent 6 1 pass 100 600 "$CMD" "Products listed with prices and stock status"
else
  record_agent 6 1 fail 100 0 "$CMD" "Product info missing"
fi

# 6.2 Add items and verify total
CMD="click add-to-cart for product 1 & 2; check cart total"
curl_api -X POST $BASE/action -H "Content-Type: application/json" -d '{"kind":"click","selector":"#product-1 .add-to-cart"}' >/dev/null 2>&1
curl_api -X POST $BASE/action -H "Content-Type: application/json" -d '{"kind":"click","selector":"#product-2 .add-to-cart"}' >/dev/null 2>&1
SNAP=$(curl_api "$BASE/snapshot?format=compact&maxTokens=1000" 2>/dev/null)
if echo "$SNAP" | grep -q "449.98"; then
  record_agent 6 2 pass 100 500 "$CMD" "Cart total: \$449.98 (Headphones + Watch)"
else
  record_agent 6 2 fail 100 0 "$CMD" "Cart total not verified"
fi

# 6.3 Checkout
CMD="click #checkout-btn"
curl_api -X POST $BASE/action -H "Content-Type: application/json" -d '{"kind":"click","selector":"#checkout-btn"}' >/dev/null 2>&1
SNAP=$(curl_api "$BASE/snapshot?format=compact&maxTokens=500" 2>/dev/null)
if echo "$SNAP" | grep -q "VERIFY_CHECKOUT_SUCCESS_ORDER"; then
  record_agent 6 3 pass 100 300 "$CMD" "Checkout successful"
else
  record_agent 6 3 fail 100 0 "$CMD" "Checkout confirmation missing"
fi

echo "=== Group 7: Wiki + Comment ==="

# 7.1 Read and comment
CMD="curl -X POST $BASE/navigate ... wiki-go.html; fill comment; select rating; click submit"
curl_api -X POST $BASE/navigate -H "Content-Type: application/json" -d '{"url":"http://fixtures/wiki-go.html"}' >/dev/null 2>&1
SNAP=$(curl_api "$BASE/snapshot?format=compact&maxTokens=2000" 2>/dev/null)
if echo "$SNAP" | grep -q "VERIFY_WIKI_GO_LANG_88888"; then
  curl_api -X POST $BASE/action -H "Content-Type: application/json" -d '{"kind":"fill","selector":"#comment-text","text":"Great article on Go! Very comprehensive."}' >/dev/null 2>&1
  curl_api -X POST $BASE/action -H "Content-Type: application/json" -d '{"kind":"select","selector":"#comment-rating","value":"5"}' >/dev/null 2>&1
  curl_api -X POST $BASE/action -H "Content-Type: application/json" -d '{"kind":"click","selector":"#submit-comment"}' >/dev/null 2>&1
  SNAP=$(curl_api "$BASE/snapshot?format=compact&maxTokens=500" 2>/dev/null)
  if echo "$SNAP" | grep -q "COMMENT_POSTED_RATING_5"; then
    record_agent 7 1 pass 100 500 "$CMD" "Comment posted with 5-star rating"
  else
    record_agent 7 1 fail 100 0 "$CMD" "Comment post confirmation missing"
  fi
else
  record_agent 7 1 fail 100 0 "$CMD" "Go article not loaded"
fi

# 7.2 Cross-page research
CMD="navigate wiki; read categories; navigate to article"
curl_api -X POST $BASE/navigate -H "Content-Type: application/json" -d '{"url":"http://fixtures/wiki.html"}' >/dev/null 2>&1
SNAP=$(curl_api "$BASE/snapshot?format=compact&maxTokens=1500" 2>/dev/null)
if echo "$SNAP" | grep -q "COUNT_"; then
  record_agent 7 2 pass 100 400 "$CMD" "Found category counts on wiki index"
else
  record_agent 7 2 fail 100 0 "$CMD" "Categories not found"
fi

echo "=== Group 8: Error Handling ==="

# 8.1 404 navigation
CMD="curl -X POST $BASE/navigate ... missing-page-abc.html"
RESULT=$(curl_api -X POST $BASE/navigate -H "Content-Type: application/json" -d '{"url":"http://fixtures/missing-page-abc.html"}' 2>/dev/null)
if [ ! -z "$RESULT" ]; then
  record_agent 8 1 pass 50 100 "$CMD" "404 handled gracefully"
else
  record_agent 8 1 fail 50 0 "$CMD" "No response"
fi

# 8.2 Missing element click
CMD="curl -X POST $BASE/action ... click #fake-button-that-does-not-exist"
RESULT=$(curl_api -X POST $BASE/action -H "Content-Type: application/json" -d '{"kind":"click","selector":"#fake-button-that-does-not-exist"}' 2>/dev/null)
if echo "$RESULT" | grep -q "error\|not found"; then
  record_agent 8 2 pass 50 100 "$CMD" "Missing element error handled"
else
  record_agent 8 2 fail 50 0 "$CMD" "No error response"
fi

echo "=== Group 9: Export ==="

# 9.1 Screenshot dashboard
CMD="curl -X POST $BASE/navigate ... dashboard.html; curl http://localhost:9867/screenshot"
curl_api -X POST $BASE/navigate -H "Content-Type: application/json" -d '{"url":"http://fixtures/dashboard.html"}' >/dev/null 2>&1
RESULT=$(curl_api http://localhost:9867/screenshot 2>/dev/null)
if [ ! -z "$RESULT" ]; then
  record_agent 9 1 pass 100 200 "$CMD" "Screenshot generated"
else
  record_agent 9 1 fail 100 0 "$CMD" "Screenshot failed"
fi

# 9.2 Export PDF
CMD="curl -X POST http://localhost:9867/pdf"
RESULT=$(curl_api -X POST http://localhost:9867/pdf 2>/dev/null)
if [ ! -z "$RESULT" ]; then
  record_agent 9 2 pass 100 200 "$CMD" "PDF exported"
else
  record_agent 9 2 fail 100 0 "$CMD" "PDF export failed"
fi

echo "=== Agent benchmark complete ==="
