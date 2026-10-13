#!/usr/bin/env bash
# Build a ready-to-flash Raspberry Pi OS Lite (Bookworm, arm64) image with
# Cage, Chromium and KioskMate, using pi-gen in Docker. See docs/IMAGE.md.
#
#   image/build.sh                    build the .deb and the image
#   VERSION=0.9.0 image/build.sh      version for the .deb and the image name
#   KIOSKMATE_DEB=path.deb image/build.sh   use an existing arm64 package
#
# Optional environment: PI_GEN_REF, PI_GEN_REPO, BUILD_DIR, OUT_DIR,
# SSH_PUBKEY (enables SSH with key login only), KIOSK_PASSWORD (console and
# SSH password for the kiosk user; default: password login disabled),
# LOCALE_DEFAULT, KEYBOARD_KEYMAP, KEYBOARD_LAYOUT, TIMEZONE_DEFAULT,
# DEPLOY_COMPRESSION (xz, zip, gz or none).
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
PI_GEN_REPO="${PI_GEN_REPO:-https://github.com/RPi-Distro/pi-gen.git}"
# Pinned pi-gen release (Raspberry Pi OS Bookworm arm64). Update deliberately.
PI_GEN_REF="${PI_GEN_REF:-2026-09-15-raspios-bookworm-arm64}"
VERSION="${VERSION:-0.0.0-dev}"
BUILD_DIR="${BUILD_DIR:-$ROOT/build/image}"
OUT_DIR="${OUT_DIR:-$ROOT/dist/image}"
STAGE_NAME="stage-kioskmate"
PI_GEN="$BUILD_DIR/pi-gen"

for TOOL in git docker python3 go; do
  command -v "$TOOL" >/dev/null 2>&1 || { echo "missing required tool: $TOOL" >&2; exit 1; }
done

if [ -n "${KIOSKMATE_DEB:-}" ]; then
  DEB="$(realpath "$KIOSKMATE_DEB")"
else
  python3 "$ROOT/scripts/package-deb.py" --version "$VERSION" --arch arm64 --root "$ROOT"
  DEB="$ROOT/dist/kioskmate_${VERSION}_arm64.deb"
fi
[ -f "$DEB" ] || { echo "package not found: $DEB" >&2; exit 1; }

echo "Cloning pi-gen $PI_GEN_REF"
rm -rf "$PI_GEN"
mkdir -p "$BUILD_DIR"
git -c advice.detachedHead=false clone --depth 1 --branch "$PI_GEN_REF" "$PI_GEN_REPO" "$PI_GEN"

# Stages 0-2 build Raspberry Pi OS Lite; only our stage exports an image.
cp -a "$ROOT/image/$STAGE_NAME" "$PI_GEN/$STAGE_NAME"
touch "$PI_GEN/stage2/SKIP_IMAGES"
install -m 0644 "$DEB" "$PI_GEN/$STAGE_NAME/01-kioskmate/files/kioskmate.deb"

# pi-gen refuses to disable the first-boot user rename without a password.
LOCK_PASSWORD=0
if [ -z "${KIOSK_PASSWORD:-}" ]; then
  KIOSK_PASSWORD="$(python3 -c 'import secrets; print(secrets.token_urlsafe(24))')"
  LOCK_PASSWORD=1
fi

ENABLE_SSH=0
if [ -n "${SSH_PUBKEY:-}" ]; then
  ENABLE_SSH=1
fi

conf() { printf '%s=%q\n' "$1" "$2"; }
umask 077
{
  conf IMG_NAME "kioskmate-${VERSION}"
  conf PI_GEN_RELEASE "KioskMate ${VERSION}"
  conf RELEASE bookworm
  conf DEPLOY_COMPRESSION "${DEPLOY_COMPRESSION:-xz}"
  conf TARGET_HOSTNAME kioskmate
  conf FIRST_USER_NAME kiosk
  conf FIRST_USER_PASS "$KIOSK_PASSWORD"
  conf DISABLE_FIRST_BOOT_USER_RENAME 1
  conf ENABLE_SSH "$ENABLE_SSH"
  if [ "$ENABLE_SSH" = "1" ]; then
    conf PUBKEY_SSH_FIRST_USER "$SSH_PUBKEY"
    conf PUBKEY_ONLY_SSH 1
  fi
  conf LOCALE_DEFAULT "${LOCALE_DEFAULT:-en_GB.UTF-8}"
  conf KEYBOARD_KEYMAP "${KEYBOARD_KEYMAP:-gb}"
  conf KEYBOARD_LAYOUT "${KEYBOARD_LAYOUT:-English (UK)}"
  conf TIMEZONE_DEFAULT "${TIMEZONE_DEFAULT:-Europe/London}"
  conf STAGE_LIST "stage0 stage1 stage2 $STAGE_NAME"
  # Read by $STAGE_NAME/01-kioskmate/00-run.sh.
  printf 'export KIOSKMATE_LOCK_PASSWORD=%s\n' "$LOCK_PASSWORD"
} > "$PI_GEN/config"
umask 022

echo "Building the image with pi-gen (privileged Docker, takes a while)"
(cd "$PI_GEN" && ./build-docker.sh)

mkdir -p "$OUT_DIR"
shopt -s nullglob
OUTPUTS=("$PI_GEN"/deploy/*kioskmate-"${VERSION}"*)
if [ "${#OUTPUTS[@]}" -eq 0 ]; then
  echo "pi-gen finished without an image in $PI_GEN/deploy" >&2
  exit 1
fi
cp -a "${OUTPUTS[@]}" "$OUT_DIR/"
cp -a "$PI_GEN"/deploy/build*.log "$OUT_DIR/" 2>/dev/null || true
(cd "$OUT_DIR" && for FILE in "${OUTPUTS[@]}"; do basename "$FILE"; done | xargs sha256sum > SHA256SUMS)
echo "Image written to $OUT_DIR:"
ls -lh "$OUT_DIR"
