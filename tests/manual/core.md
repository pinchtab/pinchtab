# Pinchtab Test Plan

**Goal:** Establish a repeatable test suite to validate Pinchtab for stable release.

**Scope:** Functional correctness, regression, edge cases. Covers headless and headed modes, single and multi-agent scenarios.

**How to run:** Each scenario can be executed manually via `curl` or scripted. Integration tests requiring Chrome should use build tag `integration`.

---

## 1. Core Endpoints — Headless Single Agent

### 1.1 Health & Startup

| # | Scenario | Steps | Expected |
|---|----------|-------|----------|
| H1 | Health check | `GET /health` | 200, `{"status":"ok"}` | ✅ Unit test (error path) |
| H2 | Startup headless | `BRIDGE_HEADLESS=true ./pinchtab` | Launches, binds port, Chrome not visible |
| H3 | Startup headed | `./pinchtab` | Chrome window opens, visible |
| H4 | Custom port | `BRIDGE_PORT=9999 ./pinchtab` | Binds to 9999 |
| H5 | Auth token required | `BRIDGE_TOKEN=secret ./pinchtab`, then `GET /health` without token | 401 |
| H6 | Auth token accepted | `GET /health` with `Authorization: Bearer secret` | 200 |
| H7 | Graceful shutdown | Send SIGINT | Chrome closes, port released, exit 0 |

### 1.2 Navigation

| # | Scenario | Steps | Expected |
|---|----------|-------|----------|
| N1 | Basic navigate | `POST /navigate {"url":"https://example.com"}` | 200, title="Example Domain", url matches |
| N2 | Navigate returns title | Navigate to `https://www.bbc.co.uk` | Title = "BBC - Home" (or similar) |
| N3 | Navigate SPA (slow title) | Navigate to `https://x.com` | Title may be empty (known limitation for heavy SPAs) |
| N4 | Navigate with newTab | `POST /navigate {"url":"...","newTab":true}` | New tab created, new tabId returned, tab count +1 |
| N5 | Navigate invalid URL | `POST /navigate {"url":"not-a-url"}` | Error response, no crash |
| N6 | Navigate missing URL | `POST /navigate {}` | 400, clear error message |
| N7 | Navigate bad JSON | `POST /navigate {broken` | 400, JSON parse error |
| N8 | Navigate timeout | Navigate to extremely slow/hanging URL | Returns within timeout, error or partial |

### 1.3 Snapshot (Accessibility Tree)

| # | Scenario | Steps | Expected |
|---|----------|-------|----------|
| S1 | Basic snapshot | Navigate to example.com, `GET /snapshot` | Valid JSON, nodes array, refs (e0, e1...) |
| S2 | Interactive filter | `GET /snapshot?filter=interactive` | Only buttons, links, inputs returned |
| S3 | Depth filter | `GET /snapshot?depth=2` | Tree truncated at depth 2 |
| S4 | Text format | `GET /snapshot?format=text` | Indented plain text, not JSON |
| S5 | YAML format | `GET /snapshot?format=yaml` | Valid YAML output |
| S6 | Diff mode | Snapshot twice with `?diff=true` on second | Second returns only changes |
| S7 | Diff first call | `GET /snapshot?diff=true` (no prior snapshot) | Full snapshot (no previous to diff against) |
| S8 | File output | `GET /snapshot?output=file&path=/tmp/test.json` | File written, response confirms path |
| S9 | Snapshot with tabId | `GET /snapshot?tabId=<specific>` | Returns snapshot for that specific tab |
| S10 | Snapshot no tab | Close all tabs, `GET /snapshot` | Error: no tab available |
| S11 | Large page | Navigate to Wikipedia article, snapshot | Handles 20K+ token pages without error |
| S12 | Ref stability | Snapshot → click → snapshot | Refs for unchanged elements stay the same |

### 1.4 Text Extraction

| # | Scenario | Steps | Expected |
|---|----------|-------|----------|
| T1 | Readability mode | Navigate to BBC article, `GET /text` | Clean article text, no nav/ads |
| T2 | Raw mode | `GET /text?mode=raw` | Full innerText including nav/footer |
| T3 | Text with tabId | `GET /text?tabId=<specific>` | Text from correct tab |
| T4 | Text no tab | `GET /text` with no tabs | Error response |
| T5 | Token efficiency | `/text` on Google | ~150 tokens (not 700+, language blob removed) |

