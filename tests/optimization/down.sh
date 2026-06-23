#!/usr/bin/env bash
# tests/optimization/down.sh
#
# Tear down whatever optimization benchmark instance is running.
# Safe to call even if nothing is up.
set -euo pipefail

# Remove the standalone cloak container if present
docker rm -f optimization-pinchtab >/dev/null 2>&1 || true

# Bring down the compose stack (this is a no-op if it wasn't started by us)
cd "$(git rev-parse --show-toplevel)/tests/tools" 2>/dev/null || true
docker compose -f docker-compose.yml down --remove-orphans 2>/dev/null || true

echo "optimization benchmark containers removed"
