#!/usr/bin/env bash
# Tests for image/stage-kioskmate/02-firstboot/files/kioskmate-firstboot.sh.
# System tools (nmcli, hostnamectl, raspi-config, ...) are replaced by stubs
# that log their arguments, so the script runs anywhere without root.
#
#   bash tests/image/firstboot-test.sh
set -uo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
SCRIPT="$ROOT/image/stage-kioskmate/02-firstboot/files/kioskmate-firstboot.sh"
EXAMPLE="$ROOT/image/stage-kioskmate/02-firstboot/files/kioskmate.txt"
FAILED=0
PASSED=0

check() {
  local name="$1"
  shift
  if "$@"; then
    PASSED=$((PASSED + 1))
  else
    FAILED=$((FAILED + 1))
    echo "FAIL: $name" >&2
  fi
}

# setup creates a fresh sandbox and exports the script's override variables.
setup() {
  WORK="$(mktemp -d)"
  mkdir -p "$WORK/bin" "$WORK/zoneinfo/Europe" "$WORK/state" "$WORK/journald"
  touch "$WORK/zoneinfo/Europe/Berlin"
  for tool in nmcli hostnamectl timedatectl raspi-config systemctl runuser getent; do
    cat > "$WORK/bin/$tool" <<STUB
#!/usr/bin/env bash
echo "$tool \$*" >> "$WORK/calls.log"
[ "$tool" = getent ] && echo "kiosk:x:1000:1000::$WORK/home:/bin/bash"
exit 0
STUB
    chmod +x "$WORK/bin/$tool"
  done
  printf '127.0.0.1\tlocalhost\n127.0.1.1\tkioskmate\n' > "$WORK/hosts"
  printf '[all]\ndtoverlay=vc4-kms-v3d\nmax_framebuffers=2\n' > "$WORK/config.txt"
  printf 'MemTotal:        3884096 kB\n' > "$WORK/meminfo"
  echo "[Journal]" > "$WORK/journald/50-kioskmate.conf"
  : > "$WORK/calls.log"
  export PATH="$WORK/bin:$PATH"
  export KIOSKMATE_FIRSTBOOT_FILE="$WORK/kioskmate.txt"
  export KIOSKMATE_FIRSTBOOT_STATE="$WORK/state"
  export KIOSKMATE_FIRSTBOOT_HOSTS="$WORK/hosts"
  export KIOSKMATE_FIRSTBOOT_ZONEINFO="$WORK/zoneinfo"
  export KIOSKMATE_FIRSTBOOT_BOOTCONFIG="$WORK/config.txt"
  export KIOSKMATE_FIRSTBOOT_MEMINFO="$WORK/meminfo"
  export KIOSKMATE_FIRSTBOOT_JOURNALD="$WORK/journald/50-kioskmate.conf"
}

run_main() {
  bash "$SCRIPT" > "$WORK/out.log" 2>&1
}

teardown() { rm -rf "$WORK"; }

# --- The shipped example applies without errors and changes nothing risky ---
setup
cp "$EXAMPLE" "$WORK/kioskmate.txt"
run_main
check "example: finishes" test -f "$WORK/state/firstboot.done"
check "example: no errors" grep -q "done with 0 error(s)" "$WORK/out.log"
check "example: status line appended" grep -q "Applied by kioskmate-firstboot" "$WORK/kioskmate.txt"
check "example: no read-only overlay" bash -c "! grep -q do_overlayfs '$WORK/calls.log'"
check "example: CMA raised on 4 GB board" grep -qx "dtoverlay=vc4-kms-v3d,cma-512" "$WORK/config.txt"
check "example: journal stays in RAM" test -f "$WORK/journald/50-kioskmate.conf"
teardown

