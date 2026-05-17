#!/usr/bin/env bash
# Symlink every skill under ./skills into ./.claude/skills so Claude Code
# picks them up as repo-local skills (loaded only when CWD is inside this
# repo). Editing files under ./skills/<name>/ is reflected immediately
# because we use symlinks.
#
# Override the target directory with CLAUDE_SKILLS_DIR=<path> if you want
# to install into a different Claude home (e.g. ~/.claude/skills for a
# global install).
set -euo pipefail

cd "$(dirname "$0")/.."

TARGET_DIR="${CLAUDE_SKILLS_DIR:-$(pwd)/.claude/skills}"
SOURCE_DIR="$(pwd)/skills"

if [[ ! -d "$SOURCE_DIR" ]]; then
  echo "no ./skills directory at $SOURCE_DIR" >&2
  exit 1
fi

mkdir -p "$TARGET_DIR"

installed=0
skipped=0
for skill in "$SOURCE_DIR"/*/; do
  [[ -d "$skill" ]] || continue
  name=$(basename "$skill")
  if [[ ! -f "$skill/SKILL.md" ]]; then
    echo "  skip $name (no SKILL.md)"
    skipped=$((skipped + 1))
    continue
  fi

  link="$TARGET_DIR/$name"
  src="${skill%/}"

  if [[ -L "$link" ]]; then
    current=$(readlink "$link")
    if [[ "$current" == "$src" ]]; then
      echo "  ok   $name (already linked)"
      installed=$((installed + 1))
      continue
    fi
    rm "$link"
  elif [[ -e "$link" ]]; then
    echo "  conflict: $link exists and is not a symlink — skipping" >&2
    skipped=$((skipped + 1))
    continue
  fi

  ln -s "$src" "$link"
  echo "  link $name -> $src"
  installed=$((installed + 1))
done

echo
echo "installed $installed skill(s) into $TARGET_DIR (skipped $skipped)"
