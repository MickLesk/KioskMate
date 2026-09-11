#!/usr/bin/env python3
import argparse
import gzip
import hashlib
import io
import json
import os
import re
import shutil
import subprocess
import tarfile
from pathlib import Path


SUPPORTED_ARCHES = {
    "amd64": "amd64",
    "arm64": "arm64",
}
DEBIAN_VERSION = re.compile(r"^[0-9][0-9A-Za-z.+:~\-]*$")


def main() -> int:
    parser = argparse.ArgumentParser(description="Build KioskMate Debian packages without dpkg-deb.")
    parser.add_argument("--version", required=True)
    parser.add_argument("--arch", choices=sorted(SUPPORTED_ARCHES), action="append", required=True)
    parser.add_argument("--root", default=Path(__file__).resolve().parents[1])
    args = parser.parse_args()

    if not DEBIAN_VERSION.fullmatch(args.version):
        parser.error("version must be a valid Debian package version")

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
    sbom = create_sbom(root, pkg / "usr/bin/kioskmate", version, arch)
    sbom_text = json.dumps(sbom, indent=2, sort_keys=True) + "\n"
    write_text(pkg / "usr/share/doc/kioskmate/sbom.spdx.json", sbom_text)
    write_text(dist / f"kioskmate_{version}_{arch}.spdx.json", sbom_text)

    write_text(pkg / "DEBIAN/control", control_file(version, arch))
    write_text(pkg / "DEBIAN/preinst", maintainer_preinst())
    write_text(pkg / "DEBIAN/postinst", maintainer_postinst())
    write_text(pkg / "DEBIAN/prerm", maintainer_prerm())
    write_text(pkg / "DEBIAN/postrm", maintainer_postrm())

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


def create_sbom(root: Path, binary: Path, version: str, arch: str) -> dict:
    output = subprocess.run(
        ["go", "list", "-m", "-json", "all"],
        cwd=root,
        check=True,
        capture_output=True,
        text=True,
    ).stdout
    modules = decode_json_stream(output)
    packages = [
        {
            "SPDXID": "SPDXRef-Package-KioskMate",
            "name": "KioskMate",
            "versionInfo": version,
            "downloadLocation": "NOASSERTION",
            "filesAnalyzed": False,
            "licenseConcluded": "NOASSERTION",
            "licenseDeclared": "NOASSERTION",
            "checksums": [{"algorithm": "SHA256", "checksumValue": hashlib.sha256(binary.read_bytes()).hexdigest()}],
            "externalRefs": [{"referenceCategory": "PACKAGE-MANAGER", "referenceType": "purl", "referenceLocator": f"pkg:github/MickLesk/KioskMate@{version}"}],
        }
    ]
    relationships = []
    for index, module in enumerate(modules[1:], start=1):
        module_path = module.get("Path", "")
        module_version = module.get("Version", "") or "unknown"
        spdx_id = f"SPDXRef-GoModule-{index}"
        packages.append({
            "SPDXID": spdx_id,
            "name": module_path,
            "versionInfo": module_version,
            "downloadLocation": "NOASSERTION",
            "filesAnalyzed": False,
            "licenseConcluded": "NOASSERTION",
            "licenseDeclared": "NOASSERTION",
            "externalRefs": [{"referenceCategory": "PACKAGE-MANAGER", "referenceType": "purl", "referenceLocator": f"pkg:golang/{module_path}@{module_version}"}],
        })
        relationships.append({"spdxElementId": "SPDXRef-Package-KioskMate", "relationshipType": "DEPENDS_ON", "relatedSpdxElement": spdx_id})
    return {
        "spdxVersion": "SPDX-2.3",
        "dataLicense": "CC0-1.0",
        "SPDXID": "SPDXRef-DOCUMENT",
        "name": f"KioskMate-{version}-{arch}",
        "documentNamespace": f"https://github.com/MickLesk/KioskMate/releases/download/v{version}/sbom-{arch}",
        "creationInfo": {"created": "1970-01-01T00:00:00Z", "creators": ["Tool: KioskMate package-deb.py"]},
        "packages": packages,
        "relationships": relationships,
    }


