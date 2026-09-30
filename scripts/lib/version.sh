# shellcheck shell=bash

version_ge() {
  local current="${1#v}"
  local minimum="${2#v}"
  local current_major current_minor current_patch
  local minimum_major minimum_minor minimum_patch

  IFS='.' read -r current_major current_minor current_patch <<< "${current}"
  IFS='.' read -r minimum_major minimum_minor minimum_patch <<< "${minimum}"

  current_minor=${current_minor:-0}
  current_patch=${current_patch:-0}
  minimum_minor=${minimum_minor:-0}
  minimum_patch=${minimum_patch:-0}

  if [ "$current_major" -ne "$minimum_major" ]; then
    [ "$current_major" -gt "$minimum_major" ]
    return
  fi

  if [ "$current_minor" -ne "$minimum_minor" ]; then
    [ "$current_minor" -gt "$minimum_minor" ]
    return
  fi

  [ "$current_patch" -ge "$minimum_patch" ]
}
