# Pinchtab — TODO

**Philosophy**: 12MB binary. HTTP API. Minimal deps (chromedp + stdlib). Internal tool, not a product.
If a feature needs a GUI, complex config, or "target users" — it's probably wrong.

---

## Completed

### P0–P5: Core (38 tests, 0 lint issues)
Safety, file split, Go idioms, testability, session persistence, stealth basics,
ref caching, action registry, hover/select/scroll actions, smart diff, text format,
readability `/text`, BridgeAPI interface, handler tests, nil guard, deprecated flag removal.
Navigate timeout fix (readyState polling). Restore concurrency limiting (max 3 tabs, 2 navigations).

### P6: Productivity
Action chaining (`POST /actions`), `/cookies` endpoint, LaunchAgent/systemd auto-start,
config file (`~/.pinchtab/config.json`).

### P7: Output Formats
File-based output (`?output=file`), YAML format (`?format=yaml`), Dockerfile (Alpine + Chromium).

### P8: Stealth & Human Interaction (partial — see P8-FIX below)
Human mouse movement (bezier curves in `human.go`), human typing (variable delays, typo simulation),
`GET /stealth/status`, `POST /fingerprint/rotate`, `stealth.js` injected via `AddScriptToEvaluateOnNewDocument`.
Covers: navigator overrides, WebGL vendor spoof, plugin emulation, canvas noise, font metrics noise,
WebRTC blocking, timezone/hardware spoofing, Chrome flag hardening (20+ flags).

---

## P8-FIX: Stealth Correctness (senior review, 2026-02-16)

P8 shipped endpoints that work but has bugs that make stealth detectable.
These must be fixed before P8 can be called complete.

### 8F-1: stealth.js — duplicate property definitions [HIGH]
`hardwareConcurrency` defined at line ~66 (static 8) and line ~170 (random 4-8).
Same for `deviceMemory`. Second `Object.defineProperty` on non-configurable property
is undefined behavior.

**Fix:** Remove the static definitions (lines 62-66). Keep only the randomized
versions. Consolidate all navigator overrides into one block at the top of the file.

### 8F-2: stealth.js — non-deterministic hardware values per page load [HIGH]
Every navigation produces different `hardwareConcurrency`, `deviceMemory` values
via `Math.random()`. Real browsers have consistent hardware within a session.
Detection systems check for this inconsistency.

**Fix:** Seed a deterministic PRNG at the top of stealth.js using a session-stable value.
Use the seeded PRNG for hardware/identity values. Keep `Math.random()` only for canvas noise
(which should vary per call).

```javascript
const _seed = (function() {
  let s = 12345; // Injected from Go config per session
  return function() {
    s = (s * 1103515245 + 12345) & 0x7fffffff;
    return s / 0x7fffffff;
  };
})();
```

### 8F-3: stealth.js — measureText returns plain object [HIGH]
`{ ...metrics, width: metrics.width * (1 + noise) }` loses the `TextMetrics` prototype.
Detection via `instanceof TextMetrics` fails.

**Fix:** Use a Proxy to intercept only the `width` getter:
```javascript
return new Proxy(metrics, {
  get(target, prop) {
    if (prop === 'width') return target.width * (1 + noise);
    return target[prop];
  }
});
```

### 8F-4: stealth.js — toDataURL mutates source canvas [HIGH]
Current implementation calls `getImageData`, adds noise, `putImageData` back to the
original canvas before exporting. This permanently corrupts the canvas. Noise compounds
on repeated exports.

**Fix:** Create an offscreen canvas, copy, noise, export from the copy:
```javascript
HTMLCanvasElement.prototype.toDataURL = function(...args) {
  const offscreen = document.createElement('canvas');
  offscreen.width = this.width;
  offscreen.height = this.height;
  const offCtx = offscreen.getContext('2d');
  offCtx.drawImage(this, 0, 0);
  const imageData = offCtx.getImageData(0, 0, this.width, this.height);
  for (let i = 0; i < imageData.data.length; i += 4) {
    imageData.data[i] += (_seed() - 0.5) * 0.5;
    imageData.data[i+1] += (_seed() - 0.5) * 0.5;
    imageData.data[i+2] += (_seed() - 0.5) * 0.5;
  }
  offCtx.putImageData(imageData, 0, 0);
  return originalToDataURL.apply(offscreen, args);
};
```

