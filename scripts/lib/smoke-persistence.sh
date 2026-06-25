# shellcheck shell=bash
# Profile-persistence round-trip helpers (P3a). Verifies localStorage + cookie
# survive a container stop/restart when /data is backed by a Docker volume.
# Mutates globals: CONTAINER_STARTED, FIXTURES_STARTED, HOST_PORT.

setup_profile_volume() {
  local name="$1"
  [ -n "$name" ] || fail "setup_profile_volume: name required"
  # Remove any stale volume so we start from an empty profile.
  if docker volume inspect "$name" >/dev/null 2>&1; then
    docker volume rm "$name" >/dev/null 2>&1 || true
  fi
  docker volume create "$name" >/dev/null || fail "failed to create docker volume $name"
  echo "$name"
}

cleanup_profile_volume() {
  local name="$1"
  [ -n "$name" ] || return 0
  if ! docker volume inspect "$name" >/dev/null 2>&1; then
    return 0
  fi
  # Tolerate "volume in use" so a failed leg doesn't mask the original error.
  if ! docker volume rm "$name" >/dev/null 2>&1; then
    echo "  warn: docker volume rm $name failed (likely still in use); leaving for manual cleanup" >&2
  fi
}

_read_marker_from_managed_instance() {
  local host_port="$1"
  local token="$2"
  local fixtures_url="$3"

  local nav_body
  nav_body="$(curl -fsS -X POST \
    -H "Authorization: Bearer ${token}" \
    -H "Content-Type: application/json" \
    -d "{\"url\":\"${fixtures_url}/persistence-marker.html\",\"waitFor\":\"selector\",\"waitSelector\":\"#marker\"}" \
    "http://127.0.0.1:${host_port}/navigate" 2>&1)" \
    || fail "navigate to persistence-marker.html failed: $nav_body"

  local tab_id
  tab_id="$(echo "$nav_body" | jq -r '.tabId // empty')"
  [ -n "$tab_id" ] || fail "navigate response missing tabId: $nav_body"

  local html_body
  html_body="$(curl -fsS \
    -H "Authorization: Bearer ${token}" \
    "http://127.0.0.1:${host_port}/tabs/${tab_id}/html?selector=%23marker" 2>&1)" \
    || fail "GET /tabs/${tab_id}/html for #marker failed: $html_body"

  local raw_html
  raw_html="$(echo "$html_body" | jq -r '.html // empty')"
  [ -n "$raw_html" ] || fail "marker html was empty: $html_body"

  local marker
  marker="$(printf '%s' "$raw_html" | sed -E 's/<[^>]+>//g' | tr -d '[:space:]')"
  [ -n "$marker" ] || fail "marker text was empty after strip: $raw_html"
  [ "$marker" != "pending" ] || fail "marker text was still 'pending' (fixture JS did not run): $raw_html"

  echo "$marker"
}

assert_persistence_round_trip() {
  local provider="$1"
  local image="$2"
  local name="$3"
  local config_path="$4"
  local fixtures_host="$5"
  local fixtures_port="$6"
  local fixtures_url="$7"
  local host_fixtures_url="$8"
  local token="$9"
  local root="${10}"
  local profile_volume="${11}"

  [ -n "$profile_volume" ] || fail "assert_persistence_round_trip requires a profile volume"

  echo "→ Phase 1: capture marker (fresh volume, first boot)"
  local first_marker
  first_marker="$(_read_marker_from_managed_instance "$HOST_PORT" "$token" "$fixtures_url")"
  echo "  first-boot marker: $first_marker"

  echo "→ Phase 2: stop container (graceful) to flush profile to volume $profile_volume"
  # Must stop the managed instance via the API first: only the full graceful
  # path (Browser.close + wait-for-exit) makes Chrome flush localStorage to
  # leveldb. `docker stop` alone is too fast.
  local inst_id
  inst_id="$(curl -fsS -H "Authorization: Bearer ${token}" \
    "http://127.0.0.1:${HOST_PORT}/instances" 2>/dev/null \
    | jq -r '.[]? | select(.status=="running") | .id' | head -1)"
  if [ -n "$inst_id" ]; then
    echo "  stopping managed instance $inst_id"
    curl -fsS -X POST -H "Authorization: Bearer ${token}" \
      "http://127.0.0.1:${HOST_PORT}/instances/${inst_id}/stop" >/dev/null 2>&1 || true
    local i
    for i in $(seq 1 30); do
      local still
      still="$(curl -fsS -H "Authorization: Bearer ${token}" \
        "http://127.0.0.1:${HOST_PORT}/instances/${inst_id}" 2>/dev/null \
        | jq -r '.status // "gone"')"
      if [ "$still" != "running" ]; then
        break
      fi
      sleep 1
    done
  fi
  # Extend grace period beyond the 10s default in case I/O is still in flight.
  docker stop --time 30 "$name" >/dev/null || fail "docker stop $name failed"
  teardown_container "$name"
  CONTAINER_STARTED=0
  FIXTURES_STARTED=0

  echo "→ Phase 3: restart container with identical name + config + token + volume"
  PROFILE_VOLUME="$profile_volume" start_provider_container \
    "$name" "$image" "$config_path" "$fixtures_host" "$fixtures_port" "$root"
  HOST_PORT="$(resolve_host_port "$name")"
  echo "  Container $name restarted — PinchTab on host port $HOST_PORT"

  start_fixture_server "$name" "$fixtures_port"
  wait_fixtures_ready "$name" "$fixtures_url" "$host_fixtures_url"

  echo "  Waiting for PinchTab health on :${HOST_PORT}..."
  wait_for_health "$HOST_PORT" "$token"
  echo "  Waiting for managed ${provider} instance (post-restart)..."
  wait_for_instance_running "$HOST_PORT" "$token"

  echo "→ Phase 4: re-read marker and assert equality"
  local second_marker
  second_marker="$(_read_marker_from_managed_instance "$HOST_PORT" "$token" "$fixtures_url")"
  echo "  post-restart marker: $second_marker"

  if [ "$first_marker" != "$second_marker" ]; then
    fail "profile persistence round-trip failed: first=${first_marker} second=${second_marker}"
  fi
  echo "  marker matches across restart — localStorage + cookie persisted via $profile_volume"
}

