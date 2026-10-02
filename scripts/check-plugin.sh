#!/usr/bin/env bash
set -euo pipefail

cd "$(dirname "$0")/.."

SCOPE="all"
for arg in "$@"; do
  case "$arg" in
    openclaw|grok|all) SCOPE="$arg" ;;
    *)
      echo "unknown argument: $arg" >&2
      echo "usage: check-plugin.sh [all|openclaw|grok]" >&2
      exit 1
      ;;
  esac
done

check_openclaw() {
  echo "📦 OpenClaw plugin checks"
  echo ""

  if [ ! -d "plugins/openclaw/node_modules" ]; then
    echo "  Installing dependencies..."
    (cd plugins/openclaw && npm install --silent)
    echo "  ✓ Dependencies installed"
    echo ""
  fi

  echo "  Validating JSON schemas..."
  node -e "JSON.parse(require('fs').readFileSync('plugins/openclaw/package.json', 'utf8'))"
  node -e "JSON.parse(require('fs').readFileSync('plugins/openclaw/openclaw.plugin.json', 'utf8'))"
  echo "  ✓ JSON valid"

  echo ""
  echo "  Verifying package contents..."
  (cd plugins/openclaw && npm pack --dry-run 2>&1 | grep -E "^npm notice [0-9]" | awk '{print "    " $4 " (" $3 ")"}')
  echo "  ✓ Package verified"

  echo ""
  echo "✅ OpenClaw plugin checks passed"
}

check_grok() {
  echo "📦 Grok plugin checks"
  echo ""

  echo "  Verifying synced skills..."
  node plugins/sync-skills.mjs grok --check
  echo "  ✓ Skills"

  echo ""
  echo "  Validating JSON..."
  node -e "JSON.parse(require('fs').readFileSync('plugins/grok/.grok-plugin/plugin.json', 'utf8'))"
  node -e "JSON.parse(require('fs').readFileSync('plugins/grok/.mcp.json', 'utf8'))"
  node -e "JSON.parse(require('fs').readFileSync('.grok-plugin/marketplace.json', 'utf8'))"
  node -e "
    const fs = require('fs');
    const mcp = JSON.parse(fs.readFileSync('plugins/grok/.mcp.json', 'utf8'));
    const server = mcp.mcpServers && mcp.mcpServers.pinchtab;
    if (!server || server.command !== 'pinchtab' || !Array.isArray(server.args) || server.args[0] !== 'mcp') {
      throw new Error('plugins/grok/.mcp.json must define mcpServers.pinchtab as { command: \"pinchtab\", args: [\"mcp\"] }');
    }
    const marketplace = JSON.parse(fs.readFileSync('.grok-plugin/marketplace.json', 'utf8'));
    const plugin = (marketplace.plugins || []).find((entry) => entry.name === 'pinchtab');
    if (!plugin || !plugin.source || plugin.source.path !== './plugins/grok') {
      throw new Error('.grok-plugin/marketplace.json must list pinchtab at ./plugins/grok');
    }
  "
  echo "  ✓ JSON valid"

  echo ""
  echo "  Verifying required files..."
  for path in \
    plugins/grok/README.md \
    plugins/grok/.grok-plugin/plugin.json \
    plugins/grok/.mcp.json \
    plugins/grok/skills/pinchtab/SKILL.md \
    plugins/grok/skills/pinchtab-mcp/SKILL.md
  do
    if [ ! -f "$path" ]; then
      echo "  missing $path" >&2
      exit 1
    fi
    echo "    $path"
  done
  echo "  ✓ Files present"

  if command -v grok >/dev/null 2>&1; then
    echo ""
    echo "  Running grok plugin validate..."
    grok plugin validate ./plugins/grok
    echo "  ✓ grok plugin validate"
  else
    echo ""
    echo "  skipping grok plugin validate (grok not on PATH)"
  fi

  echo ""
  echo "✅ Grok plugin checks passed"
}

if [ "$SCOPE" = "all" ] || [ "$SCOPE" = "openclaw" ]; then
  check_openclaw
  echo ""
fi
if [ "$SCOPE" = "all" ] || [ "$SCOPE" = "grok" ]; then
  check_grok
fi
