# Stealth-score Site Index

Per-site playbooks live alongside this file. Each describes WHAT to capture;
HOW to drive PinchTab is in `tests/stealth-score/subagent-context.md`.

Process the sites in this order. Fast / static sites come first so they always
get captured even if a long run runs into time limits; heavier JS-driven sites
and multi-page navs are at the bottom.

1. [sannysoft](sannysoft.md) — static table, lowest noise. WebGL/permissions/UA rows.
2. [rebrowser](rebrowser.md) — modern Chromium leak tests (exposeFunctionLeak etc).
3. [deviceandbrowserinfo](deviceandbrowserinfo.md) — bot/human verdict + suspicious signals.
4. [iphey](iphey.md) — reliability badge, proxy/VPN/DNS leak flags.
5. [whoer](whoer.md) — anonymity %, proxy/VPN detection, browser consistency.
6. [browserscan](browserscan.md) — bot-detection per-test rows + IP / WebRTC / canvas.
7. [pixelscan](pixelscan.md) — requires a Start Check click; bot score + automation flags.
8. [fingerprint-scan](fingerprint-scan.md) — bot risk score, automation flags.
9. [incolumitas](incolumitas.md) — most comprehensive: behavioural + TLS/JA3 + WebRTC + proxy + timezone.
10. [fvision](fvision.md) — fv.pro privacy/leak summary (canvas/WebGL/fonts).
11. [amiunique](amiunique.md) — per-attribute uniqueness ratios.
12. [browserleaks](browserleaks.md) — multi-nav: /canvas + /webgl + /fonts + /tls.
13. [creepjs](creepjs.md) — heavy JS suite; trust score + fingerprint hashes.
14. [coveryourtracks](coveryourtracks.md) — EFF tracking-resistance test; needs a button click.
15. [fingerprint-demo](fingerprint-demo.md) — FingerprintJS commercial demo; SDK can be slow/refuse headless.

Add sites by creating a new `<id>.md` playbook here and listing it above. The
agent processes whatever it finds in this index in order.