# Stale profile-lock recovery (P3b) helpers. After the P3a round-trip,
# plants SingletonLock/Socket/Cookie referencing an unused PID and asserts
# PinchTab recovers. Plant container chowns to 1000:1000 so the non-root
# pinchtab user can later read+delete the files.

_pick_unused_pid() {
  local pid=99999
  local guard=0
  while kill -0 "$pid" 2>/dev/null; do
    pid=$((RANDOM + 10000))
    guard=$((guard + 1))
    if [ "$guard" -gt 100 ]; then
      fail "could not find an unused PID after 100 attempts"
    fi
  done
  echo "$pid"
}

# Plants the three Singleton* files inside the volume. profile_subpath is
# volume-relative (no leading slash), e.g. "profiles/cloak-docker-smoke".
plant_stale_lock() {
  local volume_name="$1"
  local profile_subpath="$2"
  local fake_pid="$3"

  [ -n "$volume_name" ]    || fail "plant_stale_lock: volume_name required"
  [ -n "$profile_subpath" ] || fail "plant_stale_lock: profile_subpath required"
  [ -n "$fake_pid" ]       || fail "plant_stale_lock: fake_pid required"

  # Short-lived root container so we can chown to 1000:1000; the PinchTab
  # container itself never runs as root.
  docker run --rm \
    --user 0:0 \
    -v "${volume_name}:/data:rw" \
    -e "PROFILE_SUBPATH=${profile_subpath}" \
    -e "FAKE_PID=${fake_pid}" \
    alpine:3.20 sh -lc '
      set -eu
      profile_dir="/data/${PROFILE_SUBPATH}"
      if [ ! -d "$profile_dir" ]; then
        echo "plant_stale_lock: profile dir $profile_dir does not exist; current /data layout:" >&2
        find /data -maxdepth 3 -type d -printf "%p\n" 2>/dev/null | sed -n "1,40p" >&2
        exit 1
      fi
      # Sentinel "STALE_PLANT_" lets assert_lock_files_gone distinguish
      # planted files from fresh ones Chrome re-creates after recovery.
      content="STALE_PLANT_${FAKE_PID}_$(date +%s)"
      for f in SingletonLock SingletonSocket SingletonCookie; do
        printf "%s" "$content" > "$profile_dir/$f"
      done
      chown -R 1000:1000 "$profile_dir"
      echo "planted stale lock files in $profile_dir (pid=$FAKE_PID)"
      ls -la "$profile_dir" | sed -n "1,20p"
    ' >&2 || fail "plant_stale_lock: helper container failed"
}

# Asserts no planted sentinel files remain. Chrome may re-create fresh
# Singleton* files post-recovery; only those still carrying the
# "STALE_PLANT_" sentinel indicate recovery missed them.
assert_lock_files_gone() {
  local volume_name="$1"
  local profile_subpath="$2"

  [ -n "$volume_name" ]    || fail "assert_lock_files_gone: volume_name required"
  [ -n "$profile_subpath" ] || fail "assert_lock_files_gone: profile_subpath required"

  local out
  out="$(docker run --rm \
    -v "${volume_name}:/data:ro" \
    -e "PROFILE_SUBPATH=${profile_subpath}" \
    alpine:3.20 sh -lc '
      set -eu
      profile_dir="/data/${PROFILE_SUBPATH}"
      if [ ! -d "$profile_dir" ]; then
        echo "MISSING_DIR"
        exit 0
      fi
      survivors=""
      for f in SingletonLock SingletonSocket SingletonCookie; do
        if [ -e "$profile_dir/$f" ] || [ -L "$profile_dir/$f" ]; then
          # Skip symlinks: Chrome re-creates SingletonLock as a symlink to
          # its own live PID — healthy state, not a stale plant.
          if [ -L "$profile_dir/$f" ]; then
            continue
          fi
          if grep -q "STALE_PLANT_" "$profile_dir/$f" 2>/dev/null; then
            survivors="${survivors}${f} "
          fi
        fi
      done
      if [ -n "$survivors" ]; then
        echo "SURVIVORS: $survivors"
        ls -la "$profile_dir" | sed -n "1,40p"
        exit 1
      fi
      echo "OK"
    ' 2>&1)" || fail "assert_lock_files_gone: stale singleton files remain in ${profile_subpath}: $out"

  echo "  $out"
}

