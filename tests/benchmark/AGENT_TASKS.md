# PinchTab Agent Benchmark

Natural language tasks to test how well an agent uses PinchTab from skill docs alone.

## Instructions

1. Read `../../skills/pinchtab/SKILL.md` — this is your only guide
2. For each task, figure out which commands to use
3. **Log every command executed**
4. Record: `./record-agent-step.sh <group> <step> <pass|fail> <in> <out> "commands" "notes"`

## Environment

- PinchTab: `http://localhost:9867`, token: `benchmark-token`
- Fixtures: `http://fixtures/` (running in Docker as `fixtures` hostname)
- Pages: `/`, `/wiki.html`, `/wiki-go.html`, `/articles.html`, `/search.html`,
  `/form.html`, `/dashboard.html`, `/ecommerce.html`, `/spa.html`, `/login.html`

---

## Group 0: Setup Verification

### 0.1 Confirm PinchTab is running
Check that the PinchTab server is healthy and a Chrome instance is active.

**Verify**: Server responds with `status: ok` and at least one running instance.

### 0.2 Navigate to fixtures and confirm reachable
Navigate to `http://fixtures/` and confirm the benchmark fixtures server is reachable.

**Verify**: Page title contains "Benchmark" or "PinchTab".

---

## Group 1: Reading & Extracting Real Content

### 1.1 Get the full list of categories from the wiki index
Navigate to `http://fixtures/wiki.html` and extract all category names with their article counts.

**Verify**: Can name at least 2 categories and their article counts (e.g. "Programming Languages: 12 articles").

### 1.2 Navigate by clicking a link
From the wiki index, click the "Go (programming language)" link to navigate to the Go article.

**Verify**: You are now on the Go article page (not the wiki index).

### 1.3 Extract structured data from a table
On the Go article, read the infobox table and answer: Who designed Go, and what year did it first appear?

**Verify**: Answer contains "Robert Griesemer" (or "Rob Pike" or "Ken Thompson") and "2009".

### 1.4 Count list items
On the Go article, count how many key features are listed.

**Verify**: Answer is 6 (verify against `FEATURE_COUNT_6`).

### 1.5 Read all article headlines from articles page
Navigate to `http://fixtures/articles.html` and list all article titles.

**Verify**: Found at least 3 articles including "The Future of Artificial Intelligence".

### 1.6 Read dashboard metrics
Navigate to `http://fixtures/dashboard.html` and extract: Total Users, Revenue, and Conversion Rate.

**Verify**: Found `24,582` users AND `$1,284,930` revenue.

---

## Group 2: Search & Dynamic Interaction

### 2.1 Use wiki search to find a page
On `http://fixtures/wiki.html`, search for "golang" using the search form. Do not navigate directly — use the search input.

**Verify**: Ended up on the Go article page after search.

### 2.2 Search with no results
On `http://fixtures/search.html`, search for something with no results (use "xyznonexistent").

**Verify**: Page handled it gracefully (no crash, some response rendered).

### 2.3 Search for AI content
On `http://fixtures/search.html`, search for "artificial intelligence" and verify a result appeared.

**Verify**: Result contains "The Future of Artificial Intelligence".

---

## Group 3: Complex Form

### 3.1 Fill and submit a complete form
Navigate to `http://fixtures/form.html`. Complete the entire form:
- Full name: "Agent Test User"
- Email: "agent@benchmark.test"  
- Phone: "+44 20 9999 0000"
- Country: United Kingdom
- Subject: Technical Support
- Message: "Testing PinchTab form automation"
- Check newsletter
- Set priority to High
- Submit

**Verify**: Form submitted successfully. Confirmation shows name "AGENT_TEST_USER".

### 3.2 Reset and refill
After submitting, if the form is still accessible (or navigate back), verify you can identify the reset button.

**Verify**: Reset/back button or form element found in snapshot.

---

## Group 4: SPA State Management