### 1.5 Actions

| # | Scenario | Steps | Expected |
|---|----------|-------|----------|
| A1 | Click by ref | Snapshot, find button ref, `POST /action {"kind":"click","ref":"eN"}` | Click executed, page state changes |
| A2 | Type by ref | Find input ref, `POST /action {"kind":"type","ref":"eN","text":"hello"}` | Text entered in input |
| A3 | Fill by ref | `POST /action {"kind":"fill","ref":"eN","text":"hello"}` | Input value set (no key events) |
| A4 | Press key | `POST /action {"kind":"press","key":"Enter"}` | Key pressed |
| A5 | Focus | `POST /action {"kind":"focus","ref":"eN"}` | Element focused |
| A6 | Hover | `POST /action {"kind":"hover","ref":"eN"}` | Hover state triggered |
| A7 | Select option | Find select ref, `POST /action {"kind":"select","ref":"eN","value":"opt1"}` | Option selected |
| A8 | Scroll | `POST /action {"kind":"scroll","direction":"down"}` | Page scrolls |
| A9 | Unknown kind | `POST /action {"kind":"dance"}` | 400, lists valid kinds |
| A10 | Missing kind | `POST /action {"ref":"e0"}` | 400, clear error about missing kind |
| A11 | Ref not found | `POST /action {"kind":"click","ref":"e9999"}` | Error: ref not found |
| A12 | CSS selector | `POST /action {"kind":"click","selector":"#submit"}` | Click by CSS selector |
| A13 | Action no tab | Close tabs, `POST /action {"kind":"click","ref":"e0"}` | Error: no tab |
| A14 | Batch actions | `POST /actions [{"kind":"click","ref":"e1"},{"kind":"type","ref":"e2","text":"hi"}]` | Both executed in order |
| A15 | Batch empty | `POST /actions []` | Error: empty batch |
| A16 | Human click | `POST /action {"kind":"humanClick","ref":"eN"}` | Bezier mouse movement + click |
| A17 | Human type | `POST /action {"kind":"humanType","ref":"eN","text":"hello"}` | Natural typing with variable delays |

### 1.6 Tabs

| # | Scenario | Steps | Expected |
|---|----------|-------|----------|
| TB1 | List tabs | `GET /tabs` | Array of tab objects with id, url, title |
| TB2 | New tab | `POST /tab {"action":"new","url":"https://example.com"}` | New tab opened, id returned |
| TB3 | Close tab | `POST /tab {"action":"close","tabId":"<id>"}` | Tab closed, tab count -1 |
| TB4 | Close without tabId | `POST /tab {"action":"close"}` | 400, tabId required |
| TB5 | Bad action | `POST /tab {"action":"explode"}` | 400, invalid action |
| TB6 | Max tabs | Open 3+ tabs (default limit) | Error or blocks at limit |

### 1.7 Screenshots

| # | Scenario | Steps | Expected |
|---|----------|-------|----------|
| SS1 | Basic screenshot | `GET /screenshot` | JPEG image data |
| SS2 | Raw screenshot | `GET /screenshot?raw=true` | Raw JPEG bytes (no base64) |
| SS3 | Screenshot no tab | `GET /screenshot` with no tabs | Error |

### 1.8 JavaScript Evaluation

| # | Scenario | Steps | Expected |
|---|----------|-------|----------|
| E1 | Simple eval | `POST /evaluate {"expression":"1+1"}` | `{"result":"2"}` |
| E2 | DOM eval | `POST /evaluate {"expression":"document.title"}` | Returns page title |
| E3 | Missing expression | `POST /evaluate {}` | 400, error message |
| E4 | Bad JSON | `POST /evaluate {broken` | 400, parse error |
| E5 | Eval no tab | `POST /evaluate {"expression":"1"}` with no tabs | Error |

### 1.9 Cookies

