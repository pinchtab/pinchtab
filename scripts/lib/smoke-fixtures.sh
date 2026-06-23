# shellcheck shell=bash
# Fixture HTTP server startup inside the leg container.
#   start_fixture_server <name> <fixtures_port>  — backgrounds the perl static server
#   wait_fixtures_ready <name> <fixtures_url> <host_fixtures_url> [tries=30]

start_fixture_server() {
  local name="$1"
  local fixtures_port="$2"
  docker exec "$name" sh -lc \
    "FIXTURES_ROOT=/fixtures FIXTURES_PORT=${fixtures_port} /usr/bin/perl /usr/local/bin/fixture-server.pl >/tmp/pinchtab-fixtures.log 2>&1 &"
  FIXTURES_STARTED=1
}

wait_fixtures_ready() {
  local name="$1"
  local fixtures_url="$2"
  local host_fixtures_url="$3"
  local tries="${4:-30}"
  for _ in $(seq 1 "$tries"); do
    if curl -fsS "${host_fixtures_url}/index.html" >/dev/null 2>&1 &&
      docker exec "$name" curl -fsS "${fixtures_url}/index.html" >/dev/null 2>&1; then
      return 0
    fi
    sleep 1
  done
  fail "fixture server was not reachable from the PinchTab container at ${fixtures_url}"
}
