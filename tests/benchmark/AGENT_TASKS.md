# PinchTab Agent Benchmark

Natural language tasks to test how well an agent uses PinchTab from skill docs alone.

## Instructions

1. Read `../../skills/pinchtab/SKILL.md` — this is your only guide
2. For each task, figure out which commands to use
3. **Log every command executed**
4. Do not score your own work as pass/fail unless a task is trivially objective.
5. Prefer recording each step as `answer` with the actual response/result:
   `./scripts/record-step.sh --type agent <group> <step> answer "<what you saw>" "notes"`
6. A separate verification pass should later stamp the step with:
   `./scripts/verify-step.sh --type agent <group> <step> <pass|fail|skip> "verification notes"`

### Recommended setup: env vars + `./scripts/pt` wrapper

In normal PinchTab deployments the binary is installed on the host and
invoked directly:

```bash
export PINCHTAB_TOKEN=<token>
export PINCHTAB_TAB=$(pinchtab nav http://example.com)
pinchtab snap -i -c
pinchtab click '#submit'
```

The benchmark harness runs PinchTab **inside a Docker container**, so every
invocation normally has to be prefixed with
`docker exec -e PINCHTAB_TOKEN=... -e PINCHTAB_SERVER=... -e PINCHTAB_TAB=... benchmark-pinchtab-1 pinchtab ...`
— ~140 characters of boilerplate per command. The repo ships a tiny wrapper
at `tests/benchmark/scripts/pt` that handles the Docker preamble so you can
use the same terse CLI as a host install:

```bash
# Capture the shared tab ID. pinchtab prints only the tab ID when stdout
# is a pipe, so $(...) capture works directly.
export PINCHTAB_TAB=$(./scripts/pt nav http://fixtures/)

# Every subsequent tab-scoped command auto-targets $PINCHTAB_TAB:
./scripts/pt snap -i -c
./scripts/pt eval "document.title"
./scripts/pt eval --await-promise "window.fetchPayload()"
./scripts/pt click '#submit'
./scripts/pt drag '#piece' --drag-x 12 --drag-y -158

# Record answer/result for later verification:
./scripts/record-step.sh --type agent 1 2 answer "clicked submit and saw success banner" "worked"
```

The wrapper is a ~60-line shell script; read `scripts/pt` for the exact
forwarding rules. Flags after `pt` are forwarded verbatim to `pinchtab`, so
anything that works with `pinchtab ...` works with `./scripts/pt ...`.

Explicit `--tab <id>` on any command still wins over `PINCHTAB_TAB`.

For deferred verification, keep the answer payload factual. Record the
actual page text, count, answer, or state transition you observed, not your own
judgment about whether that matches the benchmark oracle.

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
Make a request to the server with a **wrong** token (`PINCHTAB_TOKEN=wrong-token ./scripts/pt health`) and confirm it is rejected. The `pt` wrapper always injects the benchmark token by default, so you must explicitly override it to test auth rejection.

**Verify**: Response is HTTP 401 or contains `unauthorized`.

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
Navigate to `http://fixtures/iframe.html` and extract content from inside the embedded same-origin iframe.

**Verify**: The iframe's inner content includes `IFRAME_INNER_CONTENT_LOADED`.

### 19.2 Type into iframe input (native frame scope)
Use `pinchtab frame '#content-frame'` to scope into the iframe, then `fill` the input with "Hello World" and `click` the Save button. Verify via a scoped `snap` — `text --full` doesn't pierce iframes. Reset with `pinchtab frame main` afterwards.

**Verify**: Scoped snapshot contains `IFRAME_INPUT_RECEIVED_HELLO_WORLD`.

---

## Group 20: Dialogs

### 20.1 Accept alert
Navigate to `http://fixtures/alerts.html`. Click the "Click for Alert" button and dismiss the alert.

**Verify**: The page result contains `DIALOG_ALERT_DISMISSED`.

### 20.2 Cancel confirm
Click the "Click for Confirm" button and cancel the confirm dialog.

**Verify**: The page result contains `DIALOG_CONFIRM_CANCELLED`.

---

## Group 21: Async / awaitPromise

### 21.1 Await a promise-returning function
Navigate to `http://fixtures/async.html`. The page exposes `window.fetchPayload()`, which returns a `Promise` that resolves after a short delay. Use `eval` to call it and retrieve the **resolved** value, not a Promise wrapper.

**Verify**: The resolved value contains `ASYNC_PAYLOAD_READY_42`.

### 21.2 Await a promise resolving to an object
On the same page, call `window.fetchUser()` and retrieve the resolved object so you can read a field from it.

**Verify**: The resolved object's `name` field equals `ASYNC_USER_NAME_ADA`.

---

## Group 22: Mouse Drag & Drop

