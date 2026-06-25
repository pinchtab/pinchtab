# creepjs — CreepJS Browser Fingerprinting

**URL**: https://creepjs.org/

CreepJS runs ~50 fingerprint tests on load and computes a "trust score" plus
counts of inconsistent signals ("lies"). The page takes 20-40 s to finish.

## Steps

1. **Open in a new tab** — `nav https://creepjs.org/ --new-tab --print-tab-id`. Save the tab ID.
2. **Wait for the trust score to render**. The page shows a progress line like "Analyzing your browser... N/55" while running; once done, the actual numeric trust score appears. Try (in order):
   - `wait --text "trust score"` (timeout 30 s)
   - If that hits a value cell that still says "Analyzing", add `wait 5000` and retry the read.
3. **Read the rendered report**. Prefer `text` (Readability-filtered) first. If sections are missing, retry with `text --full`.
4. **Extract** the values listed below. If a metric truly isn't present, record `"unavailable"` + the reason in `notes`.

## Metrics

| key                  | what to look for                                                 |
|----------------------|------------------------------------------------------------------|
| trust_score          | numeric 0-100 next to "trust score" (may be absent on new layout)|
| lies_count           | integer next to "lies" header (often "lies (N)")                 |
| fingerprint_id       | short hex hash labelled "fingerprint" near the top               |
| bot                  | yes/no/probably from the "bot" or "bot pattern" row              |
| headless             | yes/no/probably from the "headless rating" / "headless" row      |
| system               | OS+arch short string (e.g. "Linux x86_64")                       |
| gpu                  | GPU vendor/model string from the GPU row                         |
| timezone             | reported JS timezone (e.g. "UTC", "Europe/Rome")                 |
| language             | navigator.language reported                                      |
| canvas_hash          | canvas fingerprint hash (short hex)                              |
| audio_fingerprint    | audio fingerprint hash if shown                                  |
| fonts_count          | number of fonts detected                                         |
| cpu_cores            | hardware concurrency value                                       |
| coverage             | "coverage %" value if shown — proportion of tests that ran       |
| stack_lies           | any specific stack/error-trace lies flagged                      |
| webgl_hash           | WebGL fingerprint hash if shown                                  |

## Common gotchas

- CreepJS is heavy. Don't trust the first snapshot — re-read after a 5 s wait if the trust score cell still shows "Analyzing".
- Some rows render as separate cells; you may need `text "#trust-score"` style selectors or a fresh `snap` to find the value.
