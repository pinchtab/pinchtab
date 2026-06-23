---
name: pinchtab-stealth-score
description: "Run the PinchTab stealth-score sweep against 15 bot-detection / fingerprint sites (sannysoft, rebrowser, deviceandbrowserinfo, iphey, whoer, browserscan, pixelscan, fingerprint-scan, incolumitas, fvision, amiunique, browserleaks, creepjs, coveryourtracks, fingerprint-demo). Starts a Docker PinchTab container per browser (chrome / cloak / both), spawns a blind agent that drives PinchTab through plain-English per-site playbooks, captures structured metrics, prints a side-by-side comparison highlighting divergent metrics, and appends to history.jsonl for cross-session tracking. Use when asked to 'run stealth score', 'compare cloak vs chrome detection', 'measure stealth', or '/pinchtab-stealth-score'."
---

# PinchTab Stealth Score

Drive PinchTab through a list of public bot-detection sites under each browser
(`chrome`, `cloak`, or `both`), and collect the metrics that matter
most for analyst comparison. The Docker plumbing rebuilds the PinchTab image
from current source so you're benchmarking the working tree.

The shape is the same as `/pinchtab-opt`: one container per run, one blind
agent that reads English playbooks and drives PinchTab through `./scripts/pt`,
records per-site metrics, and the orchestrator summarizes.

## Argument Parsing

- `/pinchtab-stealth-score` → default to `both`
- `/pinchtab-stealth-score chrome` → chrome only
- `/pinchtab-stealth-score cloak` → cloak only
- `/pinchtab-stealth-score both` → run both sequentially

Anything else → print this section and abort.

## Site Catalogue

The agent processes the sites listed in `tests/stealth-score/sites/index.md`
(currently 15). The list is dynamic — to add or remove sites you only edit
that index and the matching `<id>.md` playbook. The skill itself doesn't hard-
code site names.

Current sites (15) — `sannysoft`, `rebrowser`, `deviceandbrowserinfo`, `iphey`,
`whoer`, `browserscan`, `pixelscan`, `fingerprint-scan`, `incolumitas`,
`fvision`, `amiunique`, `browserleaks` (multi-page: canvas+webgl+fonts+tls),
`creepjs`, `coveryourtracks`, `fingerprint-demo`.

Expected duration: ~12-15 min per browser once images are cached, so a `both`
run takes ~25-30 min plus first-time image build (~10 min for cloak).

## Path Resolution

```bash
PROJECT_ROOT=$(git rev-parse --show-toplevel)
SCORE_DIR="$PROJECT_ROOT/tests/stealth-score"
RESULTS_DIR="$SCORE_DIR/results"
TIMESTAMP=$(date -u +%Y%m%dT%H%M%SZ)
mkdir -p "$RESULTS_DIR"
```

## Prerequisites

Docker must be running.

```bash
docker info >/dev/null 2>&1 || { echo "Docker not running"; exit 1; }
```

If port 9867 on the host is already taken (e.g. a native PinchTab server), free
it first — both the chrome and cloak containers bind 9867:

```bash
pinchtab daemon stop 2>&1 || true
sleep 2
pkill -9 -f "pinchtab " 2>/dev/null || true
```

## Execution

For each requested browser, run the same loop:

### 1. Bring up the container

```bash
"$SCORE_DIR/up.sh" "$PROVIDER"
```

`up.sh` builds the appropriate image if absent (chrome-smoke or cloakbrowser),
writes a browser config with open `allowedDomains`, and starts a container
named `stealth-score-pinchtab` on host port 9867. It exits non-zero if the
container fails to become healthy.

To force a rebuild: `REBUILD=1 "$SCORE_DIR/up.sh" "$PROVIDER"`.

### 2. Seed the report JSON

```bash
REPORT_FILE="$RESULTS_DIR/${PROVIDER}_${TIMESTAMP}.json"
cat > "$REPORT_FILE" <<JSON
{
  "provider": "${PROVIDER}",
  "timestamp": "${TIMESTAMP}",
  "started_at": "$(date -u +%Y-%m-%dT%H:%M:%SZ)",
  "completed_at": null,
  "sites_processed": 0,
  "sites": []
}
JSON
```

### 3. Spawn the agent

Use the **Agent** tool. Prompt template — replace `{PROVIDER}`, `{REPORT_FILE}`,
`{PROJECT_ROOT}`:

