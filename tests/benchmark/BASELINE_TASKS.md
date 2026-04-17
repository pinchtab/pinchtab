# PinchTab Benchmark Tasks (Baseline)

Reproducible benchmark using explicit curl commands against controlled fixture pages.
Each task maps 1:1 to an agent task in AGENT_TASKS.md for direct comparison.

## Environment

- **PinchTab Server**: `http://localhost:9867`
- **Fixtures Server**: `http://fixtures/` (inside Docker network)
- **Token**: `benchmark-token`
- **Auth Header**: `Authorization: Bearer benchmark-token`

## MANDATORY: Docker

```bash
cd ~/dev/pinchtab/tests/benchmark
docker compose up -d --build
# Wait for healthy: curl -sf -H "Authorization: Bearer benchmark-token" http://localhost:9867/health
```

## Recording

```bash
# Baseline (no tokens, optional response bytes):
./scripts/record-step.sh --type baseline <group> <step> <pass|fail> "notes"

# Minimal (just pass/fail):
./scripts/record-step.sh <group> <step> <pass|fail> "notes"
```

**On failure, include in notes:**
- What was expected
- What was actually returned
- HTTP status code / error message

## Tab Reuse

`POST /navigate` creates a new tab by default. To avoid multi-tab issues, the
runner must:

1. Capture `tabId` from the first navigate response (step 0.2).
2. Pass `"tabId":"TAB_ID"` in every subsequent navigate request body.
3. Use tab-scoped endpoints for actions and snapshots:
   - `POST /tabs/TAB_ID/action` instead of `POST /action`
   - `GET /tabs/TAB_ID/snapshot?...` instead of `GET /snapshot?...`
   - `GET /tabs/TAB_ID/text` instead of `GET /text`
   - `POST /tabs/TAB_ID/back` instead of `POST /back`
   - `GET /tabs/TAB_ID/screenshot` instead of `GET /screenshot`
   - `GET /tabs/TAB_ID/pdf` instead of `GET /pdf`

All curl examples below use `TAB_ID` as a placeholder. Replace with the actual
tab ID captured in step 0.2.

---

## Group 0: Setup & Diagnosis

### 0.1 Server reachable
```bash
curl -sf http://localhost:9867/health \
  -H "Authorization: Bearer benchmark-token"
```
**Pass if**: `status == "ok"` in response body.

### 0.2 Auth required
```bash
curl -s -o /dev/null -w "%{http_code}" http://localhost:9867/health
```
**Pass if**: HTTP status is `401` (auth rejected without token).

### 0.3 Auth works with token
```bash
curl -s -o /dev/null -w "%{http_code}" http://localhost:9867/health \
  -H "Authorization: Bearer benchmark-token"
```
**Pass if**: HTTP status is `200`.

### 0.4 Instance available
```bash
curl -sf http://localhost:9867/health \
  -H "Authorization: Bearer benchmark-token"
```
**Pass if**: Response contains `defaultInstance.status == "running"`. If not, run:
```bash
curl -X POST http://localhost:9867/instances/start \
  -H "Authorization: Bearer benchmark-token" \
  -H "Content-Type: application/json" \
  -d '{}'
```
And re-verify the new instance is running.

### 0.5 List existing tabs
```bash
curl -sf http://localhost:9867/tabs \
  -H "Authorization: Bearer benchmark-token"
```
**Pass if**: Returns a JSON array (possibly empty) without error.

### 0.6 Clean stale tabs
For each tab returned by step 0.5, close it:
```bash
curl -X POST http://localhost:9867/tabs/TAB_ID/close \
  -H "Authorization: Bearer benchmark-token"
```
Then verify cleanup:
```bash
curl -sf http://localhost:9867/tabs \
  -H "Authorization: Bearer benchmark-token"
```
**Pass if**: After cleanup, the tab list is empty or contains only an about:blank tab.

### 0.7 Network reach to target
```bash
curl -sf -X POST http://localhost:9867/navigate \
  -H "Authorization: Bearer benchmark-token" \
  -H "Content-Type: application/json" \
  -d '{"url":"http://fixtures/"}'
```
**Capture**: Save the `tabId` from the JSON response. All subsequent commands use this tab ID.
```bash
curl -sf "http://localhost:9867/tabs/TAB_ID/snapshot?format=compact&maxTokens=500" \
  -H "Authorization: Bearer benchmark-token"
```
**Pass if**: Navigate returns HTTP 200 AND snapshot contains `VERIFY_HOME_LOADED_12345`.

### 0.8 Capture initial tab ID
```bash
curl -sf http://localhost:9867/tabs \
  -H "Authorization: Bearer benchmark-token"
```
**Pass if**: The captured `TAB_ID` from step 0.7 appears in the tabs list.

---

## Group 1: Reading & Extracting

### 1.1 Wiki categories
```bash
curl -X POST http://localhost:9867/navigate \
  -H "Authorization: Bearer benchmark-token" \
  -H "Content-Type: application/json" \
  -d '{"tabId":"TAB_ID","url":"http://fixtures/wiki.html"}'

curl "http://localhost:9867/tabs/TAB_ID/snapshot?format=compact&maxTokens=1500" \
  -H "Authorization: Bearer benchmark-token"
```
**Pass if**: Contains `VERIFY_WIKI_INDEX_55555` AND `COUNT_LANGUAGES_12` AND `COUNT_TOOLS_15`.

### 1.2 Click a link
```bash
curl -X POST http://localhost:9867/tabs/TAB_ID/action \
  -H "Authorization: Bearer benchmark-token" \
  -H "Content-Type: application/json" \
  -d '{"kind":"click","selector":"#link-go","waitNav":true}'
```
**Pass if**: HTTP 200 with `{"success":true}`.

### 1.3 Table extraction
```bash
curl "http://localhost:9867/tabs/TAB_ID/snapshot?format=compact&maxTokens=2000" \
  -H "Authorization: Bearer benchmark-token"
```
**Pass if**: Contains `VERIFY_WIKI_GO_LANG_88888` AND `Robert Griesemer` AND `2009`.

### 1.4 Count list items
```bash
curl "http://localhost:9867/tabs/TAB_ID/snapshot?format=compact&maxTokens=2000" \
  -H "Authorization: Bearer benchmark-token"
```
**Pass if**: Contains `FEATURE_COUNT_6`.

### 1.5 Article headlines
```bash
curl -X POST http://localhost:9867/navigate \
  -H "Authorization: Bearer benchmark-token" \
  -H "Content-Type: application/json" \
  -d '{"tabId":"TAB_ID","url":"http://fixtures/articles.html"}'

curl "http://localhost:9867/tabs/TAB_ID/snapshot?format=compact&maxTokens=2000" \
  -H "Authorization: Bearer benchmark-token"
```
**Pass if**: Contains `The Future of Artificial Intelligence` AND `Climate Action in 2026` AND `Mars Colony`.

### 1.6 Dashboard metrics
```bash
curl -X POST http://localhost:9867/navigate \
  -H "Authorization: Bearer benchmark-token" \
  -H "Content-Type: application/json" \
  -d '{"tabId":"TAB_ID","url":"http://fixtures/dashboard.html"}'

curl "http://localhost:9867/tabs/TAB_ID/snapshot?format=compact&maxTokens=2000" \
  -H "Authorization: Bearer benchmark-token"
```
**Pass if**: Contains `24,582` AND `$1,284,930` AND `4.28%`.

---

## Group 2: Search & Dynamic

### 2.1 Wiki search
```bash
curl -X POST http://localhost:9867/navigate \
  -H "Authorization: Bearer benchmark-token" \
  -H "Content-Type: application/json" \
  -d '{"tabId":"TAB_ID","url":"http://fixtures/wiki.html"}'

curl -X POST http://localhost:9867/tabs/TAB_ID/action \
  -H "Authorization: Bearer benchmark-token" \
  -H "Content-Type: application/json" \
  -d '{"kind":"fill","selector":"#wiki-search-input","text":"golang"}'

curl -X POST http://localhost:9867/tabs/TAB_ID/action \
  -H "Authorization: Bearer benchmark-token" \
  -H "Content-Type: application/json" \
  -d '{"kind":"click","selector":"#wiki-search-btn","waitNav":true}'

curl "http://localhost:9867/tabs/TAB_ID/snapshot?format=compact&maxTokens=2000" \
  -H "Authorization: Bearer benchmark-token"
```
**Pass if**: Snapshot contains `VERIFY_WIKI_GO_LANG_88888` (search redirected to Go page).

### 2.2 No results search
```bash
curl -X POST http://localhost:9867/navigate \
  -H "Authorization: Bearer benchmark-token" \
  -H "Content-Type: application/json" \
  -d '{"tabId":"TAB_ID","url":"http://fixtures/search.html"}'

curl -X POST http://localhost:9867/tabs/TAB_ID/action \
  -H "Authorization: Bearer benchmark-token" \
  -H "Content-Type: application/json" \
  -d '{"kind":"fill","selector":"#search-input","text":"xyznonexistent"}'

curl -X POST http://localhost:9867/tabs/TAB_ID/action \
  -H "Authorization: Bearer benchmark-token" \
  -H "Content-Type: application/json" \
  -d '{"kind":"click","selector":"#search-btn"}'

curl "http://localhost:9867/tabs/TAB_ID/snapshot?format=compact&maxTokens=500" \
  -H "Authorization: Bearer benchmark-token"
```
**Pass if**: All curls return HTTP 200 AND snapshot shows no-results message.

### 2.3 AI content search
```bash
curl -X POST http://localhost:9867/navigate \
  -H "Authorization: Bearer benchmark-token" \
  -H "Content-Type: application/json" \
  -d '{"tabId":"TAB_ID","url":"http://fixtures/search.html"}'

curl -X POST http://localhost:9867/tabs/TAB_ID/action \
  -H "Authorization: Bearer benchmark-token" \
  -H "Content-Type: application/json" \
  -d '{"kind":"fill","selector":"#search-input","text":"artificial intelligence"}'

curl -X POST http://localhost:9867/tabs/TAB_ID/action \
  -H "Authorization: Bearer benchmark-token" \
  -H "Content-Type: application/json" \
  -d '{"kind":"click","selector":"#search-btn"}'

curl "http://localhost:9867/tabs/TAB_ID/snapshot?format=compact&maxTokens=1000" \
  -H "Authorization: Bearer benchmark-token"
```
**Pass if**: Snapshot contains `The Future of Artificial Intelligence`.

