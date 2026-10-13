#!/bin/bash -e
# First-boot configuration from /boot/firmware/kioskmate.txt.

install -m 755 files/kioskmate-firstboot.sh "${ROOTFS_DIR}/usr/local/sbin/kioskmate-firstboot"
install -m 644 files/kioskmate-firstboot.service "${ROOTFS_DIR}/etc/systemd/system/kioskmate-firstboot.service"
install -m 644 files/kioskmate.txt "${ROOTFS_DIR}/boot/firmware/kioskmate.txt"
install -m 644 files/kioskmate.txt "${ROOTFS_DIR}/usr/share/doc/kioskmate/kioskmate.txt.example"

on_chroot << EOF
systemctl enable kioskmate-firstboot.service
EOF
