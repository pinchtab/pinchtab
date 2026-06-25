# sannysoft — Sannysoft Antibot Test Table

**URL**: https://bot.sannysoft.com/

A static table of automated checks (WebDriver, Permissions, WebGL, Languages,
…) — each row shows "passed" (green) or "failed" (red). One of the lowest-noise
sites in this set.

## Steps

1. **Open in a new tab** — `nav https://bot.sannysoft.com/ --new-tab --print-tab-id`.
2. **Wait for the table** — `wait --text "WebDriver"` (timeout 15 s). The table renders nearly synchronously; no JS spinning.
3. **Read** — `text` works; if rows are truncated try `text --full`.
4. **Extract** the row values below.

## Metrics

For each metric, the value cell after the row label is either "passed" or "failed" (sometimes a short reason). Capture exactly what the cell shows.

| key                  | row label                                          |
|----------------------|----------------------------------------------------|
| webdriver            | "WebDriver (New)" or "WebDriver"                   |
| webdriver_advanced   | "WebDriver Advanced"                               |
| chrome_obj           | "Chrome (New)" / window.chrome                     |
| permissions          | "Permissions"                                      |
| plugins_length       | "Plugins Length (Old)"                             |
| languages            | "Languages (Old)"                                  |
| webgl_vendor         | "WebGL Vendor"                                     |
| webgl_renderer       | "WebGL Renderer"                                   |
| broken_image         | "Broken Image Dimensions"                          |
| webgl_image_hash     | "WebGL Image Hash" row if present                  |
| iframe_chrome        | "iframe.contentWindow.chrome" or similar           |
| function_to_string   | "Function.prototype.toString" row                  |
| toSource             | "toSource" row if present                          |
| audio_codecs         | "AudioCodecs" or audio codec support row           |
| video_codecs         | "VideoCodecs" or video codec support row           |
| media_devices        | navigator.mediaDevices probe                       |
| user_agent           | UA string shown at the top of the page             |
| permissions_state    | notification permission row if present             |

## Common gotchas

- WebGL rows are the most informative for comparing chrome vs cloak — cloak should report a believable consumer GPU (NVIDIA / Intel / Apple) while headless chrome typically reports "no webgl context".
- Sannysoft's first table is the "modern detect" set; the second table is the "old detect" set. Use the modern table for the rows above.