---

## Group 3: Form

### 3.1 Complete form
```bash
curl -X POST http://localhost:9867/navigate \
  -H "Authorization: Bearer benchmark-token" \
  -H "Content-Type: application/json" \
  -d '{"tabId":"TAB_ID","url":"http://fixtures/form.html"}'

curl -X POST http://localhost:9867/tabs/TAB_ID/action \
  -H "Authorization: Bearer benchmark-token" \
  -H "Content-Type: application/json" \
  -d '{"kind":"fill","selector":"#fullname","text":"John Benchmark"}'

curl -X POST http://localhost:9867/tabs/TAB_ID/action \
  -H "Authorization: Bearer benchmark-token" \
  -H "Content-Type: application/json" \
  -d '{"kind":"fill","selector":"#email","text":"john@benchmark.test"}'

curl -X POST http://localhost:9867/tabs/TAB_ID/action \
  -H "Authorization: Bearer benchmark-token" \
  -H "Content-Type: application/json" \
  -d '{"kind":"fill","selector":"#phone","text":"+44 20 1234 5678"}'

curl -X POST http://localhost:9867/tabs/TAB_ID/action \
  -H "Authorization: Bearer benchmark-token" \
  -H "Content-Type: application/json" \
  -d '{"kind":"select","selector":"#country","value":"uk"}'

curl -X POST http://localhost:9867/tabs/TAB_ID/action \
  -H "Authorization: Bearer benchmark-token" \
  -H "Content-Type: application/json" \
  -d '{"kind":"select","selector":"#subject","value":"support"}'

curl -X POST http://localhost:9867/tabs/TAB_ID/action \
  -H "Authorization: Bearer benchmark-token" \
  -H "Content-Type: application/json" \
  -d '{"kind":"fill","selector":"#message","text":"This is a benchmark test message for PinchTab automation."}'

curl -X POST http://localhost:9867/tabs/TAB_ID/action \
  -H "Authorization: Bearer benchmark-token" \
  -H "Content-Type: application/json" \
  -d '{"kind":"click","selector":"#newsletter"}'

curl -X POST http://localhost:9867/tabs/TAB_ID/action \
  -H "Authorization: Bearer benchmark-token" \
  -H "Content-Type: application/json" \
  -d '{"kind":"click","selector":"input[name=priority][value=high]"}'

curl -X POST http://localhost:9867/tabs/TAB_ID/action \
  -H "Authorization: Bearer benchmark-token" \
  -H "Content-Type: application/json" \
  -d '{"kind":"click","selector":"#submit-btn"}'

curl "http://localhost:9867/tabs/TAB_ID/snapshot?format=compact&maxTokens=1000" \
  -H "Authorization: Bearer benchmark-token"
```
**Pass if**: Contains `VERIFY_FORM_SUBMITTED_SUCCESS` AND `SUBMISSION_DATA_NAME_JOHN_BENCHMARK`.

### 3.2 Reset/refill
```bash
curl "http://localhost:9867/tabs/TAB_ID/snapshot?filter=interactive&format=compact" \
  -H "Authorization: Bearer benchmark-token"
```
**Pass if**: Contains reset button or form element (after submission, page still has interactive elements).

---

## Group 4: SPA

### 4.1 Read app state
```bash
curl -X POST http://localhost:9867/navigate \
  -H "Authorization: Bearer benchmark-token" \
  -H "Content-Type: application/json" \
  -d '{"tabId":"TAB_ID","url":"http://fixtures/spa.html?reset=1"}'

curl "http://localhost:9867/tabs/TAB_ID/snapshot?format=compact&maxTokens=1500" \
  -H "Authorization: Bearer benchmark-token"
```
**Pass if**: Contains `VERIFY_SPA_PAGE_99999` AND `TASK_STATS_TOTAL_3_ACTIVE_2_DONE_1`.

### 4.2 Add task
```bash
curl -X POST http://localhost:9867/tabs/TAB_ID/action \
  -H "Authorization: Bearer benchmark-token" \
  -H "Content-Type: application/json" \
  -d '{"kind":"fill","selector":"#new-task-input","text":"Deploy to production"}'

curl -X POST http://localhost:9867/tabs/TAB_ID/action \
  -H "Authorization: Bearer benchmark-token" \
  -H "Content-Type: application/json" \
  -d '{"kind":"select","selector":"#priority-select","value":"high"}'

curl -X POST http://localhost:9867/tabs/TAB_ID/action \
  -H "Authorization: Bearer benchmark-token" \
  -H "Content-Type: application/json" \
  -d '{"kind":"click","selector":"#add-task-btn"}'

curl "http://localhost:9867/tabs/TAB_ID/snapshot?format=compact&maxTokens=1500" \
  -H "Authorization: Bearer benchmark-token"
```
**Pass if**: Contains `TASK_ADDED_DEPLOY_TO_PRODUCTION_PRIORITY_HIGH`.

### 4.3 Delete task
```bash
curl -X POST http://localhost:9867/tabs/TAB_ID/action \
  -H "Authorization: Bearer benchmark-token" \
  -H "Content-Type: application/json" \
  -d '{"kind":"click","selector":"#task-1 .delete-task"}'

curl "http://localhost:9867/tabs/TAB_ID/snapshot?format=compact&maxTokens=500" \
  -H "Authorization: Bearer benchmark-token"
```
**Pass if**: Total count is `3` (started with 3, added 1, deleted 1 = 3).

---

## Group 5: Login

### 5.1 Invalid login
```bash
curl -X POST http://localhost:9867/navigate \
  -H "Authorization: Bearer benchmark-token" \
  -H "Content-Type: application/json" \
  -d '{"tabId":"TAB_ID","url":"http://fixtures/login.html"}'

curl -X POST http://localhost:9867/tabs/TAB_ID/action \
  -H "Authorization: Bearer benchmark-token" \
  -H "Content-Type: application/json" \
  -d '{"kind":"fill","selector":"#username","text":"baduser"}'

curl -X POST http://localhost:9867/tabs/TAB_ID/action \
  -H "Authorization: Bearer benchmark-token" \
  -H "Content-Type: application/json" \
  -d '{"kind":"fill","selector":"#password","text":"wrongpassword"}'

curl -X POST http://localhost:9867/tabs/TAB_ID/action \
  -H "Authorization: Bearer benchmark-token" \
  -H "Content-Type: application/json" \
  -d '{"kind":"click","selector":"#login-btn"}'

curl "http://localhost:9867/tabs/TAB_ID/snapshot?format=compact&maxTokens=500" \
  -H "Authorization: Bearer benchmark-token"
```
**Pass if**: Contains `INVALID_CREDENTIALS_ERROR`.

### 5.2 Valid login
```bash
curl -X POST http://localhost:9867/tabs/TAB_ID/action \
  -H "Authorization: Bearer benchmark-token" \
  -H "Content-Type: application/json" \
  -d '{"kind":"fill","selector":"#username","text":"benchmark"}'

curl -X POST http://localhost:9867/tabs/TAB_ID/action \
  -H "Authorization: Bearer benchmark-token" \
  -H "Content-Type: application/json" \
  -d '{"kind":"fill","selector":"#password","text":"test456"}'

curl -X POST http://localhost:9867/tabs/TAB_ID/action \
  -H "Authorization: Bearer benchmark-token" \
  -H "Content-Type: application/json" \
  -d '{"kind":"click","selector":"#login-btn"}'

curl "http://localhost:9867/tabs/TAB_ID/snapshot?format=compact&maxTokens=500" \
  -H "Authorization: Bearer benchmark-token"
```
**Pass if**: Contains `VERIFY_LOGIN_SUCCESS_DASHBOARD` AND `SESSION_TOKEN_ACTIVE_TRUE`.

---

## Group 6: E-commerce

### 6.1 Research products
```bash
curl -X POST http://localhost:9867/navigate \
  -H "Authorization: Bearer benchmark-token" \
  -H "Content-Type: application/json" \
  -d '{"tabId":"TAB_ID","url":"http://fixtures/ecommerce.html"}'

curl "http://localhost:9867/tabs/TAB_ID/snapshot?format=compact&maxTokens=2000" \
  -H "Authorization: Bearer benchmark-token"
```
**Pass if**: Contains `VERIFY_SHOP_PAGE_44444` AND `$149.99` AND `$299.99` AND `$49.99` AND `Out of Stock`.

### 6.2 Add to cart
```bash
curl -X POST http://localhost:9867/tabs/TAB_ID/action \
  -H "Authorization: Bearer benchmark-token" \
  -H "Content-Type: application/json" \
  -d '{"kind":"click","selector":"#product-1 .add-to-cart"}'

curl -X POST http://localhost:9867/tabs/TAB_ID/action \
  -H "Authorization: Bearer benchmark-token" \
  -H "Content-Type: application/json" \
  -d '{"kind":"click","selector":"#product-2 .add-to-cart"}'

curl "http://localhost:9867/tabs/TAB_ID/snapshot?format=compact&maxTokens=1000" \
  -H "Authorization: Bearer benchmark-token"
```
**Pass if**: Contains `CART_ITEM_WIRELESS_HEADPHONES` AND `CART_ITEM_SMART_WATCH_PRO` AND `449.98`.

### 6.3 Checkout
```bash
curl -X POST http://localhost:9867/tabs/TAB_ID/action \
  -H "Authorization: Bearer benchmark-token" \
  -H "Content-Type: application/json" \
  -d '{"kind":"click","selector":"#checkout-btn"}'

curl "http://localhost:9867/tabs/TAB_ID/snapshot?format=compact&maxTokens=2000" \
  -H "Authorization: Bearer benchmark-token"
```
**Pass if**: Contains `VERIFY_CHECKOUT_SUCCESS_ORDER` AND `ORDER_TOTAL_449_98`.

---

## Group 7: Content + Interaction