| # | Scenario | Steps | Expected |
|---|----------|-------|----------|
| C1 | Get cookies | Navigate to site, `GET /cookies` | Returns cookie array |
| C2 | Set cookies | `POST /cookies {"url":"...","cookies":[...]}` | Cookies set |
| C3 | Get cookies no tab | `GET /cookies` with no tabs | Error |
| C4 | Set cookies bad JSON | `POST /cookies {broken` | 400 |
| C5 | Set cookies empty | `POST /cookies {"url":"...","cookies":[]}` | Error or no-op |

### 1.10 File Upload

**Test assets:** `tests/assets/upload-test.html` (HTML page with file inputs), `tests/assets/test-upload.png` (1x1 PNG).
Navigate to the test page first: `POST /navigate {"url":"file://<repo>/tests/assets/upload-test.html"}`

| # | Scenario | Steps | Expected |
|---|----------|-------|----------|
| UP1 | Upload local file | `POST /upload {"selector":"#single","paths":["tests/assets/test-upload.png"]}` | `{"status":"ok","files":1}` |
| UP2 | Upload base64 data URL | `POST /upload {"selector":"#single","files":["data:image/png;base64,..."]}` | `{"status":"ok","files":1}` |
| UP3 | Upload raw base64 | `POST /upload {"selector":"#single","files":["iVBOR..."]}` | `{"status":"ok","files":1}` |
| UP4 | Upload multiple files | `POST /upload {"selector":"#multi","paths":["tests/assets/test-upload.png","tests/assets/test-upload.png"]}` | `{"status":"ok","files":2}` |
| UP5 | Combined paths + base64 | `POST /upload {"selector":"#multi","paths":["tests/assets/test-upload.png"],"files":["data:image/png;base64,..."]}` | `{"status":"ok","files":2}` |
| UP6 | Default selector | `POST /upload {"paths":["tests/assets/test-upload.png"]}` (no selector) | Uses `input[type=file]`, succeeds |
| UP7 | Invalid selector | `POST /upload {"selector":"#nonexistent","paths":["tests/assets/test-upload.png"]}` | 500, selector error |
| UP8 | Missing files and paths | `POST /upload {"selector":"input[type=file]"}` | 400, error |
| UP9 | File not found | `POST /upload {"paths":["/tmp/nonexistent.jpg"]}` | 400, file not found |
| UP10 | Invalid base64 | `POST /upload {"files":["not-valid!!!"]}` | 400, decode error |
| UP11 | Bad JSON body | `POST /upload {broken` | 400, parse error |
| UP12 | No tab | `POST /upload {"paths":["tests/assets/test-upload.png"]}` with no tabs | Error |

### 1.11 PDF Export

| # | Scenario | Steps | Expected |
|---|----------|-------|----------|
| PD1 | PDF base64 | Navigate to page, capture `TAB_ID`, `GET /tabs/TAB_ID/pdf` | JSON with `format: "pdf"` and `base64` field |
| PD2 | PDF raw bytes | `GET /tabs/TAB_ID/pdf?raw=true` | Content-Type `application/pdf`, valid PDF bytes |
| PD3 | PDF save to file | `GET /tabs/TAB_ID/pdf?output=file` | JSON with `path` and `size` |
| PD4 | PDF custom path | `GET /tabs/TAB_ID/pdf?output=file&path=/tmp/test.pdf` | File written to `/tmp/test.pdf` |
| PD5 | PDF landscape | `GET /tabs/TAB_ID/pdf?landscape=true&raw=true` | Valid PDF in landscape |
| PD6 | PDF scale | `GET /tabs/TAB_ID/pdf?scale=0.5&raw=true` | Valid PDF with scaled content |
| PD7 | PDF no tab | `GET /tabs/nonexistent/pdf` | 404 error |
| PD8 | PDF specific tab | `GET /tabs/TAB_ID/pdf` | PDF of specified tab |

### 1.12 Stealth

