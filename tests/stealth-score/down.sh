#!/usr/bin/env bash
# tests/stealth-score/down.sh — tear down the stealth-score docker container.
set -euo pipefail
docker rm -f stealth-score-pinchtab >/dev/null 2>&1 || true
echo "stealth-score-pinchtab removed"
