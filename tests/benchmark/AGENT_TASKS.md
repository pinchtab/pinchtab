# PinchTab Agent Benchmark

Natural language tasks to test how well an agent uses PinchTab from skill docs alone.

## Instructions

1. Read `../../skills/pinchtab/SKILL.md` — this is your only guide
2. For each task, figure out which commands to use
3. **Log every command executed**
4. Record: `./scripts/record-step.sh --type agent <group> <step> <pass|fail> --tokens <in> <out> "notes"`

## Environment

- PinchTab: `http://localhost:9867`, token: `benchmark-token`
- Fixtures: `http://fixtures/` (running in Docker as `fixtures` hostname)
- Pages: `/`, `/wiki.html`, `/wiki-go.html`, `/articles.html`, `/search.html`,
  `/form.html`, `/dashboard.html`, `/ecommerce.html`, `/spa.html`, `/login.html`

---

## Group 0: Setup & Diagnosis

### 0.1 Server reachable
Check that the PinchTab server is healthy.

**Verify**: Server responds with `status: ok`.

### 0.2 Auth is required
Make a request to the server WITHOUT an auth token and confirm it is rejected.

**Verify**: Response is HTTP 401 (or other auth-error status).

### 0.3 Auth works with token
Repeat the same request WITH the bearer token and confirm it succeeds.

**Verify**: Response is HTTP 200.

### 0.4 Instance available
Confirm at least one Chrome instance is running. If none exist, start one.

**Verify**: Health response shows `defaultInstance.status == "running"` (or after starting one, the new instance is running).

### 0.5 List existing tabs
Get the current list of open tabs.

**Verify**: A list (possibly empty) is returned without error.

### 0.6 Clean stale tabs
If any tabs from previous runs are open, close them so the benchmark starts from a known state.

**Verify**: After cleanup, the tab list is empty (or only contains a single about:blank tab).

### 0.7 Network reach to target
Navigate to `http://fixtures/` and confirm the fixtures server is reachable from PinchTab.

**Verify**: Navigate returns successfully and the page contains benchmark content.

### 0.8 Capture initial tab ID
Save the tab ID returned by the navigate in 0.7. Use this tab ID for all subsequent tasks to avoid creating new tabs.

**Verify**: A tab ID was captured and matches what `GET /tabs` reports.

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
Navigate to `http://fixtures/spa.html?reset=1` (the `?reset=1` query param clears any previous localStorage state so the SPA starts with its default 3 tasks). Read the current task list — how many tasks exist, how many are active vs done?

**Verify**: Found 3 total, 2 active, 1 done (verify `TASK_STATS_TOTAL_3_ACTIVE_2_DONE_1`).

### 4.2 Add a new high-priority task
Add a task called "Automate deployment" with high priority.

**Verify**: Task appeared in the list (`TASK_ADDED_AUTOMATE_DEPLOYMENT_PRIORITY_HIGH`).

### 4.3 Delete a task
Delete the task titled "Write benchmark tests".

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
Add Wireless Headphones and Smart Watch Pro to cart. What is the total?

**Verify**: Cart total is $449.98 (149.99 + 299.99).

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

## Group 10: Nested Interactions & Modal Dialogs

### 10.1 Open and interact with modal on dashboard
Navigate to `http://fixtures/dashboard.html`. Find and click the Settings button (selector: `#settings-btn`) to open the modal dialog.

**Verify**: Modal appeared — snapshot contains "Dashboard Settings".

### 10.2 Modify settings and close modal
In the modal, select "Dark" from the theme dropdown (`#theme-select`), then click the Save button (`#modal-save`). After the modal closes, check the page content.

**Verify**: Page contains `THEME_DARK_APPLIED`.

---

## Group 11: State Persistence & Page Reload

### 11.1 Add an item and verify after page reload
Navigate to `http://fixtures/spa.html?reset=1` (starts with clean state). Add a task titled exactly "Persistent Task Test". Then navigate away to `http://fixtures/` and back to `http://fixtures/spa.html` (WITHOUT the reset param) to verify localStorage persistence.

**Verify**: After reload, the task still appears in the list (`TASK_PERSISTENT_TEST_FOUND_AFTER_RELOAD`).

### 11.2 Logout and log back in
From the logged-in dashboard, click Sign Out to log out. Then log in again with username "benchmark" / password "test456".

**Verify**: Successfully logged back in and dashboard shows `SESSION_RENEWED`.

---

## Group 12: Multi-Page Navigation & Back Button

### 12.1 Navigate through multiple pages and return
Starting from `http://fixtures/`, navigate to wiki → Go article → back to wiki → back to home.

**Verify**: Successfully returned to home page (title contains "Benchmark" or "Home").

### 12.2 Compare data across pages
Navigate to wiki.html, note the total article count from categories. Navigate to articles.html, count articles there. Compare totals.

**Verify**: Can report totals from both pages and explain difference (`COMPARISON_DATA_FOUND`).

---

## Group 13: Form State & Multi-Step Submission

### 13.1 Submit form without email
Navigate to `http://fixtures/form.html`. Fill only the name field ("Validator Test"), leave email blank, click Submit. The browser's native required-field validation will prevent submission.