### 4.1 Read initial app state
Navigate to `http://fixtures/spa.html`. Read the current task list — how many tasks exist, how many are active vs done?

**Verify**: Found 3 total, 2 active, 1 done (verify `TASK_STATS_TOTAL_3_ACTIVE_2_DONE_1`).

### 4.2 Add a new high-priority task
Add a task called "Automate deployment" with high priority.

**Verify**: Task appeared in the list (`TASK_ADDED_AUTOMATE_DEPLOYMENT_PRIORITY_HIGH`).

### 4.3 Delete a task
Delete the first existing task ("Write benchmark tests").

**Verify**: Task count changed (went from 4 to 3).

---

## Group 5: Login Flow

### 5.1 Attempt login with wrong credentials
Navigate to `http://fixtures/login.html`. Try to log in with username "admin" and password "wrong".

**Verify**: Error message appeared (`INVALID_CREDENTIALS_ERROR`).

### 5.2 Login successfully
Clear the form and log in with username "benchmark" / password "test456".

**Verify**: Dashboard appeared after login (`VERIFY_LOGIN_SUCCESS_DASHBOARD`).

---

## Group 6: Multi-Step E-commerce

### 6.1 Research products before buying
Navigate to `http://fixtures/ecommerce.html`. List all available products with their prices. Which product is out of stock?

**Verify**: Found Wireless Headphones ($149.99), Smart Watch Pro ($299.99), Portable Charger ($49.99). Mechanical Keyboard is out of stock.

### 6.2 Add two items and verify total
Add Wireless Headphones and Portable Charger to cart. What is the total?

**Verify**: Cart total is $199.98 (149.99 + 49.99).

### 6.3 Complete checkout
Click checkout to complete the order.

**Verify**: Order confirmation shows (`VERIFY_CHECKOUT_SUCCESS_ORDER`).

---

## Group 7: Content + Interaction Combined

### 7.1 Read and comment on Go article
Navigate to `http://fixtures/wiki-go.html`. Read the article, then post a comment with rating 5 stars and text "Excellent reference".

**Verify**: Comment posted (`COMMENT_POSTED_RATING_5_TEXT_RECEIVED`).

### 7.2 Cross-page research task
Navigate to wiki index, find which category has the most articles, then navigate to one of its listed items.

**Verify**: Successfully navigated to at least one article page after reading the category counts.

---

## Group 8: Error Handling

### 8.1 Handle 404 gracefully
Try to navigate to a page that doesn't exist: `http://fixtures/missing-page-abc.html`.

**Verify**: Got a response (404 or error), no crash, server still responsive after.

### 8.2 Handle missing element gracefully
On any page, try to click an element with ID `#fake-button-that-does-not-exist`.

**Verify**: Got a clear error message, not a crash or hang.

---

## Group 9: Export

### 9.1 Screenshot a complex page
Navigate to `http://fixtures/dashboard.html` and take a screenshot.

**Verify**: Screenshot generated (file saved or base64 returned).

### 9.2 Export a page as PDF
Export the dashboard as a PDF.

**Verify**: PDF generated.

---

## Summary

| Group | Tasks | Description |
|-------|-------|-------------|
| 0 | 2 | Setup Verification |
| 1 | 6 | Reading & Extracting Content |
| 2 | 3 | Search & Dynamic Interaction |
| 3 | 2 | Complex Form |
| 4 | 3 | SPA State Management |
| 5 | 2 | Login Flow |
| 6 | 3 | Multi-Step E-commerce |
| 7 | 2 | Content + Interaction Combined |
| 8 | 2 | Error Handling |
| 9 | 2 | Export |

**Total: 27 tasks**

## Key Differences from Baseline

The agent must:
- Choose between `/text`, `/snapshot`, `/action`, `/navigate` appropriately
- Decide when to use `filter=interactive` vs full snapshot
- Handle multi-step flows without step-by-step curl guidance
- Extract and interpret structured data (tables, lists, counts)
- Detect state changes after interactions
