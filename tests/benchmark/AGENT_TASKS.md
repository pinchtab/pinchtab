# PinchTab Agent Benchmark

Natural language tasks to test how well an agent can use PinchTab with only the skill documentation.

## Instructions for Agent

1. Read the PinchTab skill at `../../skills/pinchtab/SKILL.md`
2. For each task below, figure out how to accomplish it using PinchTab
3. **Log every command you execute** (curl, CLI, etc.)
4. Record results: `./record-agent-step.sh <group> <step> <pass|fail> <in> <out> "commands executed" "result notes"`
5. Use the fixtures server at `http://fixtures/` for test pages

## Environment

- **PinchTab Server**: `http://localhost:9867`
- **Token**: `benchmark-token`
- **Fixtures**: `http://fixtures/` (home, articles, search, form, dashboard, ecommerce)

---

## Group 1: Navigation

### 1.1 Open the benchmark home page
Navigate to the fixtures home page at `http://fixtures/`

**Verify**: Page title contains "Benchmark" or page loads successfully

### 1.2 Check the home page content
Get the text or content from the current page. Look for "VERIFY_HOME_LOADED_12345".

**Verify**: Found the verification string

### 1.3 Go to the articles section
Navigate to the articles page.

**Verify**: Page loaded (check title or content)

### 1.4 Read article headlines
Extract the article titles from the page.

**Verify**: Can identify at least one article title (e.g., "The Future of Artificial Intelligence")

### 1.5 Open the dashboard
Navigate to the dashboard page.

**Verify**: Dashboard loaded

### 1.6 Read dashboard metrics
Extract the key metrics from the dashboard (Total Users, Revenue, Orders).

**Verify**: Can extract at least one metric value (e.g., "24,582" users)

---

## Group 2: Search Flow

### 2.1 Go to the search page
Navigate to the search page at `http://fixtures/search.html`.

**Verify**: Search page loaded

### 2.2 Find the search input
Get a snapshot of interactive elements and identify the search input field.

**Verify**: Found a search input element

### 2.3 Search for "artificial intelligence"
Type "artificial intelligence" into the search box and submit the search.

**Verify**: Search was submitted

### 2.4 Check search results
Verify that search results appeared containing the AI article.

**Verify**: Results contain "The Future of Artificial Intelligence"

---

## Group 3: Form Interaction

### 3.1 Open the contact form
Navigate to the form page at `http://fixtures/form.html`.

**Verify**: Form page loaded

### 3.2 Fill out the form
Complete the contact form with:
- Name: "Test User"
- Email: "test@example.com"
- Country: United Kingdom
- Subject: Technical Support

**Verify**: Fields were filled

### 3.3 Submit the form
Click the submit button to send the form.

**Verify**: Form submitted successfully (look for success message)

---

## Group 4: E-commerce Flow

### 4.1 Browse the product catalog
Navigate to the e-commerce page at `http://fixtures/ecommerce.html`.

**Verify**: Product catalog loaded

### 4.2 Find product prices
Extract the price of the "Wireless Headphones" product.

**Verify**: Found price ($149.99)

### 4.3 Add item to cart
Add the Wireless Headphones to the shopping cart.

**Verify**: Item added to cart

### 4.4 Complete checkout
Click checkout to complete the purchase.

**Verify**: Checkout completed (look for success message)

---

## Group 5: Error Handling

### 5.1 Try to access a non-existent page
Navigate to a page that doesn't exist (e.g., `/nonexistent.html`).

**Verify**: Error handled gracefully (no crash, got error response or 404)

### 5.2 Try to click something that doesn't exist
Attempt to click an element with selector `#fake-button-xyz`.

**Verify**: Got an error message (not a crash)

---

## Group 6: Screenshot & Export

### 6.1 Take a screenshot
Navigate to any page and capture a screenshot.

**Verify**: Screenshot file generated or base64 returned

### 6.2 Export page as PDF
Export the current page as a PDF.

**Verify**: PDF file generated or base64 returned

---

## Summary

| Group | Tasks | Description |
|-------|-------|-------------|
| 1 | 6 | Navigation & Content Reading |
| 2 | 4 | Search Flow |
| 3 | 3 | Form Interaction |
| 4 | 4 | E-commerce Flow |
| 5 | 2 | Error Handling |
| 6 | 2 | Screenshot & Export |

**Total: 21 tasks**

## Comparison Metrics

After running both benchmarks, compare:

| Metric | Baseline | Agent Mode |
|--------|----------|------------|
| Pass Rate | | |
| Total Tokens | | |
| Cost | | |
| Avg Tokens/Step | | |
| Commands Used | (explicit) | (agent choice) |

The agent mode tests:
- Skill documentation quality
- Agent's ability to choose correct endpoints
- Token efficiency of natural language vs explicit commands
