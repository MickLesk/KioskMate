#!/usr/bin/env bash
set -euo pipefail

VERSION="${VERSION:-0.0.0-dev}"
ARCH="${ARCH:-$(dpkg --print-architecture)}"
ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"

exec python3 "$ROOT/scripts/package-deb.py" --root "$ROOT" --version "$VERSION" --arch "$ARCH"
