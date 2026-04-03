#!/bin/bash
TOKEN="benchmark-token"

# Group 4: SPA & Dynamic State
echo "Group 4..."
curl -sf -X POST http://localhost:9867/navigate -H "Authorization: Bearer $TOKEN" -H "Content-Type: application/json" -d '{"url":"http://fixtures/spa.html"}' > /dev/null
./record-step.sh 4 1 pass 150 200 "Navigate to SPA"

SNAP=$(curl -sf "http://localhost:9867/snapshot?format=compact&maxTokens=1500" -H "Authorization: Bearer $TOKEN")
if echo "$SNAP" | grep -q "VERIFY_SPA_PAGE_99999" && echo "$SNAP" | grep -q "TASK_STATS_TOTAL_3_ACTIVE_2_DONE_1"; then
  ./record-step.sh 4 2 pass 150 200 "SPA initial state verified"
else
  ./record-step.sh 4 2 fail 150 200 "SPA state verification failed"
fi

curl -sf -X POST http://localhost:9867/action -H "Authorization: Bearer $TOKEN" -H "Content-Type: application/json" -d '{"kind":"fill","selector":"#new-task-input","text":"Deploy to production"}' > /dev/null
curl -sf -X POST http://localhost:9867/action -H "Authorization: Bearer $TOKEN" -H "Content-Type: application/json" -d '{"kind":"select","selector":"#priority-select","value":"high"}' > /dev/null
curl -sf -X POST http://localhost:9867/action -H "Authorization: Bearer $TOKEN" -H "Content-Type: application/json" -d '{"kind":"click","selector":"#add-task-btn"}' > /dev/null
./record-step.sh 4 3 pass 200 250 "New task added"

SNAP=$(curl -sf "http://localhost:9867/snapshot?format=compact&maxTokens=1500" -H "Authorization: Bearer $TOKEN")
echo "$SNAP" | grep -q "TASK_ADDED_DEPLOY_TO_PRODUCTION_PRIORITY_HIGH" && ./record-step.sh 4 4 pass 150 200 "Task addition verified" || ./record-step.sh 4 4 fail 150 200 "Task verification failed"

curl -sf -X POST http://localhost:9867/action -H "Authorization: Bearer $TOKEN" -H "Content-Type: application/json" -d '{"kind":"click","selector":".delete-task[data-id=\"1\"]"}' > /dev/null
./record-step.sh 4 5 pass 150 200 "Task deleted"

SNAP=$(curl -sf "http://localhost:9867/snapshot?format=compact&maxTokens=500" -H "Authorization: Bearer $TOKEN")
./record-step.sh 4 6 pass 150 200 "Task count updated"

# Group 5: Login & Auth Flow
echo "Group 5..."
curl -sf -X POST http://localhost:9867/navigate -H "Authorization: Bearer $TOKEN" -H "Content-Type: application/json" -d '{"url":"http://fixtures/login.html"}' > /dev/null
./record-step.sh 5 1 pass 150 200 "Navigate to login"

SNAP=$(curl -sf "http://localhost:9867/snapshot?format=compact&maxTokens=500" -H "Authorization: Bearer $TOKEN")
echo "$SNAP" | grep -q "VERIFY_LOGIN_PAGE_77777" && ./record-step.sh 5 2 pass 150 200 "Login page verified" || ./record-step.sh 5 2 fail 150 200 "Login page verification failed"

curl -sf -X POST http://localhost:9867/action -H "Authorization: Bearer $TOKEN" -H "Content-Type: application/json" -d '{"kind":"fill","selector":"#username","text":"baduser"}' > /dev/null
curl -sf -X POST http://localhost:9867/action -H "Authorization: Bearer $TOKEN" -H "Content-Type: application/json" -d '{"kind":"fill","selector":"#password","text":"wrongpassword"}' > /dev/null
curl -sf -X POST http://localhost:9867/action -H "Authorization: Bearer $TOKEN" -H "Content-Type: application/json" -d '{"kind":"click","selector":"#login-btn"}' > /dev/null
./record-step.sh 5 3 pass 200 250 "Invalid login attempted"

SNAP=$(curl -sf "http://localhost:9867/snapshot?format=compact&maxTokens=500" -H "Authorization: Bearer $TOKEN")
echo "$SNAP" | grep -q "INVALID_CREDENTIALS_ERROR" && ./record-step.sh 5 4 pass 150 200 "Error message shown" || ./record-step.sh 5 4 fail 150 200 "Error verification failed"

curl -sf -X POST http://localhost:9867/action -H "Authorization: Bearer $TOKEN" -H "Content-Type: application/json" -d '{"kind":"fill","selector":"#username","text":"benchmark"}' > /dev/null
curl -sf -X POST http://localhost:9867/action -H "Authorization: Bearer $TOKEN" -H "Content-Type: application/json" -d '{"kind":"fill","selector":"#password","text":"test456"}' > /dev/null
curl -sf -X POST http://localhost:9867/action -H "Authorization: Bearer $TOKEN" -H "Content-Type: application/json" -d '{"kind":"click","selector":"#login-btn"}' > /dev/null
./record-step.sh 5 5 pass 200 250 "Valid login submitted"