| # | Scenario | Steps | Expected |
|---|----------|-------|----------|
| ST1 | Stealth status | `GET /stealth/status` | JSON with feature booleans and score |
| ST2 | Webdriver hidden | Navigate, eval `navigator.webdriver` | `undefined` |
| ST3 | Chrome runtime present | Eval `!!window.chrome.runtime` | `true` |
| ST4 | Plugins present | Eval `navigator.plugins.length` | > 0 |
| ST5 | Fingerprint rotate | `POST /fingerprint/rotate {"os":"windows"}` | New fingerprint applied |
| ST6 | Fingerprint rotate random | `POST /fingerprint/rotate {}` | Random OS selected |
| ST7 | Fingerprint no tab | `POST /fingerprint/rotate {}` with no tabs | Error |
| ST8 | Bot detection site | Navigate to `bot.sannysoft.com` | Most checks pass (green) |

### 1.13 Configuration

| # | Scenario | Steps | Expected |
|---|----------|-------|----------|
| CF1 | Config file | Create `~/.pinchtab/config.json` with port override, start | Uses config port | ✅ Unit test |
| CF2 | Env overrides config | Set `BRIDGE_PORT` env + config file port | Env wins | ✅ Unit test |
| CF3 | CDP_URL external Chrome | `CDP_URL=ws://... ./pinchtab` | Connects to existing Chrome, no launch |
| CF4 | Custom profile dir | `BRIDGE_PROFILE=/tmp/test-profile ./pinchtab` | Uses specified profile |
| CF5 | No restore | `BRIDGE_NO_RESTORE=true ./pinchtab` | Doesn't restore previous tabs |

### 1.16 Security & Validation

| # | Scenario | Steps | Expected |
|---|----------|-------|----------|
| SEC1 | Profile name with ".." | `POST /profiles {"name":"../../../etc/passwd"}` | 400, invalid profile name |
| SEC2 | Profile name with "/" | `POST /profiles {"name":"test/profile"}` | 400, path separators not allowed |
| SEC3 | Profile name with "\" | `POST /profiles {"name":"test\\profile"}` | 400, path separators not allowed |
| SEC4 | Profile name empty | `POST /profiles {"name":""}` | 400, name required |
| SEC5 | Valid profile names | `POST /profiles {"name":"valid-profile"}` | 201, profile created |
| SEC6 | SSRF prevention | Proxy to non-localhost (e.g., example.com via /tabs/{tabId}/navigate) | 400, localhost required or fails safely |
| SEC7 | Proxy safe URL | Proxy normal navigate via /tabs/{tabId}/navigate | 200, works as expected |

### 1.14 CLI Subcommands

| # | Scenario | Steps | Expected |
|---|----------|-------|----------|
| CL1 | Navigate | `pinchtab nav https://example.com` | JSON with title and url |
| CL2 | Snapshot interactive | `pinchtab snap -i -c` | Compact output, interactive elements only |
| CL3 | Snapshot diff | `pinchtab snap -d` | Only changes since last snapshot |
| CL4 | Click | `pinchtab click e5` | Action success |
| CL5 | Type | `pinchtab type e12 hello world` | Text typed into element |
| CL6 | Press | `pinchtab press Enter` | Key pressed |
| CL7 | Text extract | `pinchtab text` | JSON with url, title, text |
| CL8 | Text raw | `pinchtab text --raw` | Raw innerText |
| CL9 | Screenshot | `pinchtab ss -o /tmp/test.jpg` | File saved, size reported |
| CL10 | Evaluate | `pinchtab eval "document.title"` | Returns page title |
| CL11 | PDF | `pinchtab pdf --tab <TAB_ID> -o /tmp/test.pdf` | PDF file saved |
| CL12 | Tabs list | `pinchtab tabs` | JSON array of tabs |
| CL13 | Tab new | `pinchtab tabs new https://example.com` | New tab opened |
| CL14 | Health | `pinchtab health` | JSON with status ok |
| CL15 | Help | `pinchtab help` | Help text with all commands |
| CL16 | Custom URL | `PINCHTAB_URL=http://localhost:9877 pinchtab health` | Connects to custom URL |
| CL17 | Auth token | `PINCHTAB_TOKEN=secret pinchtab health` (server has BRIDGE_TOKEN=secret) | Auth succeeds |
| CL18 | Server down | `pinchtab health` (no server running) | Error: connection failed |

### 1.15 Session Persistence

