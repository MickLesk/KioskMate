#!/usr/bin/env bash

# Read arguments
ARG_EARLY=false
ARG_UPDATE=false
for arg in "$@"; do
  case "$arg" in
    early) ARG_EARLY=true ;;
    update) ARG_UPDATE=true ;;
  esac
done

# Determine system architecture
echo -e "Determining system architecture..."

REPO="${KIOSKMATE_REPO:-MickLesk/KioskMate}"

BITS=$(getconf LONG_BIT)
case "$(uname -m)" in
    x86_64) ARCH="amd64" ;;
    aarch64) ARCH="arm64" ;;
    *) { echo "Architecture $(uname -m) running $BITS-bit operating system is not supported."; exit 1; } ;;
esac

[ "$BITS" -eq 64 ] || { echo "Architecture $ARCH running $BITS-bit operating system is not supported."; exit 1; }
echo "Architecture $ARCH running $BITS-bit operating system is supported."

# Download the latest .deb package
echo -e "\nDownloading the latest release..."

TMP_DIR=$(mktemp -d)
chmod 755 "$TMP_DIR"

JSON=$(wget -qO- "https://api.github.com/repos/${REPO}/releases" | tr -d '\r\n')
if $ARG_EARLY; then
  DEB_REG='"prerelease":\s*(true|false).*?"browser_download_url":\s*"\K[^\"]*_'$ARCH'\.deb'
else
  DEB_REG='"prerelease":\s*false.*?"browser_download_url":\s*"\K[^\"]*_'$ARCH'\.deb'
fi

DEB_URL=$(echo "$JSON" | grep -oP "$DEB_REG" | head -n 1)
DEB_PATH="${TMP_DIR}/$(basename "$DEB_URL")"

[ -z "$DEB_URL" ] && { echo "Download url for .deb file not found."; exit 1; }
wget --show-progress -q -O "$DEB_PATH" "$DEB_URL" || { echo "Failed to download the .deb file."; exit 1; }

# Install the latest .deb package
echo -e "\nInstalling the latest release..."

command -v apt &> /dev/null || { echo "Package manager apt was not found."; exit 1; }
sudo apt install -y "$DEB_PATH" || { echo "Installation of .deb file failed."; exit 1; }

# Install dashboard font support for Home Assistant text, icons and emoji.
if [ "${KIOSKMATE_SKIP_FONT_PACKAGES:-false}" != "true" ]; then
  echo -e "\nInstalling dashboard font support..."
  for PACKAGE in fonts-noto-core fonts-noto-color-emoji; do
    if dpkg -s "$PACKAGE" >/dev/null 2>&1; then
      echo "$PACKAGE already installed."
    elif apt-cache show "$PACKAGE" >/dev/null 2>&1; then
      sudo apt install -y --no-install-recommends "$PACKAGE" || echo "Optional package $PACKAGE could not be installed."
    else
      echo "Optional package $PACKAGE is not available on this system."
    fi
  done
fi

# Create the systemd user service
echo -e "\nCreating systemd user service..."

SERVICE_NAME="kioskmate.service"
SERVICE_FILE="$HOME/.config/systemd/user/$SERVICE_NAME"
mkdir -p "$(dirname "$SERVICE_FILE")" || { echo "Failed to create directory for $SERVICE_FILE."; exit 1; }

SERVICE_CONTENT="[Unit]
Description=KioskMate
After=graphical-session.target
Wants=network-online.target

[Service]
Environment=DISPLAY=:0
Environment=XDG_RUNTIME_DIR=%t
ExecStart=/usr/bin/kioskmate
Restart=on-failure
RestartSec=5s

[Install]
WantedBy=default.target"

SERVICE_CREATE=true
if [ -f "$SERVICE_FILE" ]; then
  if $ARG_UPDATE; then
    SERVICE_CREATE=true
  else
    read -p "Service $SERVICE_FILE exists, overwrite? (y/N) " overwrite
    [[ ${overwrite:-n} == [Yy]* ]] || SERVICE_CREATE=false
  fi
fi

if $SERVICE_CREATE; then
    echo "$SERVICE_CONTENT" > "$SERVICE_FILE" || { echo "Failed to write to $SERVICE_FILE."; exit 1; }
    systemctl --user daemon-reload || { echo "Failed to reload systemd user units."; exit 1; }
    systemctl --user enable "$(basename "$SERVICE_FILE")" || { echo "Failed to enable service $SERVICE_FILE."; exit 1; }
    echo "Service $SERVICE_FILE enabled."
else
    echo "Service $SERVICE_FILE not created."
fi

if $ARG_UPDATE; then
  systemctl --user stop "${SERVICE_NAME}" 2>/dev/null || true
  pkill -u "$(id -u)" -f '(^|/)kioskmate( |$)|/usr/lib/kioskmate/kioskmate' 2>/dev/null || true
  sleep 1
  if systemctl --user --quiet is-active "${SERVICE_NAME}"; then
    systemctl --user restart "${SERVICE_NAME}"
    echo "Existing $SERVICE_NAME restarted."
  else
    systemctl --user start "${SERVICE_NAME}" || { echo "Failed to start ${SERVICE_NAME}."; exit 1; }
    echo "$SERVICE_NAME started."
  fi
  exit 0
fi

# Export display variables
echo -e "\nExporting display variables..."

if [ -z "$DISPLAY" ]; then
    export DISPLAY=":0"
    echo "DISPLAY was not set, defaulting to \"$DISPLAY\"."
else
    echo "DISPLAY is set to \"$DISPLAY\"."
fi

if [ -n "$WAYLAND_DISPLAY" ]; then
    echo "WAYLAND_DISPLAY is set to \"$WAYLAND_DISPLAY\"."
else
    echo "WAYLAND_DISPLAY is not set; KioskMate detects the active Wayland socket at runtime."
fi

read -p $'\nStart kioskmate user service now? (Y/n) ' setup

if [[ ${setup:-y} == [Yy]* ]]; then
    systemctl --user daemon-reload || { echo "Failed to reload systemd user units."; exit 1; }
    systemctl --user restart "${SERVICE_NAME}" || { echo "Failed to start ${SERVICE_NAME}."; exit 1; }
    echo "$SERVICE_NAME started."
    echo
    /usr/bin/kioskmate --admin-info || true
else
    echo "Start later with: systemctl --user start ${SERVICE_NAME}"
    echo "Show admin recovery info with: kioskmate --admin-info"
fi

exit 0