### 7.1 Read & comment
```bash
curl -X POST http://localhost:9867/navigate \
  -H "Authorization: Bearer benchmark-token" \
  -H "Content-Type: application/json" \
  -d '{"tabId":"TAB_ID","url":"http://fixtures/wiki-go.html"}'

curl "http://localhost:9867/tabs/TAB_ID/snapshot?format=compact&maxTokens=2000" \
  -H "Authorization: Bearer benchmark-token"

curl -X POST http://localhost:9867/tabs/TAB_ID/action \
  -H "Authorization: Bearer benchmark-token" \
  -H "Content-Type: application/json" \
  -d '{"kind":"fill","selector":"#comment-text","text":"Great article on Go! Very comprehensive."}'

curl -X POST http://localhost:9867/tabs/TAB_ID/action \
  -H "Authorization: Bearer benchmark-token" \
  -H "Content-Type: application/json" \
  -d '{"kind":"select","selector":"#comment-rating","value":"5"}'

curl -X POST http://localhost:9867/tabs/TAB_ID/action \
  -H "Authorization: Bearer benchmark-token" \
  -H "Content-Type: application/json" \
  -d '{"kind":"click","selector":"#submit-comment"}'

curl "http://localhost:9867/tabs/TAB_ID/snapshot?format=compact&maxTokens=2000" \
  -H "Authorization: Bearer benchmark-token"
```
**Pass if**: First snapshot contains `VERIFY_WIKI_GO_LANG_88888` AND final snapshot contains `COMMENT_POSTED_RATING_5_TEXT_RECEIVED`.

### 7.2 Cross-page research
```bash
curl -X POST http://localhost:9867/navigate \
  -H "Authorization: Bearer benchmark-token" \
  -H "Content-Type: application/json" \
  -d '{"tabId":"TAB_ID","url":"http://fixtures/wiki.html"}'

curl "http://localhost:9867/tabs/TAB_ID/snapshot?format=compact&maxTokens=1500" \
  -H "Authorization: Bearer benchmark-token"

curl -X POST http://localhost:9867/tabs/TAB_ID/action \
  -H "Authorization: Bearer benchmark-token" \
  -H "Content-Type: application/json" \
  -d '{"kind":"click","selector":"#link-go","waitNav":true}'

curl "http://localhost:9867/tabs/TAB_ID/snapshot?format=compact&maxTokens=500" \
  -H "Authorization: Bearer benchmark-token"
```
**Pass if**: First snapshot contains `COUNT_LANGUAGES_12` AND second snapshot contains `VERIFY_WIKI_GO_LANG_88888`.

---

## Group 8: Error Handling

### 8.1 404 handling
```bash
curl -X POST http://localhost:9867/navigate \
  -H "Authorization: Bearer benchmark-token" \
  -H "Content-Type: application/json" \
  -d '{"tabId":"TAB_ID","url":"http://fixtures/nonexistent-page-xyz.html"}'
```
**Pass if**: Returns response without crash (HTTP 200 with error page, or structured error).

### 8.2 Missing element
```bash
curl -X POST http://localhost:9867/tabs/TAB_ID/action \
  -H "Authorization: Bearer benchmark-token" \
  -H "Content-Type: application/json" \
  -d '{"kind":"click","selector":"#element-that-does-not-exist"}'
```
**Pass if**: Error response with clear message (not crash).

---

## Group 9: Export

### 9.1 Screenshot
```bash
curl -X POST http://localhost:9867/navigate \
  -H "Authorization: Bearer benchmark-token" \
  -H "Content-Type: application/json" \
  -d '{"tabId":"TAB_ID","url":"http://fixtures/dashboard.html"}'

curl http://localhost:9867/tabs/TAB_ID/screenshot \
  -H "Authorization: Bearer benchmark-token" \
  --output /tmp/benchmark-screenshot.png

ls -la /tmp/benchmark-screenshot.png
```
**Pass if**: File exists and size > 10240 bytes.

### 9.2 PDF export
```bash
curl http://localhost:9867/tabs/TAB_ID/pdf \
  -H "Authorization: Bearer benchmark-token" \
  --output /tmp/benchmark-dashboard.pdf

ls -la /tmp/benchmark-dashboard.pdf
```
**Pass if**: File exists and size > 10240 bytes.

---

## Group 10: Modals

### 10.1 Open modal
```bash
curl -X POST http://localhost:9867/navigate \
  -H "Authorization: Bearer benchmark-token" \
  -H "Content-Type: application/json" \
  -d '{"tabId":"TAB_ID","url":"http://fixtures/dashboard.html"}'

curl -X POST http://localhost:9867/tabs/TAB_ID/action \
  -H "Authorization: Bearer benchmark-token" \
  -H "Content-Type: application/json" \
  -d '{"kind":"click","selector":"#settings-btn"}'

curl "http://localhost:9867/tabs/TAB_ID/snapshot?format=compact&maxTokens=1000" \
  -H "Authorization: Bearer benchmark-token"
```
**Pass if**: Snapshot contains `Dashboard Settings`.

### 10.2 Modal interaction
```bash
curl -X POST http://localhost:9867/tabs/TAB_ID/action \
  -H "Authorization: Bearer benchmark-token" \
  -H "Content-Type: application/json" \
  -d '{"kind":"select","selector":"#theme-select","value":"dark"}'

curl -X POST http://localhost:9867/tabs/TAB_ID/action \
  -H "Authorization: Bearer benchmark-token" \
  -H "Content-Type: application/json" \
  -d '{"kind":"click","selector":"#modal-save"}'

curl "http://localhost:9867/tabs/TAB_ID/snapshot?format=compact&maxTokens=500" \
  -H "Authorization: Bearer benchmark-token"
```
**Pass if**: Contains `THEME_DARK_APPLIED`.

---

## Group 11: Persistence

### 11.1 State after reload
```bash
# Start fresh (reset param clears localStorage on this load only)
curl -X POST http://localhost:9867/navigate \
  -H "Authorization: Bearer benchmark-token" \
  -H "Content-Type: application/json" \
  -d '{"tabId":"TAB_ID","url":"http://fixtures/spa.html?reset=1"}'

# Add the persistent task
curl -X POST http://localhost:9867/tabs/TAB_ID/action \
  -H "Authorization: Bearer benchmark-token" \
  -H "Content-Type: application/json" \
  -d '{"kind":"fill","selector":"#new-task-input","text":"Persistent Task Test"}'

curl -X POST http://localhost:9867/tabs/TAB_ID/action \
  -H "Authorization: Bearer benchmark-token" \
  -H "Content-Type: application/json" \
  -d '{"kind":"click","selector":"#add-task-btn"}'

# Navigate away then back WITHOUT reset param — state should persist in localStorage
curl -X POST http://localhost:9867/navigate \
  -H "Authorization: Bearer benchmark-token" \
  -H "Content-Type: application/json" \
  -d '{"tabId":"TAB_ID","url":"http://fixtures/"}'

curl -X POST http://localhost:9867/navigate \
  -H "Authorization: Bearer benchmark-token" \
  -H "Content-Type: application/json" \
  -d '{"tabId":"TAB_ID","url":"http://fixtures/spa.html"}'

curl "http://localhost:9867/tabs/TAB_ID/snapshot?format=compact&maxTokens=1500" \
  -H "Authorization: Bearer benchmark-token"
```
**Pass if**: Contains `TASK_PERSISTENT_TEST_FOUND_AFTER_RELOAD`.

### 11.2 Logout/re-login
```bash
curl -X POST http://localhost:9867/navigate \
  -H "Authorization: Bearer benchmark-token" \
  -H "Content-Type: application/json" \
  -d '{"tabId":"TAB_ID","url":"http://fixtures/login.html"}'

curl -X POST http://localhost:9867/tabs/TAB_ID/action \
  -H "Authorization: Bearer benchmark-token" \
  -H "Content-Type: application/json" \
  -d '{"kind":"fill","selector":"#username","text":"benchmark"}'

curl -X POST http://localhost:9867/tabs/TAB_ID/action \
  -H "Authorization: Bearer benchmark-token" \
  -H "Content-Type: application/json" \
  -d '{"kind":"fill","selector":"#password","text":"test456"}'

curl -X POST http://localhost:9867/tabs/TAB_ID/action \
  -H "Authorization: Bearer benchmark-token" \
  -H "Content-Type: application/json" \
  -d '{"kind":"click","selector":"#login-btn"}'

curl -X POST http://localhost:9867/tabs/TAB_ID/action \
  -H "Authorization: Bearer benchmark-token" \
  -H "Content-Type: application/json" \
  -d '{"kind":"click","selector":"#logout-btn"}'

curl -X POST http://localhost:9867/tabs/TAB_ID/action \
  -H "Authorization: Bearer benchmark-token" \
  -H "Content-Type: application/json" \
  -d '{"kind":"fill","selector":"#username","text":"benchmark"}'

curl -X POST http://localhost:9867/tabs/TAB_ID/action \
  -H "Authorization: Bearer benchmark-token" \
  -H "Content-Type: application/json" \
  -d '{"kind":"fill","selector":"#password","text":"test456"}'

curl -X POST http://localhost:9867/tabs/TAB_ID/action \
  -H "Authorization: Bearer benchmark-token" \
  -H "Content-Type: application/json" \
  -d '{"kind":"click","selector":"#login-btn"}'

curl "http://localhost:9867/tabs/TAB_ID/snapshot?format=compact&maxTokens=500" \
  -H "Authorization: Bearer benchmark-token"
```
**Pass if**: Contains `VERIFY_LOGIN_SUCCESS_DASHBOARD` AND `SESSION_RENEWED`.

---

## Group 12: Multi-page Nav

### 12.1 Navigate & return
```bash
curl -X POST http://localhost:9867/navigate \
  -H "Authorization: Bearer benchmark-token" \
  -H "Content-Type: application/json" \
  -d '{"tabId":"TAB_ID","url":"http://fixtures/"}'

curl -X POST http://localhost:9867/navigate \
  -H "Authorization: Bearer benchmark-token" \
  -H "Content-Type: application/json" \
  -d '{"tabId":"TAB_ID","url":"http://fixtures/wiki.html"}'

curl -X POST http://localhost:9867/tabs/TAB_ID/action \
  -H "Authorization: Bearer benchmark-token" \
  -H "Content-Type: application/json" \
  -d '{"kind":"click","selector":"#link-go","waitNav":true}'

curl -X POST http://localhost:9867/tabs/TAB_ID/back \
  -H "Authorization: Bearer benchmark-token"

curl -X POST http://localhost:9867/tabs/TAB_ID/back \
  -H "Authorization: Bearer benchmark-token"

curl "http://localhost:9867/tabs/TAB_ID/snapshot?format=compact&maxTokens=500" \
  -H "Authorization: Bearer benchmark-token"
```
**Pass if**: Final snapshot contains `VERIFY_HOME_LOADED_12345` (returned to home).