| # | Scenario | Steps | Expected |
|---|----------|-------|----------|
| SP1 | Tab restore | Open tabs, stop, restart | Previous tabs restored |
| SP2 | Cookie persistence | Log into site, restart | Still logged in |
| SP3 | No restore flag | Open tabs, restart with `BRIDGE_NO_RESTORE=true` | Clean start, no old tabs |

---

## 2. Headed Mode — Additional Scenarios

| # | Scenario | Steps | Expected |
|---|----------|-------|----------|
| HM1 | Chrome visible | Start without `BRIDGE_HEADLESS` | Chrome window visible, interactable |
| HM2 | Manual + API coexist | Manually browse in Chrome + API calls | Both work, no conflicts |
| HM3 | Manual login persists | Log into GitHub manually in Chrome window | `/cookies` shows session, persists on restart |

---

## 3. Multi-Agent Scenarios

Multiple agents (or concurrent scripts) hitting the same Pinchtab instance.

| # | Scenario | Steps | Expected |
|---|----------|-------|----------|
| MA1 | Concurrent reads | Two agents snapshot different tabs by tabId simultaneously | Both get correct results |
| MA2 | Concurrent navigates | Agent A navigates tab1, Agent B navigates tab2 (by tabId) | Both succeed, no cross-talk |
| MA3 | Tab isolation | Agent A works on tab1, Agent B on tab2 | Actions don't leak across tabs |
| MA4 | Concurrent actions | Agent A clicks on tab1, Agent B types on tab2 | Both succeed |
| MA5 | Ref collision | Both agents snapshot same tab, get same refs | Refs consistent, actions work for both |
| MA6 | Rapid fire | 10 concurrent `/snapshot` requests | All return valid data, no 500s |
| MA7 | Tab limit | Two agents both try to open tabs up to limit | One gets error, no crash |
| MA8 | No tabId default | Agent A navigates, Agent B calls `/snapshot` (no tabId) | Returns *some* tab (undefined which — this is the active tab tracking issue) |

---

## 4. Stealth Integration Tests (require Chrome)

Run with: `go test -tags integration -v`

### Automated (in `integration_test.go`) ✅
| # | Test | Status |
|---|------|--------|
| SI1 | TestStealthScriptInjected — `navigator.webdriver === undefined` | ✅ Pass |
| SI2 | TestCanvasNoiseApplied — `toDataURL` differs per call | ✅ Pass |
| SI3 | TestFontMetricsNoise — Proxy-wrapped TextMetrics, positive widths | ✅ Pass |
| SI4 | TestWebGLVendorSpoofed — `UNMASKED_VENDOR_WEBGL = "Intel Inc."` | ⏭️ Skip (headless, no GPU) |
| SI5 | TestPluginsPresent — `navigator.plugins.length >= 3` | ✅ Pass |
| SI6 | TestFingerprintRotation — CDP UA override, Edge UA after rotate | ✅ Pass |
| SI7 | TestCDPTimezoneOverride — `Intl.DateTimeFormat` returns spoofed TZ | ✅ Pass |
| SI8 | TestStealthStatusEndpoint — score >= 50, level high/medium | ✅ Pass |

### Manual (quarterly against detection sites)
| # | Scenario | Steps | Expected |
|---|----------|-------|----------|
| SI9 | bot.sannysoft.com | Navigate, screenshot | Most items green |
| SI10 | creepjs | Navigate to `abrahamjuliot.github.io/creepjs/` | Trust score reasonable |
| SI11 | browserleaks | Navigate to `browserleaks.com/javascript` | No automation flags |

---

## 5. Docker Scenarios

| # | Scenario | Steps | Expected |
|---|----------|-------|----------|
| D1 | Docker build | `docker build -t pinchtab .` | Builds successfully |
| D2 | Docker run | `docker run -d -p 9867:9867 pinchtab` | Container starts, health returns 200 |
| D3 | Docker navigate | Navigate to example.com from host | Works, returns title |
| D4 | Docker snapshot | `GET /snapshot` from host | Valid JSON |
| D5 | Docker with token | `docker run -e BRIDGE_TOKEN=secret ...` | Auth enforced |
| D6 | Docker CHROME_BINARY | Verify Chromium binary path consumed | Uses container Chromium |
| D7 | Docker CHROME_FLAGS | Set custom flags via env | Flags applied |

