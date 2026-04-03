# PinchTab Benchmark Tasks

Reproducible benchmark tasks using controlled fixture pages with verifiable content.

## Environment

- **PinchTab Server**: `http://localhost:9867`
- **Fixtures Server**: `http://fixtures/`
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
./record-step.sh <group> <step> <pass|fail> <in_tokens> <out_tokens> "notes"
```

**On failure, include in notes:**
- What was expected
- What was actually returned
- HTTP status code / error message

---

## Group 0: Setup & Configuration Verification

Verify the environment is correctly configured before running tests.

### 0.1 Load PinchTab skill
Read `../../skills/pinchtab/SKILL.md`.
```bash
./record-step.sh 0 1 pass <in> <out> "Skill loaded"
```
**Pass if**: Skill loaded successfully.

### 0.2 Verify PinchTab health
```bash
curl -sf http://localhost:9867/health \
  -H "Authorization: Bearer benchmark-token" | jq .
```
**Pass if**: `status == "ok"`, `authRequired == true`, `instances >= 1`.

### 0.3 Verify fixtures server
```bash
curl -sf http://localhost:9867/navigate \
  -X POST -H "Authorization: Bearer benchmark-token" \
  -H "Content-Type: application/json" \
  -d '{"url":"http://fixtures/"}' | jq .
```
**Pass if**: Response contains `"url": "http://fixtures/"`.

### 0.4 Verify Chrome instance running
```bash
curl -sf http://localhost:9867/health \
  -H "Authorization: Bearer benchmark-token" | jq '.defaultInstance.status'
```
**Pass if**: Value is `"running"`.

---

## Group 1: Navigation & Content Extraction

### 1.1 Navigate to fixtures home
```bash
curl -X POST http://localhost:9867/navigate \
  -H "Authorization: Bearer benchmark-token" \
  -H "Content-Type: application/json" \
  -d '{"url":"http://fixtures/"}'
```
**Pass if**: HTTP 200.

### 1.2 Verify home content via snapshot
```bash
curl "http://localhost:9867/snapshot?format=compact&maxTokens=1000" \
  -H "Authorization: Bearer benchmark-token"
```
**Pass if**: Contains `VERIFY_HOME_LOADED_12345`.

### 1.3 Navigate to wiki index
```bash
curl -X POST http://localhost:9867/navigate \
  -H "Authorization: Bearer benchmark-token" \
  -H "Content-Type: application/json" \
  -d '{"url":"http://fixtures/wiki.html"}'
```
**Pass if**: HTTP 200.

### 1.4 Verify wiki content and count categories
```bash
curl "http://localhost:9867/snapshot?format=compact&maxTokens=1500" \
  -H "Authorization: Bearer benchmark-token"
```
**Pass if**: Contains `VERIFY_WIKI_INDEX_55555` AND `COUNT_LANGUAGES_12` AND `COUNT_TOOLS_15`.

### 1.5 Click through to Go article
```bash
curl -X POST http://localhost:9867/action \
  -H "Authorization: Bearer benchmark-token" \
  -H "Content-Type: application/json" \
  -d '{"kind":"click","selector":"#link-go","waitNav":true}'
```
**Pass if**: HTTP 200 with `{"success":true}`.

**Note**: Use `waitNav: true` when clicking a link that causes page navigation. Without it, PinchTab returns a 409 "navigation_changed" error to protect against unexpected navigation during form interactions.

### 1.6 Verify Go article loaded and extract key facts
```bash
curl "http://localhost:9867/snapshot?format=compact&maxTokens=2000" \
  -H "Authorization: Bearer benchmark-token"
```
**Pass if**: Contains `VERIFY_WIKI_GO_LANG_88888` AND `Robert Griesemer` AND `2009` AND `FEATURE_COUNT_6`.

### 1.7 Extract specific table data (designer)
```bash
curl http://localhost:9867/text \
  -H "Authorization: Bearer benchmark-token"
