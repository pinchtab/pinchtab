#!/usr/bin/env bash
# docker-npm-install-smoke.sh — Clean-room smoke for the published npm install path.
#
# Packages the npm wrapper and runs the real `npm install -g <tarball>` flow
# inside a fresh node container, then boots the CLI and authenticates against
# /health (npm/scripts/npm-cli-e2e.sh). This is the path that a published
# `npm install -g pinchtab` actually takes — postinstall detects it is NOT a
# source checkout, so it resolves the package version and downloads the matching
# release binary. That version-resolution step (readPackageVersion over the
# compiled dist/src layout) is what regressed in #602 and is only exercised on a
# true published install, never in a source checkout, so unit tests alone miss
# it.
#
# Why a container: it is a clean environment — no host node_modules, no host npm
# global prefix, no host HOME state. The wrapper is packed and installed exactly
# as an end user would get it. The host repo is never mutated (the version bump
# for the binary download happens on a throwaway copy inside the container).
#
# The local source tree ships version 0.0.0, which has no release binary, so we
# pack against the latest published release tag purely so postinstall has a real
# binary to fetch. The wrapper CODE under test is still the local working tree.
#
# Skips (exit 77) — never a hard failure — when the environment cannot run it:
#   - docker unavailable / daemon down
#   - no vX.Y.Z release tag to pin the binary to
#   - release assets unreachable (offline / release not published yet)
#
# Usage:
#   ./dev smoke                       # runs as part of the default smoke group
#   bash scripts/docker-npm-install-smoke.sh   # standalone
#
# Env overrides:
#   PINCHTAB_NPM_SMOKE_NODE_IMAGE   Node image to pack/install in (default: node:22-bookworm)
#   PINCHTAB_NPM_SMOKE_VERSION      Override the release version to pin/download

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
ROOT="$(cd "$SCRIPT_DIR/.." && pwd)"

NODE_IMAGE="${PINCHTAB_NPM_SMOKE_NODE_IMAGE:-node:22-bookworm}"
GITHUB_REPO="pinchtab/pinchtab"

CONTAINER="pinchtab-npm-smoke-${RANDOM}${RANDOM}"
TMP_DIR="$(mktemp -d "${TMPDIR:-/tmp}/pinchtab-npm-smoke.XXXXXX")"
CONTAINER_STARTED=0

cleanup() {
  set +e
  if [ "${CONTAINER_STARTED}" -eq 1 ]; then
    docker rm -f "${CONTAINER}" >/dev/null 2>&1 || true
  fi
  rm -rf "${TMP_DIR}"
}
trap cleanup EXIT

skip() {
  echo "SKIP: $*"
  exit 77
}

fail() {
  echo "FAIL: $*" >&2
  exit 1
}

require_cmd() {
  command -v "$1" >/dev/null 2>&1 || fail "$1 not found in PATH"
}

require_cmd git
require_cmd tar
require_cmd curl

command -v docker >/dev/null 2>&1 || skip "docker not found in PATH"
docker info >/dev/null 2>&1 || skip "docker daemon not available"

# Pin the binary download to a real published release. The working-tree version
# (0.0.0) has no release asset, so postinstall would 404 on the download for a
# reason unrelated to the wrapper code we are testing.
VERSION="${PINCHTAB_NPM_SMOKE_VERSION:-}"
if [ -z "${VERSION}" ]; then
  VERSION="$(git -C "${ROOT}" tag --list 'v*' --sort=-v:refname | head -1 | sed 's/^v//')"
fi
if [ -z "${VERSION}" ]; then
  skip "no vX.Y.Z release tag found to resolve a download binary"
fi

# Preflight: are the release assets actually reachable? A clean skip when offline
# or when the release has not been published yet keeps this out of the failure
# column for environmental reasons.
CHECKSUMS_URL="https://github.com/${GITHUB_REPO}/releases/download/v${VERSION}/checksums.txt"
if ! curl -fsIL --max-time 20 -o /dev/null "${CHECKSUMS_URL}"; then
  skip "release assets for v${VERSION} unreachable (offline or not yet published)"
fi

echo "Packaging npm wrapper (working tree) against release v${VERSION} in ${NODE_IMAGE}"

# Clean export of the wrapper: tracked working-tree state minus build artifacts.
# Nothing from the host node_modules/dist/tarballs comes along, so the container
# rebuilds from a pristine state via `npm ci`. The repo layout is preserved (npm/
# alongside skills/) because prepack's stage-skills.js copies the SKILL source
# from the repo-root skills/ dir two levels above npm/scripts.
tar -C "${ROOT}" \
  --exclude='npm/node_modules' \
  --exclude='npm/dist' \
  --exclude='npm/pinchtab-*.tgz' \
  -cf "${TMP_DIR}/pkg.tar" npm skills

# The in-container recipe: extract, install deps from the lockfile, bump the
# version to the pinned release, pack, then run the real global-install e2e.
cat > "${TMP_DIR}/run.sh" <<'RECIPE'
set -euo pipefail
mkdir -p /work
tar -xf /tmp/pkg.tar -C /work
cd /work/npm

echo "==> npm ci"
npm ci

echo "==> pin version to v${TARGET_VERSION} for the binary download"
npm version "${TARGET_VERSION}" --no-git-tag-version --allow-same-version >/dev/null

echo "==> npm pack"
npm pack >/dev/null
TARBALL="/work/npm/$(ls -t pinchtab-*.tgz | head -1)"
echo "    packed ${TARBALL}"

echo "==> npm install -g + CLI auth e2e"
bash scripts/npm-cli-e2e.sh "${TARBALL}"
RECIPE

docker run -d --name "${CONTAINER}" "${NODE_IMAGE}" sleep 900 >/dev/null
CONTAINER_STARTED=1

docker cp "${TMP_DIR}/pkg.tar" "${CONTAINER}:/tmp/pkg.tar" >/dev/null
docker cp "${TMP_DIR}/run.sh" "${CONTAINER}:/tmp/run.sh" >/dev/null

if ! docker exec -e TARGET_VERSION="${VERSION}" "${CONTAINER}" bash /tmp/run.sh; then
  fail "npm install e2e failed inside ${NODE_IMAGE}"
fi

echo ""
echo "npm install e2e passed (packaged + installed v${VERSION} in a clean ${NODE_IMAGE} container)"
