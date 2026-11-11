#!/bin/bash -e
# Fewer SD card writes: journal in RAM, zram swap instead of a swap file.

install -d "${ROOTFS_DIR}/etc/systemd/journald.conf.d"
install -m 644 files/journald-kioskmate.conf "${ROOTFS_DIR}/etc/systemd/journald.conf.d/50-kioskmate.conf"

# Raspberry Pi OS Trixie may already set up zram itself (rpi-swap). Only add
# our own configuration when nothing else configures zram.
if [ -d "${ROOTFS_DIR}/usr/lib/systemd/system-generators" ] \
	&& [ -x "${ROOTFS_DIR}/usr/lib/systemd/system-generators/zram-generator" ] \
	&& [ ! -e "${ROOTFS_DIR}/etc/systemd/zram-generator.conf" ] \
	&& [ ! -e "${ROOTFS_DIR}/usr/lib/systemd/zram-generator.conf" ]; then
	install -m 644 files/zram-kioskmate.conf "${ROOTFS_DIR}/etc/systemd/zram-generator.conf"
	echo "zram swap configured"
else
	echo "zram: generator missing or already configured; leaving swap as the base image sets it"
fi

# Bookworm-era swap file on the SD card: off when zram is there.
if [ -e "${ROOTFS_DIR}/etc/systemd/zram-generator.conf" ] || [ -e "${ROOTFS_DIR}/usr/lib/systemd/zram-generator.conf" ]; then
	on_chroot << CHROOT
if systemctl list-unit-files dphys-swapfile.service >/dev/null 2>&1; then
	systemctl disable dphys-swapfile.service || true
fi
CHROOT
fi
