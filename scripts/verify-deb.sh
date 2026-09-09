#!/usr/bin/env bash
set -euo pipefail

DEB="${1:?usage: verify-deb.sh package.deb}"
TMP="$(mktemp -d)"
trap 'rm -rf "$TMP"' EXIT

dpkg-deb --info "$DEB" >/dev/null
dpkg-deb --contents "$DEB" | grep -q 'usr/bin/kioskmate$'
dpkg-deb --contents "$DEB" | grep -q 'usr/lib/systemd/user/kioskmate.service$'

test "$(dpkg-deb -f "$DEB" Package)" = "kioskmate"
test -n "$(dpkg-deb -f "$DEB" Version)"
case "$(dpkg-deb -f "$DEB" Architecture)" in
  amd64|arm64) ;;
  *) echo "unsupported package architecture" >&2; exit 1 ;;
esac

dpkg-deb --control "$DEB" "$TMP"
test "$(tail -c 1 "$TMP/control" | wc -l)" -eq 1
for script in preinst postinst prerm; do
  test -x "$TMP/$script"
  bash -n "$TMP/$script"
done

if grep -R -E 'sed .*config\.json|127\\?\.0\\?\.0\\?\.1.*0\.0\.0\.0' "$TMP"; then
  echo "maintainer scripts must not rewrite Admin bind settings" >&2
  exit 1
fi

echo "verified $DEB"
