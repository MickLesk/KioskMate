#!/bin/bash -e
# Optional packages that not every Bookworm archive snapshot carries:
# wlopm switches the display off and on under Cage; overlayroot backs the
# READ_ONLY=yes option of kioskmate.txt (raspi-config would otherwise try to
# download it on the kiosk). Warn instead of failing the build.
for PACKAGE in wlopm overlayroot; do
	if ! apt-get install -y --no-install-recommends "$PACKAGE"; then
		echo "WARNING: package $PACKAGE is not available; continuing without it" >&2
	fi
done
