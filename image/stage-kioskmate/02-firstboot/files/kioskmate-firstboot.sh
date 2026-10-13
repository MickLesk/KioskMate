#!/usr/bin/env bash
# Apply /boot/firmware/kioskmate.txt once on the first boot of a KioskMate
# image: Wi-Fi, hostname, time zone, Admin password, start URL and the
# optional read-only root. Run by kioskmate-firstboot.service as root.
#
# The file is parsed, never sourced. Secret values are blanked afterwards.
set -uo pipefail

CONFIG_FILE="${KIOSKMATE_FIRSTBOOT_FILE:-/boot/firmware/kioskmate.txt}"
STATE_DIR="${KIOSKMATE_FIRSTBOOT_STATE:-/var/lib/kioskmate}"
HOSTS_FILE="${KIOSKMATE_FIRSTBOOT_HOSTS:-/etc/hosts}"
ZONEINFO="${KIOSKMATE_FIRSTBOOT_ZONEINFO:-/usr/share/zoneinfo}"
KIOSK_USER="${KIOSK_USER:-kiosk}"
KIOSKMATE_BIN="${KIOSKMATE_BIN:-/usr/bin/kioskmate}"
WIFI_CONNECTION="kioskmate-wifi"
KEYS="WIFI_SSID WIFI_PASSWORD WIFI_COUNTRY HOSTNAME TIMEZONE ADMIN_PASSWORD START_URL READ_ONLY"
SECRET_KEYS="WIFI_PASSWORD ADMIN_PASSWORD"

ERRORS=0
declare -A CFG=()

log() { echo "kioskmate-firstboot: $*"; }
fail() {
  log "ERROR: $*"
  ERRORS=$((ERRORS + 1))
}

trim() {
  local value="$1"
  value="${value#"${value%%[![:space:]]*}"}"
  value="${value%"${value##*[![:space:]]}"}"
  printf '%s' "$value"
}

# read_config fills CFG from KEY=value lines; comments, blank lines and
# unknown keys are skipped. Windows line endings are accepted.
read_config() {
  local line key value
  while IFS= read -r line || [ -n "$line" ]; do
    line="${line%$'\r'}"
    line="$(trim "$line")"
    case "$line" in '' | '#'*) continue ;; esac
    if [[ "$line" != *=* ]]; then
      log "ignoring line without '=': ${line%%[[:space:]]*}"
      continue
    fi
    key="$(trim "${line%%=*}")"
    value="$(trim "${line#*=}")"
    if [ "${#value}" -ge 2 ]; then
      case "$value" in
        \"*\" | \'*\') value="${value:1:${#value}-2}" ;;
      esac
    fi
    case " $KEYS " in
      *" $key "*) CFG[$key]="$value" ;;
      *) log "ignoring unknown key $key" ;;
    esac
  done < "$CONFIG_FILE"
}

get() { printf '%s' "${CFG[$1]:-}"; }

is_yes() {
  case "${1,,}" in yes | y | true | 1 | on) return 0 ;; *) return 1 ;; esac
}

# Run kioskmate as the kiosk user so it edits ~kiosk/.config/kioskmate.
run_kioskmate() {
  local home
  home="$(getent passwd "$KIOSK_USER" | cut -d: -f6)"
  if [ -z "$home" ]; then
    echo "user $KIOSK_USER does not exist" >&2
    return 1
  fi
  runuser -u "$KIOSK_USER" -- env -i HOME="$home" PATH=/usr/local/bin:/usr/bin:/bin "$KIOSKMATE_BIN" "$@"
}

apply_wifi_country() {
  local country
  country="$(get WIFI_COUNTRY)"
  [ -n "$country" ] || return 0
  country="${country^^}"
  if [[ ! "$country" =~ ^[A-Z]{2}$ ]]; then
    fail "WIFI_COUNTRY must be a two-letter country code"
    return
  fi
  if raspi-config nonint do_wifi_country "$country"; then
    log "Wi-Fi country set to $country"
  else
    fail "could not set Wi-Fi country $country"
  fi
}

apply_wifi() {
  local ssid password
  ssid="$(get WIFI_SSID)"
  password="$(get WIFI_PASSWORD)"
  [ -n "$ssid" ] || return 0
  if [ -n "$password" ] && { [ "${#password}" -lt 8 ] || [ "${#password}" -gt 63 ]; } && [[ ! "$password" =~ ^[0-9A-Fa-f]{64}$ ]]; then
    fail "WIFI_PASSWORD must have 8 to 63 characters"
    return
  fi
  nmcli connection delete "$WIFI_CONNECTION" >/dev/null 2>&1 || true
  local args=(connection add type wifi con-name "$WIFI_CONNECTION" ssid "$ssid" connection.autoconnect yes)
  if [ -n "$password" ]; then
    args+=(wifi-sec.key-mgmt wpa-psk wifi-sec.psk "$password")
  fi
  if ! nmcli "${args[@]}" >/dev/null; then
    fail "could not add the Wi-Fi connection"
    return
  fi
  log "Wi-Fi connection for the configured SSID added"
  # Not fatal: the network may be out of range now; NetworkManager retries.
  nmcli --wait 30 connection up "$WIFI_CONNECTION" >/dev/null 2>&1 || log "Wi-Fi not connected yet; NetworkManager keeps trying"
}