### 22.1 Drag a piece into Zone A
Navigate to `http://fixtures/drag.html`. The page contains a draggable square (`#piece`) and three target zones (`#zone-a`, `#zone-b`, `#zone-c`). Drag the piece so its center ends up over **Zone A**.

**Verify**: The page shows `LAST_DROP=DROP_ZONE_A_OK`.

### 22.2 Drag to Zone B, then Zone C
Without reloading the page, drag the piece next into **Zone B**, and then into **Zone C**. The page records an ordered drop sequence.

**Verify**: The page shows `DROP_SEQUENCE=DROP_ZONE_A_OK,DROP_ZONE_B_OK,DROP_ZONE_C_OK` (all three drops in order).

---

## Group 23: Async / Loading state

### 23.1 Wait for async content
Navigate to `http://fixtures/loading.html`. The page shows a spinner and the text "Loading..." for ~1.5 seconds, then replaces it with the final payload. Wait for the final content to appear before reporting completion.

**Verify**: The final snapshot contains `VERIFY_LOADING_COMPLETE_88888`.

---

## Group 24: Keyboard events

### 24.1 Press Escape
Navigate to `http://fixtures/keyboard.html` (it auto-focuses an input so keyboard events land). Press the Escape key.

**Verify**: Page text contains `KEYBOARD_ESCAPE_PRESSED`.

### 24.2 Press 'a' then Enter
Without reloading, press the `a` key, then the `Enter` key.

**Verify**: Page text now contains all three markers in order: `KEYBOARD_ESCAPE_PRESSED`, `KEYBOARD_KEY_A_PRESSED`, `KEYBOARD_ENTER_PRESSED`.

---

## Group 25: Tab panels

### 25.1 Switch to the Settings tab
Navigate to `http://fixtures/tabs.html`. The Profile tab is shown by default. Click the Settings tab.

**Verify**: After the click, the page content includes `TAB_SETTINGS_CONTENT` (and the Profile content is no longer visible).

### 25.2 Switch to the Billing tab
Click the Billing tab.

**Verify**: Page content now includes `TAB_BILLING_CONTENT`.

---

## Group 26: Accordion

### 26.1 Open section A
Navigate to `http://fixtures/accordion.html`. Click the header for Section A to expand it.

**Verify**: Page text includes `ACCORDION_SECTION_A_OPEN`.

### 26.2 Open section B
Click the header for Section B. Because the accordion is exclusive-expand, Section A should close.

**Verify**: Page text includes `ACCORDION_SECTION_B_OPEN`, and Section A's `aria-expanded` attribute is now `"false"` (use `eval` to check the attribute if needed).

---

## Group 27: Contenteditable editor

### 27.1 Type into the rich-text editor
Navigate to `http://fixtures/editor.html`. Type the text `Hello rich text` into the `#editor` div. `contenteditable` elements don't have `.value`, so use `type` (keyboard events), not `fill`.

**Verify**: Page text contains `EDITOR_CHARS=15` and the mirror shows `Hello rich text`.

### 27.2 Commit by pressing Enter
Press Enter (the editor intercepts Enter to commit the current buffer to a separate marker).

**Verify**: Page text contains `EDITOR_COMMITTED=Hello rich text`.

---

## Group 28: Range slider

### 28.1 Move slider into HIGH bucket
Navigate to `http://fixtures/range.html`. Set the `#volume` range slider to 90.

**Verify**: Page text contains both `RANGE_VALUE_90` and `BUCKET_HIGH`.

### 28.2 Move slider into LOW bucket
Without reloading, change the slider to 10.

**Verify**: Page text now contains `RANGE_VALUE_10` and `BUCKET_LOW`.

---

## Group 29: Pagination

### 29.1 Advance to page 2
Navigate to `http://fixtures/pagination.html`. Click the Next button.

**Verify**: Page text contains `PAGE_2_FIRST_ITEM` and `PAGE_2_OF_3`.

### 29.2 Reach the last page; Next disabled
Click Next once more.

**Verify**: Page text contains `PAGE_3_FIRST_ITEM` and `PAGE_3_OF_3`, and the Next button's `disabled` property is `true` (use `eval` to check).

---

## Group 30: Custom dropdown menu

### 30.1 Pick the "Beta" item
Navigate to `http://fixtures/dropdown.html`. The page has a custom dropdown — click the toggle button (`#dropdown-toggle`) to open the menu, then click the "Beta" item.

**Verify**: Page text contains `DROPDOWN_SELECTED=BETA`.

### 30.2 Reopen and pick "Gamma"
Reopen the dropdown and select "Gamma". (Each selection auto-closes the menu, so you need to click the toggle again before selecting.)

**Verify**: Page text contains `DROPDOWN_SELECTED=GAMMA`.

---

## Group 31: Nested iframes (3 levels deep)

