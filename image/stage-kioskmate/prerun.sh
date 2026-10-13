#!/bin/bash -e
# pi-gen stage on top of stage2 (Raspberry Pi OS Lite): start from its rootfs.

if [ ! -d "${ROOTFS_DIR}" ]; then
	copy_previous
fi
