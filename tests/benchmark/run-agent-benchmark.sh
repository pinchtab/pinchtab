#!/bin/bash
#
# PinchTab Agent Benchmark Runner with Subagent
#
# Spawns a fresh subagent with clean context to run the benchmark.
# Tracks total token cost including skill loading overhead.
#
# Usage:
#   ./run-agent-benchmark.sh [options]
#
# Options:
#   --model MODEL       Model to use (default: claude-sonnet-4)
#   --skip-docker       Skip Docker setup, use existing server
#   --dry-run           Show what would run without executing
#   --help              Show this help

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
RESULTS_DIR="${SCRIPT_DIR}/results"
TIMESTAMP=$(date +%Y%m%d_%H%M%S)

# Defaults
MODEL="${MODEL:-claude-sonnet-4}"
SKIP_DOCKER="${SKIP_DOCKER:-false}"
DRY_RUN="${DRY_RUN:-false}"
PINCHTAB_SERVER="${PINCHTAB_SERVER:-http://localhost:9867}"
PINCHTAB_TOKEN="${PINCHTAB_TOKEN:-benchmark-token}"

# Parse arguments
while [[ $# -gt 0 ]]; do
    case $1 in
        --model) MODEL="$2"; shift 2 ;;
        --skip-docker) SKIP_DOCKER=true; shift ;;
        --dry-run) DRY_RUN=true; shift ;;
        --help) 
            head -20 "$0" | tail -15
            exit 0
            ;;
        *) echo "Unknown option: $1"; exit 1 ;;
    esac
done

mkdir -p "${RESULTS_DIR}"

echo "=== PinchTab Agent Benchmark ==="
echo "Model: ${MODEL}"
echo "Timestamp: ${TIMESTAMP}"
echo "Results: ${RESULTS_DIR}"
echo ""

# Start Docker environment if needed
if [[ "${SKIP_DOCKER}" != "true" ]]; then
    echo "Starting PinchTab Docker environment..."
    cd "${SCRIPT_DIR}"
    docker compose down --remove-orphans 2>/dev/null || true
    docker compose build
    docker compose up -d
    
    echo "Waiting for PinchTab to be healthy..."
    for i in {1..60}; do
        if curl -sf -H "Authorization: Bearer ${PINCHTAB_TOKEN}" \
           "${PINCHTAB_SERVER}/health" >/dev/null 2>&1; then
            echo "PinchTab is ready!"
            break
        fi
        if [[ $i -eq 60 ]]; then
            echo "ERROR: PinchTab failed to start"
            docker compose logs pinchtab
            exit 1
        fi
        sleep 2
    done
else
    echo "Skipping Docker setup (--skip-docker)"
fi

# Verify connectivity
echo ""
echo "Verifying PinchTab connectivity..."
VERSION=$(curl -sf -H "Authorization: Bearer ${PINCHTAB_TOKEN}" \
    "${PINCHTAB_SERVER}/version" 2>/dev/null | jq -r '.version // "unknown"')
echo "PinchTab Version: ${VERSION}"

# Initialize report
REPORT_FILE="${RESULTS_DIR}/benchmark_${TIMESTAMP}.json"
cat > "${REPORT_FILE}" << EOF
{
  "benchmark": {
    "timestamp": "${TIMESTAMP}",
    "model": "${MODEL}",
    "pinchtab_version": "${VERSION}"
  },
  "totals": {
    "input_tokens": 0,
    "output_tokens": 0,
    "total_tokens": 0,
    "estimated_cost_usd": 0,
    "steps_passed": 0,
    "steps_failed": 0,
    "steps_skipped": 0
  },
  "groups": [],
  "steps": []
}
EOF

if [[ "${DRY_RUN}" == "true" ]]; then
    echo ""
    echo "[DRY RUN] Would spawn subagent with task:"
    echo "---"
    cat << 'TASK'
Read the benchmark skill at tests/benchmark/skill/SKILL.md and execute all tasks.
For each step:
1. Run the curl command
2. Check the pass/fail criteria
3. Record result with: ./record-step.sh <group> <step> <pass|fail> <input_tokens> <output_tokens> "notes"

Start with step 0.1 to record the skill loading cost.
After all steps, run ./finalize-report.sh

Work from directory: tests/benchmark/
TASK
    echo "---"
    exit 0
fi

echo ""
echo "=== Spawning Benchmark Agent ==="
echo ""

# The subagent task
TASK=$(cat << 'EOF'
You are running the PinchTab benchmark. Your working directory is tests/benchmark/

1. First, read the skill file at skill/SKILL.md
2. Record step 0.1 with the tokens used to load and understand the skill
3. Execute each task group in order (1-8)
4. For each step:
   - Run the exact curl command shown
   - Check the response against pass/fail criteria
   - Record with: ./record-step.sh <group> <step> <pass|fail> <input_tokens> <output_tokens> "brief notes"
5. After all steps, run: ./finalize-report.sh
6. Report the final summary

Be precise. Record accurate token counts from your usage. Do not skip steps.
EOF
)

# If using OpenClaw, spawn subagent
if command -v openclaw &> /dev/null; then
    echo "Spawning via OpenClaw sessions_spawn..."
    # This would be called via the OpenClaw API
    echo "Task: ${TASK}"
    echo ""
    echo "NOTE: Run this task manually or integrate with your agent framework."
else
    echo "OpenClaw not found. Run the benchmark manually:"
    echo ""
    echo "1. cd ${SCRIPT_DIR}"
    echo "2. Read skill/SKILL.md"
    echo "3. Execute each step and record with ./record-step.sh"
    echo "4. Run ./finalize-report.sh when done"
fi

echo ""
echo "Report will be written to: ${REPORT_FILE}"