def decode_json_stream(source: str) -> list[dict]:
    decoder = json.JSONDecoder()
    offset = 0
    values = []
    while offset < len(source):
        while offset < len(source) and source[offset].isspace():
            offset += 1
        if offset >= len(source):
            break
        value, offset = decoder.raw_decode(source, offset)
        values.append(value)
    return values


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
Recommends: wlopm, pipewire-pulse | pulseaudio
Suggests: kscreen
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
  backup_config "$HOME_DIR/.config/kioskmate/config.json"
done
exit 0
"""


def maintainer_postinst() -> str:
    return """#!/usr/bin/env bash
set -e
reload_user_units() {
  for RUNTIME in /run/user/*; do
    [ -d "$RUNTIME" ] || continue
    UID_NAME="$(basename "$RUNTIME")"
    USER_NAME="$(getent passwd "$UID_NAME" | cut -d: -f1)"
    [ -n "$USER_NAME" ] || continue
    [ -S "$RUNTIME/bus" ] || continue
    user_systemctl "$RUNTIME" "$USER_NAME" daemon-reload >/dev/null 2>&1 || true
    # prerm stops the user service during upgrades; bring enabled instances back up.
    if user_systemctl "$RUNTIME" "$USER_NAME" --quiet is-enabled kioskmate.service >/dev/null 2>&1; then
      user_systemctl "$RUNTIME" "$USER_NAME" start kioskmate.service >/dev/null 2>&1 || true
    fi
  done
}
user_systemctl() {
  RUNTIME="$1"
  USER_NAME="$2"
  shift 2
  XDG_RUNTIME_DIR="$RUNTIME" DBUS_SESSION_BUS_ADDRESS="unix:path=$RUNTIME/bus" runuser -u "$USER_NAME" -- systemctl --user "$@"
}
if command -v systemctl >/dev/null 2>&1; then
  reload_user_units
fi
exit 0
"""


def maintainer_prerm() -> str:
    return """#!/usr/bin/env bash
set -e
stop_user_units() {
  for RUNTIME in /run/user/*; do
    [ -d "$RUNTIME" ] || continue
    UID_NAME="$(basename "$RUNTIME")"
    USER_NAME="$(getent passwd "$UID_NAME" | cut -d: -f1)"
    [ -n "$USER_NAME" ] || continue
    [ -S "$RUNTIME/bus" ] || continue
    user_systemctl "$RUNTIME" "$USER_NAME" stop kioskmate.service >/dev/null 2>&1 || true
  done
}
user_systemctl() {
  RUNTIME="$1"
  USER_NAME="$2"
  shift 2
  XDG_RUNTIME_DIR="$RUNTIME" DBUS_SESSION_BUS_ADDRESS="unix:path=$RUNTIME/bus" runuser -u "$USER_NAME" -- systemctl --user "$@"
}
if command -v systemctl >/dev/null 2>&1; then
  stop_user_units
fi
exit 0
"""


def maintainer_postrm() -> str:
    return """#!/usr/bin/env bash
set -e
reload_user_units() {
  for RUNTIME in /run/user/*; do
    [ -d "$RUNTIME" ] || continue
    UID_NAME="$(basename "$RUNTIME")"
    USER_NAME="$(getent passwd "$UID_NAME" | cut -d: -f1)"
    [ -n "$USER_NAME" ] || continue
    [ -S "$RUNTIME/bus" ] || continue
    user_systemctl "$RUNTIME" "$USER_NAME" daemon-reload >/dev/null 2>&1 || true
  done
}
user_systemctl() {
  RUNTIME="$1"
  USER_NAME="$2"
  shift 2
  XDG_RUNTIME_DIR="$RUNTIME" DBUS_SESSION_BUS_ADDRESS="unix:path=$RUNTIME/bus" runuser -u "$USER_NAME" -- systemctl --user "$@"
}
if [ "$1" = "remove" ] || [ "$1" = "purge" ]; then
  if command -v systemctl >/dev/null 2>&1; then
    reload_user_units
  fi
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
    if rel in ("preinst", "postinst", "prerm", "postrm") or rel == "bin/kioskmate":
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
