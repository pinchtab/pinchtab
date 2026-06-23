# whoer — Whoer.net Anonymity Check

**URL**: https://whoer.net/

Anonymity score plus proxy/VPN detection, browser-fingerprint consistency, and
bot-like behaviour flags.

## Steps

1. **Open in a new tab** — `nav https://whoer.net/ --new-tab --print-tab-id --dismiss-banners`.
2. **Wait for the score** — `wait --text "%" --timeout 20000` (anonymity is shown as a percentage).
3. **Read** — `text` first.
4. **Extract** the metrics below.

## Metrics

| key                       | what to look for                                              |
|---------------------------|---------------------------------------------------------------|
| anonymity_score_pct       | numeric % next to "Your anonymity" / "Anonymity"              |
| proxy_detected            | yes/no — proxy detection verdict                              |
| vpn_detected              | yes/no — VPN detection verdict                                |
| browser_consistency       | consistent / inconsistent verdict                             |
| bot_signals               | list of bot-like behaviour flags                              |
| ip_country                | reported IP country / city                                    |
| dns                       | reported DNS provider                                         |
| timezone                  | reported timezone                                             |
| webrtc_leak               | yes/no — WebRTC IP leak                                       |

## Gotchas

- Whoer's site has heavy ads; `--dismiss-banners` helps but if a sticky banner blocks reads after settle, `scroll 800` past it.
- The anonymity score includes IP geo, timezone consistency, and JS features — interpret comparatively between providers rather than as an absolute pass/fail.
