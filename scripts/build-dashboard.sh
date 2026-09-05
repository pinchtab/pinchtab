#!/bin/bash
# Build React dashboard and copy to internal/dashboard/dashboard/
set -e

cd "$(dirname "$0")/.."

BUN_MIN_VERSION="1.2.0"

confirm() {
  if [ ! -t 0 ]; then
    return 1
  fi

  echo -n "$1 [y/N] "
  read -r answer
  [[ "$answer" =~ ^[Yy]$ ]]
}

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

if ! command -v bun &> /dev/null; then
  echo "❌ ERROR: Bun is required to build the dashboard"
  echo "   Install it with: curl -fsSL https://bun.sh/install | bash"
  exit 1
fi

BUN_VERSION="$(bun -v)"
if ! version_ge "$BUN_VERSION" "$BUN_MIN_VERSION"; then
  echo "❌ ERROR: Bun $BUN_VERSION is too old for dashboard/bun.lock"
  echo "   Clean installs require Bun $BUN_MIN_VERSION+"
  echo "   Upgrade with: curl -fsSL https://bun.sh/install | bash"
  exit 1
fi

echo "📦 Building React dashboard..."
cd dashboard

# Sync deps on every build so stale dashboard node_modules do not drift from bun.lock.
echo "📥 Syncing dashboard dependencies..."
bun install --frozen-lockfile

# Generate TypeScript types from Go structs (ensures types are in sync)
TYGO="${GOPATH:-$HOME/go}/bin/tygo"
if [ -x "$TYGO" ]; then
  echo "🔄 Generating TypeScript types..."
  "$TYGO" generate
elif command -v tygo &> /dev/null; then
  echo "🔄 Generating TypeScript types..."
  tygo generate
else
  if command -v go &> /dev/null && confirm "Install tygo via go install?"; then
    go install github.com/gzuidhof/tygo@latest
    TYGO="${GOPATH:-$HOME/go}/bin/tygo"
    if [ -x "$TYGO" ]; then
      echo "🔄 Generating TypeScript types..."
      "$TYGO" generate
    elif command -v tygo &> /dev/null; then
      echo "🔄 Generating TypeScript types..."
      tygo generate
    else
      echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
      echo "⚠️  WARNING: tygo install completed but binary was not found on PATH"
      echo "   Restart your shell or ensure \$GOPATH/bin is on PATH."
      echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
    fi
  else
    echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
    echo "⚠️  WARNING: tygo not found — skipping TypeScript type generation"
    echo "   Types in the dashboard might fall out of sync with Go structs."
    echo "   Install it with: go install github.com/gzuidhof/tygo@latest"
    echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
  fi
fi

# Normalize tygo output with prettier so generation doesn't dirty git
if [ -f "src/generated/types.ts" ]; then
  bun run types:format
fi

bun run build

echo "📋 Copying build to internal/dashboard/dashboard/..."
cd ..

# Clear old dashboard assets (keep favicon.png)
rm -rf internal/dashboard/dashboard/assets/
rm -f internal/dashboard/dashboard/dashboard.html

# Copy React build and rename entry HTML.
# Vite always outputs index.html; Go's embed serves it as dashboard.html to
# avoid http.FileServer's automatic index.html handling at /dashboard/.
cp -r dashboard/dist/* internal/dashboard/dashboard/
mv internal/dashboard/dashboard/index.html internal/dashboard/dashboard/dashboard.html

# Stamp the bundle with the hash of the source it was built from. The inputs are
# declared once in dashboard/bundle-inputs.txt; the Go side
# (internal/dashboard.SourceStamp) reads the same file and hashes the same way,
# and the unit suite fails when the embedded stamp no longer matches the tree.
sha256_stdin() {
  if command -v sha256sum &> /dev/null; then
    sha256sum | cut -d' ' -f1
  else
    shasum -a 256 | cut -d' ' -f1
  fi
}
bundle_inputs() {
  local input
  grep -v '^[[:space:]]*#' dashboard/bundle-inputs.txt | grep -v '^[[:space:]]*$' | while IFS= read -r input; do
    [ -e "dashboard/$input" ] || continue
    if [ -d "dashboard/$input" ]; then
      find "dashboard/$input" -type f -not -path '*/.*'
    else
      echo "dashboard/$input"
    fi
  done | sed 's#^dashboard/##' | LC_ALL=C sort
}
STAMP="$(bundle_inputs | while IFS= read -r rel; do
  printf '%s\0%s\n' "$rel" "$(sha256_stdin < "dashboard/$rel")"
done | sha256_stdin)"
echo "$STAMP" > internal/dashboard/dashboard/bundle.stamp
echo "🔖 Bundle stamp: $STAMP"

echo "✅ Dashboard built: internal/dashboard/dashboard/"