---

## 6. Configuration — Extended

| # | Scenario | Steps | Expected |
|---|----------|-------|----------|
| CF6 | Chrome version override | `BRIDGE_CHROME_VERSION=134.0.0.0 ./pinchtab` | UA string uses 134 |
| CF7 | Chrome version default | Start without `BRIDGE_CHROME_VERSION` | UA uses 133.0.6943.98 |
| CF8 | Chrome version in fingerprint | Set version, then `POST /fingerprint/rotate` | Rotated fingerprint uses same Chrome version |

---

## 7. Error Handling & Edge Cases

| # | Scenario | Steps | Expected |
|---|----------|-------|----------|
| ER1 | Chrome crash recovery | Kill Chrome process while Pinchtab running | Pinchtab detects, returns errors (not hang) |
| ER2 | Large page snapshot | Navigate to page with 1000+ a11y nodes | Returns full snapshot, no timeout |
| ER3 | Binary page | Navigate to PDF/image URL | Handles gracefully (error or empty snapshot) |
| ER4 | Rapid navigate | 5 navigates in 1 second | No crash, last one wins |
| ER5 | Unicode content | Navigate to page with CJK/emoji/RTL | Text and snapshot handle correctly |
| ER6 | Empty page | Navigate to `about:blank` | Snapshot returns minimal tree |
| ER7 | Connection refused (CDP_URL) | `CDP_URL=ws://localhost:1111` | Clear error message, exits |
| ER8 | Port in use | Start two instances on same port | Second fails with clear error |

---

## 8. Known Issues (from QA rounds)

Track these separately — they are known bugs, not test failures.

| # | Issue | Severity | Status | Notes |
|---|-------|----------|--------|-------|
| K1 | Active tab tracking unreliable after navigate | 🔴 P0 | ✅ FIXED | Confirmed working in autorun hour 00. |
| K2 | Tab close hangs | 🟡 P1 | ✅ FIXED | Hour 07: switched to `target.CloseTarget` (browser-level CDP). No more hangs. |
| K3 | x.com title always empty | 🟢 P2 | 🔧 IMPROVED | Hour 03: added `waitTitle` param to navigate. Agents can wait for SPA titles. |
| K4 | Chrome flag warning banner | 🟢 P2 | ✅ FIXED | Removed deprecated flag (hour 05); CDP stealth handles it. |
| K5 | Stealth PRNG weak (8F-2) | 🟡 P1 | ✅ FIXED | Now uses Mulberry32 with Go-injected seed. |
| K6 | Chrome UA hardcoded to 131 (8F-6) | 🟡 P1 | ✅ FIXED | Configurable via `BRIDGE_CHROME_VERSION`, default 133. |
| K7 | Fingerprint rotation JS-only (8F-7) | 🟢 P2 | ✅ FIXED | Now uses CDP `Emulation.SetUserAgentOverride` for UA/platform/language. |
| K8 | Timezone hardcoded EST (8F-9) | 🟢 P2 | ✅ FIXED | Now uses CDP `Emulation.SetTimezoneOverride` via `BRIDGE_TIMEZONE` env var. |
| K9 | Stealth status hardcoded (8F-10) | 🟢 P2 | ✅ FIXED | Now probes browser when tab available. |

---

## 9. Release Criteria for Stable v1.0

### Must Pass (P0)
- All Section 1 scenarios (core endpoints) pass in headless
- All Section 1 scenarios pass in headed
- K1 (active tab tracking) fixed OR documented as "always use tabId"
- K2 (tab close hangs) fixed
- Zero crashes across full test suite
- `go test ./...` 100% pass (currently: ✅ 54 unit tests)
- `go test -tags integration` pass (currently: ✅ 6 pass, 1 skip headless, 1 skip headed-only)

### Should Pass (P1)
- Section 3 multi-agent scenarios MA1-MA5 pass
- Stealth passes `bot.sannysoft.com` basic checks
- Session persistence works (SP1, SP2)

### Nice to Have (P2)
- Coverage > 30%
- K3-K4 addressed or documented (K5-K9 all fixed)
- Performance benchmarks baselined