### 31.1 Click a button in the deepest frame
Navigate to `http://fixtures/iframe-nested.html`. The outer page embeds `#level-2`, which in turn embeds `#level-3`, which contains `#deep-button`. Drill through the two frame hops and click the button. Verify via a scoped `snap` before resetting to `main`.

**Verify**: Scoped snapshot contains `DEEP_CLICKED=YES_LEVEL_3`.

---

## Group 32: Dynamic iframe (inserted after load)

### 32.1 Wait for a late iframe, then interact with it
Navigate to `http://fixtures/iframe-dynamic.html`. The iframe is inserted ~1.2 s after load (use `wait --text "IFRAME_DYNAMIC_ATTACHED"`). Then scope into `#late-frame`, fill `#iframe-input` with "Late World", click `#iframe-submit`, and verify the inner result marker.

**Verify**: Scoped snapshot contains `IFRAME_INPUT_RECEIVED_LATE_WORLD`.

---

## Group 33: srcdoc iframe

### 33.1 Interact with an inline-content iframe
Navigate to `http://fixtures/iframe-srcdoc.html`. The iframe's content is inlined via `srcdoc`; it has no `src` URL. Scope into `#srcdoc-frame`, fill `#inline-input` with `srcdoc`, click `#inline-submit`, and verify.

**Verify**: Scoped snapshot contains `INLINE_RECEIVED_SRCDOC`.

---

## Group 34: Sandboxed iframe

### 34.1 Click inside a sandboxed iframe
Navigate to `http://fixtures/iframe-sandbox.html`. The iframe has `sandbox="allow-scripts allow-same-origin"`. Scope into `#sandboxed`, click `#sandbox-button`, and verify.

**Verify**: Scoped snapshot contains `SANDBOX_CLICKED=YES`.

---

## Group 35: Long-form article (Medium/Substack style)

### 35.1 Read the article with default `text`
Navigate to `http://fixtures/article.html`. Extract the page's main content using the default `text` (Readability) mode. Default mode is the right choice for article-style pages.

**Verify**: The extracted text contains both `ARTICLE_PUBLISHED_2026_04_15` and `ARTICLE_WORD_COUNT_MARKER_323` (both inside the article body — Readability keeps them).

### 35.2 See the chrome that Readability drops
Re-extract with `text --full` and confirm the footer is included this time.

**Verify**: `--full` output contains `FOOTER_COPYRIGHT_MARKER` (which the default mode trims).

---

## Group 36: Search results page (SERP)

### 36.1 Find a specific result by id
Navigate to `http://fixtures/serp.html`. The page has 6 result cards (`#r-1`..`#r-6`). Extract just the third card's content. A scoped snapshot (`snap --selector "#r-3"`) is the direct path.

**Verify**: The scoped output contains `RESULT_3_TITLE` and `RESULT_3_SNIPPET_MARKER`.

### 36.2 Count all result cards
Use a full text extraction to verify all six results are present in one pass.

**Verify**: Output contains all of `RESULT_1_TITLE` through `RESULT_6_TITLE` and the summary `SERP_RESULT_COUNT_6`. Default Readability trims SERPs — use `text --full`.

---

## Group 37: Q&A thread (Stack-Overflow style)

### 37.1 Find the accepted answer id
Navigate to `http://fixtures/qa.html`. The accepted answer carries `data-accepted="true"` on its `<div>` wrapper. Use `eval` to return that element's `id`.

**Verify**: The `eval` result equals `"a-2"`.

### 37.2 Extract the accepted answer's body
Scope a snapshot to `#a-2` and verify the body content.

**Verify**: Scoped snapshot contains `ANSWER_2_BODY_MARKER` and `ACCEPTED_ANSWER_ID_A2`.

---

## Group 38: Pricing table

### 38.1 Read just the Pro plan
Navigate to `http://fixtures/pricing.html`. The page has three plan cards (`#plan-free`, `#plan-pro`, `#plan-enterprise`). Read only the Pro plan using a scoped snapshot.

**Verify**: Scoped snapshot contains `PLAN_PRO_PRICE_29` and `PLAN_PRO_LIMIT_5000_requests per day`.

### 38.2 Compare all three plans
Extract the full page (pricing grids are a Readability anti-pattern — reach for `--full`).

**Verify**: Output contains `PLAN_FREE_PRICE_0`, `PLAN_PRO_PRICE_29`, and `PLAN_ENTERPRISE_PRICE_CUSTOM`.

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
| 35 | 2 | Long-form article |
| 36 | 2 | Search results page |
| 37 | 2 | Q&A thread |
| 38 | 2 | Pricing table |

**Total: 85 tasks**

## Key Differences from Baseline

The agent must:
- Choose between `/text`, `/snapshot`, `/action`, `/navigate` appropriately
- Decide when to use `filter=interactive` vs full snapshot
- Handle multi-step flows without step-by-step curl guidance
- Extract and interpret structured data (tables, lists, counts)
- Detect state changes after interactions