```
**Pass if**: Text contains `Google LLC` AND `Ken Thompson`.

### 1.8 Navigate to articles and extract all headlines
```bash
curl -X POST http://localhost:9867/navigate \
  -H "Authorization: Bearer benchmark-token" \
  -H "Content-Type: application/json" \
  -d '{"url":"http://fixtures/articles.html"}'

curl "http://localhost:9867/snapshot?format=compact&maxTokens=2000" \
  -H "Authorization: Bearer benchmark-token"
```
**Pass if**: Snapshot contains all 3 article titles: `The Future of Artificial Intelligence`, `Climate Action in 2026`, `Mars Colony`.

---

## Group 2: Search & Dynamic Content

### 2.1 Navigate to wiki search
```bash
curl -X POST http://localhost:9867/navigate \
  -H "Authorization: Bearer benchmark-token" \
  -H "Content-Type: application/json" \
  -d '{"url":"http://fixtures/wiki.html"}'
```
**Pass if**: HTTP 200.

### 2.2 Get snapshot to find search input ref
```bash
curl "http://localhost:9867/snapshot?filter=interactive&format=compact" \
  -H "Authorization: Bearer benchmark-token"
```
**Pass if**: Contains search input element. Note the ref.

### 2.3 Fill search query
```bash
curl -X POST http://localhost:9867/action \
  -H "Authorization: Bearer benchmark-token" \
  -H "Content-Type: application/json" \
  -d '{"kind":"fill","selector":"#wiki-search-input","text":"golang"}'
```
**Pass if**: HTTP 200.

### 2.4 Submit search
```bash
curl -X POST http://localhost:9867/action \
  -H "Authorization: Bearer benchmark-token" \
  -H "Content-Type: application/json" \
  -d '{"kind":"click","selector":"#wiki-search-btn"}'
```
**Pass if**: HTTP 200.

### 2.5 Verify navigation to Go article
```bash
curl "http://localhost:9867/snapshot?format=compact&maxTokens=2000" \
  -H "Authorization: Bearer benchmark-token"
```
**Pass if**: Contains `VERIFY_WIKI_GO_LANG_88888` (search redirected to Go page).

### 2.6 Search for something with no results
```bash
curl -X POST http://localhost:9867/navigate \
  -H "Authorization: Bearer benchmark-token" \
  -H "Content-Type: application/json" \
  -d '{"url":"http://fixtures/search.html"}'

curl -X POST http://localhost:9867/action \
  -H "Authorization: Bearer benchmark-token" \
  -H "Content-Type: application/json" \
  -d '{"kind":"fill","selector":"#search-input","text":"xyznonexistent"}'

curl -X POST http://localhost:9867/action \
  -H "Authorization: Bearer benchmark-token" \
  -H "Content-Type: application/json" \
  -d '{"kind":"click","selector":"#search-btn"}'
```
**Pass if**: HTTP 200 (graceful no-results state).

---

## Group 3: Complex Form Interaction

### 3.1 Navigate to contact form
```bash
curl -X POST http://localhost:9867/navigate \
  -H "Authorization: Bearer benchmark-token" \
  -H "Content-Type: application/json" \
  -d '{"url":"http://fixtures/form.html"}'
```
**Pass if**: HTTP 200.

### 3.2 Get snapshot of interactive elements
```bash
curl "http://localhost:9867/snapshot?filter=interactive&format=compact" \
  -H "Authorization: Bearer benchmark-token"
```
**Pass if**: Contains all expected fields: fullname, email, country, subject, message.

### 3.3 Fill full name
```bash
curl -X POST http://localhost:9867/action \
  -H "Authorization: Bearer benchmark-token" \
  -H "Content-Type: application/json" \
  -d '{"kind":"fill","selector":"#fullname","text":"John Benchmark"}'
```
**Pass if**: HTTP 200.

### 3.4 Fill email
```bash
curl -X POST http://localhost:9867/action \
  -H "Authorization: Bearer benchmark-token" \
  -H "Content-Type: application/json" \
  -d '{"kind":"fill","selector":"#email","text":"john@benchmark.test"}'
