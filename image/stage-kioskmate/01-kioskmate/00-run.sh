#!/bin/bash -e
# Install the KioskMate package (copied here by image/build.sh) plus Chromium
# and switch the kiosk user to the desktop-free Cage mode on tty1.

if [ ! -f files/kioskmate.deb ]; then
	echo "files/kioskmate.deb is missing; build the image with image/build.sh" >&2
	exit 1
fi
install -m 644 files/kioskmate.deb "${ROOTFS_DIR}/tmp/kioskmate.deb"

on_chroot << EOF
set -e
# Raspberry Pi OS ships Chromium as "chromium"; older snapshots used "chromium-browser".
apt-get install -y chromium || apt-get install -y chromium-browser
apt-get install -y /tmp/kioskmate.deb
rm -f /tmp/kioskmate.deb
/usr/share/kioskmate/cage/setup-cage.sh enable "${FIRST_USER_NAME}"
EOF

if [ "${KIOSKMATE_LOCK_PASSWORD:-0}" = "1" ]; then
	# build.sh generated a throwaway password: disable password logins for the
	# kiosk user. SSH keys (PUBKEY_SSH_FIRST_USER) and sudo keep working.
	on_chroot << EOF
passwd -l "${FIRST_USER_NAME}"
EOF
fi
