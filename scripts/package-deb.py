#!/usr/bin/env python3
import argparse
import gzip
import io
import os
import shutil
import subprocess
import tarfile
from pathlib import Path


SUPPORTED_ARCHES = {
    "amd64": "amd64",
    "arm64": "arm64",
}


def main() -> int:
    parser = argparse.ArgumentParser(description="Build KioskMate Debian packages without dpkg-deb.")
    parser.add_argument("--version", required=True)
    parser.add_argument("--arch", choices=sorted(SUPPORTED_ARCHES), action="append", required=True)
    parser.add_argument("--root", default=Path(__file__).resolve().parents[1])
    args = parser.parse_args()

    root = Path(args.root).resolve()
    dist = root / "dist"
    dist.mkdir(parents=True, exist_ok=True)

    for arch in args.arch:
        build_package(root, dist, args.version, arch)

    return 0


def build_package(root: Path, dist: Path, version: str, arch: str) -> None:
    pkg = dist / f"kioskmate_{version}_{arch}"
    if pkg.exists():
        shutil.rmtree(pkg)

    paths = [
        pkg / "DEBIAN",
        pkg / "usr/bin",
        pkg / "usr/share/doc/kioskmate",
        pkg / "usr/lib/systemd/user",
    ]
    for path in paths:
        path.mkdir(parents=True, exist_ok=True)

    env = os.environ.copy()
    env["GOOS"] = "linux"
    env["GOARCH"] = SUPPORTED_ARCHES[arch]
    env["CGO_ENABLED"] = "0"
    subprocess.run(
        [
            "go",
            "build",
            "-trimpath",
            f"-ldflags=-s -w -X main.version={version}",
            "-o",
            str(pkg / "usr/bin/kioskmate"),
            "./cmd/kioskmate",
        ],
        cwd=root,
        env=env,
        check=True,
    )

    shutil.copy2(root / "README.md", pkg / "usr/share/doc/kioskmate/README.md")
    shutil.copy2(root / "packaging/systemd/kioskmate.service", pkg / "usr/lib/systemd/user/kioskmate.service")

    write_text(pkg / "DEBIAN/control", control_file(version, arch))
    write_text(pkg / "DEBIAN/preinst", maintainer_preinst())
    write_text(pkg / "DEBIAN/postinst", maintainer_postinst())

    control_tar = make_tar_gz(pkg / "DEBIAN", ".")
    data_tar = make_tar_gz(pkg / "usr", "./usr")
    deb = b"!<arch>\n"
    deb += ar_member("debian-binary/", b"2.0\n")
    deb += ar_member("control.tar.gz/", control_tar)
    deb += ar_member("data.tar.gz/", data_tar)

    out = dist / f"kioskmate_{version}_{arch}.deb"
    out.write_bytes(deb)
    shutil.rmtree(pkg)
    print(out)


def write_text(path: Path, content: str) -> None:
    if not content.endswith("\n"):
        raise ValueError(f"{path} must end with a newline")
    path.write_text(content, encoding="utf-8", newline="\n")


def control_file(version: str, arch: str) -> str:
    return f"""Package: kioskmate
Version: {version}
Section: net
Priority: optional
Architecture: {arch}
Maintainer: MickLesk
Depends: chromium | chromium-browser | google-chrome-stable, fonts-noto-color-emoji
Recommends: wlopm | kscreen, pipewire-pulse | pulseaudio
Description: KioskMate browser supervisor for Home Assistant kiosks
 Go-based supervisor, Admin API and watchdog for an external kiosk browser.
"""


def maintainer_preinst() -> str:
    return """#!/usr/bin/env bash
set -e
backup_config() {
  FILE="$1"
  [ -f "$FILE" ] || return 0
  cp -p "$FILE" "$FILE.bak" >/dev/null 2>&1 || true
}
for HOME_DIR in /home/*; do
  [ -d "$HOME_DIR" ] || continue
  OLD_LOWER="touch""kio"
  OLD_TITLE="Touch""Kio"
  OLD_V2="$OLD_LOWER-v2"
  OLD_BRAND="go-""kiosk"
  backup_config "$HOME_DIR/.config/kioskmate/config.json"
  backup_config "$HOME_DIR/.config/$OLD_BRAND/config.json"
  backup_config "$HOME_DIR/.config/$OLD_V2/config.json"
  backup_config "$HOME_DIR/.config/$OLD_LOWER/Arguments.json"
  backup_config "$HOME_DIR/.config/$OLD_TITLE/Arguments.json"
done
exit 0
"""


