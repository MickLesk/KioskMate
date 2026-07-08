#!/usr/bin/env bash
set -euo pipefail

VERSION="${VERSION:-0.0.0-dev}"
ARCH="${ARCH:-$(dpkg --print-architecture)}"
ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
OUT="${ROOT}/dist"
PKG="${OUT}/kioskmate_${VERSION}_${ARCH}"

case "$ARCH" in
  amd64) GOARCH="amd64" ;;
  arm64) GOARCH="arm64" ;;
  *) echo "Unsupported architecture: $ARCH" >&2; exit 1 ;;
esac

rm -rf "$PKG"
mkdir -p "$PKG/DEBIAN" "$PKG/usr/bin" "$PKG/usr/share/doc/kioskmate" "$PKG/usr/lib/systemd/user" "$OUT"

GOOS=linux GOARCH="$GOARCH" go build -trimpath -ldflags="-s -w -X main.version=${VERSION}" -o "$PKG/usr/bin/kioskmate" ./cmd/kioskmate
cp "$ROOT/README.md" "$PKG/usr/share/doc/kioskmate/README.md"
cp "$ROOT/packaging/systemd/kioskmate.service" "$PKG/usr/lib/systemd/user/kioskmate.service"

cat > "$PKG/DEBIAN/control" <<CONTROL
Package: kioskmate
Version: ${VERSION}
Section: net
Priority: optional
Architecture: ${ARCH}
Maintainer: MickLesk
Depends: chromium | chromium-browser | google-chrome-stable, fonts-noto-color-emoji
Recommends: wlopm | kscreen, pipewire-pulse | pulseaudio
Description: KioskMate browser supervisor for Home Assistant kiosks
 Go-based supervisor, Admin API and watchdog for an external kiosk browser.
CONTROL

cat > "$PKG/DEBIAN/preinst" <<'PREINST'
#!/usr/bin/env bash
set -e
backup_config() {
  FILE="$1"
  [ -f "$FILE" ] || return 0
  cp -p "$FILE" "$FILE.bak" >/dev/null 2>&1 || true
}
for HOME_DIR in /home/*; do
  [ -d "$HOME_DIR" ] || continue
  backup_config "$HOME_DIR/.config/kioskmate/config.json"
done
exit 0
PREINST
chmod 0755 "$PKG/DEBIAN/preinst"

cat > "$PKG/DEBIAN/postinst" <<'POSTINST'
#!/usr/bin/env bash
set -e
backup_config() {
  FILE="$1"
  [ -f "$FILE" ] || return 0
  cp -p "$FILE" "$FILE.bak" >/dev/null 2>&1 || true
}
for HOME_DIR in /home/*; do
  [ -d "$HOME_DIR" ] || continue
  CONFIG="$HOME_DIR/.config/kioskmate/config.json"
  backup_config "$CONFIG"
  if [ -f "$CONFIG" ]; then
    sed -i 's/"bind": "127\.0\.0\.1"/"bind": "0.0.0.0"/' "$CONFIG" || true
    sed -i 's/"bind": "localhost"/"bind": "0.0.0.0"/' "$CONFIG" || true
  fi
done
reload_user_units() {
  for RUNTIME in /run/user/*; do
    [ -d "$RUNTIME" ] || continue
    UID_NAME="$(basename "$RUNTIME")"
    USER_NAME="$(getent passwd "$UID_NAME" | cut -d: -f1)"
    [ -n "$USER_NAME" ] || continue
    XDG_RUNTIME_DIR="$RUNTIME" runuser -u "$USER_NAME" -- systemctl --user daemon-reload >/dev/null 2>&1 || true
  done
}
if command -v systemctl >/dev/null 2>&1; then
  systemctl --global enable kioskmate.service >/dev/null 2>&1 || true
  reload_user_units
fi
exit 0
POSTINST
chmod 0755 "$PKG/DEBIAN/postinst"

dpkg-deb --build --root-owner-group "$PKG" "${OUT}/kioskmate_${VERSION}_${ARCH}.deb"