apply_hostname() {
  local name
  name="$(get HOSTNAME)"
  [ -n "$name" ] || return 0
  if [[ ! "$name" =~ ^[A-Za-z0-9]([A-Za-z0-9-]{0,61}[A-Za-z0-9])?$ ]]; then
    fail "HOSTNAME may only contain letters, digits and hyphens (max. 63)"
    return
  fi
  if ! hostnamectl set-hostname "$name"; then
    fail "could not set hostname $name"
    return
  fi
  if grep -q '^127\.0\.1\.1[[:space:]]' "$HOSTS_FILE"; then
    sed -i "s/^127\.0\.1\.1[[:space:]].*/127.0.1.1\t$name/" "$HOSTS_FILE"
  else
    printf '127.0.1.1\t%s\n' "$name" >> "$HOSTS_FILE"
  fi
  log "hostname set to $name"
}

apply_timezone() {
  local zone
  zone="$(get TIMEZONE)"
  [ -n "$zone" ] || return 0
  if [[ ! "$zone" =~ ^[A-Za-z0-9_+-]+(/[A-Za-z0-9_+-]+)*$ ]] || [ ! -f "$ZONEINFO/$zone" ]; then
    fail "unknown TIMEZONE $zone"
    return
  fi
  if timedatectl set-timezone "$zone"; then
    log "time zone set to $zone"
  else
    fail "could not set time zone $zone"
  fi
}

apply_admin_password() {
  local password
  password="$(get ADMIN_PASSWORD)"
  [ -n "$password" ] || return 0
  if printf '%s' "$password" | run_kioskmate --set-admin-password-stdin; then
    log "Admin password set"
  else
    fail "could not set the Admin password"
  fi
}

apply_start_url() {
  local url
  url="$(get START_URL)"
  [ -n "$url" ] || return 0
  if run_kioskmate --set-start-url "$url"; then
    log "start URL set"
  else
    fail "could not set the start URL"
  fi
}

# Blank secret values in place and leave a status line for whoever opens the
# file on a computer later. The boot partition is FAT: write a temp file in
# the same directory and rename it.
finish_config_file() {
  local tmp="$CONFIG_FILE.tmp" pattern
  pattern="$(echo "$SECRET_KEYS" | tr ' ' '|')"
  if ! sed -E "s/^([[:space:]]*($pattern)[[:space:]]*=).*/\1/" "$CONFIG_FILE" > "$tmp"; then
    rm -f "$tmp"
    fail "could not blank secrets in $CONFIG_FILE"
    return
  fi
  printf '\n# Applied by kioskmate-firstboot on %s with %d error(s); passwords were removed.\n# Details: journalctl -u kioskmate-firstboot\n' "$(date -u '+%Y-%m-%d %H:%M UTC')" "$ERRORS" >> "$tmp"
  mv -f "$tmp" "$CONFIG_FILE"
  sync
}

apply_read_only() {
  is_yes "$(get READ_ONLY)" || return 0
  if [ "$ERRORS" -gt 0 ]; then
    log "READ_ONLY=yes skipped because of the errors above; fix them and run: sudo raspi-config nonint do_overlayfs 0"
    return 0
  fi
  log "enabling the overlay file system (read-only root) and rebooting"
  if raspi-config nonint do_overlayfs 0; then
    systemctl --no-block reboot
  else
    log "ERROR: raspi-config could not enable the overlay file system"
  fi
}

main() {
  if [ ! -f "$CONFIG_FILE" ]; then
    log "$CONFIG_FILE not found; nothing to do"
    return 0
  fi
  read_config
  apply_wifi_country
  apply_wifi
  apply_hostname
  apply_timezone
  apply_admin_password
  apply_start_url
  mkdir -p "$STATE_DIR"
  date -u '+%Y-%m-%dT%H:%M:%SZ' > "$STATE_DIR/firstboot.done"
  finish_config_file
  log "done with $ERRORS error(s)"
  apply_read_only
  # Always succeed: a failed unit would not make a second attempt any better,
  # and the kiosk should still start. Errors are in the journal and the file.
  return 0
}

if [ "${BASH_SOURCE[0]}" = "$0" ]; then
  main "$@"
fi
