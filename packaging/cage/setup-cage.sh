#!/usr/bin/env bash
# Switch a KioskMate install from the desktop session to the Cage kiosk
# compositor, or back. Run as root:
#   setup-cage.sh enable <user>    install cage, start KioskMate on tty1
#   setup-cage.sh disable <user>   return to the desktop user service
set -euo pipefail

ACTION="${1:-}"
KIOSK_USER="${2:-}"
SHARE="/usr/share/kioskmate/cage"
UNIT="kioskmate-cage@${KIOSK_USER}.service"

usage() { echo "usage: $0 enable|disable <user>" >&2; exit 2; }
[ -n "$ACTION" ] && [ -n "$KIOSK_USER" ] || usage
[ "$(id -u)" -eq 0 ] || { echo "run as root (sudo)" >&2; exit 1; }
id "$KIOSK_USER" >/dev/null 2>&1 || { echo "user $KIOSK_USER does not exist" >&2; exit 1; }

user_systemctl() {
  local uid
  uid="$(id -u "$KIOSK_USER")"
  if [ -d "/run/user/$uid" ]; then
    runuser -u "$KIOSK_USER" -- env XDG_RUNTIME_DIR="/run/user/$uid" systemctl --user "$@" || true
  fi
}

case "$ACTION" in
  enable)
    if ! command -v cage >/dev/null 2>&1; then
      apt-get install -y --no-install-recommends cage
    fi
    for PACKAGE in wlr-randr wlopm; do
      dpkg -s "$PACKAGE" >/dev/null 2>&1 || apt-get install -y --no-install-recommends "$PACKAGE" || true
    done
    install -m 0644 "$SHARE/pam-kioskmate-cage" /etc/pam.d/kioskmate-cage
    # Seat access for the compositor (DRM, input devices).
    for GROUP in video render input; do
      getent group "$GROUP" >/dev/null && usermod -aG "$GROUP" "$KIOSK_USER"
    done
    # Only one KioskMate may run; the desktop user service would fight Cage.
    user_systemctl disable --now kioskmate.service
    systemctl disable getty@tty1.service >/dev/null 2>&1 || true
    if systemctl is-enabled display-manager.service >/dev/null 2>&1; then
      systemctl disable display-manager.service
      echo "Desktop login manager disabled; re-enable it with: $0 disable $KIOSK_USER"
    fi
    systemctl set-default graphical.target >/dev/null
    systemctl daemon-reload
    systemctl enable "$UNIT"
    echo "Enabled $UNIT. Reboot to start KioskMate without a desktop."
    ;;
  disable)
    systemctl disable --now "$UNIT" || true
    systemctl enable getty@tty1.service >/dev/null 2>&1 || true
    for DM in lightdm gdm3 sddm; do
      if systemctl list-unit-files "$DM.service" >/dev/null 2>&1 && systemctl cat "$DM.service" >/dev/null 2>&1; then
        systemctl enable "$DM.service" && break
      fi
    done
    user_systemctl enable kioskmate.service
    echo "Desktop session restored. Reboot to apply."
    ;;
  *) usage ;;
esac
