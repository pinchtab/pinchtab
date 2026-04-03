# PinchTab Benchmark Tasks

Reproducible benchmark tasks using controlled fixture pages with verifiable content.

## MANDATORY: Docker Environment

This benchmark MUST run against Docker with the fixtures server.

```bash
cd ~/dev/pinchtab/tests/benchmark
docker compose up -d --build
# Wait for both pinchtab and fixtures to be healthy
```

## Environment

- **PinchTab Server**: `http://localhost:9867`
- **Fixtures Server**: `http://fixtures` (from inside Docker) or `http://localhost:8080` (from host)
- **Token**: `benchmark-token`
- **Auth Header**: `Authorization: Bearer benchmark-token`

## Recording Results

After each step:
```bash
./record-step.sh <group> <step> <pass|fail> <input_tokens> <output_tokens> "notes"
```

**For FAILED steps, include detailed failure info in notes:**
- What was expected (e.g., "Expected VERIFY_HOME_LOADED_12345")
- What was actually received (e.g., "Got 404 page not found")
- HTTP status code if relevant
- Any error messages from the API

Example:
```bash
./record-step.sh 1 2 fail 100 50 "Expected VERIFY_HOME_LOADED_12345, got: 'Page not found'. HTTP 404."
```

Failed steps are logged to `results/errors.log` for debugging.

## Verification Pattern

Each fixture page contains verification strings like `VERIFY_HOME_LOADED_12345`.
**Pass criteria**: Response text MUST contain the exact verification string.

---

## Group 0: Skill Loading Cost

### 0.1 Record skill loading overhead
Read `../../skills/pinchtab/SKILL.md`.
Record the tokens used to load and understand the skill.
```bash
./record-step.sh 0 1 pass <input_tokens> <output_tokens> "Skill loaded"
```

---

## Group 1: Navigation Basics

### 1.1 Navigate to fixtures home
```bash
curl -X POST http://localhost:9867/navigate \
  -H "Authorization: Bearer benchmark-token" \
  -H "Content-Type: application/json" \
  -d '{"url":"http://fixtures/"}'
```
**Pass if**: HTTP 200 response.

### 1.2 Verify home page loaded
```bash
curl "http://localhost:9867/snapshot?format=compact&maxTokens=1000" \
  -H "Authorization: Bearer benchmark-token"
```
**Pass if**: Response contains `VERIFY_HOME_LOADED_12345`.

### 1.3 Navigate to articles page
```bash
curl -X POST http://localhost:9867/navigate \
  -H "Authorization: Bearer benchmark-token" \
  -H "Content-Type: application/json" \
  -d '{"url":"http://fixtures/articles.html"}'
```
**Pass if**: HTTP 200.

### 1.4 Verify articles page content
```bash
curl "http://localhost:9867/snapshot?format=compact&maxTokens=1000" \
  -H "Authorization: Bearer benchmark-token"
```
**Pass if**: Response contains `VERIFY_ARTICLES_PAGE_67890` AND `The Future of Artificial Intelligence`.

### 1.5 Navigate to dashboard
```bash
curl -X POST http://localhost:9867/navigate \
  -H "Authorization: Bearer benchmark-token" \
  -H "Content-Type: application/json" \
  -d '{"url":"http://fixtures/dashboard.html"}'
```
**Pass if**: HTTP 200.

### 1.6 Verify dashboard metrics
```bash
curl "http://localhost:9867/snapshot?format=compact&maxTokens=1500" \
  -H "Authorization: Bearer benchmark-token"
```
**Pass if**: Response contains `VERIFY_DASHBOARD_PAGE_33333` AND `24,582` (Total Users) AND `$1,284,930` (Revenue).

---

## Group 2: Element Interaction - Search Flow

### 2.1 Navigate to search page
```bash
curl -X POST http://localhost:9867/navigate \
  -H "Authorization: Bearer benchmark-token" \
  -H "Content-Type: application/json" \
  -d '{"url":"http://fixtures/search.html"}'
```
**Pass if**: HTTP 200.

### 2.2 Verify search page loaded
```bash
curl http://localhost:9867/text \
  -H "Authorization: Bearer benchmark-token"
```
**Pass if**: Response contains `VERIFY_SEARCH_PAGE_11111`.