### 12.2 Cross-page compare
```bash
curl -X POST http://localhost:9867/navigate \
  -H "Authorization: Bearer benchmark-token" \
  -H "Content-Type: application/json" \
  -d '{"tabId":"TAB_ID","url":"http://fixtures/wiki.html"}'

curl "http://localhost:9867/tabs/TAB_ID/snapshot?format=compact&maxTokens=1500" \
  -H "Authorization: Bearer benchmark-token"

curl -X POST http://localhost:9867/navigate \
  -H "Authorization: Bearer benchmark-token" \
  -H "Content-Type: application/json" \
  -d '{"tabId":"TAB_ID","url":"http://fixtures/articles.html"}'

curl "http://localhost:9867/tabs/TAB_ID/snapshot?format=compact&maxTokens=1500" \
  -H "Authorization: Bearer benchmark-token"
```
**Pass if**: Wiki snapshot contains `COUNT_LANGUAGES_12` AND articles snapshot contains article titles.

---

## Group 13: Form Validation

### 13.1 Required field
```bash
curl -X POST http://localhost:9867/navigate \
  -H "Authorization: Bearer benchmark-token" \
  -H "Content-Type: application/json" \
  -d '{"tabId":"TAB_ID","url":"http://fixtures/form.html"}'

curl -X POST http://localhost:9867/tabs/TAB_ID/action \
  -H "Authorization: Bearer benchmark-token" \
  -H "Content-Type: application/json" \
  -d '{"kind":"fill","selector":"#fullname","text":"Validator Test"}'

curl -X POST http://localhost:9867/tabs/TAB_ID/action \
  -H "Authorization: Bearer benchmark-token" \
  -H "Content-Type: application/json" \
  -d '{"kind":"click","selector":"#submit-btn"}'

curl "http://localhost:9867/tabs/TAB_ID/snapshot?format=compact&maxTokens=1000" \
  -H "Authorization: Bearer benchmark-token"
```
**Pass if**: Snapshot does NOT contain `VERIFY_FORM_SUBMITTED_SUCCESS` (submission blocked by validation).

### 13.2 Optional field
```bash
curl -X POST http://localhost:9867/navigate \
  -H "Authorization: Bearer benchmark-token" \
  -H "Content-Type: application/json" \
  -d '{"tabId":"TAB_ID","url":"http://fixtures/form.html"}'

curl -X POST http://localhost:9867/tabs/TAB_ID/action \
  -H "Authorization: Bearer benchmark-token" \
  -H "Content-Type: application/json" \
  -d '{"kind":"fill","selector":"#fullname","text":"No Phone User"}'

curl -X POST http://localhost:9867/tabs/TAB_ID/action \
  -H "Authorization: Bearer benchmark-token" \
  -H "Content-Type: application/json" \
  -d '{"kind":"fill","selector":"#email","text":"nophone@test.com"}'

curl -X POST http://localhost:9867/tabs/TAB_ID/action \
  -H "Authorization: Bearer benchmark-token" \
  -H "Content-Type: application/json" \
  -d '{"kind":"select","selector":"#country","value":"de"}'

curl -X POST http://localhost:9867/tabs/TAB_ID/action \
  -H "Authorization: Bearer benchmark-token" \
  -H "Content-Type: application/json" \
  -d '{"kind":"select","selector":"#subject","value":"feedback"}'

curl -X POST http://localhost:9867/tabs/TAB_ID/action \
  -H "Authorization: Bearer benchmark-token" \
  -H "Content-Type: application/json" \
  -d '{"kind":"click","selector":"#submit-btn"}'

curl "http://localhost:9867/tabs/TAB_ID/snapshot?format=compact&maxTokens=1000" \
  -H "Authorization: Bearer benchmark-token"
```
**Pass if**: Contains `VERIFY_FORM_SUBMITTED_SUCCESS` AND `OPTIONAL_FIELD_SKIPPED_SUCCESS`.

---

## Group 14: Dynamic Content

### 14.1 Load more
```bash
curl -X POST http://localhost:9867/navigate \
  -H "Authorization: Bearer benchmark-token" \
  -H "Content-Type: application/json" \
  -d '{"tabId":"TAB_ID","url":"http://fixtures/ecommerce.html"}'

curl -X POST http://localhost:9867/tabs/TAB_ID/action \
  -H "Authorization: Bearer benchmark-token" \
  -H "Content-Type: application/json" \
  -d '{"kind":"click","selector":"#load-more-btn"}'

curl "http://localhost:9867/tabs/TAB_ID/snapshot?format=compact&maxTokens=2000" \
  -H "Authorization: Bearer benchmark-token"
```
**Pass if**: Contains `ADDITIONAL_PRODUCTS_LOADED`.

### 14.2 Lazy-loaded item
```bash
curl -X POST http://localhost:9867/tabs/TAB_ID/action \
  -H "Authorization: Bearer benchmark-token" \
  -H "Content-Type: application/json" \
  -d '{"kind":"click","selector":"#product-5 .add-to-cart"}'

curl "http://localhost:9867/tabs/TAB_ID/snapshot?format=compact&maxTokens=1000" \
  -H "Authorization: Bearer benchmark-token"
```
**Pass if**: Contains `CART_UPDATED_WITH_LAZY_PRODUCT`.

---

## Group 15: Data Aggregation

### 15.1 Financial calc
```bash
curl -X POST http://localhost:9867/navigate \
  -H "Authorization: Bearer benchmark-token" \
  -H "Content-Type: application/json" \
  -d '{"tabId":"TAB_ID","url":"http://fixtures/dashboard.html"}'

curl "http://localhost:9867/tabs/TAB_ID/snapshot?format=compact&maxTokens=2000" \
  -H "Authorization: Bearer benchmark-token"
```
**Pass if**: Contains `$1,284,930` (revenue) AND `$384,930` (profit) AND `PROFIT_MARGIN_CALCULATED`.

### 15.2 Multi-page comparison
```bash
curl -X POST http://localhost:9867/navigate \
  -H "Authorization: Bearer benchmark-token" \
  -H "Content-Type: application/json" \
  -d '{"tabId":"TAB_ID","url":"http://fixtures/wiki-go.html"}'

curl "http://localhost:9867/tabs/TAB_ID/snapshot?format=compact&maxTokens=1000" \
  -H "Authorization: Bearer benchmark-token"

curl -X POST http://localhost:9867/navigate \
  -H "Authorization: Bearer benchmark-token" \
  -H "Content-Type: application/json" \
  -d '{"tabId":"TAB_ID","url":"http://fixtures/wiki-python.html"}'

curl "http://localhost:9867/tabs/TAB_ID/snapshot?format=compact&maxTokens=1000" \
  -H "Authorization: Bearer benchmark-token"

curl -X POST http://localhost:9867/navigate \
  -H "Authorization: Bearer benchmark-token" \
  -H "Content-Type: application/json" \
  -d '{"tabId":"TAB_ID","url":"http://fixtures/wiki-rust.html"}'

curl "http://localhost:9867/tabs/TAB_ID/snapshot?format=compact&maxTokens=1000" \
  -H "Authorization: Bearer benchmark-token"
```
**Pass if**: Go snapshot contains `FEATURE_COUNT_6` AND Python snapshot contains `FEATURE_COUNT_7` AND `COMPARISON_TABLE_BUILT` AND Rust snapshot contains `FEATURE_COUNT_5`.

---

## Group 16: Hover & Tooltips

### 16.1 Hover reveals info
```bash
curl -X POST http://localhost:9867/navigate \
  -H "Authorization: Bearer benchmark-token" \
  -H "Content-Type: application/json" \
  -d '{"tabId":"TAB_ID","url":"http://fixtures/hovers.html"}'

curl -X POST http://localhost:9867/tabs/TAB_ID/action \
  -H "Authorization: Bearer benchmark-token" \
  -H "Content-Type: application/json" \
  -d '{"kind":"hover","selector":"#avatar-1"}'

curl "http://localhost:9867/tabs/TAB_ID/snapshot?format=compact&maxTokens=1000" \
  -H "Authorization: Bearer benchmark-token"
```
**Pass if**: Snapshot contains `HOVER_REVEALED_USER_1`.

### 16.2 Hover swap
```bash
curl -X POST http://localhost:9867/tabs/TAB_ID/action \
  -H "Authorization: Bearer benchmark-token" \
  -H "Content-Type: application/json" \
  -d '{"kind":"hover","selector":"#avatar-2"}'

curl "http://localhost:9867/tabs/TAB_ID/snapshot?format=compact&maxTokens=1000" \
  -H "Authorization: Bearer benchmark-token"
```
**Pass if**: Snapshot contains `HOVER_REVEALED_USER_2`.

---

## Group 17: Scrolling

### 17.1 Scroll by pixels
```bash
curl -X POST http://localhost:9867/navigate \
  -H "Authorization: Bearer benchmark-token" \
  -H "Content-Type: application/json" \
  -d '{"tabId":"TAB_ID","url":"http://fixtures/scroll.html"}'

curl -X POST http://localhost:9867/tabs/TAB_ID/action \
  -H "Authorization: Bearer benchmark-token" \
  -H "Content-Type: application/json" \
  -d '{"kind":"scroll","scrollY":1500}'

curl "http://localhost:9867/tabs/TAB_ID/snapshot?format=compact&maxTokens=1500" \
  -H "Authorization: Bearer benchmark-token"
```
**Pass if**: Snapshot contains `SCROLL_MIDDLE_MARKER`.