```
**Pass if**: HTTP 200.

### 3.5 Fill phone
```bash
curl -X POST http://localhost:9867/action \
  -H "Authorization: Bearer benchmark-token" \
  -H "Content-Type: application/json" \
  -d '{"kind":"fill","selector":"#phone","text":"+44 20 1234 5678"}'
```
**Pass if**: HTTP 200.

### 3.6 Select country
```bash
curl -X POST http://localhost:9867/action \
  -H "Authorization: Bearer benchmark-token" \
  -H "Content-Type: application/json" \
  -d '{"kind":"select","selector":"#country","value":"uk"}'
```
**Pass if**: HTTP 200.

### 3.7 Select subject
```bash
curl -X POST http://localhost:9867/action \
  -H "Authorization: Bearer benchmark-token" \
  -H "Content-Type: application/json" \
  -d '{"kind":"select","selector":"#subject","value":"support"}'
```
**Pass if**: HTTP 200.

### 3.8 Fill message textarea
```bash
curl -X POST http://localhost:9867/action \
  -H "Authorization: Bearer benchmark-token" \
  -H "Content-Type: application/json" \
  -d '{"kind":"fill","selector":"#message","text":"This is a benchmark test message for PinchTab automation."}'
```
**Pass if**: HTTP 200.

### 3.9 Check newsletter checkbox
```bash
curl -X POST http://localhost:9867/action \
  -H "Authorization: Bearer benchmark-token" \
  -H "Content-Type: application/json" \
  -d '{"kind":"click","selector":"#newsletter"}'
```
**Pass if**: HTTP 200.

### 3.10 Select high priority radio
```bash
curl -X POST http://localhost:9867/action \
  -H "Authorization: Bearer benchmark-token" \
  -H "Content-Type: application/json" \
  -d '{"kind":"click","selector":"input[name=priority][value=high]"}'
```
**Pass if**: HTTP 200.

### 3.11 Submit form
```bash
curl -X POST http://localhost:9867/action \
  -H "Authorization: Bearer benchmark-token" \
  -H "Content-Type: application/json" \
  -d '{"kind":"click","selector":"#submit-btn"}'
```
**Pass if**: HTTP 200.

### 3.12 Verify submission with data check
```bash
curl "http://localhost:9867/snapshot?format=compact&maxTokens=1000" \
  -H "Authorization: Bearer benchmark-token"
```
**Pass if**: Contains `VERIFY_FORM_SUBMITTED_SUCCESS` AND `SUBMISSION_DATA_NAME_JOHN_BENCHMARK`.

---

## Group 4: SPA & Dynamic State

### 4.1 Navigate to task manager SPA
```bash
curl -X POST http://localhost:9867/navigate \
  -H "Authorization: Bearer benchmark-token" \
  -H "Content-Type: application/json" \
  -d '{"url":"http://fixtures/spa.html"}'
```
**Pass if**: HTTP 200.

### 4.2 Verify initial state
```bash
curl "http://localhost:9867/snapshot?format=compact&maxTokens=1500" \
  -H "Authorization: Bearer benchmark-token"
```
**Pass if**: Contains `VERIFY_SPA_PAGE_99999` AND `TASK_STATS_TOTAL_3_ACTIVE_2_DONE_1`.

### 4.3 Add a new task
```bash
curl -X POST http://localhost:9867/action \
  -H "Authorization: Bearer benchmark-token" \
  -H "Content-Type: application/json" \
  -d '{"kind":"fill","selector":"#new-task-input","text":"Deploy to production"}'

curl -X POST http://localhost:9867/action \
  -H "Authorization: Bearer benchmark-token" \
  -H "Content-Type: application/json" \
  -d '{"kind":"select","selector":"#priority-select","value":"high"}'

curl -X POST http://localhost:9867/action \
  -H "Authorization: Bearer benchmark-token" \
  -H "Content-Type: application/json" \
  -d '{"kind":"click","selector":"#add-task-btn"}'
```
**Pass if**: All HTTP 200.

### 4.4 Verify task was added
```bash
curl "http://localhost:9867/snapshot?format=compact&maxTokens=1500" \
  -H "Authorization: Bearer benchmark-token"
