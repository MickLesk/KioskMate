#!/usr/bin/env bash
set -euo pipefail

DEB="${1:?usage: verify-deb.sh package.deb}"
TMP="$(mktemp -d)"
trap 'rm -rf "$TMP"' EXIT

dpkg-deb --info "$DEB" >/dev/null
CONTENTS="$TMP/contents"
# Read the archive completely before grepping; pipefail plus grep -q can close
# stdout early and make dpkg-deb/tar report a misleading broken-pipe error.
dpkg-deb --contents "$DEB" > "$CONTENTS"
grep -q 'usr/bin/kioskmate$' "$CONTENTS"
grep -q 'usr/lib/systemd/user/kioskmate.service$' "$CONTENTS"

test "$(dpkg-deb -f "$DEB" Package)" = "kioskmate"
VERSION="$(dpkg-deb -f "$DEB" Version)"
ARCH="$(dpkg-deb -f "$DEB" Architecture)"
test -n "$VERSION"
case "$ARCH" in
  amd64|arm64) ;;
  *) echo "unsupported package architecture" >&2; exit 1 ;;
esac

dpkg-deb --control "$DEB" "$TMP"
test "$(tail -c 1 "$TMP/control" | wc -l)" -eq 1
for script in preinst postinst prerm postrm; do
  test -x "$TMP/$script"
  bash -n "$TMP/$script"
done

ROOTFS="$TMP/rootfs"
mkdir -p "$ROOTFS"
dpkg-deb --extract "$DEB" "$ROOTFS"
test -x "$ROOTFS/usr/bin/kioskmate"
test -r "$ROOTFS/usr/lib/systemd/user/kioskmate.service"
if [ "$ARCH" = "$(dpkg --print-architecture)" ]; then
  test "$("$ROOTFS/usr/bin/kioskmate" --version)" = "$VERSION"
fi

if grep -E 'sed .*config\.json|127\\?\.0\\?\.0\\?\.1.*0\.0\.0\.0' "$TMP"/{preinst,postinst,prerm,postrm}; then
  echo "maintainer scripts must not rewrite Admin bind settings" >&2
  exit 1
fi

echo "verified $DEB"