### 17.2 Scroll to footer
```bash
curl -X POST http://localhost:9867/tabs/TAB_ID/action \
  -H "Authorization: Bearer benchmark-token" \
  -H "Content-Type: application/json" \
  -d '{"kind":"scroll","selector":"#footer"}'

curl "http://localhost:9867/tabs/TAB_ID/snapshot?format=compact&maxTokens=1500" \
  -H "Authorization: Bearer benchmark-token"
```
**Pass if**: Snapshot contains `SCROLL_REACHED_FOOTER`.

---

## Group 18: File Download

### 18.1 Download a file
```bash
curl "http://localhost:9867/tabs/TAB_ID/download?url=http://fixtures/download-sample.txt" \
  -H "Authorization: Bearer benchmark-token" | \
  jq -r .data | base64 -d > /tmp/benchmark-download.txt

grep "DOWNLOAD_FILE_CONTENT_VERIFIED" /tmp/benchmark-download.txt
```
**Pass if**: File exists and contains `DOWNLOAD_FILE_CONTENT_VERIFIED`.

**Note**: The download endpoint returns JSON with `{contentType, data (base64), size, url}`, not the raw file. Decode `.data` to get the file contents. For PinchTab to download from internal hosts (like `fixtures`), the domain must be in `security.downloadAllowedDomains` config.

---

## Group 19: iFrame

### 19.1 Read iframe content
```bash
curl -X POST http://localhost:9867/navigate \
  -H "Authorization: Bearer benchmark-token" \
  -H "Content-Type: application/json" \
  -d '{"tabId":"TAB_ID","url":"http://fixtures/iframe.html"}'

curl "http://localhost:9867/tabs/TAB_ID/snapshot?format=compact&maxTokens=2000" \
  -H "Authorization: Bearer benchmark-token"
```
**Pass if**: Snapshot contains `IFRAME_INNER_CONTENT_LOADED`.

### 19.2 Type into iframe input (native frame scope)
```bash
# Scope into the same-origin iframe
curl -X POST http://localhost:9867/frame?tabId=TAB_ID \
  -H "Authorization: Bearer benchmark-token" \
  -H "Content-Type: application/json" \
  -d '{"target":"#content-frame"}'

# Selectors now resolve inside the iframe
curl -X POST http://localhost:9867/tabs/TAB_ID/action \
  -H "Authorization: Bearer benchmark-token" \
  -H "Content-Type: application/json" \
  -d '{"kind":"fill","selector":"#iframe-input","text":"Hello World"}'

curl -X POST http://localhost:9867/tabs/TAB_ID/action \
  -H "Authorization: Bearer benchmark-token" \
  -H "Content-Type: application/json" \
  -d '{"kind":"click","selector":"#iframe-submit"}'

# Verify via a scoped snap (text --full doesn't pierce frames)
curl "http://localhost:9867/tabs/TAB_ID/snapshot?format=compact&maxTokens=500" \
  -H "Authorization: Bearer benchmark-token"

# Reset scope
curl -X POST http://localhost:9867/frame?tabId=TAB_ID \
  -H "Authorization: Bearer benchmark-token" \
  -H "Content-Type: application/json" \
  -d '{"target":"main"}'
```
**Pass if**: The scoped snapshot contains `IFRAME_INPUT_RECEIVED_HELLO_WORLD`.

**Note**: `POST /frame` sets a stateful scope on the tab. Subsequent selector-based `/action` and `/snapshot` calls resolve inside the scoped iframe. Reset with `{"target":"main"}` when done. Cross-origin iframes are not currently exposed this way — fall back to `/evaluate` + `contentDocument` for those (when same-origin policy permits).

---

## Group 20: Dialogs

### 20.1 Accept alert
```bash
curl -X POST http://localhost:9867/navigate \
  -H "Authorization: Bearer benchmark-token" \
  -H "Content-Type: application/json" \
  -d '{"tabId":"TAB_ID","url":"http://fixtures/alerts.html"}'

curl -X POST http://localhost:9867/tabs/TAB_ID/action \
  -H "Authorization: Bearer benchmark-token" \
  -H "Content-Type: application/json" \
  -d '{"kind":"click","selector":"#alert-btn","dialogAction":"accept"}'

curl "http://localhost:9867/tabs/TAB_ID/snapshot?format=compact&maxTokens=500" \
  -H "Authorization: Bearer benchmark-token"
```
**Pass if**: Snapshot contains `DIALOG_ALERT_DISMISSED`.

### 20.2 Cancel confirm
```bash
curl -X POST http://localhost:9867/tabs/TAB_ID/action \
  -H "Authorization: Bearer benchmark-token" \
  -H "Content-Type: application/json" \
  -d '{"kind":"click","selector":"#confirm-btn","dialogAction":"dismiss"}'

curl "http://localhost:9867/tabs/TAB_ID/snapshot?format=compact&maxTokens=500" \
  -H "Authorization: Bearer benchmark-token"
```
**Pass if**: Snapshot contains `DIALOG_CONFIRM_CANCELLED`.

**Note**: Use `dialogAction` on the click action to auto-accept or auto-dismiss a JS dialog that the click opens. Without it, the click would hang until `/dialog` is called from a separate request.

---

## Group 21: Async / awaitPromise

### 21.1 Await a promise-returning function
```bash
curl -X POST http://localhost:9867/navigate \
  -H "Authorization: Bearer benchmark-token" \
  -H "Content-Type: application/json" \
  -d '{"tabId":"TAB_ID","url":"http://fixtures/async.html"}'

# Without awaitPromise: returns {} (unresolved Promise representation)
curl -X POST http://localhost:9867/tabs/TAB_ID/evaluate \
  -H "Authorization: Bearer benchmark-token" \
  -H "Content-Type: application/json" \
  -d '{"expression":"window.fetchPayload()","awaitPromise":true}'
```
**Pass if**: Response `.result` equals `"ASYNC_PAYLOAD_READY_42"`. Also verify that repeating the same call with `awaitPromise` omitted returns `{}` (proves `awaitPromise` is actually doing work).

### 21.2 Await a promise resolving to an object
```bash
curl -X POST http://localhost:9867/tabs/TAB_ID/evaluate \
  -H "Authorization: Bearer benchmark-token" \
  -H "Content-Type: application/json" \
  -d '{"expression":"window.fetchUser()","awaitPromise":true}'
```
**Pass if**: Response `.result.name` equals `"ASYNC_USER_NAME_ADA"` (structured value survives the round-trip).

**Note**: See SKILL.md and `references/api.md` — `awaitPromise:true` on `/evaluate` makes the server wait for the returned Promise to settle before responding. Omit the flag when you want the raw Promise reference.

---

## Group 22: Mouse Drag & Drop

### 22.1 Drag piece into Zone A (high-level `drag` action)
```bash
curl -X POST http://localhost:9867/navigate \
  -H "Authorization: Bearer benchmark-token" \
  -H "Content-Type: application/json" \
  -d '{"tabId":"TAB_ID","url":"http://fixtures/drag.html"}'

# Piece center starts at (92, 358); Zone A center is at (104, 200).
# Drag offset: (+12, -158).
curl -X POST http://localhost:9867/tabs/TAB_ID/action \
  -H "Authorization: Bearer benchmark-token" \
  -H "Content-Type: application/json" \
  -d '{"kind":"drag","selector":"#piece","dragX":12,"dragY":-158}'

curl "http://localhost:9867/tabs/TAB_ID/text" \
  -H "Authorization: Bearer benchmark-token"
```
**Pass if**: Page text contains `LAST_DROP=DROP_ZONE_A_OK`.

### 22.2 Low-level sequence: drag into Zone B, then Zone C
```bash
# Zone centers in viewport coords (for the default window size):
#   piece starts: (92, 358)
#   Zone A:       (104, 200)
#   Zone B:       (344, 200)
#   Zone C:       (584, 400)
# If the viewport differs, query these with /evaluate +
#   Array.from(['piece','zone-a','zone-b','zone-c']).map(id => ...getBoundingClientRect()).

# Drag piece (now at Zone A) to Zone B via explicit mouse-down → mouse-move → mouse-up.
curl -X POST http://localhost:9867/tabs/TAB_ID/action \
  -H "Authorization: Bearer benchmark-token" \
  -H "Content-Type: application/json" \
  -d '{"kind":"mouse-move","x":104,"y":200}'

curl -X POST http://localhost:9867/tabs/TAB_ID/action \
  -H "Authorization: Bearer benchmark-token" \
  -H "Content-Type: application/json" \
  -d '{"kind":"mouse-down","x":104,"y":200,"button":"left"}'

curl -X POST http://localhost:9867/tabs/TAB_ID/action \
  -H "Authorization: Bearer benchmark-token" \
  -H "Content-Type: application/json" \
  -d '{"kind":"mouse-move","x":344,"y":200}'

curl -X POST http://localhost:9867/tabs/TAB_ID/action \
  -H "Authorization: Bearer benchmark-token" \
  -H "Content-Type: application/json" \
  -d '{"kind":"mouse-up","x":344,"y":200,"button":"left"}'

# Now drag Zone B → Zone C
curl -X POST http://localhost:9867/tabs/TAB_ID/action \
  -H "Authorization: Bearer benchmark-token" \
  -H "Content-Type: application/json" \
  -d '{"kind":"mouse-down","x":344,"y":200,"button":"left"}'

curl -X POST http://localhost:9867/tabs/TAB_ID/action \
  -H "Authorization: Bearer benchmark-token" \
  -H "Content-Type: application/json" \
  -d '{"kind":"mouse-move","x":584,"y":400}'

curl -X POST http://localhost:9867/tabs/TAB_ID/action \
  -H "Authorization: Bearer benchmark-token" \
  -H "Content-Type: application/json" \
  -d '{"kind":"mouse-up","x":584,"y":400,"button":"left"}'

curl "http://localhost:9867/tabs/TAB_ID/text" \
  -H "Authorization: Bearer benchmark-token"
```
**Pass if**: Page text contains `DROP_SEQUENCE=DROP_ZONE_A_OK,DROP_ZONE_B_OK,DROP_ZONE_C_OK`.

