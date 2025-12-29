#!/usr/bin/env bash
set -euo pipefail

# Simple local link checker for Markdown docs.
# - Scans README.md and docs/*.md
# - Resolves relative links per-source file
# - Reports missing local targets; ignores http(s) links

ROOT_DIR=$(cd "$(dirname "$0")/.." && pwd)

broken=0

check_file() {
  local src="$1"
  local base_dir
  base_dir=$(dirname "$src")

  awk 'match($0,/\[[^\]]+\]\(([^)]+)\)/,m){print m[1]}' "$src" \
    | sed 's/#.*$//' \
    | sed 's/^[[:space:]]*//;s/[[:space:]]*$//' \
    | sort -u \
    | while read -r link; do
        [[ -z "$link" ]] && continue
        [[ "$link" =~ ^https?:// ]] && continue

        local path
        if [[ "$link" == /* ]]; then
          path="$ROOT_DIR$link"
        else
          path="$base_dir/$link"
        fi

        if command -v realpath >/dev/null 2>&1; then
          path=$(realpath -m "$path")
        fi

        if [[ -e "$path" ]]; then
          :
        else
          echo "BROKEN: $src -> $link (resolved: $path)"
          broken=$((broken+1))
        fi
      done
}

check_file "$ROOT_DIR/README.md"
for f in "$ROOT_DIR"/docs/*.md; do
  check_file "$f"
done

echo "TOTAL_BROKEN: $broken"
if [[ $broken -gt 0 ]]; then
  exit 1
fi