# shellcheck shell=bash
# Runs the API basic E2E suite inside a leg's container. Overrides via
# PINCHTAB_PARITY_E2E_SCENARIOS (or PINCHTAB_CLOAKBROWSER_E2E_SCENARIOS on
# the cloak leg, for backwards compat).
# Requires globals: ROOT, RUNNER_IMAGE, TOKEN, FIXTURES_HOST, FIXTURES_URL.

run_e2e_scenarios() {
  local provider="$1"
  local name="$2"

  local -a default_scenarios=(
    "actions-basic.sh"
    "browser-basic.sh"
    "clipboard-basic.sh"
    "console-basic.sh"
    "emulation-basic.sh"
    "files-basic.sh"
    "inspect-basic.sh"
    "tabs-basic.sh"
  )
  local -a scenarios=()
  local -a scenario_args=()

  local override="${PINCHTAB_PARITY_E2E_SCENARIOS:-}"
  if [ -z "$override" ] && [ "$provider" = "cloak" ]; then
    override="${PINCHTAB_CLOAKBROWSER_E2E_SCENARIOS:-}"
  fi

  if [ -n "$override" ]; then
    # shellcheck disable=SC2206
    scenarios=($override)
  else
    scenarios=("${default_scenarios[@]}")
  fi

  for scenario in "${scenarios[@]}"; do
    [ -n "$scenario" ] || continue
    scenario_args+=("scenario=$scenario")
  done

  if [ "${#scenario_args[@]}" -eq 0 ]; then
    echo "Skipping API E2E scenarios (empty scenario list for provider=${provider})."
    return
  fi

  echo "Running API E2E scenarios against provider=${provider} in Docker:"
  printf '  - %s\n' "${scenarios[@]}"

  docker run --rm \
    --network "container:${name}" \
    -v "$ROOT/tests/e2e":/e2e:ro \
    -e "FIXTURES_HOST=${FIXTURES_HOST}" \
    -e "E2E_SERVER=http://127.0.0.1:9867" \
    -e "E2E_SERVER_TOKEN=${TOKEN}" \
    -e "FIXTURES_URL=${FIXTURES_URL}" \
    -e "E2E_HELPER=api" \
    -e "E2E_SCENARIO_DIR=scenarios/api" \
    -e "E2E_REQUIRED_COMMANDS=curl jq" \
    -e "E2E_READY_TARGETS=E2E_SERVER|60|E2E_SERVER_TOKEN" \
    -e "E2E_SUMMARY_TITLE=Browser parity (${provider}) API E2E scenarios" \
    "$RUNNER_IMAGE" \
    /bin/sh -lc 'printf "127.0.0.1 %s\n" "$FIXTURES_HOST" >> /etc/hosts; exec /bin/bash /e2e/run.sh "$@"' \
    _ "${scenario_args[@]}" \
    || fail "API E2E scenarios failed for provider=${provider}"
}
