#!/usr/bin/env bash
#
# Starts the benchmark Docker services needed for the agent-browser lane and
# initializes a fresh report file for the next run.

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
BENCH_DIR="${SCRIPT_DIR}/.."
RESULTS_DIR="${BENCH_DIR}/results"
CURRENT_REPORT_PTR="${RESULTS_DIR}/current_agent_browser_report.txt"
mkdir -p "${RESULTS_DIR}"

TIMESTAMP=$(date +%Y%m%d_%H%M%S)
REPORT_FILE="${RESULTS_DIR}/agent_browser_benchmark_${TIMESTAMP}.json"

cd "${BENCH_DIR}"

echo "Starting benchmark services for agent-browser..."
docker compose up -d --build fixtures agent-browser

echo "Waiting for fixtures to respond from inside the agent-browser container..."
for _ in $(seq 1 30); do
  if docker compose exec -T agent-browser curl -sf http://fixtures/ >/dev/null 2>&1; then
    break
  fi
  sleep 1
done

if ! docker compose exec -T agent-browser curl -sf http://fixtures/ >/dev/null 2>&1; then
  echo "ERROR: fixtures are not reachable from the agent-browser container"
  exit 1
fi

: > "${RESULTS_DIR}/agent_browser_commands.ndjson"

cat > "${REPORT_FILE}" << EOF
{
  "benchmark": {
    "type": "agent-browser",
    "timestamp": "${TIMESTAMP}",
    "driver": "agent-browser",
    "model": "${BENCHMARK_MODEL:-unknown}",
    "runner": "${BENCHMARK_RUNNER:-manual}"
  },
  "totals": {
    "input_tokens": 0,
    "output_tokens": 0,
    "total_tokens": 0,
    "estimated_cost_usd": 0,
    "tool_calls": 0,
    "steps_passed": 0,
    "steps_failed": 0,
    "steps_skipped": 0
  },
  "steps": []
}
EOF

printf '%s\n' "${REPORT_FILE}" > "${CURRENT_REPORT_PTR}"

echo "Initialized agent-browser benchmark report:"
echo "  ${REPORT_FILE}"
echo ""
echo "Next steps:"
echo "  1. Read ./AGENT_BROWSER_INSTRUCTIONS.md"
echo "  2. Load the live CLI skill with ./scripts/ab skills get agent-browser --full"
echo "  3. Use ./scripts/ab ... to drive agent-browser inside Docker"
echo "  4. Record each completed benchmark step with:"
echo "     ./scripts/record-step.sh --type agent-browser <group> <step> <pass|fail|skip|answer> \"what you saw\" \"notes\""
echo "  5. Summarize the report with ./scripts/finalize-report.sh"
