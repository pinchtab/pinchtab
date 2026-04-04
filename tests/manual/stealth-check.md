# Manual Stealth Checks

## Purpose

This document records how to manually validate PinchTab stealth on public anti-bot and fingerprint sites.

It is not a replacement for e2e tests.

Use it to answer different questions:

- how does a third-party detection site classify the current runtime?
- which public sites are useful gates?
- which public sites are better used as debugging labs?

Current baseline observations in this document were collected on March 23, 2026 with `full` stealth in both headless and headed runs.

Service-worker stealth parity is manual-only. The automated E2E suites no longer provision a localhost secure-context fixture path for it.

## Environment Used

Baseline headless run:

- local server on `127.0.0.1:9988`
- `full` stealth enabled
- headless browser
- manual page navigation through the local API
- visible page text extracted with `/evaluate`

Headed recheck:

- local server on `127.0.0.1:9988`
- `full` stealth enabled
- headed browser
- manual page navigation through the local API
- visible page text extracted with `/evaluate`

Important headed note:

- verify `/stealth/status` actually reports `headless = false` before trusting the run

Important environment notes:

- host timezone during the sweep was `Europe/Rome`
- several sites geolocated the network to Germany / Frankfurt
- IP and DNS outputs varied across sites
- some site findings are therefore environment-sensitive

## Start The Local Server

Build:

```bash
go build -o ./pinchtab ./cmd/pinchtab
```

Run:

```bash
PINCHTAB_CONFIG=/tmp/pinchtab-live-full.json ./pinchtab server
```

Verify the runtime contract:

```bash
curl -sf -H 'Authorization: Bearer live-test-token' \
  http://127.0.0.1:9988/stealth/status
```

Expected for this manual suite:

- `level = full`
- `launchMode = allocator`
- `webdriverMode = native_baseline`

## Generic Manual Flow

1. Navigate to the public site.

```bash
curl -sf -H 'Authorization: Bearer live-test-token' \
  -H 'Content-Type: application/json' \
  http://127.0.0.1:9988/navigate \
  -d '{"url":"https://pinchtab.com"}'
```

2. Wait for the page to settle.

Typical waits:

- 5 seconds for most sites
- 6 to 7 seconds for heavier pages such as CreepJS or AmiUnique

3. Extract the visible page text.

```bash
curl -sf -H 'Authorization: Bearer live-test-token' \
  -H 'Content-Type: application/json' \
  http://127.0.0.1:9988/evaluate \
  -d '{"expression":"JSON.stringify({title:document.title,body:document.body.innerText.slice(0,6000)})"}'
```

4. If the site is a debugging lab rather than a one-page verdict, inspect targeted values with additional `evaluate` calls.

## Secure-Context Manual Checks

Service workers require a secure context. Use one of these when you want to validate service-worker stealth manually:

- `https://...` on an origin you control
- `http://localhost`
- `http://127.0.0.1`

Recommended manual fixture:

- `/Users/luigi/dev/prj/pinchtab/pinchtab/tests/e2e/fixtures/stealth-service-worker.html`

What to verify:

- the report becomes `ready`
- `webdriver` remains hidden or `false`
- service-worker `userAgent` matches the page
- service-worker `platform` matches the page
- service-worker `hardwareConcurrency` matches the page
- service-worker `deviceMemory` matches the page

## Sites We Tested

### Pixelscan

URL:

- `https://pixelscan.net/bot-check`

Wait:

- 5 seconds

What to record:

- overall `Bot Behavior Detected` or not
- top cards
- detailed rows for:
  - `TamperedFunctions`
  - `IsDevtoolOpen`
  - `NavigatorWebdriver`
  - `ChromeDriverSignature`
  - `HeadlessChrome`

Current baseline:

- still fails
- summary shows `Webdriver Detected` and `CDP Detected`
- detailed rows still show `TamperedFunctions Detected` and `IsDevtoolOpen Detected`

Headed recheck:

- no material improvement
- still showed `Bot Behavior Detected`
- detailed rows still showed `TamperedFunctions Detected` and `IsDevtoolOpen Detected`

Important note:

- use the detailed rows, not only the top summary cards

### Bot.Sannysoft

URL:

- `https://bot.sannysoft.com/`

Wait:

- 5 seconds

What to record:

- `WebDriver`
- `WebDriver Advanced`
- plugin presence
- languages
- WebGL vendor / renderer
- permission state

Current baseline:

- passes WebDriver and WebDriver Advanced
- good smoke test
- too forgiving to use as the main gate

### BrowserLeaks

Recommended URLs:

- `https://browserleaks.com/javascript`
- `https://browserleaks.com/webrtc`
- `https://browserleaks.com/canvas`
- `https://browserleaks.com/webgl`

Do not rely on:

- `https://browserleaks.com/` homepage alone

Wait:

- 5 seconds

What to record on `javascript`:

- `webdriver`
- `userAgentData`
- plugins
- network information
- timezone
- locale
- audio and speech synthesis exposure

Current baseline on `javascript`:

- no direct pass/fail verdict
- useful raw collector page
- current run showed `webdriver false`, populated `userAgentData`, PDF viewer plugins, `Europe/Rome` timezone, and a visible network info block

### Whoer

URL:

- `https://whoer.net/`

Wait:

- 6 seconds

