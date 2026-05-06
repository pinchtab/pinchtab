#!/usr/bin/env bash
set -euo pipefail

: "${PINCHTAB_BASE_URL:?missing PINCHTAB_BASE_URL}"
: "${PINCHTAB_TOKEN:?missing PINCHTAB_TOKEN}"
: "${FIXTURES_URL:?missing FIXTURES_URL}"

if [[ -n "${ANTHROPIC_API_KEY:-}" && ! -f /root/.openclaw/openclaw.json ]]; then
  echo "bootstrapping openclaw config from ANTHROPIC_API_KEY..."
  openclaw onboard \
    --non-interactive \
    --accept-risk \
    --mode local \
    --auth-choice anthropic-api-key \
    --anthropic-api-key "$ANTHROPIC_API_KEY" \
    --workspace /root/workspace \
    --skip-health \
    >/artifacts/onboard.log 2>&1 || {
      echo "openclaw onboard failed:" >&2
      tail -40 /artifacts/onboard.log >&2
      exit 1
    }
  for agent in main alpha beta; do
    mkdir -p "/root/.openclaw/agents/$agent/agent"
    cat >"/root/.openclaw/agents/$agent/agent/auth-profiles.json" <<JSON
{
  "version": 1,
  "profiles": {
    "anthropic-default": {
      "type": "api_key",
      "provider": "anthropic",
      "key": "${ANTHROPIC_API_KEY}",
      "displayName": "Anthropic"
    }
  },
  "order": {
    "anthropic": ["anthropic-default"]
  },
  "lastGood": {
    "anthropic": "anthropic-default"
  }
}
JSON
  done
  # Register the non-default agents with openclaw. `onboard` only sets up
  # `main`; without this `openclaw agent --agent alpha` errors with
  # "Unknown agent id". Per docs/cli/agents.md, non-interactive mode requires
  # both a name and --workspace; --agent-dir points at the dir we already
  # populated with auth-profiles.json so the agent inherits Anthropic auth.
  for agent in alpha beta; do
    mkdir -p "/root/workspace-$agent"
    openclaw agents add "$agent" \
      --workspace "/root/workspace-$agent" \
      --agent-dir "/root/.openclaw/agents/$agent/agent" \
      --model anthropic/claude-sonnet-4-5 \
      --non-interactive \
      >>/artifacts/onboard.log 2>&1 || {
        echo "openclaw agents add $agent failed:" >&2
        tail -40 /artifacts/onboard.log >&2
        exit 1
      }
  done
fi

python3 - /root/.openclaw/openclaw.json <<'PY'
import json, os
from pathlib import Path
path = Path('/root/.openclaw/openclaw.json')
obj = json.loads(path.read_text()) if path.exists() else {}
anthropic_key = os.environ.get('ANTHROPIC_API_KEY', '')
if anthropic_key:
    auth_profiles = obj.setdefault('auth', {}).setdefault('profiles', {})
    auth_profiles['anthropic-default'] = {
        'provider': 'anthropic',
        'mode': 'api_key',
    }
    auth_order = obj.setdefault('auth', {}).setdefault('order', {})
    auth_order['anthropic'] = ['anthropic-default']
    agents = obj.setdefault('agents', {}).setdefault('defaults', {})
    agents['model'] = 'anthropic/claude-sonnet-4-5'
plugins = obj.setdefault('plugins', {}).setdefault('entries', {})
plugins.setdefault('pinchtab', {})['enabled'] = True
pinchtab_cfg = plugins.setdefault('pinchtab', {}).setdefault('config', {})
pinchtab_cfg['baseUrl'] = 'http://pinchtab:9999'
pinchtab_cfg['token'] = 'smoke-token'
pinchtab_cfg['registerBrowserTool'] = True
plugins.setdefault('browser', {})['enabled'] = False
allow = obj.setdefault('plugins', {}).get('allow')
if isinstance(allow, list) and 'pinchtab' not in allow:
    allow.append('pinchtab')
obj['browser'] = {'enabled': False}
# Default onboard profile is "coding", which excludes browser/plugin tools
# (per docs/tools/index.md). Switch to "full" and explicitly allow the
# pinchtab/browser tools so the agents can actually invoke them.
tools = obj.setdefault('tools', {})
tools['profile'] = 'full'
allow = tools.get('allow')
if not isinstance(allow, list):
    allow = []
for name in ('pinchtab', 'browser'):
    if name not in allow:
        allow.append(name)
tools['allow'] = allow
path.write_text(json.dumps(obj, indent=2) + '\n')
PY

