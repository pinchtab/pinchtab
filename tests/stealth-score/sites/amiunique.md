# amiunique — AmIUnique Fingerprint Report

**URL**: https://amiunique.org/fingerprint

AmIUnique computes a fingerprint and reports how unique it is across observed
browsers. The page is divided into JS attributes and HTTP-header attributes —
both must render before the uniqueness verdict appears.

## Steps

1. **Open in a new tab** — `nav https://amiunique.org/fingerprint --new-tab --print-tab-id`.
2. **Wait for the uniqueness verdict** — `wait --text "Almost"` or `wait --text "fingerprint"` (timeout 30 s).
3. **Read** — `text` first. If the JS-attributes section still says "No data available", `wait 5000` and retry; that section sometimes lags.
4. **Extract** the metrics below.

## Metrics

| key                       | what to look for                                              |
|---------------------------|---------------------------------------------------------------|
| uniqueness_verdict        | "Almost! Only N browsers..." / "Yes! You are unique" / "shared" |
| similarity_ratio          | "1 in N" ratio or percentage                                  |
| attributes_count          | number of attributes reported in the JS+HTTP tables           |
| js_attributes_count       | number of JS-collected attributes (or "unavailable")          |
| http_attributes_count     | number of HTTP-collected attributes                           |
| http_headers_distinct     | true/false — HTTP headers flagged as distinct                 |
| user_agent_uniqueness     | User-Agent attribute similarity ratio                         |
| accept_header_uniqueness  | Accept header similarity ratio                                |
| language_uniqueness       | Accept-Language similarity ratio                              |
| fonts_uniqueness          | Fonts attribute similarity ratio (JS section)                 |
| canvas_uniqueness         | Canvas attribute similarity ratio                             |
| webgl_uniqueness          | WebGL attribute similarity ratio                              |
| screen_resolution         | screen dimensions reported                                    |
| timezone                  | reported timezone                                             |

## Common gotchas

- If the JS attributes section permanently shows "No data available" — that's a fingerprintable signal in itself (means the browser blocked the data export). Record `attributes_count` as the HTTP-only count and put a note about the JS section.
