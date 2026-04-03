#!/bin/bash
#
# PinchTab Agent Benchmark Runner
#
# This script sets up the benchmark environment and runs the agent benchmark.
# It tracks token usage via the LLM provider's API and produces a structured report.
#
# Usage:
#   ./run-benchmark.sh [options]
#
# Options:
#   --model MODEL       Model to use (default: claude-sonnet-4)
#   --provider PROVIDER Provider (anthropic, openai, gemini)
#   --output DIR        Output directory for results (default: ./results)
#   --skip-docker       Skip Docker setup, use existing server
#   --server URL        PinchTab server URL (default: http://localhost:9867)
#   --token TOKEN       PinchTab auth token (default: benchmark-token)
#   --help              Show this help

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
RESULTS_DIR="${SCRIPT_DIR}/results"
TIMESTAMP=$(date +%Y%m%d_%H%M%S)
REPORT_FILE="${RESULTS_DIR}/benchmark_${TIMESTAMP}.json"
SUMMARY_FILE="${RESULTS_DIR}/benchmark_${TIMESTAMP}_summary.md"

# Defaults
MODEL="${MODEL:-claude-sonnet-4}"
PROVIDER="${PROVIDER:-anthropic}"
SKIP_DOCKER="${SKIP_DOCKER:-false}"
PINCHTAB_SERVER="${PINCHTAB_SERVER:-http://localhost:9867}"
PINCHTAB_TOKEN="${PINCHTAB_TOKEN:-benchmark-token}"

# Parse arguments
while [[ $# -gt 0 ]]; do
    case $1 in
        --model) MODEL="$2"; shift 2 ;;
        --provider) PROVIDER="$2"; shift 2 ;;
        --output) RESULTS_DIR="$2"; shift 2 ;;
        --skip-docker) SKIP_DOCKER=true; shift ;;
        --server) PINCHTAB_SERVER="$2"; shift 2 ;;
        --token) PINCHTAB_TOKEN="$2"; shift 2 ;;
        --help) 
            head -25 "$0" | tail -20
            exit 0
            ;;
        *) echo "Unknown option: $1"; exit 1 ;;
    esac
done

mkdir -p "${RESULTS_DIR}"

echo "=== PinchTab Agent Benchmark ==="
echo "Model: ${MODEL}"
echo "Provider: ${PROVIDER}"
echo "Results: ${RESULTS_DIR}"
echo "Timestamp: ${TIMESTAMP}"
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
fi

# Verify connectivity
echo ""
echo "Verifying PinchTab connectivity..."
HEALTH=$(curl -sf -H "Authorization: Bearer ${PINCHTAB_TOKEN}" \
    "${PINCHTAB_SERVER}/health" 2>/dev/null || echo '{"status":"error"}')
echo "Health: ${HEALTH}"

VERSION=$(curl -sf -H "Authorization: Bearer ${PINCHTAB_TOKEN}" \
    "${PINCHTAB_SERVER}/version" 2>/dev/null || echo '{"version":"unknown"}')
echo "Version: ${VERSION}"

# Initialize report
cat > "${REPORT_FILE}" << EOF
{
  "benchmark": {
    "timestamp": "${TIMESTAMP}",
    "model": "${MODEL}",
    "provider": "${PROVIDER}",
    "pinchtab_server": "${PINCHTAB_SERVER}",
    "pinchtab_version": ${VERSION}
  },
  "totals": {
    "input_tokens": 0,
    "output_tokens": 0,
    "total_tokens": 0,
    "estimated_cost_usd": 0,
    "steps_passed": 0,
    "steps_failed": 0,
    "steps_skipped": 0,
    "duration_ms": 0
  },
  "groups": [],
  "steps": []
}
EOF

echo ""
echo "=== Benchmark Environment Ready ==="
echo ""
echo "To run the benchmark, execute the AGENT_SCRIPT.md steps using your agent."
echo "For each step, record results using the helper script:"
echo ""
echo "  ./record-step.sh <group> <step> <pass|fail|skip> <input_tokens> <output_tokens> [notes]"
echo ""
echo "Example:"
echo "  ./record-step.sh 1 1 pass 150 45 'Navigation completed in 1.2s'"
echo ""
echo "When finished, generate the summary with:"
echo "  ./finalize-report.sh"
echo ""
echo "Report file: ${REPORT_FILE}"