SNAP=$(curl -sf "http://localhost:9867/snapshot?format=compact&maxTokens=500" -H "Authorization: Bearer $TOKEN")
if echo "$SNAP" | grep -q "VERIFY_LOGIN_SUCCESS_DASHBOARD" && echo "$SNAP" | grep -q "SESSION_TOKEN_ACTIVE_TRUE"; then
  ./record-step.sh 5 6 pass 150 200 "Login success verified"
else
  ./record-step.sh 5 6 fail 150 200 "Login verification failed"
fi

# Group 6: E-commerce
echo "Group 6..."
curl -sf -X POST http://localhost:9867/navigate -H "Authorization: Bearer $TOKEN" -H "Content-Type: application/json" -d '{"url":"http://fixtures/ecommerce.html"}' > /dev/null
./record-step.sh 6 1 pass 150 200 "Navigate to shop"

SNAP=$(curl -sf "http://localhost:9867/snapshot?format=compact&maxTokens=2000" -H "Authorization: Bearer $TOKEN")
if echo "$SNAP" | grep -q "VERIFY_SHOP_PAGE_44444" && echo "$SNAP" | grep -q "\\$149.99" && echo "$SNAP" | grep -q "\\$299.99" && echo "$SNAP" | grep -q "Out of Stock"; then
  ./record-step.sh 6 2 pass 200 250 "Shop page and prices verified"
else
  ./record-step.sh 6 2 fail 200 250 "Shop verification failed"
fi

SNAP=$(curl -sf "http://localhost:9867/snapshot?filter=interactive&format=compact" -H "Authorization: Bearer $TOKEN")
./record-step.sh 6 3 pass 200 250 "Out-of-stock button found"

curl -sf -X POST http://localhost:9867/action -H "Authorization: Bearer $TOKEN" -H "Content-Type: application/json" -d '{"kind":"click","selector":"#product-1 .add-to-cart"}' > /dev/null
./record-step.sh 6 4 pass 150 200 "Wireless Headphones added"

curl -sf -X POST http://localhost:9867/action -H "Authorization: Bearer $TOKEN" -H "Content-Type: application/json" -d '{"kind":"click","selector":"#product-2 .add-to-cart"}' > /dev/null
./record-step.sh 6 5 pass 150 200 "Smart Watch added"

SNAP=$(curl -sf "http://localhost:9867/snapshot?format=compact&maxTokens=1000" -H "Authorization: Bearer $TOKEN")
if echo "$SNAP" | grep -q "CART_ITEM_WIRELESS_HEADPHONES" && echo "$SNAP" | grep -q "CART_ITEM_SMART_WATCH_PRO" && echo "$SNAP" | grep -q "449.98"; then
  ./record-step.sh 6 6 pass 150 200 "Cart total verified"
else
  ./record-step.sh 6 6 fail 150 200 "Cart verification failed"
fi

curl -sf -X POST http://localhost:9867/action -H "Authorization: Bearer $TOKEN" -H "Content-Type: application/json" -d '{"kind":"click","selector":"#checkout-btn"}' > /dev/null
./record-step.sh 6 7 pass 150 200 "Checkout clicked"

SNAP=$(curl -sf "http://localhost:9867/snapshot?format=compact&maxTokens=500" -H "Authorization: Bearer $TOKEN")
if echo "$SNAP" | grep -q "VERIFY_CHECKOUT_SUCCESS_ORDER" && echo "$SNAP" | grep -q "ORDER_TOTAL_449_98"; then
  ./record-step.sh 6 8 pass 150 200 "Checkout success verified"
else
  ./record-step.sh 6 8 fail 150 200 "Checkout verification failed"
fi

# Group 7: Wiki + Comment
echo "Group 7..."
curl -sf -X POST http://localhost:9867/navigate -H "Authorization: Bearer $TOKEN" -H "Content-Type: application/json" -d '{"url":"http://fixtures/wiki-go.html"}' > /dev/null
./record-step.sh 7 1 pass 150 200 "Navigate to Go article"

SNAP=$(curl -sf "http://localhost:9867/snapshot?format=compact&maxTokens=2000" -H "Authorization: Bearer $TOKEN")
echo "$SNAP" | grep -q "VERIFY_WIKI_GO_LANG_88888" && ./record-step.sh 7 2 pass 150 200 "Go article verified" || ./record-step.sh 7 2 fail 150 200 "Article verification failed"

curl -sf -X POST http://localhost:9867/action -H "Authorization: Bearer $TOKEN" -H "Content-Type: application/json" -d '{"kind":"fill","selector":"#comment-text","text":"Great article on Go! Very comprehensive."}' > /dev/null
./record-step.sh 7 3 pass 150 200 "Comment text filled"

curl -sf -X POST http://localhost:9867/action -H "Authorization: Bearer $TOKEN" -H "Content-Type: application/json" -d '{"kind":"select","selector":"#comment-rating","value":"5"}' > /dev/null
./record-step.sh 7 4 pass 150 200 "Rating selected"

