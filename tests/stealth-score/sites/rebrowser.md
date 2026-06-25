# rebrowser — Rebrowser Bot Detector

**URL**: https://bot-detector.rebrowser.net/

Targets modern Chromium automation leaks (Puppeteer / Playwright / chromedp
fingerprints): exposeFunctionLeak, sourceUrlLeak, mainWorldExecution, etc.
Compact result table.

## Steps

1. **Open in a new tab** — `nav https://bot-detector.rebrowser.net/ --new-tab --print-tab-id --dismiss-banners`.
2. **Wait for the test table** — `wait --text "exposeFunctionLeak"` or `wait --text "mainWorldExecution"`. Timeout 20 s.
3. **Read** — `text` first; `text --full` if rows are missing.
4. **Extract** the metrics below.

## Metrics

| key                       | row label                                  |
|---------------------------|--------------------------------------------|
| expose_function_leak      | "exposeFunctionLeak"                       |
| source_url_leak           | "sourceUrlLeak"                            |
| main_world_execution      | "mainWorldExecution"                       |
| dummy_function            | "dummyFn" or similar puppeteer probe        |
| navigator_webdriver       | navigator.webdriver value reported          |
| user_agent                | UA string reported                          |
| overall_verdict           | any headline pass/fail label                |

For each, capture either "passed"/"failed" or the literal value shown.

## Gotchas

- Page is small and fast — if `wait` returns immediately, that means the table is already there. Don't re-wait unnecessarily.
- New tests are added over time; capture any additional rows you see verbatim with kebab-case keys.
