# browserscan — BrowserScan Bot Detection

**URL**: https://www.browserscan.net/bot-detection

The browserscan bot-detection page runs ~25 client-side checks and renders a
headline verdict plus per-test status. The page is JS-driven; checks run after
load and the final verdict can take 15-30 s to appear.

## Steps

1. **Open in a new tab** — `nav https://www.browserscan.net/bot-detection --new-tab --print-tab-id`.
2. **Wait for verdict to render** — try (in order):
   - `wait --text "You're not a Robot"` (or "You are a Robot") — timeout 30 s
   - If neither resolves, `wait --text "WebDriver"` and re-read.
3. **Dismiss any consent banner** — `nav <url> --new-tab --dismiss-banners` already does this on initial nav; if a banner blocks reads later, click the visible Accept button.
4. **Read** — `text` first; fall back to `text --full` if value cells are empty.
5. **Extract** the values below.

## Metrics

| key                       | what to look for                                          |
|---------------------------|-----------------------------------------------------------|
| robot_verdict             | headline verdict ("You're not a Robot" / "You're a Robot" / "Test Results: Normal") |
| webdriver                 | value cell for the WebDriver row                          |
| cdp                       | value cell for the CDP row                                |
| native_user_agent         | value cell for "Native UserAgent"                         |
| hardware_concurrency      | numeric value (e.g. "8" or "4")                           |
| timezone_consistency      | ok/warn/error label for the Timezone row                  |
| ua_browser                | browser name + version reported                           |
| ip_address                | IP shown on the page                                      |
| ip_country                | country / city reported                                   |
| dns_leak                  | yes/no — DNS-leak verdict                                 |
| webrtc_leak               | yes/no — WebRTC IP leak                                   |
| canvas_fingerprint        | canvas hash if shown                                      |
| webgl_fingerprint         | WebGL hash if shown                                       |
| client_rects              | client-rects fingerprint hash                             |
| fonts_count               | number of fonts detected                                  |
| hsts_supported            | yes/no                                                    |
| port_scan_open            | any open-port indicators flagged                          |
| ssl_fingerprint           | TLS/JA3 hash if shown                                     |

## Common gotchas

- The page sometimes renders only labels without value cells if it gives up waiting on a slow check. In that case, scroll the page (`scroll 800`) and re-read.
- IP and location values may be masked as `---.---.---.---` until checks complete — that's expected for the first few seconds.
