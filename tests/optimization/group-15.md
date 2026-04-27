# Group 15: Complex Data Extraction & Aggregation

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

### 15.3 Extract raw HTML from a target element
Navigate to `http://fixtures/article.html` and extract the HTML of the main `article` element.

**Verify**: Extracted HTML contains `VERIFY_ARTICLE_PAGE_41414`.

### 15.4 Inspect a stable computed CSS property
Navigate to `http://fixtures/pricing.html` and inspect the computed CSS for `#plan-pro`. Report the computed `display` value.

**Verify**: The computed value is `flex`.

---