### 2.3 Get search input ref
```bash
curl "http://localhost:9867/snapshot?filter=interactive&format=compact" \
  -H "Authorization: Bearer benchmark-token"
```
**Pass if**: Response contains a ref for the search input (id="search-input"). Note the ref.

### 2.4 Fill search query
Use the ref from 2.3:
```bash
curl -X POST http://localhost:9867/action \
  -H "Authorization: Bearer benchmark-token" \
  -H "Content-Type: application/json" \
  -d '{"kind":"fill","selector":"#search-input","text":"artificial intelligence"}'
```
**Pass if**: HTTP 200, success response.

### 2.5 Click search button
```bash
curl -X POST http://localhost:9867/action \
  -H "Authorization: Bearer benchmark-token" \
  -H "Content-Type: application/json" \
  -d '{"kind":"click","selector":"#search-btn"}'
```
**Pass if**: HTTP 200.

### 2.6 Verify search results
```bash
curl http://localhost:9867/text \
  -H "Authorization: Bearer benchmark-token"
```
**Pass if**: Response contains `RESULT_FOUND_THE_FUTURE_OF_ARTIFICIAL_INTELLIGENCE`.

---

## Group 3: Form Interaction

### 3.1 Navigate to form page
```bash
curl -X POST http://localhost:9867/navigate \
  -H "Authorization: Bearer benchmark-token" \
  -H "Content-Type: application/json" \
  -d '{"url":"http://fixtures/form.html"}'
```
**Pass if**: HTTP 200.

### 3.2 Verify form page loaded
```bash
curl http://localhost:9867/text \
  -H "Authorization: Bearer benchmark-token"
```
**Pass if**: Response contains `VERIFY_FORM_PAGE_22222`.

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

### 3.5 Select country dropdown
```bash
curl -X POST http://localhost:9867/action \
  -H "Authorization: Bearer benchmark-token" \
  -H "Content-Type: application/json" \
  -d '{"kind":"select","selector":"#country","value":"uk"}'
```
**Pass if**: HTTP 200.

### 3.6 Select subject dropdown
```bash
curl -X POST http://localhost:9867/action \
  -H "Authorization: Bearer benchmark-token" \
  -H "Content-Type: application/json" \
  -d '{"kind":"select","selector":"#subject","value":"support"}'
```
**Pass if**: HTTP 200.

### 3.7 Check newsletter checkbox
```bash
curl -X POST http://localhost:9867/action \
  -H "Authorization: Bearer benchmark-token" \
  -H "Content-Type: application/json" \
  -d '{"kind":"click","selector":"#newsletter"}'
```
**Pass if**: HTTP 200.

### 3.8 Submit form
```bash
curl -X POST http://localhost:9867/action \
  -H "Authorization: Bearer benchmark-token" \
  -H "Content-Type: application/json" \
  -d '{"kind":"click","selector":"#submit-btn"}'
```
**Pass if**: HTTP 200.

### 3.9 Verify form submission
```bash
curl http://localhost:9867/text \
  -H "Authorization: Bearer benchmark-token"
```
**Pass if**: Response contains `VERIFY_FORM_SUBMITTED_SUCCESS` AND `SUBMISSION_DATA_NAME_JOHN_BENCHMARK`.

---

## Group 4: E-commerce Flow

### 4.1 Navigate to shop
```bash
curl -X POST http://localhost:9867/navigate \
  -H "Authorization: Bearer benchmark-token" \
  -H "Content-Type: application/json" \
  -d '{"url":"http://fixtures/ecommerce.html"}'
```
**Pass if**: HTTP 200.

### 4.2 Verify shop page
```bash
curl http://localhost:9867/text \
  -H "Authorization: Bearer benchmark-token"
```
**Pass if**: Response contains `VERIFY_SHOP_PAGE_44444` AND `Wireless Headphones` AND `$149.99`.

### 4.3 Add first product to cart
```bash
curl -X POST http://localhost:9867/action \
  -H "Authorization: Bearer benchmark-token" \
  -H "Content-Type: application/json" \
  -d '{"kind":"click","selector":"#product-1 .add-to-cart"}'
```
**Pass if**: HTTP 200.

### 4.4 Verify cart updated
```bash
curl http://localhost:9867/text \
  -H "Authorization: Bearer benchmark-token"
```
**Pass if**: Response contains `CART_ITEM_WIRELESS_HEADPHONES`.

