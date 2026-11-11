#!/usr/bin/env python3
"""Check the Raspberry Pi image build against the real package archives.

The image is only built manually (it takes hours), so a renamed or missing
package would otherwise surface late. This script:

  * checks that the pinned pi-gen tag (PI_GEN_REF in image/build.sh) exists,
  * downloads the arm64 package indexes of Debian and the Raspberry Pi archive
    for the release (RELEASE in image/build.sh),
  * fails when a required package (00-packages-nr, chromium) is missing and
    warns about optional ones (the loop in 00-packages/01-run-chroot.sh).

Usage: python3 scripts/check-image-packages.py
"""
import lzma
import re
import subprocess
import sys
import urllib.request
from pathlib import Path

ROOT = Path(__file__).resolve().parent.parent
STAGE = ROOT / "image" / "stage-kioskmate"
PI_GEN_REPO = "https://github.com/RPi-Distro/pi-gen.git"


def build_default(name: str) -> str:
    text = (ROOT / "image" / "build.sh").read_text()
    match = re.search(r'^%s="\$\{%s:-([^}]+)\}"' % (name, name), text, re.M)
    if not match:
        sys.exit(f"{name} default not found in image/build.sh")
    return match.group(1)


def required_packages() -> list[str]:
    names = []
    for line in (STAGE / "00-packages" / "00-packages-nr").read_text().splitlines():
        names += line.split("#", 1)[0].split()
    # 01-kioskmate installs Chromium ("chromium", older archives "chromium-browser").
    names.append("chromium|chromium-browser")
    return names


def optional_packages() -> list[str]:
    text = (STAGE / "00-packages" / "01-run-chroot.sh").read_text()
    match = re.search(r"for PACKAGE in ([^;]+);", text)
    return match.group(1).split() if match else []


def package_index(url: str) -> set[str]:
    request = urllib.request.Request(url, headers={"User-Agent": "kioskmate-ci"})
    with urllib.request.urlopen(request, timeout=120) as response:
        data = lzma.decompress(response.read()).decode("utf-8", "replace")
    names = set(re.findall(r"^Package: (\S+)$", data, re.M))
    names |= {p.strip().split(" ")[0] for p in re.findall(r"^Provides: (.+)$", data, re.M) for p in p.split(",")}
    return names


def main() -> int:
    ref = build_default("PI_GEN_REF")
    release = build_default("RELEASE")
    errors = 0

    tags = subprocess.run(["git", "ls-remote", "--tags", PI_GEN_REPO, ref], capture_output=True, text=True, timeout=120)
    if tags.returncode != 0 or f"refs/tags/{ref}" not in tags.stdout:
        print(f"ERROR: pi-gen tag {ref} not found in {PI_GEN_REPO}")
        errors += 1
    else:
        print(f"ok: pi-gen tag {ref}")
    if release not in ref:
        print(f"ERROR: RELEASE={release} does not match PI_GEN_REF={ref}")
        errors += 1

    indexes = [
        f"https://deb.debian.org/debian/dists/{release}/main/binary-arm64/Packages.xz",
        f"https://deb.debian.org/debian/dists/{release}/contrib/binary-arm64/Packages.xz",
        f"https://archive.raspberrypi.com/debian/dists/{release}/main/binary-arm64/Packages.xz",
    ]
    available: set[str] = set()
    for url in indexes:
        try:
            names = package_index(url)
        except Exception as exc:  # noqa: BLE001 - report and fail below
            print(f"ERROR: could not read {url}: {exc}")
            errors += 1
            continue
        print(f"ok: {len(names)} packages in {url}")
        available |= names

    for spec in required_packages():
        alternatives = spec.split("|")
        found = [name for name in alternatives if name in available]
        if found:
            print(f"ok: required {found[0]}")
        else:
            print(f"ERROR: required package {spec} not in {release} (arm64)")
            errors += 1
    for name in optional_packages():
        if name in available:
            print(f"ok: optional {name}")
        else:
            print(f"WARNING: optional package {name} not in {release} (arm64); the image builds without it")

    print("image package check:", "failed" if errors else "passed")
    return 1 if errors else 0


if __name__ == "__main__":
    sys.exit(main())
