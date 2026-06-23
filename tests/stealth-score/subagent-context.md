# Stealth Score Subagent Context

You are driving PinchTab in a Docker container against public bot-detection
sites. Your job: visit each site in `tests/stealth-score/sites/index.md`,
follow its playbook, and write a structured JSON report.

Do not touch Docker, do not switch providers, do not read other runs'
artifacts. The orchestrator already brought up the container for you.

Do not read anything under `tests/stealth-score/results/`.

## Inputs You Receive

- `PROVIDER`: `chrome` or `cloak` — already configured in the container.
- `REPORT_FILE`: absolute path of the pre-seeded JSON file to append to.
- `PROJECT_ROOT`: absolute path of the pinchtab repo.

## Working Directory and Wrapper

Always run from `tests/tools` so the `./scripts/pt` wrapper resolves:

```bash
cd $PROJECT_ROOT/tests/tools
export PINCHTAB_CONTAINER=stealth-score-pinchtab
export PINCHTAB_TOKEN=stealth-score-token
```

`./scripts/pt` forwards every argument to the `pinchtab` CLI inside the
container. Use it exactly like the local CLI:

```bash
./scripts/pt health
TAB=$(./scripts/pt nav https://example.com --new-tab --print-tab-id)
./scripts/pt --tab "$TAB" snap
```

Read `$PROJECT_ROOT/skills/pinchtab/SKILL.md` for the full command surface.

## Session

Create one session up-front and reuse it across all sites:

```bash
export PINCHTAB_SESSION=$(./scripts/pt session create --agent-id stealth-${PROVIDER})
```

## Per-Site Loop

For every site listed in `$PROJECT_ROOT/tests/stealth-score/sites/index.md`:

1. Read its playbook (`tests/stealth-score/sites/<id>.md`). It tells you the
   URL, the readiness signal, any click/scroll/wait the page needs, and the
   metrics to extract.
2. Drive PinchTab to follow the playbook. Use refs from `snap` when targeting
   elements; use `wait --text` / `wait --not-text` to settle SPAs. Don't
   hard-code sleeps unless the playbook explicitly says so.
3. Extract the listed metrics from `text` / `snap` output. Record what you
   actually saw. If a metric is genuinely unreadable, record `"unavailable"`
   and explain in `notes`. Do **not** invent values. Do **not** carry values
   between sites.
4. Append a record to `REPORT_FILE`:

```json
{
  "site": "<id>",
  "url": "<url from playbook>",
  "tab_id": "<from --print-tab-id>",
  "metrics": {
    "<key>": "<value>",
    ...
  },
  "notes": "<optional one-line caveats>"
}
```

Safe-append helper:

```bash
python3 - <<'PY'
import json
path = "<REPORT_FILE>"
entry = { "site": "<id>", "url": "<url>", "tab_id": "<id>", "metrics": {...}, "notes": "..." }
data = json.load(open(path))
data["sites"].append(entry)
json.dump(data, open(path, "w"), indent=2)
PY
```

## Quality Bar

| Good                                       | Bad                          |
|--------------------------------------------|------------------------------|
| `trust_score: "62.0"`                      | `trust_score: "see page"`    |
| `webdriver: "passed"`                      | `webdriver: "ok"`            |
| `robot_verdict: "You're not a Robot"`      | `robot_verdict: "no"`        |
| `bot_score: "0.12 (Likely human)"`         | `bot_score: "low"`           |

If a site shows a Cloudflare challenge ("Just a moment...", "attention
required"), record `metrics: {}` and `notes: "cloudflare challenge"` and
move on — do not call `/solve` without explicit user approval.

## Completion

After every site has a record:

```bash
python3 - <<'PY'
import json, time
path = "<REPORT_FILE>"
d = json.load(open(path))
d["completed_at"] = time.strftime("%Y-%m-%dT%H:%M:%SZ", time.gmtime())
d["sites_processed"] = len(d["sites"])
json.dump(d, open(path, "w"), indent=2)
PY
```

Print exactly this as your final stdout line:

```
STEALTH_SCORE_RUN_COMPLETE provider=<provider> sites_processed=<n>
```