**Note**: Either path (the high-level `drag` action with `dragX`/`dragY` offsets, or the explicit `mouse-down → mouse-move → mouse-up` sequence at absolute viewport coordinates) works. Use `drag` for simple point-to-point drags, and the low-level trio when the target site depends on intermediate `mousemove` events or when you need precise pacing.

---

## Group 23: Async / Loading state

### 23.1 Wait for async content to replace a loading spinner
```bash
curl -X POST http://localhost:9867/navigate \
  -H "Authorization: Bearer benchmark-token" \
  -H "Content-Type: application/json" \
  -d '{"tabId":"TAB_ID","url":"http://fixtures/loading.html"}'

# Poll the snapshot until the final marker appears, or call /wait with a text
# predicate. Using /evaluate with awaitPromise is another valid path.
for i in 1 2 3 4 5 6 7 8 9 10; do
  SNAP=$(curl -sf "http://localhost:9867/tabs/TAB_ID/snapshot?format=compact&maxTokens=1000" \
    -H "Authorization: Bearer benchmark-token")
  echo "$SNAP" | grep -q "VERIFY_LOADING_COMPLETE_88888" && break
  sleep 0.3
done

curl "http://localhost:9867/tabs/TAB_ID/snapshot?format=compact&maxTokens=1000" \
  -H "Authorization: Bearer benchmark-token"
```
**Pass if**: Final snapshot contains `VERIFY_LOADING_COMPLETE_88888`.

**Note**: The fixture delays replacing the spinner by ~1.5 s. Agents using
`pinchtab wait --text "VERIFY_LOADING_COMPLETE_88888" --timeout 5000` hit
this in a single call. Baseline callers can poll or `sleep` between
snapshots.

---

## Group 24: Keyboard events

### 24.1 Press Escape to emit a marker
```bash
curl -X POST http://localhost:9867/navigate \
  -H "Authorization: Bearer benchmark-token" \
  -H "Content-Type: application/json" \
  -d '{"tabId":"TAB_ID","url":"http://fixtures/keyboard.html"}'

curl -X POST http://localhost:9867/tabs/TAB_ID/action \
  -H "Authorization: Bearer benchmark-token" \
  -H "Content-Type: application/json" \
  -d '{"kind":"press","key":"Escape"}'

curl "http://localhost:9867/tabs/TAB_ID/text?mode=raw&format=text" \
  -H "Authorization: Bearer benchmark-token"
```
**Pass if**: Page text contains `KEYBOARD_ESCAPE_PRESSED`. Use `mode=raw` because the log div is short enough for Readability to drop it.

### 24.2 Sequential keys (a then Enter)
```bash
curl -X POST http://localhost:9867/tabs/TAB_ID/action \
  -H "Authorization: Bearer benchmark-token" \
  -H "Content-Type: application/json" \
  -d '{"kind":"press","key":"a"}'

curl -X POST http://localhost:9867/tabs/TAB_ID/action \
  -H "Authorization: Bearer benchmark-token" \
  -H "Content-Type: application/json" \
  -d '{"kind":"press","key":"Enter"}'

curl "http://localhost:9867/tabs/TAB_ID/text" \
  -H "Authorization: Bearer benchmark-token"
```
**Pass if**: Page text contains both `KEYBOARD_KEY_A_PRESSED` and `KEYBOARD_ENTER_PRESSED` (and the Escape marker from 24.1 is still there, proving the log accumulates).

**Note**: `press` issues a single keypress and requires the page to already have focus on an element that listens for `keydown`. The fixture auto-focuses `#focus-input` on load.

---

## Group 25: Tab panels

### 25.1 Switch to Settings tab
```bash
curl -X POST http://localhost:9867/navigate \
  -H "Authorization: Bearer benchmark-token" \
  -H "Content-Type: application/json" \
  -d '{"tabId":"TAB_ID","url":"http://fixtures/tabs.html"}'

# Verify initial panel
curl "http://localhost:9867/tabs/TAB_ID/text?mode=raw&format=text" \
  -H "Authorization: Bearer benchmark-token"

curl -X POST http://localhost:9867/tabs/TAB_ID/action \
  -H "Authorization: Bearer benchmark-token" \
  -H "Content-Type: application/json" \
  -d '{"kind":"click","selector":"#tab-settings"}'

curl "http://localhost:9867/tabs/TAB_ID/text?mode=raw&format=text" \
  -H "Authorization: Bearer benchmark-token"
```
**Pass if**: Initial response contains `TAB_PROFILE_CONTENT` AND final response contains `TAB_SETTINGS_CONTENT` (and the profile content is now hidden).

### 25.2 Switch to Billing tab
```bash
curl -X POST http://localhost:9867/tabs/TAB_ID/action \
  -H "Authorization: Bearer benchmark-token" \
  -H "Content-Type: application/json" \
  -d '{"kind":"click","selector":"#tab-billing"}'

curl "http://localhost:9867/tabs/TAB_ID/text?mode=raw&format=text" \
  -H "Authorization: Bearer benchmark-token"
```
**Pass if**: Response contains `TAB_BILLING_CONTENT`.

**Note**: Tab panels toggle via the `hidden` attribute; the non-selected panels are absent from the visible text. Use `text?mode=raw` so the extraction reflects what the user actually sees.

---

## Group 26: Accordion

### 26.1 Open section A
```bash
curl -X POST http://localhost:9867/navigate \
  -H "Authorization: Bearer benchmark-token" \
  -H "Content-Type: application/json" \
  -d '{"tabId":"TAB_ID","url":"http://fixtures/accordion.html"}'

curl -X POST http://localhost:9867/tabs/TAB_ID/action \
  -H "Authorization: Bearer benchmark-token" \
  -H "Content-Type: application/json" \
  -d '{"kind":"click","selector":"#section-a .section-header"}'

curl "http://localhost:9867/tabs/TAB_ID/text?mode=raw&format=text" \
  -H "Authorization: Bearer benchmark-token"
```
**Pass if**: Response contains `ACCORDION_SECTION_A_OPEN`.

### 26.2 Open section B — A auto-closes
```bash
curl -X POST http://localhost:9867/tabs/TAB_ID/action \
  -H "Authorization: Bearer benchmark-token" \
  -H "Content-Type: application/json" \
  -d '{"kind":"click","selector":"#section-b .section-header"}'

curl "http://localhost:9867/tabs/TAB_ID/text?mode=raw&format=text" \
  -H "Authorization: Bearer benchmark-token"

# Also verify the A body text is no longer visible by inspecting aria-expanded.
curl -X POST http://localhost:9867/tabs/TAB_ID/evaluate \
  -H "Authorization: Bearer benchmark-token" \
  -H "Content-Type: application/json" \
  -d '{"expression":"document.getElementById(\"section-a\").getAttribute(\"aria-expanded\")"}'
```
**Pass if**: Text response contains `ACCORDION_SECTION_B_OPEN`, AND the `evaluate` response for the section-a state returns `"false"`.

**Note**: Accordion uses an exclusive-expand pattern (only one section open). CSS `max-height:0` collapses the closed bodies, but the text content is still in the DOM — confirming aria state is the most reliable check.

---

## Group 27: Contenteditable editor

### 27.1 Type into a `contenteditable` div
```bash
curl -X POST http://localhost:9867/navigate \
  -H "Authorization: Bearer benchmark-token" \
  -H "Content-Type: application/json" \
  -d '{"tabId":"TAB_ID","url":"http://fixtures/editor.html"}'

curl -X POST http://localhost:9867/tabs/TAB_ID/action \
  -H "Authorization: Bearer benchmark-token" \
  -H "Content-Type: application/json" \
  -d '{"kind":"type","selector":"#editor","text":"Hello rich text"}'

curl "http://localhost:9867/tabs/TAB_ID/text?mode=raw&format=text" \
  -H "Authorization: Bearer benchmark-token"
```
**Pass if**: Response contains `EDITOR_CHARS=15` AND the mirror echoes `Hello rich text`.

### 27.2 Commit by pressing Enter
```bash
curl -X POST http://localhost:9867/tabs/TAB_ID/action \
  -H "Authorization: Bearer benchmark-token" \
  -H "Content-Type: application/json" \
  -d '{"kind":"press","key":"Enter"}'

curl "http://localhost:9867/tabs/TAB_ID/text?mode=raw&format=text" \
  -H "Authorization: Bearer benchmark-token"
```
**Pass if**: Response contains `EDITOR_COMMITTED=Hello rich text`.

**Note**: `contenteditable` elements don't have a `.value` property, so `fill` won't work. Use `type` so each character is dispatched via keyboard events the browser routes into the editor. `press Enter` is intercepted (preventDefault) to commit the buffer instead of inserting a newline.

---

## Group 28: Range slider

### 28.1 Set range slider to HIGH bucket
```bash
curl -X POST http://localhost:9867/navigate \
  -H "Authorization: Bearer benchmark-token" \
  -H "Content-Type: application/json" \
  -d '{"tabId":"TAB_ID","url":"http://fixtures/range.html"}'

curl -X POST http://localhost:9867/tabs/TAB_ID/action \
  -H "Authorization: Bearer benchmark-token" \
  -H "Content-Type: application/json" \
  -d '{"kind":"fill","selector":"#volume","text":"90"}'

curl "http://localhost:9867/tabs/TAB_ID/text?mode=raw&format=text" \
  -H "Authorization: Bearer benchmark-token"
```
**Pass if**: Response contains `RANGE_VALUE_90` AND `BUCKET_HIGH`.

### 28.2 Move to LOW bucket
```bash
curl -X POST http://localhost:9867/tabs/TAB_ID/action \
  -H "Authorization: Bearer benchmark-token" \
  -H "Content-Type: application/json" \
  -d '{"kind":"fill","selector":"#volume","text":"10"}'

curl "http://localhost:9867/tabs/TAB_ID/text?mode=raw&format=text" \
  -H "Authorization: Bearer benchmark-token"
```
**Pass if**: Response contains `RANGE_VALUE_10` AND `BUCKET_LOW`.

**Note**: `fill` calls the native value setter, which fires both `input` and `change` events. That's enough to drive a page that listens via `addEventListener('input', ...)`. If the target only listens to pointer drag events, `drag --drag-x N` on the thumb would be needed — but most sites use `input`/`change`.

---

## Group 29: Pagination