### 8F-5: stealth.js — toBlob doesn't actually noise independently [MEDIUM]
`toBlob` calls `this.toDataURL()` (which mutates canvas due to 8F-4) then calls
`originalToBlob` on the mutated canvas. Accidentally works. Breaks if 8F-4 is fixed.

**Fix:** Apply the same offscreen canvas pattern from 8F-4 to `toBlob`.

### 8F-6: Chrome UA version mismatch and outdated [HIGH]
`main.go` sets `Chrome/131.0.0.0`. `fingerprint/rotate` uses `Chrome/120.0.0.0`.
Both outdated (current stable is 133+). Inconsistency between HTTP header UA and
JS navigator.userAgent is a strong detection signal.

**Fix:** Make UA version configurable via `BRIDGE_CHROME_VERSION` env / config.
Default to recent version. Both launch flags and fingerprint rotation must use the same value.
```go
var chromeVersion = envOr("BRIDGE_CHROME_VERSION", "133.0.6943.98")
```

### 8F-7: Fingerprint rotation uses JS overrides, not CDP [MEDIUM]
`Object.defineProperty` on navigator is trivially detectable:
- `Object.getOwnPropertyDescriptor(navigator, 'userAgent').get.toString()` reveals custom getter
- Doesn't affect iframes or Web Workers

**Fix:** Use CDP `Network.setUserAgentOverride` as the primary mechanism:
```go
network.SetUserAgentOverride(userAgent).
    WithPlatform(platform).
    WithAcceptLanguage(language).
    Do(ctx)
```
Keep JS overrides as backup for properties CDP doesn't cover. Wrap getters to
mimic native `toString()` output.

### 8F-8: WebRTC blocking throws errors instead of filtering [MEDIUM]
`throw new Error('WebRTC blocked')` is detectable and breaks legitimate WebRTC usage.

**Fix:** Don't block — force `iceTransportPolicy: 'relay'` and silently filter
local IP candidates from ICE events. Real browsers with strict privacy settings
behave this way.

```javascript
const OriginalRTCPeerConnection = window.RTCPeerConnection;
window.RTCPeerConnection = function(...args) {
  if (args[0]) args[0].iceTransportPolicy = 'relay';
  else args[0] = { iceTransportPolicy: 'relay' };
  const pc = new OriginalRTCPeerConnection(...args);
  // Filter local IP candidates from icecandidate events
  // ... (see full solution in pinchtab-review.md §3.5)
  return pc;
};
window.RTCPeerConnection.prototype = OriginalRTCPeerConnection.prototype;
```

### 8F-9: Timezone is hardcoded EST, doesn't use CDP [MEDIUM]
`stealth.js` hardcodes `getTimezoneOffset` to -300 (EST).
`Intl.DateTimeFormat().resolvedOptions().timeZone` still returns real system TZ.
`/fingerprint/rotate` accepts timezone but doesn't coordinate with the JS override.

**Fix:** Use CDP `Emulation.setTimezoneOverride` — one line, affects everything:
```go
emulation.SetTimezoneOverride("America/New_York").Do(ctx)
```
Remove the JS `getTimezoneOffset` override from stealth.js entirely.

### 8F-10: Stealth status reports hardcoded booleans [MEDIUM]
`handleStealthStatus` returns `true`/`false` literals. If stealth script fails
to inject (Chrome restart, new context), status still says everything is enabled.

**Fix:** Actually probe the browser by evaluating JS that checks each feature:
`navigator.webdriver === undefined`, `navigator.plugins.length === 3`,
canvas `toDataURL` called twice returns different results, etc.
See full probe script in `pinchtab-review.md` §3.7.

### 8F-11: Remove addEventListener mousemove wrapper [MEDIUM]
Wrapping `EventTarget.prototype.addEventListener` for mousemove is:
- Redundant with `human.go` bezier curves (which operate at input dispatch level)
- Breaks `removeEventListener` (wrapped function ≠ original reference)
- Only wraps 10% randomly — inconsistent behavior
- Can break React, maps, drag libraries

**Fix:** Delete the entire `addEventListener` wrapper block from stealth.js (~10 lines).

---

## P8-TEST: Test Coverage for P6-P8

Zero tests exist for any P6-P8 feature. This blocks safe refactoring.