```
**Pass if**: Contains `TASK_ADDED_DEPLOY_TO_PRODUCTION_PRIORITY_HIGH`.

### 4.5 Delete existing task
```bash
curl -X POST http://localhost:9867/action \
  -H "Authorization: Bearer benchmark-token" \
  -H "Content-Type: application/json" \
  -d '{"kind":"click","selector":".delete-task[data-id=\"1\"]"}'
```
**Pass if**: HTTP 200.

### 4.6 Verify task count updated
```bash
curl "http://localhost:9867/snapshot?format=compact&maxTokens=500" \
  -H "Authorization: Bearer benchmark-token"
```
**Pass if**: Contains `3` in total count (started with 3, deleted 1, added 1 = 3).

---

## Group 5: Login & Auth Flow

### 5.1 Navigate to login page
```bash
curl -X POST http://localhost:9867/navigate \
  -H "Authorization: Bearer benchmark-token" \
  -H "Content-Type: application/json" \
  -d '{"url":"http://fixtures/login.html"}'
```
**Pass if**: HTTP 200.

### 5.2 Verify login page
```bash
curl "http://localhost:9867/snapshot?format=compact&maxTokens=500" \
  -H "Authorization: Bearer benchmark-token"
```
**Pass if**: Contains `VERIFY_LOGIN_PAGE_77777`.

### 5.3 Try invalid credentials
```bash
curl -X POST http://localhost:9867/action \
  -H "Authorization: Bearer benchmark-token" \
  -H "Content-Type: application/json" \
  -d '{"kind":"fill","selector":"#username","text":"baduser"}'

curl -X POST http://localhost:9867/action \
  -H "Authorization: Bearer benchmark-token" \
  -H "Content-Type: application/json" \
  -d '{"kind":"fill","selector":"#password","text":"wrongpassword"}'

curl -X POST http://localhost:9867/action \
  -H "Authorization: Bearer benchmark-token" \
  -H "Content-Type: application/json" \
  -d '{"kind":"click","selector":"#login-btn"}'
```
**Pass if**: All HTTP 200.

### 5.4 Verify error shown
```bash
curl "http://localhost:9867/snapshot?format=compact&maxTokens=500" \
  -H "Authorization: Bearer benchmark-token"
```
**Pass if**: Contains `INVALID_CREDENTIALS_ERROR`.

### 5.5 Login with valid credentials
```bash
curl -X POST http://localhost:9867/action \
  -H "Authorization: Bearer benchmark-token" \
  -H "Content-Type: application/json" \
  -d '{"kind":"fill","selector":"#username","text":"benchmark"}'

curl -X POST http://localhost:9867/action \
  -H "Authorization: Bearer benchmark-token" \
  -H "Content-Type: application/json" \
  -d '{"kind":"fill","selector":"#password","text":"test456"}'

curl -X POST http://localhost:9867/action \
  -H "Authorization: Bearer benchmark-token" \
  -H "Content-Type: application/json" \
  -d '{"kind":"click","selector":"#login-btn"}'
```
**Pass if**: All HTTP 200.

### 5.6 Verify logged in
```bash
curl "http://localhost:9867/snapshot?format=compact&maxTokens=500" \
  -H "Authorization: Bearer benchmark-token"
```
**Pass if**: Contains `VERIFY_LOGIN_SUCCESS_DASHBOARD` AND `SESSION_TOKEN_ACTIVE_TRUE`.

---

## Group 6: E-commerce Flow

### 6.1 Navigate to shop
```bash
curl -X POST http://localhost:9867/navigate \
  -H "Authorization: Bearer benchmark-token" \
  -H "Content-Type: application/json" \
  -d '{"url":"http://fixtures/ecommerce.html"}'
```
**Pass if**: HTTP 200.

### 6.2 Verify shop page and read prices
```bash
curl "http://localhost:9867/snapshot?format=compact&maxTokens=2000" \
  -H "Authorization: Bearer benchmark-token"
