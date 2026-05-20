# iphey — IP/Browser Reliability (iphey.com)

**URL**: https://iphey.com/

Reports a "trustworthy / suspicious" reliability verdict, digital-identity
consistency, automation signs, and proxy/leak flags. Compact summary page.

## Steps

1. **Open in a new tab** — `nav https://iphey.com/ --new-tab --print-tab-id --dismiss-banners`.
2. **Wait for the reliability badge** — `wait --text "Trustworthy"` (or "Suspicious"). Timeout 20 s.
3. **Read** — `text` first.
4. **Extract** the metrics below.

## Metrics

| key                            | what to look for                                                   |
|--------------------------------|--------------------------------------------------------------------|
| reliability_verdict            | "Trustworthy" / "Suspicious" / "Unreliable" badge                  |
| digital_identity_consistency   | yes/no — does locale/timezone/IP geo align                         |
| automation_signs               | list of automation-related flags raised                            |
| proxy_flag                     | proxy detected: yes/no                                             |
| vpn_flag                       | VPN detected: yes/no                                               |
| datacenter_flag                | datacenter IP detected: yes/no                                     |
| webrtc_leak                    | yes/no — any WebRTC IP leaked                                      |
| dns_leak                       | yes/no — DNS-leak verdict                                          |
| fingerprint_uniqueness         | uniqueness score or ratio if shown                                 |
| timezone                       | reported timezone (string)                                         |

## Gotchas

- The reliability badge color matters (green=trustworthy, red=suspicious); capture the literal word.
- If results take >30 s to settle, record what you have and put `notes: "page slow to settle"`.