### Unit tests (no Chrome needed)
```
human_test.go
├── TestBezierCurvePoints — verify curve generates correct intermediate points
├── TestBezierCurveStepCount — verify step count scales with distance
├── TestHumanTypeActions — verify action count matches text length + corrections
├── TestHumanTypeActionsEmpty — verify error on empty text
└── TestHumanTypeFastMode — verify fast mode reduces delays

handlers_test.go (additions)
├── TestHandleActions_BatchEmpty — empty array returns error
├── TestHandleActions_BatchBadJSON — malformed batch
├── TestHandleActions_BatchNoTab — no tab returns 404
├── TestHandleCookies_NoTab — no tab returns 404
├── TestHandleCookies_BadJSON — malformed request
├── TestHandleStealthStatus — verify response shape and score calculation
├── TestHandleFingerprintRotate_BadJSON — malformed request
├── TestHandleFingerprintRotate_NoTab — no tab returns 404
├── TestHandleFingerprintRotate_RandomOS — verify random OS selection
└── TestGenerateFingerprint — all OS/browser combos produce valid fingerprints

handler_snapshot_test.go (additions)
├── TestSnapshotFileOutput — verify file written to correct path
└── TestSnapshotYAMLFormat — verify YAML marshaling produces valid output

config_test.go (additions)
├── TestLoadConfigFile — verify config.json loading
├── TestLoadConfigFileMissing — verify graceful fallback
└── TestConfigFileOverridesEnv — verify precedence
```

**Implementation note:** To unit test `humanMouseMove` and `humanType` without Chrome,
extract the pure math from CDP calls:
```go
type point struct{ x, y float64 }
func bezierPoints(from, to point, steps int, rng *rand.Rand) []point { ... }
```
Then test the math directly.

### Integration tests (build tag: `integration`, need Chrome)
```
stealth_integration_test.go
├── TestStealthScriptInjected — verify navigator.webdriver === undefined
├── TestCanvasNoiseApplied — toDataURL twice, verify different outputs
├── TestFontMetricsNoise — measureText twice, verify different widths
├── TestWebRTCFiltered — verify no local IPs leaked (after 8F-8 fix)
├── TestWebGLVendorSpoofed — UNMASKED_VENDOR_WEBGL returns "Intel Inc."
├── TestPluginsPresent — navigator.plugins has 3 entries
└── TestFingerprintRotation — call endpoint, verify navigator.userAgent changed
```

### Manual validation (quarterly)
Run against detection sites, document results in QA.md:
- https://bot.sannysoft.com/
- https://abrahamjuliot.github.io/creepjs/
- https://browserleaks.com/

---

## P8-MISC: Minor Fixes

- [ ] **humanType uses global rand** — Accept `*rand.Rand` for reproducible tests
- [ ] **yaml.v3 dependency** — Either remove (implement simple indented format, ~30 LOC) or update messaging from "zero deps" to "minimal deps"
- [ ] **Dockerfile env vars not consumed** — `CHROME_BINARY` and `CHROME_FLAGS` are set but Go code doesn't read them. Add `chromedp.ExecPath` and flag parsing.
- [ ] **Dockerfile not tested in CI** — Add `docker build` step to CI workflow
- [ ] **AGENTS.md tracked in git** — Should be in `.gitignore` (OpenClaw workspace file)

---

## P9: Multi-Agent Coordination

**Prerequisite:** P8-FIX and P8-TEST must be complete first.

Best practice today: separate tabs per agent with explicit `tabId`.

- [ ] **Tab locking** — `POST /tab/lock`, `POST /tab/unlock` with timeout-based deadlock prevention
- [ ] **Agent sessions (optional)** — Isolated browser contexts per agent
- [ ] **Ref cache versioning** — Prevent stale ref conflicts between agents
- [ ] **Tab ownership tracking** — Show owner in `/tabs` response

Start with tab locking (solves 80% of conflicts). Add sessions only if needed.

## P10: Quality of Life

- [ ] **Ad blocking** — Basic tracker blocking for cleaner snapshots
- [ ] **CSS animation disabling** — Faster page loads, more consistent snapshots
- [ ] **Debloated Chrome launch** — Strip unnecessary features
- [ ] **Randomized window sizes** — Avoid automation fingerprint

---

## Not Doing
- Desktop app / GUI — agents don't need GUIs
- Plugin system
- Proxy rotation (IP-level)
- Multi-tenant SaaS
- Selenium compatibility
- Cloud anything
- MCP protocol — HTTP is the interface
- Distributed browser clusters
- Complex workflow orchestration — agents handle their own logic
