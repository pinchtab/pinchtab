# coveryourtracks — EFF's Cover Your Tracks

**URL**: https://coveryourtracks.eff.org/

EFF's privacy/fingerprint testing tool. Focuses on how unique and trackable
your browser is across the web.

## Steps

1. **Open in a new tab** — `nav https://coveryourtracks.eff.org/ --new-tab --print-tab-id --dismiss-banners`.
2. **Click the test button** — usually `Test your browser` or `Test with a real tracking company`. Pick the **basic** test (left button) for speed. Use `find "test your browser button"` then click the returned ref, or `click "text:Test your browser" --wait-nav`.
3. **Wait for the result** — `wait --text "fingerprint" --timeout 60000`. EFF's test takes 20-40 s.
4. **Read** — `text` first.
5. **Extract** the metrics below.

## Metrics

| key                            | what to look for                                                   |
|--------------------------------|--------------------------------------------------------------------|
| fingerprint_uniqueness_pct     | percentage / "1 in N" ratio under "Browser uniqueness"             |
| tracker_protection             | verdict for "Is your browser blocking tracking ads?"               |
| invisible_tracker_protection   | verdict for "Is your browser blocking invisible trackers?"         |
| fingerprinting_resistance      | verdict — "Strong / Partial / Some / None" protection              |
| bits_of_identifying_info       | numeric "bits of identifying information"                          |
| do_not_track                   | DNT header echoed back (yes/no/unset)                              |

## Gotchas

- EFF's site is heavy; the "results" view renders well below the fold. Scroll (`scroll 1500`) after the test completes if metrics aren't in the first `text` dump.
- The "real tracking company" button leads to a slow third-party flow — stick with the basic test unless explicitly asked otherwise.
