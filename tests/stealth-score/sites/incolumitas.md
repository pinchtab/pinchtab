# incolumitas — Bot Detector (Incolumitas)

**URL**: https://bot.incolumitas.com/

One of the most comprehensive public bot/anti-detect probes — behavioural
classification plus dozens of technical checks (TCP/IP, TLS/JA3, canvas, WebGL,
WebRTC, proxy/datacenter, HTTP header anomalies, timezone consistency).
Maintained by Nik (incolumitas.com); regularly updated.

## Steps

1. **Open in a new tab** — `nav https://bot.incolumitas.com/ --new-tab --print-tab-id --dismiss-banners`.
2. **Wait for the behavioural classifier to settle** — the page often shows
   `Bot likelihood: ...` once the JS finishes. Try (in order):
   - `wait --text "Bot likelihood" --timeout 30000`
   - If that times out, `wait --text "Behavioral" --timeout 15000`.
   - Always `wait 4000` after the first hit to let secondary sections render.
3. **Scroll the page** — `scroll 1500` then `scroll 1500` again so the lower
   technical tables come into the snapshot. Re-read `text` after each scroll.
4. **Read** — `text` first; fall back to `text --full` for dense tables.
5. **Extract** the metrics below.

## Metrics

| key                              | what to look for                                                   |
|----------------------------------|--------------------------------------------------------------------|
| behavioural_score                | numeric 0-1 next to "behavioralClassificationScore" or "Bot likelihood" |
| challenge_verdict                | "human" / "bot" / "unknown" final classification line              |
| tcp_ip_fingerprint               | TCP/IP fingerprint hash or OS guess (e.g. "Linux 5.x")             |
| tls_fingerprint                  | TLS / JA3 / JA4 hash if shown                                      |
| canvas_fingerprint               | canvas hash next to "Canvas" / "canvasFp"                          |
| webgl_fingerprint                | WebGL hash next to "WebGL" / "webglFp"                             |
| proxy_vpn_datacenter             | proxy/VPN/datacenter verdict (e.g. "Datacenter IP", "Residential") |
| webrtc_leaks                     | any WebRTC IP leaked vs none                                       |
| http_headers_verdict             | header sanity check verdict (Accept-Language consistency etc.)     |
| timezone_consistency             | "consistent"/"inconsistent" verdict between JS timezone and IP geo |
| navigator_anomalies              | one-line summary of navigator inconsistencies flagged              |

## Gotchas

- The page is large; an initial `text` won't cover the technical-table sections at the bottom. Always scroll twice and re-read.
- "Bot likelihood" can render as a number or as a colored label ("human" / "automated"). Capture verbatim.
- If the page shows a Cloudflare interstitial first, record `metrics: {}` + `notes: "cloudflare interstitial"` and move on.