```
**Pass if**: Contains `VERIFY_SHOP_PAGE_44444` AND `$149.99` AND `$299.99` AND `Out of Stock`.

### 6.3 Verify out-of-stock button is disabled
```bash
curl "http://localhost:9867/snapshot?filter=interactive&format=compact" \
  -H "Authorization: Bearer benchmark-token"
```
**Pass if**: Mechanical Keyboard add-to-cart button is shown as disabled.

### 6.4 Add Wireless Headphones
```bash
curl -X POST http://localhost:9867/action \
  -H "Authorization: Bearer benchmark-token" \
  -H "Content-Type: application/json" \
  -d '{"kind":"click","selector":"#product-1 .add-to-cart"}'
```
**Pass if**: HTTP 200.

### 6.5 Add Smart Watch
```bash
curl -X POST http://localhost:9867/action \
  -H "Authorization: Bearer benchmark-token" \
  -H "Content-Type: application/json" \
  -d '{"kind":"click","selector":"#product-2 .add-to-cart"}'
```
**Pass if**: HTTP 200.

### 6.6 Verify cart total
```bash
curl "http://localhost:9867/snapshot?format=compact&maxTokens=1000" \
  -H "Authorization: Bearer benchmark-token"
```
**Pass if**: Contains `CART_ITEM_WIRELESS_HEADPHONES` AND `CART_ITEM_SMART_WATCH_PRO` AND `449.98` (total).

### 6.7 Checkout
```bash
curl -X POST http://localhost:9867/action \
  -H "Authorization: Bearer benchmark-token" \
  -H "Content-Type: application/json" \
  -d '{"kind":"click","selector":"#checkout-btn"}'
```
**Pass if**: HTTP 200.

### 6.8 Verify checkout complete
```bash
curl "http://localhost:9867/snapshot?format=compact&maxTokens=500" \
  -H "Authorization: Bearer benchmark-token"
```
**Pass if**: Contains `VERIFY_CHECKOUT_SUCCESS_ORDER` AND `ORDER_TOTAL_449_98`.

---

## Group 7: Wiki Article + Comment Flow

### 7.1 Navigate to Go article
```bash
curl -X POST http://localhost:9867/navigate \
  -H "Authorization: Bearer benchmark-token" \
  -H "Content-Type: application/json" \
  -d '{"url":"http://fixtures/wiki-go.html"}'
```
**Pass if**: HTTP 200.

### 7.2 Read and verify article facts
```bash
curl "http://localhost:9867/snapshot?format=compact&maxTokens=2000" \
  -H "Authorization: Bearer benchmark-token"
```
**Pass if**: Contains `VERIFY_WIKI_GO_LANG_88888` AND `BSD-style` AND `go.dev`.

### 7.3 Fill comment
```bash
curl -X POST http://localhost:9867/action \
  -H "Authorization: Bearer benchmark-token" \
  -H "Content-Type: application/json" \
  -d '{"kind":"fill","selector":"#comment-text","text":"Great article on Go! Very comprehensive."}'
```
**Pass if**: HTTP 200.

### 7.4 Rate the article
```bash
curl -X POST http://localhost:9867/action \
  -H "Authorization: Bearer benchmark-token" \
  -H "Content-Type: application/json" \
  -d '{"kind":"select","selector":"#comment-rating","value":"5"}'
```
**Pass if**: HTTP 200.

### 7.5 Submit comment
```bash
curl -X POST http://localhost:9867/action \
  -H "Authorization: Bearer benchmark-token" \
  -H "Content-Type: application/json" \
  -d '{"kind":"click","selector":"#submit-comment"}'
```
**Pass if**: HTTP 200.

### 7.6 Verify comment posted
```bash
curl "http://localhost:9867/snapshot?format=compact&maxTokens=500" \
  -H "Authorization: Bearer benchmark-token"
```
**Pass if**: Contains `COMMENT_POSTED_RATING_5_TEXT_RECEIVED`.

---

## Group 8: Error Handling

### 8.1 Navigate to non-existent page
```bash
curl -X POST http://localhost:9867/navigate \
  -H "Authorization: Bearer benchmark-token" \
  -H "Content-Type: application/json" \
  -d '{"url":"http://fixtures/nonexistent-page-xyz.html"}'
