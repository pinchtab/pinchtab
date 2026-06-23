# fvision — F.Vision / FV.Pro Privacy Check

**URL**: https://fv.pro/check-privacy

Fingerprint / privacy summary popular in anti-detect communities. Reports
canvas, WebGL, fonts leaks plus automation indicators.

## Steps

1. **Open in a new tab** — `nav https://fv.pro/check-privacy --new-tab --print-tab-id --dismiss-banners`.
2. **Wait for the summary** — `wait --text "fingerprint" --timeout 30000` or `wait --text "privacy"`.
3. **Read** — `text` first.
4. **Extract** the metrics below.

## Metrics

| key                       | what to look for                                              |
|---------------------------|---------------------------------------------------------------|
| privacy_score             | overall score / verdict label                                 |
| canvas_leak               | canvas fingerprint hash or "leaked" label                     |
| webgl_leak                | WebGL fingerprint hash or "leaked" label                      |
| fonts_leak                | fonts list / count / leak label                               |
| automation_indicators     | list of automation signals raised                             |
| ip_country                | reported IP country                                           |
| timezone                  | reported timezone                                             |

## Gotchas

- If fv.pro redirects to a different domain (anti-detect vendors often rebrand), follow the redirect; record the actual URL the agent observed.
- This site is community-maintained — some metrics may be missing on a given visit. Record `"unavailable"` and note which fields were absent.
