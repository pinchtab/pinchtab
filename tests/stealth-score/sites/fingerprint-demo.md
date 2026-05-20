# fingerprint-demo — FingerprintJS Demo

**URL**: https://fingerprint.com/demo/

The FingerprintJS commercial demo renders a stable visitorId and several
detection metrics (incognito, bot likelihood, browser/OS). Values are
"Loading metric value" placeholders until the SDK finishes.

## Steps

1. **Open in a new tab** — `nav https://fingerprint.com/demo/ --new-tab --print-tab-id --dismiss-banners`.
2. **Wait for the visitor ID to resolve** — `wait --not-text "Loading visitor ID" --timeout 30000`. The visitorId is the slowest field; once it shows, the rest are usually ready.
3. **Read** — `text` first. If most fields still say "Loading metric value", `wait 5000` and re-read.
4. **Extract** the metrics below.

## Metrics

| key                   | what to look for                                              |
|-----------------------|---------------------------------------------------------------|
| visitor_id            | hex-style hash next to "Visitor ID"                           |
| confidence            | confidence score next to visitorId (e.g. "0.97")              |
| incognito             | "yes"/"no" next to "Incognito"                                |
| bot_likelihood        | label or score ("Good bot" / "Bad bot" / "Not detected" / "Automation Detected") |
| browser_detected      | browser + version (e.g. "Chrome 130 on Linux")                |
| operating_system      | OS name + version reported                                    |
| ip                    | IP shown by the demo                                          |
| ip_location           | location reported next to IP                                  |
| vpn_detected          | yes/no — VPN detection result                                 |
| public_proxy          | yes/no — public-proxy detection                               |
| tor_detected          | yes/no — Tor detection                                        |
| jailbroken            | yes/no — jailbreak/root detection (often "No" for desktop)    |

## Common gotchas

- The demo embeds in an iframe. `text` may need a frame scope; if you can't read the value cells from the top frame, try `find "Visitor ID"` to locate the ref and read from that.
- The bot likelihood field may simply not be exposed in the free demo; record `"unavailable"` + reason in notes if so.