cp /root/.openclaw/openclaw.json /artifacts/openclaw.json 2>/dev/null || true
for agent in main alpha beta; do
  if [[ -f "/root/.openclaw/agents/$agent/agent/auth-profiles.json" ]]; then
    python3 - "/root/.openclaw/agents/$agent/agent/auth-profiles.json" "/artifacts/auth-profiles-$agent.redacted.json" <<'PY'
import json, sys
src, dst = sys.argv[1], sys.argv[2]
obj = json.loads(open(src).read())
for p in obj.get('profiles', {}).values():
    for k in ('key','token'):
        if isinstance(p.get(k), str) and p[k]:
            p[k] = f"<redacted len={len(p[k])} prefix={p[k][:8]}>"
open(dst, 'w').write(json.dumps(obj, indent=2))
PY
  fi
done

if [[ -n "${ANTHROPIC_API_KEY:-}" ]]; then
  echo "=== anthropic key probe ==="
  node -e "
    const key = process.env.ANTHROPIC_API_KEY;
    console.log('key length:', key.length, 'prefix:', key.slice(0, 14));
    fetch('https://api.anthropic.com/v1/models', {
      headers: {'x-api-key': key, 'anthropic-version': '2023-06-01'}
    }).then(r => r.text().then(t => console.log('status:', r.status, 'body:', t.slice(0, 200))))
      .catch(e => console.log('error:', e.message));
  " 2>&1 | tee /artifacts/anthropic-probe.log
fi

if [ ! -f /artifacts/plugin.tgz ]; then
  echo "missing /artifacts/plugin.tgz — run.sh must run 'npm pack' before docker compose up" >&2
  exit 1
fi

# Install from the same tarball npm publish would upload — closest mirror of
# the release flow. No --link, no source bind: the plugin is consumed exactly
# as a downstream user would consume the published package.
openclaw plugins install /artifacts/plugin.tgz >/artifacts/plugin-install.log 2>&1

(openclaw gateway >/artifacts/gateway.log 2>&1) &
GW_PID=$!
cleanup() {
  kill "$GW_PID" >/dev/null 2>&1 || true
  wait "$GW_PID" >/dev/null 2>&1 || true
}
trap cleanup EXIT

for _ in $(seq 1 45); do
  if openclaw health >/artifacts/health.json 2>/artifacts/health.err; then
    break
  fi
  sleep 2
done
openclaw health >/artifacts/health.json 2>/artifacts/health.err

python3 <<'PY'
import json
import subprocess
from pathlib import Path

artifacts = Path('/artifacts')
fixtures_url = 'http://fixtures:8080'
scenarios = [
    {
        'id': 'alpha',
        'agent': 'main',
        'prompt': f'Use the pinchtab tool to navigate to {fixtures_url}/alpha and reply with only the verification code on the page.',
        'expected': 'ALPHA-17',
        'paths': ['/alpha'],
        'requiredTool': 'pinchtab',
    },
    {
        'id': 'journey',
        'agent': 'main',
        'prompt': f'Use the pinchtab tool to navigate to {fixtures_url}/journey/start, click the Begin journey button, wait for the next page, and reply with only the final verification code.',
        'expected': 'ORBIT-42',
        'paths': ['/journey/start', '/journey/final'],
        'requiredTool': 'pinchtab',
    },
    {
        'id': 'chain',
        'agent': 'main',
        'prompt': f'Use the pinchtab tool to navigate to {fixtures_url}/chain/one, click through until you reach the last page, and reply with only the full final verification code.',
        'expected': 'BLUE-SUN-9',
        'paths': ['/chain/one', '/chain/two', '/chain/final'],
        'requiredTool': 'pinchtab',
    },
    {
        'id': 'browser-alias',
        'agent': 'main',
        'prompt': f'Use the browser tool to navigate to {fixtures_url}/alpha and reply with only the verification code on the page.',
        'expected': 'ALPHA-17',
        'paths': ['/alpha'],
        'requiredTool': 'browser',
    },
]

session_scenarios = [
    {
        'id': 'agent-alpha-start',
        'agent': 'alpha',
        'prompt': f'Use the pinchtab tool to navigate to {fixtures_url}/journey/start, click the Begin journey button, wait for the next page to load, and reply with only READY.',
        'expected': 'READY',
        'paths': ['/journey/start', '/journey/final'],
        'requiredTool': 'pinchtab',
    },
    {
        'id': 'agent-alpha-reuse',
        'agent': 'alpha',
        'prompt': f'Use the pinchtab tool to continue the existing browser session for this agent, navigate to {fixtures_url}/journey/final, and reply with only the verification code on the page.',
        'expected': 'ORBIT-42',
        'paths': ['/journey/final'],
        'requiredTool': 'pinchtab',
    },
    {
        'id': 'agent-beta-tab',
        'agent': 'beta',
        'prompt': f'Use the pinchtab tool to navigate to {fixtures_url}/alpha and reply with only the verification code on the page.',
        'expected': 'ALPHA-17',
        'paths': ['/alpha'],
        'requiredTool': 'pinchtab',
    },
]