What to record:

- `Your disguise` percentage
- proxy / anonymizer / blacklist sections
- browser and OS summary
- IP and timezone sections

Current baseline:

- `Your disguise: 100%`
- did not classify the setup as suspicious

Important note:

- useful for coarse anonymity checks
- too lenient to gate browser stealth work

### IPHey

URL:

- `https://iphey.com/`

Wait:

- 6 seconds

What to record:

- overall verdict
- location finding
- hardware finding
- software finding
- any remote / VNC / local-network anomaly callouts

Current baseline:

- `Your digital identity looks Unreliable`
- flagged:
  - `Local network anomaly detected: Virtual Network Computing (VNC)`
  - `Looks like you're trying to hide your location`
  - `It seems you are masking your fingerprint`

Important note:

- useful for realism pressure
- mixes browser stealth with host environment artifacts

### CreepJS

URL:

- `https://creepjs.org/`

Wait:

- 6 seconds

What to record:

- fingerprint ID
- coverage
- WebGL / GPU values
- platform
- language
- screen resolution
- timezone
- locale
- font and speech synthesis counts

Current baseline:

- provides rich fingerprint introspection
- no simple anti-bot pass/fail verdict

Use it for:

- explaining differences after a stealth change
- comparing collector-level outputs across runs

### AmIUnique

Use:

- `https://amiunique.org/fingerprint`

Do not use only:

- `https://amiunique.org/` homepage

Wait:

- 7 seconds

What to record:

- HTTP headers section
- JavaScript attributes section
- plugins
- permissions
- WebGL vendor / renderer
- screen values
- audio and codec sections

Current baseline:

- useful collector page
- no anti-bot verdict

### FV.pro

URL:

- `https://fv.pro/check-privacy/general`

Wait:

- 6 seconds

What to record:

- browser score
- fraud score
- summary mismatches
- platform / browser / videocard / language / resolution / timezone

Current baseline:

- `Browser 0%`
- `FP similarity 7%`
- explicitly flagged:
  - `Your screen is not real`

Headed recheck:

- `Browser 15%`
- `FP similarity 7%`
- still explicitly flagged:
  - `Your screen is not real`

### Botchecker

URL:

- `https://botchecker.net/`

Wait:

- 6 seconds

Current baseline:

- site reported that the service was temporarily unavailable

Rule:

- do not use as a gate until the site is operational again

### BrowserScan

URL:

- `https://www.browserscan.net/`

Wait:

- 6 seconds

What to record:

- `Bot Detection`
- `Browser fingerprint authenticity`
- penalty list
- IP timezone
- JavaScript timezone
- browser and browser version
- canvas / WebGL related notes

Current baseline:

- `Bot Detection: No`
- `Browser fingerprint authenticity: 67%`
- penalties:
  - `Different time zones`
  - `WebGL exception`
  - `IP addresses are different`
  - `DNS Leak`
  - `Different browser version`

Headed recheck:

- `Bot Detection: No`
- `Browser fingerprint authenticity: 67%`
- penalties:
  - `Different time zones`
  - `WebGL exception`
  - `IP addresses are different`
  - `DNS Leak`
  - `Different browser version`

Important notes:

- in the headed recheck, `Canvas Tampering` disappeared, but the main coherence issues remained
- this is one of the most useful public consistency checks in the current set

## Recommended Gate Order

Use these as the main external gates:

1. `pixelscan.net/bot-check`
2. `fv.pro/check-privacy/general`
3. `www.browserscan.net`
4. `iphey.com`
5. `bot.sannysoft.com`

Use these as debugging labs:

- `browserleaks.com/javascript`
- `browserleaks.com/webrtc`
- `browserleaks.com/canvas`
- `browserleaks.com/webgl`
- `creepjs.org`
- `amiunique.org/fingerprint`

Use these only as supporting context:

- `whoer.net`

Skip for now:

- `botchecker.net`

## What To Capture In Every Manual Run

For every site checked, store:

- date
- stealth level
- launch mode
- headless or headed
- site URL
- short verdict
- exact detection strings
- whether the issue looks like:
  - browser surface
  - CDP / devtools
  - environment / network

## Interpreting Failures

If a site flags:

- `TamperedFunctions`
  - suspect JS patching, especially canvas, graphics, or wrapped platform APIs
- `IsDevtoolOpen` or `CDP`
  - suspect active CDP / listener behavior, not only `navigator.webdriver`
- `Different time zones`
  - compare JS timezone, locale, IP geography, and persona settings
- `Different browser version`
  - verify that UA and client hints exactly match the running browser version
- `canvas not real` or `Canvas Tampering`
  - inspect graphics noise and readback hooks
- `screen not real`
  - inspect window / screen metric realism
- `VNC` or local-network anomaly
  - treat as environment-sensitive until reproduced on a clean host

## Current Baseline Summary

As of March 23, 2026:

- strong passes:
  - Sannysoft
  - Whoer
- mixed:
  - BrowserScan
- headed recheck improved BrowserScan only slightly
- strong failures:
  - Pixelscan
  - FV.pro
  - IPHey
- headed recheck did not clear Pixelscan and only marginally improved FV.pro
- labs without a direct pass/fail:
  - CreepJS
  - BrowserLeaks
  - AmIUnique
- unavailable:
  - Botchecker
