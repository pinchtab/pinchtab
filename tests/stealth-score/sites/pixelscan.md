# pixelscan — Pixelscan Bot Check

**URL**: https://pixelscan.net/bot-check

Pixelscan exposes a "Start Check" button — the verdict only renders after you
click it. The page text statically contains both verdict labels (`You're
Definitely a Human` and `Bot Behavior Detected`); ignore them until you've
clicked Start and the page updates.

## Steps

1. **Open in a new tab** — `nav https://pixelscan.net/bot-check --new-tab --print-tab-id`.
2. **Wait for the Start Check button** — `wait --text "Start Check"` (timeout 15 s).
3. **Click Start Check** — `find "Start Check button"` then click the returned ref, or `click "text:Start Check"`. Use `--snap-diff` so you see the page transition.
4. **Wait for the result** — pixelscan renders the verdict inline. The result is ready when one of the per-test rows (`Webdriver`, `Browser`, `Canvas`) shows a non-pending value. `wait --text "Webdriver" --timeout 30000`.
5. **Read** — `text` first.

## Metrics

| key                       | what to look for                                              |
|---------------------------|---------------------------------------------------------------|
| bot_score                 | headline ("You're Definitely a Human" / "Bot Behavior Detected" / numeric score) |
| consistent                | yes/no consistency verdict (often "Consistent" / "Inconsistent") |
| chrome_signature          | row value for "ChromeDriverSignature" — Clear/Detected        |
| automation_flags          | comma-list of rows showing "Detected"                         |
| mismatch_summary          | one-line summary of any timezone/locale/IP mismatch lines     |
| ip_address                | IP shown on the page                                          |
| timezone                  | reported JS timezone                                          |
| timezone_offset           | reported timezone offset                                      |
| language                  | reported navigator.language                                   |
| canvas_hash               | canvas fingerprint hash                                       |
| webgl_hash                | WebGL fingerprint hash                                        |
| user_agent                | UA string                                                     |
| screen_resolution         | reported screen dimensions                                    |
| webdriver_value           | navigator.webdriver value (true/false/undefined)              |
| tampered_functions        | "TamperedFunctions" row — Clear / Detected                    |
| unusual_window_properties | "UnusualWindowProperties" row — Clear / Detected              |

## Common gotchas

- The page text contains BOTH verdict strings statically — don't extract them blindly. Only trust the headline after the post-click result has rendered.
- If the click fails (overlay/banner), reload (`reload`) and try again. Pixelscan sometimes shows a Cloudflare interstitial first; record `metrics: {}` + `notes: "cloudflare interstitial"` and move on if it persists.
