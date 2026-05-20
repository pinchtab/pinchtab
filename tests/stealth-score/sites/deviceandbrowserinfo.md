# deviceandbrowserinfo — Are You A Bot?

**URL**: https://deviceandbrowserinfo.com/are_you_a_bot

A page-level bot-detection verdict combining UA checks, mouse/typing behaviour,
and fingerprint sanity.

## Steps

1. **Open in a new tab** — `nav https://deviceandbrowserinfo.com/are_you_a_bot --new-tab --print-tab-id --dismiss-banners`.
2. **Wait for the verdict** — `wait --text "bot" --timeout 20000`.
3. **Read** — `text` first.
4. **Extract** the metrics below.

## Metrics

| key                       | what to look for                                              |
|---------------------------|---------------------------------------------------------------|
| bot_activity_verdict      | "bot" / "human" / "suspicious" headline verdict               |
| suspicious_signals        | list of signals flagged (UA, mouse, typing, weak fingerprint) |
| user_agent_check          | UA verdict (passed/failed/normal)                             |
| mouse_behaviour           | mouse-activity check result                                   |
| weak_fingerprint_flag     | yes/no for "weak fingerprint"                                 |
| browser                   | browser identification string                                 |

## Gotchas

- The mouse/typing behaviour check expects real interaction; in a headless container these will almost always flag. Capture verbatim — that's the comparable signal.