```
**Pass if**: Returns response without crash (404 or error).

### 8.2 Click non-existent element
```bash
curl -X POST http://localhost:9867/action \
  -H "Authorization: Bearer benchmark-token" \
  -H "Content-Type: application/json" \
  -d '{"kind":"click","selector":"#element-that-does-not-exist"}'
```
**Pass if**: Error response with message (not crash).

### 8.3 Fill with invalid selector
```bash
curl -X POST http://localhost:9867/action \
  -H "Authorization: Bearer benchmark-token" \
  -H "Content-Type: application/json" \
  -d '{"kind":"fill","selector":"#nonexistent-input","text":"test"}'
```
**Pass if**: Error response with message.

### 8.4 Invalid action kind
```bash
curl -X POST http://localhost:9867/action \
  -H "Authorization: Bearer benchmark-token" \
  -H "Content-Type: application/json" \
  -d '{"kind":"teleport","selector":"#btn"}'
```
**Pass if**: Returns 400/422 error with clear message.

---

## Group 9: Screenshot & Export

### 9.1 Navigate to dashboard
```bash
curl -X POST http://localhost:9867/navigate \
  -H "Authorization: Bearer benchmark-token" \
  -H "Content-Type: application/json" \
  -d '{"url":"http://fixtures/dashboard.html"}'
```
**Pass if**: HTTP 200.

### 9.2 Take screenshot
```bash
curl http://localhost:9867/screenshot \
  -H "Authorization: Bearer benchmark-token" \
  --output /tmp/benchmark-screenshot.png
```
**Pass if**: File created, size > 10KB.

### 9.3 Verify screenshot has content
```bash
ls -la /tmp/benchmark-screenshot.png
```
**Pass if**: File exists and size > 10240 bytes.

### 9.4 Export PDF
```bash
curl -X POST http://localhost:9867/pdf \
  -H "Authorization: Bearer benchmark-token" \
  --output /tmp/benchmark-dashboard.pdf
```
**Pass if**: File created, size > 10KB.

---

## Group 10: Agent Identity

### 10.1 Navigate as agent alpha
```bash
curl -X POST http://localhost:9867/navigate \
  -H "Authorization: Bearer benchmark-token" \
  -H "X-Agent-Id: bench-alpha" \
  -H "Content-Type: application/json" \
  -d '{"url":"http://fixtures/wiki.html"}'
```
**Pass if**: HTTP 200.

### 10.2 Navigate as agent beta
```bash
curl -X POST http://localhost:9867/navigate \
  -H "Authorization: Bearer benchmark-token" \
  -H "X-Agent-Id: bench-beta" \
  -H "Content-Type: application/json" \
  -d '{"url":"http://fixtures/dashboard.html"}'
```
**Pass if**: HTTP 200.

### 10.3 Check alpha activity
```bash
curl "http://localhost:9867/api/activity?agentId=bench-alpha" \
  -H "Authorization: Bearer benchmark-token"
```
**Pass if**: Response contains activity for `bench-alpha`.

### 10.4 Check beta activity
```bash
curl "http://localhost:9867/api/activity?agentId=bench-beta" \
  -H "Authorization: Bearer benchmark-token"
```
**Pass if**: Response contains activity for `bench-beta`.

---

## Summary

| Group | Steps | Description |
|-------|-------|-------------|
| 0 | 4 | Setup & Configuration |
| 1 | 8 | Navigation & Content Extraction |
| 2 | 6 | Search & Dynamic Content |
| 3 | 12 | Complex Form Interaction |
| 4 | 6 | SPA & Dynamic State |
| 5 | 6 | Login & Auth Flow |
| 6 | 8 | E-commerce Flow |
| 7 | 6 | Wiki Article + Comment |
| 8 | 4 | Error Handling |
| 9 | 4 | Screenshot & Export |
| 10 | 4 | Agent Identity |

**Total: 68 steps**

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