# --- Parsing: CRLF, quotes, spaces, unknown keys; secrets blanked ---
setup
printf 'WIFI_SSID = "Home Net"\r\nWIFI_PASSWORD=supersecret\r\nWIFI_COUNTRY=de\r\nHOSTNAME=kitchen\r\nTIMEZONE=Europe/Berlin\r\nBOGUS=1\r\nADMIN_PASSWORD='"'"'admin pass'"'"'\r\n' > "$WORK/kioskmate.txt"
run_main
check "parse: Wi-Fi with SSID containing a space" grep -q "ssid Home Net" "$WORK/calls.log"
check "parse: country upper-cased" grep -q "do_wifi_country DE" "$WORK/calls.log"
check "parse: hostname set" grep -q "hostnamectl set-hostname kitchen" "$WORK/calls.log"
check "parse: hosts updated" grep -qP "^127\.0\.1\.1\tkitchen$" "$WORK/hosts"
check "parse: time zone set" grep -q "timedatectl set-timezone Europe/Berlin" "$WORK/calls.log"
check "parse: unknown key ignored" grep -q "ignoring unknown key BOGUS" "$WORK/out.log"
check "parse: Admin password passed to kioskmate" grep -q -- "--set-admin-password-stdin" "$WORK/calls.log"
check "secrets: Wi-Fi password blanked" grep -qx "WIFI_PASSWORD=" "$WORK/kioskmate.txt"
check "secrets: Admin password blanked" grep -qx "ADMIN_PASSWORD=" "$WORK/kioskmate.txt"
check "secrets: SSID kept" grep -q "WIFI_SSID" "$WORK/kioskmate.txt"
teardown

# --- Validation errors are counted and block READ_ONLY ---
setup
printf 'HOSTNAME=bad_name!\nTIMEZONE=Mars/Base\nWIFI_SSID=x\nWIFI_PASSWORD=short\nREAD_ONLY=yes\n' > "$WORK/kioskmate.txt"
run_main
check "errors: three errors reported" grep -q "done with 3 error(s)" "$WORK/out.log"
check "errors: read-only skipped" grep -q "READ_ONLY=yes skipped" "$WORK/out.log"
check "errors: no overlay enabled" bash -c "! grep -q do_overlayfs '$WORK/calls.log'"
teardown

# --- READ_ONLY=yes without errors enables the overlay and reboots ---
setup
printf 'READ_ONLY=yes\n' > "$WORK/kioskmate.txt"
run_main
check "read-only: overlay enabled" grep -q "raspi-config nonint do_overlayfs 0" "$WORK/calls.log"
check "read-only: reboot requested" grep -q "systemctl --no-block reboot" "$WORK/calls.log"
teardown

# --- Video memory ---
setup
printf 'MemTotal:         944040 kB\n' > "$WORK/meminfo"
printf 'VIDEO_MEMORY=auto\n' > "$WORK/kioskmate.txt"
run_main
check "cma: 1 GB board keeps the default" grep -qx "dtoverlay=vc4-kms-v3d" "$WORK/config.txt"
teardown

setup
printf 'VIDEO_MEMORY=no\n' > "$WORK/kioskmate.txt"
run_main
check "cma: VIDEO_MEMORY=no keeps config.txt" grep -qx "dtoverlay=vc4-kms-v3d" "$WORK/config.txt"
teardown

setup
printf '[all]\ndtoverlay=vc4-kms-v3d,cma-256\n' > "$WORK/config.txt"
printf '\n' > "$WORK/kioskmate.txt"
run_main
check "cma: customised overlay left alone" grep -qx "dtoverlay=vc4-kms-v3d,cma-256" "$WORK/config.txt"
teardown

# --- Persistent logs ---
setup
printf 'PERSISTENT_LOGS=yes\n' > "$WORK/kioskmate.txt"
run_main
check "logs: RAM journal drop-in removed" test ! -e "$WORK/journald/50-kioskmate.conf"
check "logs: journald restarted" grep -q "systemctl restart systemd-journald" "$WORK/calls.log"
teardown

# --- Missing file: nothing to do, still success ---
setup
run_main
check "missing file: success" grep -q "nothing to do" "$WORK/out.log"
teardown

echo "firstboot tests: $PASSED passed, $FAILED failed"
[ "$FAILED" -eq 0 ]
