#!/usr/bin/env bash
# Check CHANGELOG.md before a tag is pushed (lesson from 1.0.0-pre.3, whose
# release failed because the section was missing).
#
#   scripts/check-changelog.sh            structure check (CI on every push)
#   scripts/check-changelog.sh v1.2.3     also require a non-empty "## v1.2.3"
set -euo pipefail

FILE="${CHANGELOG:-CHANGELOG.md}"
VERSION="${1:-}"
status=0

headers="$(grep -E '^## ' "$FILE" | sed 's/^## //')"
first_version="$(printf '%s\n' "$headers" | grep -E '^v[0-9]' | head -n 1)"
if [ -z "$first_version" ]; then
  echo "::error file=$FILE::no '## vX.Y.Z' section found"
  exit 1
fi

dupes="$(printf '%s\n' "$headers" | sort | uniq -d)"
if [ -n "$dupes" ]; then
  echo "::error file=$FILE::duplicate sections: $dupes"
  status=1
fi

while IFS= read -r header; do
  case "$header" in
    Unreleased) ;;
    v[0-9]*.[0-9]*.[0-9]*) ;;
    *) echo "::error file=$FILE::unexpected section header '## $header' (use '## vX.Y.Z' or '## Unreleased')"; status=1 ;;
  esac
done <<< "$headers"

section() {
  awk -v version="$1" '
    $0 == "## " version { found=1; next }
    found && /^## / { exit }
    found && NF { print }
  ' "$FILE"
}

if [ -z "$(section "$first_version")" ]; then
  echo "::error file=$FILE::section '## $first_version' is empty"
  status=1
fi

if [ -n "$VERSION" ]; then
  if [ -z "$(section "$VERSION")" ]; then
    echo "::error file=$FILE::no non-empty '## $VERSION' section. Add it on main and tag that commit."
    status=1
  fi
else
  echo "newest release section: $first_version (tag it as $first_version once merged)"
fi

exit "$status"