```
You are running a PinchTab stealth-score sweep against a Docker container.

PROVIDER: {PROVIDER}
REPORT_FILE: {REPORT_FILE}
PROJECT_ROOT: {PROJECT_ROOT}

Read these files first (do NOT read anything under tests/stealth-score/results/):
1. {PROJECT_ROOT}/tests/stealth-score/subagent-context.md — environment, wrapper, recording format.
2. {PROJECT_ROOT}/tests/stealth-score/sites/index.md — list of sites to process and in what order.
3. {PROJECT_ROOT}/skills/pinchtab/SKILL.md — PinchTab command reference.

The site list currently has ~15 entries. Work through them in the order index.md gives. For each site:
- Read its playbook (sites/<id>.md). It describes what to navigate to, what to wait for, any clicks needed, and which metrics to capture.
- Drive PinchTab through the {PROJECT_ROOT}/tests/tools/scripts/pt wrapper. Before any pt call: cd {PROJECT_ROOT}/tests/tools and export PINCHTAB_CONTAINER=stealth-score-pinchtab and PINCHTAB_TOKEN=stealth-score-token.
- Extract the listed metrics from what you actually observe on the page. Capture as many of the listed metrics as you can find — partial captures are fine; record "unavailable" plus a brief reason when a metric isn't present.
- Append a JSON record to REPORT_FILE per the recording format in subagent-context.md.

Time-box each site to ~2 minutes. If a site is slow, hangs, or shows a Cloudflare challenge, record what you have, set notes accordingly, and move on.

Finalize the report (set completed_at + sites_processed) and print STEALTH_SCORE_RUN_COMPLETE on stdout as your final line.

The container is already running and healthy. Do NOT touch Docker, do NOT switch browsers, do NOT run `pinchtab server` or `daemon` — the wrapper talks to the container directly.
```

### 4. Tear down

```bash
"$SCORE_DIR/down.sh"
```

When both browsers are requested, run **sequentially**: up → agent → down for
chrome, then up → agent → down for cloak. They share port 9867.

## Summarize

After all per-browser JSON reports exist, build the side-by-side comparison
using the Go runner:

```bash
"$PROJECT_ROOT/tests/tools/scripts/runner" stealth compare \
  "$RESULTS_DIR"/*_${TIMESTAMP}.json
```

`tests/tools/scripts/runner` is a tracked self-building shim: on first use it
compiles the Go runner into the gitignored `tests/tools/scripts/.runner.bin`
and rebuilds it automatically after source changes — no manual `go build`
needed (and never build over the shim path itself; that would overwrite the
checked-in script with a binary).

The summarizer:

1. Prints a markdown comparison: headline counts, a **divergent-metrics** table
   (only rows where chrome and cloak disagree on real values), and per-site
   tables for every metric captured by either browser. Pipe directly to the user.
2. Appends one JSON line to `tests/stealth-score/history.jsonl` per
   comparison run (run id, providers, sites count, divergence count, list of
   divergent metrics). The file is kept in the repo so multi-session history
   survives.
3. Regenerates `tests/stealth-score/history.md` — last-20-runs table view.

Present the markdown table to the user as-is. Mention the history files
when reporting so the user knows where to look for trends.

## Output Layout

```
tests/stealth-score/
├── up.sh                    # boot docker container per browser
├── down.sh                  # tear down docker container
├── sites/
│   ├── index.md             # ordered list of sites to process
│   ├── sannysoft.md         # static table — webdriver/permissions/webgl rows
│   ├── rebrowser.md         # puppeteer/playwright leak probes
│   ├── deviceandbrowserinfo.md  # bot/human verdict + suspicious signals
│   ├── iphey.md             # reliability badge, proxy/VPN/DNS leak flags
│   ├── whoer.md             # anonymity %, proxy/VPN, browser consistency
│   ├── browserscan.md       # bot-detection per-test rows + IP + canvas + webrtc
│   ├── pixelscan.md         # bot score + automation flags (needs click)
│   ├── fingerprint-scan.md  # bot risk score, automation flags
│   ├── incolumitas.md       # behavioural + TLS/JA3 + WebRTC + proxy
│   ├── fvision.md           # fv.pro privacy/leak summary
│   ├── amiunique.md         # per-attribute uniqueness ratios
│   ├── browserleaks.md      # multi-nav: /canvas + /webgl + /fonts + /tls
│   ├── creepjs.md           # heavy JS suite; trust score + fingerprint hashes
│   ├── coveryourtracks.md   # EFF tracking-resistance (needs button click)
│   └── fingerprint-demo.md  # FingerprintJS commercial demo
├── subagent-context.md      # blind agent's environment + recording format
├── history.jsonl            # one-line summary per comparison run (kept in repo)
├── history.md               # last-20 runs rendered table (regenerated each run)
├── .tmp/                    # gitignored; per-browser configs written by up.sh
└── results/                 # gitignored; <provider>_<ts>.json per run
```

## Adding or Removing Sites

1. Create `tests/stealth-score/sites/<id>.md` from the existing playbooks. Keep
   the structure: URL → readiness signal → steps → metrics table → gotchas.
2. Add the new id to `tests/stealth-score/sites/index.md` in the order you
   want it processed.

That's it. No code change. The agent reads the index fresh every run.

## Notes

- This is **advisory**, not a CI gate. A non-zero detection score on cloak
  that matches chrome's baseline is a generic Chromium signal, not a
  CloakBrowser regression.
- Heavy SPA detection pages (creepjs, browserscan) take 20-40 s to settle.
  The playbooks use `wait --text` for the actual readiness signal so the
  agent doesn't read mid-load placeholders. If a page is still loading after
  the playbook's recommended waits, the agent should record `"unavailable"`
  for affected metrics and explain in `notes` — don't paper over it with
  guesses.
- The Docker container exposes PinchTab on 9867. The `./scripts/pt` wrapper
  reads `PINCHTAB_CONTAINER` env to know which container to exec into.