### 29.1 Advance to page 2
```bash
curl -X POST http://localhost:9867/navigate \
  -H "Authorization: Bearer benchmark-token" \
  -H "Content-Type: application/json" \
  -d '{"tabId":"TAB_ID","url":"http://fixtures/pagination.html"}'

curl -X POST http://localhost:9867/tabs/TAB_ID/action \
  -H "Authorization: Bearer benchmark-token" \
  -H "Content-Type: application/json" \
  -d '{"kind":"click","selector":"#next-btn"}'

curl "http://localhost:9867/tabs/TAB_ID/text?mode=raw&format=text" \
  -H "Authorization: Bearer benchmark-token"
```
**Pass if**: Response contains `PAGE_2_FIRST_ITEM` AND `PAGE_2_OF_3` (and the page-1 marker is gone).

### 29.2 Advance to last page; Next is disabled
```bash
curl -X POST http://localhost:9867/tabs/TAB_ID/action \
  -H "Authorization: Bearer benchmark-token" \
  -H "Content-Type: application/json" \
  -d '{"kind":"click","selector":"#next-btn"}'

curl "http://localhost:9867/tabs/TAB_ID/text?mode=raw&format=text" \
  -H "Authorization: Bearer benchmark-token"

curl -X POST http://localhost:9867/tabs/TAB_ID/evaluate \
  -H "Authorization: Bearer benchmark-token" \
  -H "Content-Type: application/json" \
  -d '{"expression":"document.getElementById(\"next-btn\").disabled"}'
```
**Pass if**: Text contains `PAGE_3_FIRST_ITEM` AND `PAGE_3_OF_3` AND the evaluate response is `true` (Next button is disabled on the last page).

**Note**: Pagination buttons re-render the list in place. If you re-snap after the click, refs `e0..eN` for the list items will be different than before.

---

## Group 30: Custom dropdown menu

### 30.1 Open the dropdown and pick "Beta"
```bash
curl -X POST http://localhost:9867/navigate \
  -H "Authorization: Bearer benchmark-token" \
  -H "Content-Type: application/json" \
  -d '{"tabId":"TAB_ID","url":"http://fixtures/dropdown.html"}'

# Open the menu
curl -X POST http://localhost:9867/tabs/TAB_ID/action \
  -H "Authorization: Bearer benchmark-token" \
  -H "Content-Type: application/json" \
  -d '{"kind":"click","selector":"#dropdown-toggle"}'

# Pick the "Beta" item by its data-value attribute
curl -X POST http://localhost:9867/tabs/TAB_ID/action \
  -H "Authorization: Bearer benchmark-token" \
  -H "Content-Type: application/json" \
  -d '{"kind":"click","selector":"#dropdown-menu li[data-value=\"beta\"]"}'

curl "http://localhost:9867/tabs/TAB_ID/text?mode=raw&format=text" \
  -H "Authorization: Bearer benchmark-token"
```
**Pass if**: Response contains `DROPDOWN_SELECTED=BETA`.

### 30.2 Reopen and pick "Gamma"
```bash
curl -X POST http://localhost:9867/tabs/TAB_ID/action \
  -H "Authorization: Bearer benchmark-token" \
  -H "Content-Type: application/json" \
  -d '{"kind":"click","selector":"#dropdown-toggle"}'

curl -X POST http://localhost:9867/tabs/TAB_ID/action \
  -H "Authorization: Bearer benchmark-token" \
  -H "Content-Type: application/json" \
  -d '{"kind":"click","selector":"#dropdown-menu li[data-value=\"gamma\"]"}'

curl "http://localhost:9867/tabs/TAB_ID/text?mode=raw&format=text" \
  -H "Authorization: Bearer benchmark-token"
```
**Pass if**: Response contains `DROPDOWN_SELECTED=GAMMA`.

**Note**: This is a non-native dropdown — it's a `<button>` that toggles a `<ul>`. The menu items are not `<option>` elements, so `pinchtab select` won't work. Use two clicks (toggle, then item). Click-outside also dismisses the menu, so don't do anything between the two clicks that would synthesize a body click.

---

## Group 31: Nested iframes (3 levels deep)

### 31.1 Drill into the deepest frame and click its button
```bash
curl -X POST http://localhost:9867/navigate \
  -H "Authorization: Bearer benchmark-token" \
  -H "Content-Type: application/json" \
  -d '{"tabId":"TAB_ID","url":"http://fixtures/iframe-nested.html"}'

# Hop into level 2
curl -X POST http://localhost:9867/frame?tabId=TAB_ID \
  -H "Authorization: Bearer benchmark-token" \
  -H "Content-Type: application/json" \
  -d '{"target":"#level-2"}'

# Hop further into level 3 (selector is resolved relative to current scope)
curl -X POST http://localhost:9867/frame?tabId=TAB_ID \
  -H "Authorization: Bearer benchmark-token" \
  -H "Content-Type: application/json" \
  -d '{"target":"#level-3"}'

# Click the level-3 button
curl -X POST http://localhost:9867/tabs/TAB_ID/action \
  -H "Authorization: Bearer benchmark-token" \
  -H "Content-Type: application/json" \
  -d '{"kind":"click","selector":"#deep-button"}'

curl "http://localhost:9867/tabs/TAB_ID/snapshot?format=compact&maxTokens=500" \
  -H "Authorization: Bearer benchmark-token"

curl -X POST http://localhost:9867/frame?tabId=TAB_ID \
  -H "Authorization: Bearer benchmark-token" \
  -H "Content-Type: application/json" \
  -d '{"target":"main"}'
```
**Pass if**: Scoped snapshot contains `DEEP_CLICKED=YES_LEVEL_3`.

**Note**: Each `/frame` hop is relative to the current scope, so drilling
into a nested frame requires one hop per level. Use `{"target":"main"}`
to unwind to the top document in a single call.

---

## Group 32: Dynamic iframe (inserted after load)

### 32.1 Wait for a late-inserted iframe then interact with it
```bash
curl -X POST http://localhost:9867/navigate \
  -H "Authorization: Bearer benchmark-token" \
  -H "Content-Type: application/json" \
  -d '{"tabId":"TAB_ID","url":"http://fixtures/iframe-dynamic.html"}'

# Wait for the marker that indicates the iframe was attached
curl -X POST http://localhost:9867/tabs/TAB_ID/wait \
  -H "Authorization: Bearer benchmark-token" \
  -H "Content-Type: application/json" \
  -d '{"text":"IFRAME_DYNAMIC_ATTACHED","timeout":5000}'

# Scope into it and interact
curl -X POST http://localhost:9867/frame?tabId=TAB_ID \
  -H "Authorization: Bearer benchmark-token" \
  -H "Content-Type: application/json" \
  -d '{"target":"#late-frame"}'

curl -X POST http://localhost:9867/tabs/TAB_ID/action \
  -H "Authorization: Bearer benchmark-token" \
  -H "Content-Type: application/json" \
  -d '{"kind":"fill","selector":"#iframe-input","text":"Late World"}'

curl -X POST http://localhost:9867/tabs/TAB_ID/action \
  -H "Authorization: Bearer benchmark-token" \
  -H "Content-Type: application/json" \
  -d '{"kind":"click","selector":"#iframe-submit"}'

curl "http://localhost:9867/tabs/TAB_ID/snapshot?format=compact&maxTokens=500" \
  -H "Authorization: Bearer benchmark-token"

curl -X POST http://localhost:9867/frame?tabId=TAB_ID \
  -H "Authorization: Bearer benchmark-token" \
  -H "Content-Type: application/json" \
  -d '{"target":"main"}'
```
**Pass if**: Scoped snapshot contains `IFRAME_INPUT_RECEIVED_LATE_WORLD`.

**Note**: The iframe is added to the DOM ~1.2 s after load. Snapshotting
immediately won't see it; `pinchtab wait --text` is the right primitive.

---

## Group 33: srcdoc iframe (inline HTML, no src URL)

### 33.1 Interact with an inline-content iframe
```bash
curl -X POST http://localhost:9867/navigate \
  -H "Authorization: Bearer benchmark-token" \
  -H "Content-Type: application/json" \
  -d '{"tabId":"TAB_ID","url":"http://fixtures/iframe-srcdoc.html"}'

curl -X POST http://localhost:9867/frame?tabId=TAB_ID \
  -H "Authorization: Bearer benchmark-token" \
  -H "Content-Type: application/json" \
  -d '{"target":"#srcdoc-frame"}'

curl -X POST http://localhost:9867/tabs/TAB_ID/action \
  -H "Authorization: Bearer benchmark-token" \
  -H "Content-Type: application/json" \
  -d '{"kind":"fill","selector":"#inline-input","text":"srcdoc"}'

curl -X POST http://localhost:9867/tabs/TAB_ID/action \
  -H "Authorization: Bearer benchmark-token" \
  -H "Content-Type: application/json" \
  -d '{"kind":"click","selector":"#inline-submit"}'

curl "http://localhost:9867/tabs/TAB_ID/snapshot?format=compact&maxTokens=500" \
  -H "Authorization: Bearer benchmark-token"

curl -X POST http://localhost:9867/frame?tabId=TAB_ID \
  -H "Authorization: Bearer benchmark-token" \
  -H "Content-Type: application/json" \
  -d '{"target":"main"}'
```
**Pass if**: Scoped snapshot contains `INLINE_RECEIVED_SRCDOC`.

**Note**: `srcdoc` iframes inherit the parent's origin (same-origin by
default), so frame scope works the same way as `src`-based iframes. The
returned `frameUrl` is `about:srcdoc`.

---

## Group 34: Sandboxed iframe (allow-scripts allow-same-origin)