### 4.5 Add second product
```bash
curl -X POST http://localhost:9867/action \
  -H "Authorization: Bearer benchmark-token" \
  -H "Content-Type: application/json" \
  -d '{"kind":"click","selector":"#product-2 .add-to-cart"}'
```
**Pass if**: HTTP 200.

### 4.6 Checkout
```bash
curl -X POST http://localhost:9867/action \
  -H "Authorization: Bearer benchmark-token" \
  -H "Content-Type: application/json" \
  -d '{"kind":"click","selector":"#checkout-btn"}'
```
**Pass if**: HTTP 200.

### 4.7 Verify checkout success
```bash
curl http://localhost:9867/text \
  -H "Authorization: Bearer benchmark-token"
```
**Pass if**: Response contains `VERIFY_CHECKOUT_SUCCESS_ORDER` AND `ORDER_TOTAL_449_98`.

---

## Group 5: Error Handling

### 5.1 Navigate to non-existent page
```bash
curl -X POST http://localhost:9867/navigate \
  -H "Authorization: Bearer benchmark-token" \
  -H "Content-Type: application/json" \
  -d '{"url":"http://fixtures/nonexistent-page-xyz.html"}'
```
**Pass if**: Returns error or 404 status (graceful handling, no crash).

### 5.2 Click non-existent element
```bash
curl -X POST http://localhost:9867/action \
  -H "Authorization: Bearer benchmark-token" \
  -H "Content-Type: application/json" \
  -d '{"kind":"click","selector":"#element-that-does-not-exist"}'
```
**Pass if**: Returns error message about element not found.

### 5.3 Fill with invalid selector
```bash
curl -X POST http://localhost:9867/action \
  -H "Authorization: Bearer benchmark-token" \
  -H "Content-Type: application/json" \
  -d '{"kind":"fill","selector":"#nonexistent-input","text":"test"}'
```
**Pass if**: Returns error message.

---

## Group 6: Agent Identity

### 6.1 Navigate with agent ID
```bash
curl -X POST http://localhost:9867/navigate \
  -H "Authorization: Bearer benchmark-token" \
  -H "X-Agent-Id: benchmark-agent-alpha" \
  -H "Content-Type: application/json" \
  -d '{"url":"http://fixtures/"}'
```
**Pass if**: HTTP 200.

### 6.2 Verify activity recorded
```bash
curl "http://localhost:9867/api/activity?agentId=benchmark-agent-alpha" \
  -H "Authorization: Bearer benchmark-token"
```
**Pass if**: Response contains activity entries for `benchmark-agent-alpha`.

### 6.3 Different agent action
```bash
curl -X POST http://localhost:9867/navigate \
  -H "Authorization: Bearer benchmark-token" \
  -H "X-Agent-Id: benchmark-agent-beta" \
  -H "Content-Type: application/json" \
  -d '{"url":"http://fixtures/search.html"}'
```
**Pass if**: HTTP 200.

### 6.4 Verify separate activity streams
```bash
curl "http://localhost:9867/api/activity?agentId=benchmark-agent-beta" \
  -H "Authorization: Bearer benchmark-token"
```
**Pass if**: Response contains activity for `benchmark-agent-beta` only.

---

## Summary

| Group | Steps | Description |
|-------|-------|-------------|
| 0 | 1 | Skill Loading Cost |
| 1 | 6 | Navigation Basics (fixtures) |
| 2 | 6 | Search Flow (fixtures) |
| 3 | 9 | Form Interaction (fixtures) |
| 4 | 7 | E-commerce Flow (fixtures) |
| 5 | 3 | Error Handling |
| 6 | 4 | Agent Identity |

**Total: 36 steps**

## Verification Strings Reference

| Page | Verification String |
|------|---------------------|
| Home | `VERIFY_HOME_LOADED_12345` |
| Articles | `VERIFY_ARTICLES_PAGE_67890` |
| Search | `VERIFY_SEARCH_PAGE_11111` |
| Form | `VERIFY_FORM_PAGE_22222` |
| Dashboard | `VERIFY_DASHBOARD_PAGE_33333` |
| Shop | `VERIFY_SHOP_PAGE_44444` |
| Form Submit | `VERIFY_FORM_SUBMITTED_SUCCESS` |
| Checkout | `VERIFY_CHECKOUT_SUCCESS_ORDER` |

After completing all steps:
```bash
./finalize-report.sh
```