**Verify**: Submission blocked (form stays open, no success message shown).

### 13.2 Submit form without optional phone field
Fill the form with: name "No Phone User", email "nophone@test.com", country "de", subject "feedback". Leave the phone field empty. Submit.

**Verify**: Submission succeeded and page shows `OPTIONAL_FIELD_SKIPPED_SUCCESS`.

---

## Group 14: Dynamic Content Loading

### 14.1 Load more products
Navigate to `http://fixtures/ecommerce.html`. Find and click the "Load More Products" button to reveal additional products.

**Verify**: Additional products appeared (`ADDITIONAL_PRODUCTS_LOADED`).

### 14.2 Add a lazy-loaded product to cart
After loading more products, add product #5 (USB-C Cable) to the cart.

**Verify**: Cart shows the lazy-loaded item (`CART_UPDATED_WITH_LAZY_PRODUCT`).

---

## Group 15: Complex Data Extraction & Aggregation

### 15.1 Extract and sum financial data
Navigate to `http://fixtures/dashboard.html`. Extract revenue and profit values, calculate profit margin.

**Verify**: Correctly calculated: profit_margin = (profit / revenue) * 100 (`PROFIT_MARGIN_CALCULATED`).

### 15.2 Build comparison table from multiple sources
Visit these 3 pages and compare their feature counts and key features:
- `http://fixtures/wiki-go.html` (Go: 6 features)
- `http://fixtures/wiki-python.html` (Python: 7 features)
- `http://fixtures/wiki-rust.html` (Rust: 5 features)

Report which language has the most features and name 1 feature unique to each.

**Verify**: Response is factually correct AND wiki-python.html contains `COMPARISON_TABLE_BUILT`.

---

## Group 16: Hover & Tooltips

### 16.1 Hover reveals hidden content
Navigate to `http://fixtures/hovers.html`. Hover over the first avatar to reveal its hidden info.

**Verify**: After hovering, snapshot contains `HOVER_REVEALED_USER_1`.

### 16.2 Hover swap
Hover over the second avatar.

**Verify**: Snapshot contains `HOVER_REVEALED_USER_2`.

---

## Group 17: Scrolling

### 17.1 Scroll by pixels
Navigate to `http://fixtures/scroll.html`. Scroll down approximately 1500 pixels.

**Verify**: Snapshot or text contains `SCROLL_MIDDLE_MARKER` (the mid-page marker is now in view).

### 17.2 Scroll to footer
Continue scrolling until the footer is visible (or scroll directly to the footer element).

**Verify**: Snapshot contains `SCROLL_REACHED_FOOTER`.

---

## Group 18: File Download

### 18.1 Download a file
Download `http://fixtures/download-sample.txt` to a local file.

**Verify**: The downloaded file exists and its content includes `DOWNLOAD_FILE_CONTENT_VERIFIED`.

---

## Group 19: iFrame

### 19.1 Read iframe content
Navigate to `http://fixtures/iframe.html` and extract content from inside the embedded iframe.

**Verify**: The iframe's inner content includes `IFRAME_INNER_CONTENT_LOADED`.

### 19.2 Type into iframe input
Inside the iframe, fill the input with the text "Hello World" and click the Save button.

**Verify**: The iframe shows `IFRAME_INPUT_RECEIVED_HELLO_WORLD`.

---

## Group 20: Dialogs

### 20.1 Accept alert
Navigate to `http://fixtures/alerts.html`. Click the "Click for Alert" button and dismiss the alert.

**Verify**: The page result contains `DIALOG_ALERT_DISMISSED`.

### 20.2 Cancel confirm
Click the "Click for Confirm" button and cancel the confirm dialog.

**Verify**: The page result contains `DIALOG_CONFIRM_CANCELLED`.

---

## Summary

| Group | Tasks | Description |
|-------|-------|-------------|
| 0 | 8 | Setup & Diagnosis |
| 1 | 6 | Reading & Extracting Content |
| 2 | 3 | Search & Dynamic Interaction |
| 3 | 2 | Complex Form |
| 4 | 3 | SPA State Management |
| 5 | 2 | Login Flow |
| 6 | 3 | Multi-Step E-commerce |
| 7 | 2 | Content + Interaction Combined |
| 8 | 2 | Error Handling |
| 9 | 2 | Export |
| 10 | 2 | Nested Interactions & Modal Dialogs |
| 11 | 2 | State Persistence & Page Reload |
| 12 | 2 | Multi-Page Navigation & Back Button |
| 13 | 2 | Form State & Multi-Step Submission |
| 14 | 2 | Dynamic Content Loading |
| 15 | 2 | Complex Data Extraction & Aggregation |
| 16 | 2 | Hover & Tooltips |
| 17 | 2 | Scrolling |
| 18 | 1 | File Download |
| 19 | 2 | iFrame |
| 20 | 2 | Dialogs |

**Total: 54 tasks**

## Key Differences from Baseline

The agent must:
- Choose between `/text`, `/snapshot`, `/action`, `/navigate` appropriately
- Decide when to use `filter=interactive` vs full snapshot
- Handle multi-step flows without step-by-step curl guidance
- Extract and interpret structured data (tables, lists, counts)
- Detect state changes after interactions