# Full lock-recovery round-trip on top of P3a: stops the container, plants
# stale Singleton* files, restarts, and asserts health, preserved marker,
# and that the planted sentinel files were cleared.
assert_lock_recovery_round_trip() {
  local provider="$1"
  local image="$2"
  local name="$3"
  local config_path="$4"
  local fixtures_host="$5"
  local fixtures_port="$6"
  local fixtures_url="$7"
  local host_fixtures_url="$8"
  local token="$9"
  local root="${10}"
  local profile_volume="${11}"
  local profile_subpath="${12}"

  [ -n "$profile_volume" ]  || fail "assert_lock_recovery_round_trip requires a profile volume"
  [ -n "$profile_subpath" ] || fail "assert_lock_recovery_round_trip requires a profile subpath"

  assert_persistence_round_trip \
    "$provider" "$image" "$name" "$config_path" \
    "$fixtures_host" "$fixtures_port" \
    "$fixtures_url" "$host_fixtures_url" \
    "$token" "$root" "$profile_volume"

  local seeded_marker
  seeded_marker="$(_read_marker_from_managed_instance "$HOST_PORT" "$token" "$fixtures_url")"
  echo "→ Phase L1: captured post-P3a marker for recovery leg: $seeded_marker"

  echo "→ Phase L2: stop container to plant stale lock on volume ${profile_volume}"
  local inst_id
  inst_id="$(curl -fsS -H "Authorization: Bearer ${token}" \
    "http://127.0.0.1:${HOST_PORT}/instances" 2>/dev/null \
    | jq -r '.[]? | select(.status=="running") | .id' | head -1)"
  if [ -n "$inst_id" ]; then
    curl -fsS -X POST -H "Authorization: Bearer ${token}" \
      "http://127.0.0.1:${HOST_PORT}/instances/${inst_id}/stop" >/dev/null 2>&1 || true
    local i
    for i in $(seq 1 30); do
      local still
      still="$(curl -fsS -H "Authorization: Bearer ${token}" \
        "http://127.0.0.1:${HOST_PORT}/instances/${inst_id}" 2>/dev/null \
        | jq -r '.status // "gone"')"
      [ "$still" != "running" ] && break
      sleep 1
    done
  fi
  docker stop --time 30 "$name" >/dev/null || fail "docker stop $name failed"
  teardown_container "$name"
  CONTAINER_STARTED=0
  FIXTURES_STARTED=0

  echo "→ Phase L3: plant stale singleton files in ${profile_subpath}"
  local fake_pid
  fake_pid="$(_pick_unused_pid)"
  echo "  using fake pid=${fake_pid} (verified not alive on host)"
  plant_stale_lock "$profile_volume" "$profile_subpath" "$fake_pid"

  echo "→ Phase L4: restart container — PinchTab must detect + clear stale lock"
  PROFILE_VOLUME="$profile_volume" start_provider_container \
    "$name" "$image" "$config_path" "$fixtures_host" "$fixtures_port" "$root"
  HOST_PORT="$(resolve_host_port "$name")"
  echo "  Container $name restarted — PinchTab on host port $HOST_PORT"

  start_fixture_server "$name" "$fixtures_port"
  wait_fixtures_ready "$name" "$fixtures_url" "$host_fixtures_url"

  echo "  Waiting for PinchTab health on :${HOST_PORT} (post stale-lock plant)..."
  wait_for_health "$HOST_PORT" "$token"
  echo "  Waiting for managed ${provider} instance (post-recovery)..."
  wait_for_instance_running "$HOST_PORT" "$token"

  echo "→ Phase L5: re-read marker and assert it still matches seeded UUID"
  local post_recovery_marker
  post_recovery_marker="$(_read_marker_from_managed_instance "$HOST_PORT" "$token" "$fixtures_url")"
  echo "  post-recovery marker: $post_recovery_marker"
  if [ "$seeded_marker" != "$post_recovery_marker" ]; then
    fail "lock-recovery round-trip failed: seeded=${seeded_marker} post=${post_recovery_marker}"
  fi
  echo "  marker survives stale-lock recovery — user state intact"

  echo "→ Phase L6: assert all three singleton files cleared by recovery"
  assert_lock_files_gone "$profile_volume" "$profile_subpath"
  echo "  recovery confirmed: SingletonLock/SingletonSocket/SingletonCookie removed"
}
