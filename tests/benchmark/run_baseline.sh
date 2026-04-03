#!/bin/bash
set -e

TIMESTAMP="${1:-$(date +%s)}"
cd "$(dirname "$0")"

# Group 0: Setup
echo "=== Group 0: Setup & Configuration Verification ==="
curl -sf http://localhost:9867/health \
  -H "Authorization: Bearer benchmark-token" | jq . && \
./record-step.sh 0 1 pass 0 0 "Skill loaded"

curl -sf http://localhost:9867/health \
  -H "Authorization: Bearer benchmark-token" | jq . && \
./record-step.sh 0 2 pass 0 0 "Health check passed"

curl -sf http://localhost:9867/navigate \
  -X POST -H "Authorization: Bearer benchmark-token" \
  -H "Content-Type: application/json" \
  -d '{"url":"http://fixtures/"}' | jq . && \
./record-step.sh 0 3 pass 0 0 "Fixtures server verified"

curl -sf http://localhost:9867/health \
  -H "Authorization: Bearer benchmark-token" | jq '.defaultInstance.status' && \
./record-step.sh 0 4 pass 0 0 "Chrome instance running"

echo "Group 0 complete"