### 34.1 Click a button inside a sandbox="allow-scripts allow-same-origin" iframe
```bash
curl -X POST http://localhost:9867/navigate \
  -H "Authorization: Bearer benchmark-token" \
  -H "Content-Type: application/json" \
  -d '{"tabId":"TAB_ID","url":"http://fixtures/iframe-sandbox.html"}'

curl -X POST http://localhost:9867/frame?tabId=TAB_ID \
  -H "Authorization: Bearer benchmark-token" \
  -H "Content-Type: application/json" \
  -d '{"target":"#sandboxed"}'

curl -X POST http://localhost:9867/tabs/TAB_ID/action \
  -H "Authorization: Bearer benchmark-token" \
  -H "Content-Type: application/json" \
  -d '{"kind":"click","selector":"#sandbox-button"}'

curl "http://localhost:9867/tabs/TAB_ID/snapshot?format=compact&maxTokens=500" \
  -H "Authorization: Bearer benchmark-token"

curl -X POST http://localhost:9867/frame?tabId=TAB_ID \
  -H "Authorization: Bearer benchmark-token" \
  -H "Content-Type: application/json" \
  -d '{"target":"main"}'
```
**Pass if**: Scoped snapshot contains `SANDBOX_CLICKED=YES`.

**Note**: `sandbox="allow-scripts allow-same-origin"` permits scripts and
inherits the parent origin, so frame scope works. Sandboxes without
`allow-same-origin` force a unique opaque origin and would behave like
cross-origin — out of reach for the current `/frame` scope API.

---

## Group 35: Long-form article (Medium/Substack style)

### 35.1 Readability keeps the article body
```bash
curl -X POST http://localhost:9867/navigate \
  -H "Authorization: Bearer benchmark-token" \
  -H "Content-Type: application/json" \
  -d '{"tabId":"TAB_ID","url":"http://fixtures/article.html"}'

# Default mode uses Readability; this is its sweet spot.
curl "http://localhost:9867/tabs/TAB_ID/text" \
  -H "Authorization: Bearer benchmark-token"
```
**Pass if**: Response contains `ARTICLE_PUBLISHED_2026_04_15` AND `ARTICLE_WORD_COUNT_MARKER_323` (both inside the main article body — Readability should retain them).

### 35.2 `--full` picks up the surrounding chrome Readability drops
```bash
curl "http://localhost:9867/tabs/TAB_ID/text?mode=raw&format=text" \
  -H "Authorization: Bearer benchmark-token"
```
**Pass if**: Response contains `ARTICLE_PUBLISHED_2026_04_15` AND `FOOTER_COPYRIGHT_MARKER`. Default Readability trims the footer; `--full` keeps it. Proves the two modes behave as documented.

---

## Group 36: Search results page (SERP)

### 36.1 Extract a specific result title by id
```bash
curl -X POST http://localhost:9867/navigate \
  -H "Authorization: Bearer benchmark-token" \
  -H "Content-Type: application/json" \
  -d '{"tabId":"TAB_ID","url":"http://fixtures/serp.html"}'

# Use a scoped snapshot to extract just one card — faster than full text.
curl "http://localhost:9867/tabs/TAB_ID/snapshot?selector=%23r-3&format=compact&maxTokens=500" \
  -H "Authorization: Bearer benchmark-token"
```
**Pass if**: Snapshot contains `RESULT_3_TITLE` AND `RESULT_3_SNIPPET_MARKER`.

### 36.2 Count all six result cards
```bash
curl "http://localhost:9867/tabs/TAB_ID/text?mode=raw&format=text" \
  -H "Authorization: Bearer benchmark-token"
```
**Pass if**: Response contains `RESULT_1_TITLE`, `RESULT_2_TITLE`, `RESULT_3_TITLE`, `RESULT_4_TITLE`, `RESULT_5_TITLE`, `RESULT_6_TITLE` (all 6), AND `SERP_RESULT_COUNT_6`. A SERP is the canonical case where Readability's single-article assumption fails — `--full` is required to see the list.

---

## Group 37: Q&A thread (Stack-Overflow style)

### 37.1 Find the accepted answer
```bash
curl -X POST http://localhost:9867/navigate \
  -H "Authorization: Bearer benchmark-token" \
  -H "Content-Type: application/json" \
  -d '{"tabId":"TAB_ID","url":"http://fixtures/qa.html"}'

# Use eval to find the accepted answer id — structured page data beats
# text-parsing when the marker lives in an attribute.
curl -X POST http://localhost:9867/tabs/TAB_ID/evaluate \
  -H "Authorization: Bearer benchmark-token" \
  -H "Content-Type: application/json" \
  -d '{"expression":"document.querySelector(\"[data-accepted=\\\"true\\\"]\").id"}'
```
**Pass if**: Response `.result` equals `"a-2"`. The accepted-answer marker is in a data attribute; this exercises `eval` as a structured alternative to text scraping.

### 37.2 Extract the accepted answer body text
```bash
# Snapshot scoped to the accepted answer, with max-token budget
curl "http://localhost:9867/tabs/TAB_ID/snapshot?selector=%23a-2&format=compact&maxTokens=1500" \
  -H "Authorization: Bearer benchmark-token"
```
**Pass if**: Scoped snapshot contains `ANSWER_2_BODY_MARKER` AND `ACCEPTED_ANSWER_ID_A2`. Tests `snapshot?selector=` for scoped reads — a common pattern when a page has many independent sections.

---

## Group 38: Pricing table

### 38.1 Extract the Pro plan price
```bash
curl -X POST http://localhost:9867/navigate \
  -H "Authorization: Bearer benchmark-token" \
  -H "Content-Type: application/json" \
  -d '{"tabId":"TAB_ID","url":"http://fixtures/pricing.html"}'

curl "http://localhost:9867/tabs/TAB_ID/snapshot?selector=%23plan-pro&format=compact&maxTokens=500" \
  -H "Authorization: Bearer benchmark-token"
```
**Pass if**: Snapshot contains `PLAN_PRO_PRICE_29` AND `PLAN_PRO_LIMIT_5000_requests per day`.

### 38.2 Extract all three plan prices in one pass
```bash
curl "http://localhost:9867/tabs/TAB_ID/text?mode=raw&format=text" \
  -H "Authorization: Bearer benchmark-token"
```
**Pass if**: Response contains `PLAN_FREE_PRICE_0`, `PLAN_PRO_PRICE_29`, AND `PLAN_ENTERPRISE_PRICE_CUSTOM`. Pricing tables are grid-heavy; Readability would pick one column. `--full` returns the whole surface.

---

## Summary

| Group | Tasks | Description |
|-------|-------|-------------|
| 0 | 8 | Setup & Diagnosis |
| 1 | 6 | Reading & Extracting |
| 2 | 3 | Search & Dynamic |
| 3 | 2 | Form |
| 4 | 3 | SPA |
| 5 | 2 | Login |
| 6 | 3 | E-commerce |
| 7 | 2 | Content + Interaction |
| 8 | 2 | Error Handling |
| 9 | 2 | Export |
| 10 | 2 | Modals |
| 11 | 2 | Persistence |
| 12 | 2 | Multi-page Nav |
| 13 | 2 | Form Validation |
| 14 | 2 | Dynamic Content |
| 15 | 2 | Data Aggregation |
| 16 | 2 | Hover & Tooltips |
| 17 | 2 | Scrolling |
| 18 | 1 | File Download |
| 19 | 2 | iFrame |
| 20 | 2 | Dialogs |
| 21 | 2 | Async / awaitPromise |
| 22 | 2 | Mouse Drag & Drop |
| 23 | 1 | Async / Loading state |
| 24 | 2 | Keyboard events |
| 25 | 2 | Tab panels |
| 26 | 2 | Accordion |
| 27 | 2 | Contenteditable editor |
| 28 | 2 | Range slider |
| 29 | 2 | Pagination |
| 30 | 2 | Custom dropdown menu |
| 31 | 1 | Nested iframes (3 levels) |
| 32 | 1 | Dynamic iframe |
| 33 | 1 | srcdoc iframe |
| 34 | 1 | Sandboxed iframe |
| 35 | 2 | Long-form article (text extraction) |
| 36 | 2 | Search results page (SERP) |
| 37 | 2 | Q&A thread (Stack-Overflow style) |
| 38 | 2 | Pricing table |

**Total: 85 tasks**

## Verification Strings

| Page | String |
|------|--------|
| Home | `VERIFY_HOME_LOADED_12345` |
| Articles | `VERIFY_ARTICLES_PAGE_67890` |
| Search | `VERIFY_SEARCH_PAGE_11111` |
| Form | `VERIFY_FORM_PAGE_22222` |
| Dashboard | `VERIFY_DASHBOARD_PAGE_33333` |
| Shop | `VERIFY_SHOP_PAGE_44444` |
| Wiki Index | `VERIFY_WIKI_INDEX_55555` |
| Login | `VERIFY_LOGIN_PAGE_77777` |
| Go Article | `VERIFY_WIKI_GO_LANG_88888` |
| SPA | `VERIFY_SPA_PAGE_99999` |
| Python Article | `VERIFY_WIKI_PYTHON_LANG` |
| Rust Article | `VERIFY_WIKI_RUST_LANG` |
| Async | `VERIFY_ASYNC_PAGE_77777` |
| Drag | `VERIFY_DRAG_PAGE_33333` |
| Loading | `VERIFY_LOADING_PAGE_11111` |
| Keyboard | `VERIFY_KEYBOARD_PAGE_22222` |
| Tabs | `VERIFY_TABS_PAGE_55555` |
| Accordion | `VERIFY_ACCORDION_PAGE_66666` |
| Editor | `VERIFY_EDITOR_PAGE_44444` |
| Range | `VERIFY_RANGE_PAGE_99999` |
| Pagination | `VERIFY_PAGINATION_PAGE_88888` |
| Dropdown | `VERIFY_DROPDOWN_PAGE_77777` |
| Iframe Nested (outer) | `VERIFY_IFRAME_NESTED_OUTER` |
| Iframe Nested (L2) | `VERIFY_IFRAME_NESTED_L2` |
| Iframe Nested (L3) | `VERIFY_IFRAME_NESTED_L3` |
| Iframe Dynamic | `VERIFY_IFRAME_DYNAMIC_OUTER` |
| Iframe Srcdoc | `VERIFY_IFRAME_SRCDOC_OUTER` |
| Iframe Sandbox | `VERIFY_IFRAME_SANDBOX_OUTER` |
| Article | `VERIFY_ARTICLE_PAGE_41414` |
| SERP | `SERP_PAGE_MARKER_88044` |
| Q&A | `VERIFY_QA_PAGE_72000` |
| Pricing | `VERIFY_PRICING_PAGE_30303` |
