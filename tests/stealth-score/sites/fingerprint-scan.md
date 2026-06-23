# fingerprint-scan — Fingerprint-Scan.com

**URL**: https://fingerprint-scan.com/

A bot-risk-oriented fingerprint scan; groups several JS-side checks similar to
Pixelscan or Iphey.

## Steps

1. **Open in a new tab** — `nav https://fingerprint-scan.com/ --new-tab --print-tab-id --dismiss-banners`.
2. **Wait for the verdict** — `wait --text "risk" --timeout 30000` (or "score").
3. **Read** — `text` first.
4. **Extract** the metrics below.

## Metrics

| key                       | what to look for                                              |
|---------------------------|---------------------------------------------------------------|
| bot_risk_score            | numeric risk score or label                                   |
| fingerprint_consistency   | "consistent" / "inconsistent" verdict                         |
| automation_flags          | list of automation-related signals raised                     |
| fingerprint_uniqueness    | uniqueness ratio if shown                                     |
| ip_reputation             | clean / proxy / vpn / datacenter verdict                      |
| browser_signature         | browser identification string                                 |

## Gotchas

- Some scans require clicking a "Start" button before running. If nothing renders after the initial wait, look for a Start / Scan button via `find "start" / "scan"` and click it; then re-wait.