def maintainer_postinst() -> str:
    return """#!/usr/bin/env bash
set -e
backup_config() {
  FILE="$1"
  [ -f "$FILE" ] || return 0
  cp -p "$FILE" "$FILE.bak" >/dev/null 2>&1 || true
}
for HOME_DIR in /home/*; do
  [ -d "$HOME_DIR" ] || continue
  OLD_LOWER="touch""kio"
  OLD_TITLE="Touch""Kio"
  OLD_V2="$OLD_LOWER-v2"
  OLD_BRAND="go-""kiosk"
  CONFIG="$HOME_DIR/.config/kioskmate/config.json"
  backup_config "$CONFIG"
  backup_config "$HOME_DIR/.config/$OLD_BRAND/config.json"
  backup_config "$HOME_DIR/.config/$OLD_V2/config.json"
  backup_config "$HOME_DIR/.config/$OLD_LOWER/Arguments.json"
  backup_config "$HOME_DIR/.config/$OLD_TITLE/Arguments.json"
  if [ -f "$CONFIG" ]; then
    sed -i 's/"bind": "127\\.0\\.0\\.1"/"bind": "0.0.0.0"/' "$CONFIG" || true
    sed -i 's/"bind": "localhost"/"bind": "0.0.0.0"/' "$CONFIG" || true
  fi
done
reload_user_units() {
  for RUNTIME in /run/user/*; do
    [ -d "$RUNTIME" ] || continue
    UID_NAME="$(basename "$RUNTIME")"
    USER_NAME="$(getent passwd "$UID_NAME" | cut -d: -f1)"
    [ -n "$USER_NAME" ] || continue
    XDG_RUNTIME_DIR="$RUNTIME" runuser -u "$USER_NAME" -- systemctl --user daemon-reload >/dev/null 2>&1 || true
  done
}
if command -v systemctl >/dev/null 2>&1; then
  OLD_SERVICE="touch""kio.service"
  OLD_V2_SERVICE="touch""kio-v2.service"
  OLD_BRAND_SERVICE="go-""kiosk.service"
  systemctl --global disable "$OLD_SERVICE" >/dev/null 2>&1 || true
  systemctl --global disable "$OLD_V2_SERVICE" >/dev/null 2>&1 || true
  systemctl --global disable "$OLD_BRAND_SERVICE" >/dev/null 2>&1 || true
  systemctl --global enable kioskmate.service >/dev/null 2>&1 || true
  reload_user_units
fi
exit 0
"""


def make_tar_gz(source: Path, prefix: str) -> bytes:
    raw = io.BytesIO()
    with tarfile.open(fileobj=raw, mode="w", format=tarfile.GNU_FORMAT) as tar:
        add_tree(tar, source, prefix)

    out = io.BytesIO()
    with gzip.GzipFile(fileobj=out, mode="wb", mtime=0) as gz:
        gz.write(raw.getvalue())
    return out.getvalue()


def add_tree(tar: tarfile.TarFile, base: Path, prefix: str) -> None:
    entries = sorted(base.rglob("*"), key=lambda p: p.relative_to(base).as_posix())
    dirs = set()
    for path in entries:
        parent = Path(path.relative_to(base).as_posix()).parent
        while str(parent) not in ("", "."):
            dirs.add(parent.as_posix())
            parent = parent.parent

    for directory in sorted(dirs):
        add_dir(tar, f"{prefix}/{directory}")

    for path in entries:
        rel = path.relative_to(base).as_posix()
        name = f"{prefix}/{rel}"
        if path.is_dir():
            add_dir(tar, name)
        else:
            add_file(tar, path, name, rel)


def add_dir(tar: tarfile.TarFile, name: str) -> None:
    info = tarfile.TarInfo(name)
    info.type = tarfile.DIRTYPE
    info.mode = 0o755
    info.uid = info.gid = 0
    info.uname = info.gname = "root"
    info.mtime = 0
    tar.addfile(info)


def add_file(tar: tarfile.TarFile, path: Path, name: str, rel: str) -> None:
    data = path.read_bytes()
    info = tarfile.TarInfo(name)
    info.size = len(data)
    info.uid = info.gid = 0
    info.uname = info.gname = "root"
    info.mtime = 0
    if rel in ("preinst", "postinst") or rel == "bin/kioskmate":
        info.mode = 0o755
    else:
        info.mode = 0o644
    tar.addfile(info, io.BytesIO(data))


def ar_member(name: str, data: bytes) -> bytes:
    encoded = name.encode("ascii")
    if len(encoded) > 16:
        raise ValueError(f"ar member name too long: {name}")
    header = (
        encoded.ljust(16, b" ")
        + b"0".rjust(12, b" ")
        + b"0".rjust(6, b" ")
        + b"0".rjust(6, b" ")
        + b"100644".rjust(8, b" ")
        + str(len(data)).encode("ascii").rjust(10, b" ")
        + b"`\n"
    )
    body = header + data
    if len(data) % 2:
        body += b"\n"
    return body


if __name__ == "__main__":
    raise SystemExit(main())