def run_agent_scenario(scenario):
    out_path = artifacts / f"agent-{scenario['id']}.json"
    cmd = [
        'openclaw', 'agent',
        '--agent', scenario['agent'],
        '--message', scenario['prompt'],
        '--json',
        '--timeout', '240',
    ]
    completed = subprocess.run(cmd, capture_output=True, text=True)
    if completed.returncode != 0:
        raise SystemExit(f"agent command failed for {scenario['id']}:\nSTDOUT:\n{completed.stdout}\nSTDERR:\n{completed.stderr}")
    out_path.write_text(completed.stdout)
    payload = json.loads(completed.stdout)
    # `openclaw agent --json` historically wrapped the body in {"result": {...}};
    # newer builds emit the payload at the root. Accept both shapes.
    body = payload.get('result', payload)
    text = body['payloads'][0]['text'].strip()
    tool_summary = body.get('meta', {}).get('toolSummary', {})
    tools_used = tool_summary.get('tools', []) or []
    return {
        'id': scenario['id'],
        'agent': scenario['agent'],
        'expected': scenario['expected'],
        'actual': text,
        'ok': text == scenario['expected'],
        'paths': scenario['paths'],
        'requiredTool': scenario['requiredTool'],
        'toolsUsed': tools_used,
        'toolOk': scenario['requiredTool'] in tools_used,
    }

results = [run_agent_scenario(scenario) for scenario in scenarios]
session_results = [run_agent_scenario(scenario) for scenario in session_scenarios]

log_path = artifacts / 'fixtures-access.log'
entries = []
if log_path.exists():
    for line in log_path.read_text().splitlines():
        line = line.strip()
        if not line:
            continue
        entries.append(json.loads(line))

browser_entries = [
    entry for entry in entries
    if entry.get('path', '').split('?', 1)[0] != '/health'
    and 'Chrome' in (entry.get('userAgent') or '')
]

for result in results + session_results:
    missing = []
    for path in result['paths']:
        hits = [
            entry for entry in entries
            if entry.get('path', '').split('?', 1)[0] == path
        ]
        if not hits:
            missing.append(path)
    result['fixtureLogOk'] = not missing
    result['missingFixturePaths'] = missing

session_proof = {
    'ok': all(r['ok'] and r['toolOk'] and r['fixtureLogOk'] for r in session_results),
    'sameAgentReuse': next(r for r in session_results if r['id'] == 'agent-alpha-reuse'),
    'separateAgentTab': next(r for r in session_results if r['id'] == 'agent-beta-tab'),
}

summary = {
    'ok': all(r['ok'] and r['fixtureLogOk'] and r['toolOk'] for r in results) and session_proof['ok'] and bool(browser_entries),
    'results': results,
    'sessionProof': session_proof,
    'sessionScenarios': session_results,
    'logEntries': len(entries),
    'browserRequestCount': len(browser_entries),
    'browserRequestSample': [
        {'path': e.get('path'), 'userAgent': (e.get('userAgent') or '')[:80]}
        for e in browser_entries[:5]
    ],
}
(artifacts / 'summary.json').write_text(json.dumps(summary, indent=2) + '\n')

def fmt_row(r):
    status = 'PASS' if r['ok'] and r['toolOk'] and r['fixtureLogOk'] else 'FAIL'
    actual = r['actual'].replace('\n', ' ').strip()
    if len(actual) > 60:
        actual = actual[:57] + '...'
    return f"  [{status}] {r['id']:<22} agent={r['agent']:<5} tool={r['requiredTool']:<8} -> {actual!r}"

all_results = results + session_results
print()
print('=== smoke summary ===')
print('Main scenarios:')
for r in results:
    print(fmt_row(r))
print('Session scenarios:')
for r in session_results:
    print(fmt_row(r))
passed = sum(1 for r in all_results if r['ok'] and r['toolOk'] and r['fixtureLogOk'])
print(f"Browser requests captured: {len(browser_entries)} (Chrome user-agent)")
print(f"Result: {passed}/{len(all_results)} scenarios passed -> {'OK' if summary['ok'] else 'FAIL'}")
print(f"(full details: artifacts/summary.json)")

if not summary['ok']:
    raise SystemExit(1)
PY
