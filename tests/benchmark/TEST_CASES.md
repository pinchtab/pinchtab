# PinchTab Benchmark Test Cases

85 test cases covering realistic browser automation scenarios against local fixture pages.

| # | Task | Description |
|---|------|-------------|
| **Group 0: Setup & Diagnosis** | | |
| 0.1 | Server reachable | `GET /health` returns `status: ok` |
| 0.2 | Auth required | Request without token returns 401 |
| 0.3 | Auth works with token | Request with bearer token returns 200 |
| 0.4 | Instance available | At least one instance running, or start one |
| 0.5 | List existing tabs | `GET /tabs` returns array without error |
| 0.6 | Clean stale tabs | Close any leftover tabs from previous runs |
| 0.7 | Network reach to target | Navigate to fixtures root, get success |
| 0.8 | Capture initial tab ID | Save tab ID from 0.7 for subsequent tasks |
| **Group 1: Reading & Extracting** | | |
| 1.1 | Wiki categories | Extract category names + article counts from wiki index |
| 1.2 | Click a link | From wiki, click through to Go article |
| 1.3 | Table extraction | Read infobox — who designed Go, what year |
| 1.4 | Count list items | Count key features on Go article (expect 6) |
| 1.5 | Article headlines | Navigate to articles page, list all titles |
| 1.6 | Dashboard metrics | Extract Total Users, Revenue, Conversion Rate |
| **Group 2: Search & Dynamic** | | |
| 2.1 | Wiki search | Use search form to find "golang" |
| 2.2 | No results search | Search for nonexistent term, verify graceful |
| 2.3 | AI content search | Search for "artificial intelligence" |
| **Group 3: Form** | | |
| 3.1 | Complete form | Fill all fields + submit (name, email, phone, country, subject, message, checkbox, radio) |
| 3.2 | Reset/refill | After submit, verify reset button exists |
| **Group 4: SPA** | | |
| 4.1 | Read app state | Count tasks: 3 total, 2 active, 1 done |
| 4.2 | Add task | Add "Automate deployment" with high priority |
| 4.3 | Delete task | Delete first task, verify count changed |
| **Group 5: Login** | | |
| 5.1 | Invalid login | Try wrong credentials, verify error |
| 5.2 | Valid login | Login with correct creds, verify dashboard |
| **Group 6: E-commerce** | | |
| 6.1 | Research products | List products + prices, identify out-of-stock |
| 6.2 | Add to cart | Add 2 items, verify $449.98 total |
| 6.3 | Checkout | Complete order, verify confirmation |
| **Group 7: Content + Interaction** | | |
| 7.1 | Read & comment | Read Go article, post comment with 5-star rating |
| 7.2 | Cross-page research | Find biggest category on wiki, navigate to an article in it |
| **Group 8: Error Handling** | | |
| 8.1 | 404 handling | Navigate to missing page, verify no crash |
| 8.2 | Missing element | Click nonexistent selector, verify clear error |
| **Group 9: Export** | | |
| 9.1 | Screenshot | Take screenshot of dashboard |
| 9.2 | PDF export | Export dashboard as PDF |
| **Group 10: Modals** | | |
| 10.1 | Open modal | Click Settings on dashboard, verify modal |
| 10.2 | Modal interaction | Change theme to Dark, save, verify applied |
| **Group 11: Persistence** | | |
| 11.1 | State after reload | Add task, reload SPA, verify task persists |
| 11.2 | Logout/re-login | Sign out, log back in, verify session renewed |
| **Group 12: Multi-page Nav** | | |
| 12.1 | Navigate & return | Home -> wiki -> Go -> back -> back -> home |
| 12.2 | Cross-page compare | Compare article counts between wiki and articles page |
| **Group 13: Form Validation** | | |
| 13.1 | Required field | Submit without email, verify blocked |
| 13.2 | Optional field | Submit without phone, verify success |
| **Group 14: Dynamic Content** | | |
| 14.1 | Load more | Click "Load More Products" on ecommerce |
| 14.2 | Lazy-loaded item | Add a lazy-loaded product to cart |
| **Group 15: Data Aggregation** | | |
| 15.1 | Financial calc | Extract revenue + profit, calculate margin |
| 15.2 | Multi-page comparison | Compare features across 3 wiki language pages |
| **Group 16: Hover & Tooltips** | | |
| 16.1 | Hover reveals info | Hover first avatar, verify hidden content appears |
| 16.2 | Hover swap | Hover second avatar, verify different content appears |
| **Group 17: Scrolling** | | |
| 17.1 | Scroll by pixels | Scroll down 1500px, verify mid-page marker visible |
| 17.2 | Scroll to footer | Scroll to bottom, verify footer marker visible |
| **Group 18: File Download** | | |
| 18.1 | Download a file | Download sample.txt, verify content marker present |
| **Group 19: iFrame** | | |
| 19.1 | Read iframe content | Verify content from inside iframe is accessible |
| 19.2 | Type into iframe | Fill input inside iframe, verify saved value |
| **Group 20: Dialogs** | | |
| 20.1 | Accept alert | Trigger alert, accept, verify result marker |
| 20.2 | Cancel confirm | Trigger confirm dialog, cancel, verify cancelled marker |
| **Group 21: Async / awaitPromise** | | |
| 21.1 | Await promise (string) | `eval` with `awaitPromise:true` returns resolved string payload |
| 21.2 | Await promise (object) | `eval` with `awaitPromise:true` returns resolved object fields |
| **Group 22: Mouse Drag & Drop** | | |
| 22.1 | Drag into Zone A | Use high-level `drag` action to move piece into Zone A |
| 22.2 | Low-level drag sequence | Use `mouse-down`/`mouse-move`/`mouse-up` to visit Zone B then C; verify ordered drop sequence |
| **Group 23: Async / Loading state** | | |
| 23.1 | Wait for async content | Page shows spinner for 1.5 s, then replaces it with the final marker |
| **Group 24: Keyboard events** | | |
| 24.1 | Press Escape | Verify `KEYBOARD_ESCAPE_PRESSED` marker after `press Escape` |
| 24.2 | Sequential keys | Press `a` then `Enter`, verify both markers accumulate in log |
| **Group 25: Tab panels** | | |
| 25.1 | Switch to Settings tab | Click Settings tab, verify `TAB_SETTINGS_CONTENT` replaces Profile content |
| 25.2 | Switch to Billing tab | Click Billing tab, verify `TAB_BILLING_CONTENT` |
| **Group 26: Accordion** | | |
| 26.1 | Open section A | Click Section A header, verify `ACCORDION_SECTION_A_OPEN` |
| 26.2 | Open section B | Click Section B header, verify B marker + Section A `aria-expanded=false` |
| **Group 27: Contenteditable editor** | | |
| 27.1 | Type into editor | `type` "Hello rich text" into `#editor`, verify char count + mirror |
| 27.2 | Commit with Enter | Press Enter (intercepted), verify `EDITOR_COMMITTED=...` |
| **Group 28: Range slider** | | |
| 28.1 | Set slider to HIGH | `fill #volume 90`, verify value + bucket markers |
| 28.2 | Set slider to LOW | `fill #volume 10`, verify value + bucket markers |
| **Group 29: Pagination** | | |
| 29.1 | Advance to page 2 | Click Next, verify `PAGE_2_FIRST_ITEM` and `PAGE_2_OF_3` |
| 29.2 | Last page; Next disabled | Click Next, verify page-3 markers + `disabled=true` |
| **Group 30: Custom dropdown menu** | | |
| 30.1 | Pick Beta | Click toggle, click data-value=beta, verify selection marker |
| 30.2 | Reopen + pick Gamma | Click toggle again, click Gamma, verify selection marker |
| **Group 31: Nested iframes (3 levels)** | | |
| 31.1 | Click deepest-frame button | `frame #level-2` → `frame #level-3` → click `#deep-button`; verify `DEEP_CLICKED=YES_LEVEL_3` |
| **Group 32: Dynamic iframe** | | |
| 32.1 | Wait for late iframe | `wait --text IFRAME_DYNAMIC_ATTACHED` → scope in → fill/click → verify result marker |
| **Group 33: srcdoc iframe** | | |
| 33.1 | Inline iframe content | scope `#srcdoc-frame` → fill/click inline form → verify `INLINE_RECEIVED_SRCDOC` |
| **Group 34: Sandboxed iframe** | | |
| 34.1 | Click in sandboxed frame | `sandbox="allow-scripts allow-same-origin"`; scope + click → verify `SANDBOX_CLICKED=YES` |
| **Group 35: Long-form article** | | |
| 35.1 | Default Readability mode keeps body | Verify date + word-count markers via default `text` |
| 35.2 | `--full` keeps the chrome | Verify footer marker only appears in `--full` output |
| **Group 36: Search results page** | | |
| 36.1 | Extract result #3 via scoped snapshot | `snap --selector "#r-3"`; verify title + snippet markers |
| 36.2 | All six results via `--full` | `text --full` contains `RESULT_1_TITLE`..`RESULT_6_TITLE` |
| **Group 37: Q&A thread** | | |
| 37.1 | Accepted answer id via `eval` | Query `[data-accepted="true"]` → `a-2` |
| 37.2 | Accepted answer body | Scoped snapshot of `#a-2` contains `ANSWER_2_BODY_MARKER` |
| **Group 38: Pricing table** | | |
| 38.1 | Pro plan via scoped snapshot | `#plan-pro` contains `PLAN_PRO_PRICE_29` |
| 38.2 | All 3 plans via `--full` | Output contains all three plan-price markers |