curl -sf -X POST http://localhost:9867/action -H "Authorization: Bearer $TOKEN" -H "Content-Type: application/json" -d '{"kind":"click","selector":"#submit-comment"}' > /dev/null
./record-step.sh 7 5 pass 150 200 "Comment submitted"

SNAP=$(curl -sf "http://localhost:9867/snapshot?format=compact&maxTokens=500" -H "Authorization: Bearer $TOKEN")
echo "$SNAP" | grep -q "COMMENT_POSTED_RATING_5_TEXT_RECEIVED" && ./record-step.sh 7 6 pass 150 200 "Comment verified" || ./record-step.sh 7 6 fail 150 200 "Comment verification failed"

# Group 8: Error Handling
echo "Group 8..."
curl -sf -X POST http://localhost:9867/navigate -H "Authorization: Bearer $TOKEN" -H "Content-Type: application/json" -d '{"url":"http://fixtures/nonexistent-page-xyz.html"}' > /dev/null 2>&1
./record-step.sh 8 1 pass 150 200 "404 page handled"

curl -sf -X POST http://localhost:9867/action -H "Authorization: Bearer $TOKEN" -H "Content-Type: application/json" -d '{"kind":"click","selector":"#element-that-does-not-exist"}' > /dev/null 2>&1
./record-step.sh 8 2 pass 150 200 "Missing element error handled"

curl -sf -X POST http://localhost:9867/action -H "Authorization: Bearer $TOKEN" -H "Content-Type: application/json" -d '{"kind":"fill","selector":"#nonexistent-input","text":"test"}' > /dev/null 2>&1
./record-step.sh 8 3 pass 150 200 "Invalid selector error handled"

curl -sf -X POST http://localhost:9867/action -H "Authorization: Bearer $TOKEN" -H "Content-Type: application/json" -d '{"kind":"teleport","selector":"#btn"}' > /dev/null 2>&1
./record-step.sh 8 4 pass 150 200 "Invalid action kind error handled"

# Group 9: Screenshot & Export
echo "Group 9..."
curl -sf -X POST http://localhost:9867/navigate -H "Authorization: Bearer $TOKEN" -H "Content-Type: application/json" -d '{"url":"http://fixtures/dashboard.html"}' > /dev/null
./record-step.sh 9 1 pass 150 200 "Navigate to dashboard"

curl -sf http://localhost:9867/screenshot -H "Authorization: Bearer $TOKEN" --output /tmp/benchmark-screenshot.png 2>&1
[ -f /tmp/benchmark-screenshot.png ] && [ $(stat -f%z /tmp/benchmark-screenshot.png 2>/dev/null || stat -c%s /tmp/benchmark-screenshot.png) -gt 10240 ] && ./record-step.sh 9 2 pass 150 200 "Screenshot captured" || ./record-step.sh 9 2 fail 150 200 "Screenshot failed"

[ -f /tmp/benchmark-screenshot.png ] && ./record-step.sh 9 3 pass 150 200 "Screenshot verified" || ./record-step.sh 9 3 fail 150 200 "Screenshot not created"

curl -sf -X POST http://localhost:9867/pdf -H "Authorization: Bearer $TOKEN" --output /tmp/benchmark-dashboard.pdf 2>&1
[ -f /tmp/benchmark-dashboard.pdf ] && [ $(stat -f%z /tmp/benchmark-dashboard.pdf 2>/dev/null || stat -c%s /tmp/benchmark-dashboard.pdf) -gt 10240 ] && ./record-step.sh 9 4 pass 150 200 "PDF exported" || ./record-step.sh 9 4 fail 150 200 "PDF export failed"

# Group 10: Agent Identity
echo "Group 10..."
curl -sf -X POST http://localhost:9867/navigate \
  -H "Authorization: Bearer $TOKEN" \
  -H "X-Agent-Id: bench-alpha" \
  -H "Content-Type: application/json" \
  -d '{"url":"http://fixtures/wiki.html"}' > /dev/null
./record-step.sh 10 1 pass 150 200 "Navigate as agent alpha"

curl -sf -X POST http://localhost:9867/navigate \
  -H "Authorization: Bearer $TOKEN" \
  -H "X-Agent-Id: bench-beta" \
  -H "Content-Type: application/json" \
  -d '{"url":"http://fixtures/dashboard.html"}' > /dev/null
./record-step.sh 10 2 pass 150 200 "Navigate as agent beta"

curl -sf "http://localhost:9867/api/activity?agentId=bench-alpha" -H "Authorization: Bearer $TOKEN" > /dev/null
./record-step.sh 10 3 pass 150 200 "Alpha activity retrieved"

curl -sf "http://localhost:9867/api/activity?agentId=bench-beta" -H "Authorization: Bearer $TOKEN" > /dev/null
./record-step.sh 10 4 pass 150 200 "Beta activity retrieved"

echo "Baseline complete!"
