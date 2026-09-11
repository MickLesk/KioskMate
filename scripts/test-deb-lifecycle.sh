#!/usr/bin/env bash
set -euo pipefail

OLD_DEB="${1:?usage: test-deb-lifecycle.sh old.deb new.deb}"
NEW_DEB="${2:?usage: test-deb-lifecycle.sh old.deb new.deb}"
SUDO=()
if [ "$(id -u)" -ne 0 ]; then
  SUDO=(sudo)
fi

test "$(dpkg-deb -f "$OLD_DEB" Package)" = "kioskmate"
test "$(dpkg-deb -f "$NEW_DEB" Package)" = "kioskmate"
test "$(dpkg-deb -f "$OLD_DEB" Architecture)" = "$(dpkg --print-architecture)"
test "$(dpkg-deb -f "$NEW_DEB" Architecture)" = "$(dpkg --print-architecture)"

HOME_DIR="/home/kioskmate-package-test-$$"
CONFIG_DIR="$HOME_DIR/.config/kioskmate"
cleanup() {
  "${SUDO[@]}" dpkg --remove kioskmate >/dev/null 2>&1 || true
  "${SUDO[@]}" rm -rf "$HOME_DIR"
}
trap cleanup EXIT

"${SUDO[@]}" mkdir -p "$CONFIG_DIR"
printf '%s\n' '{"version":4,"sentinel":"preserve-me"}' | "${SUDO[@]}" tee "$CONFIG_DIR/config.json" >/dev/null

"${SUDO[@]}" dpkg --unpack "$OLD_DEB" >/dev/null
test -x /usr/bin/kioskmate
test -f /usr/lib/systemd/user/kioskmate.service
test "$(dpkg-query -W -f='${db:Status-Abbrev}' kioskmate)" = "iU "

"${SUDO[@]}" dpkg --unpack "$NEW_DEB" >/dev/null
grep -q 'preserve-me' "$CONFIG_DIR/config.json"
grep -q 'preserve-me' "$CONFIG_DIR/config.json.bak"
test "$(dpkg-query -W -f='${Version}' kioskmate)" = "$(dpkg-deb -f "$NEW_DEB" Version)"

"${SUDO[@]}" dpkg --remove kioskmate >/dev/null
test ! -e /usr/bin/kioskmate
test ! -e /usr/lib/systemd/user/kioskmate.service
grep -q 'preserve-me' "$CONFIG_DIR/config.json"

echo "Debian install, upgrade and removal lifecycle verified"
