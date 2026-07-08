# KioskMate

KioskMate is a lightweight Go supervisor for Home Assistant kiosk displays. It runs a small native daemon, starts an external Chromium browser for the dashboard, exposes an embedded Admin UI, and integrates with Home Assistant through MQTT discovery.

The project is inspired by the Home Assistant kiosk workflow popularized by [TouchKio](https://github.com/leukipp/touchkio), but KioskMate starts as its own Go-based implementation with a smaller runtime footprint and a cleaner service/package layout.

## Features

- Native `kioskmate` daemon written in Go.
- External Chromium rendering instead of bundling Electron.
- Embedded Admin UI with setup token, password login and session protection.
- Kiosk page management with manual switching, rotation and time rules.
- Performance profiles for Raspberry Pi and small kiosk hardware.
- Browser watchdog for memory/CPU runaway protection.
- Browser start, stop, restart, refresh and active-page controls.
- Hardware controls for display power, brightness, audio, microphone and keyboard where supported by the OS.
- Home Assistant MQTT discovery for sensors, diagnostics and controls.
- System actions for service restart, reboot, shutdown and apt jobs.
- GitHub release updater with Debian package download and digest verification.
- Debian packages for `arm64` and `amd64`.
- JSON config at `~/.config/kioskmate/config.json`.

## Status

KioskMate starts at `0.0.1-alpha1`. Treat alpha releases as test builds: the config format and Admin UI can still change while the Go implementation is hardened on Raspberry Pi devices.

## Requirements

- Go `1.26.4` or newer for development.
- Debian-based kiosk OS for packaged installs.
- Chromium, Chromium Browser or Google Chrome on the kiosk device.
- `systemd --user` for the packaged service.
- `fonts-noto-color-emoji` for Home Assistant emoji/icon rendering.

## Run From Source

```bash
go run ./cmd/kioskmate
```

On first start KioskMate creates a config file and prints an admin setup token. Open the Admin UI and use that token once to create the first password.

Default Admin UI:

```text
http://<kiosk-ip>:33333
```

Useful recovery commands:

```bash
kioskmate --admin-info
kioskmate --doctor
kioskmate --repair
kioskmate --admin-reset
KIOSKMATE_ADMIN_PASSWORD='new-password' kioskmate --admin-password
```

## Install A Release

For Raspberry Pi / ARM64:

```bash
cd /tmp
wget https://github.com/MickLesk/KioskMate/releases/download/v0.0.1-alpha1/kioskmate_0.0.1-alpha1_arm64.deb
sudo apt install ./kioskmate_0.0.1-alpha1_arm64.deb
systemctl --user daemon-reload
systemctl --user enable --now kioskmate.service
```

For amd64, use the `_amd64.deb` asset.

## Config

```json
{
  "version": 2,
  "admin": {
    "bind": "0.0.0.0",
    "port": 33333,
    "token": "generated-token"
  },
  "kiosk": {
    "pages": [
      {"name": "Home Assistant", "url": "http://homeassistant.local:8123"}
    ],
    "browser_command": "chromium-browser",
    "extra_args": [],
    "user_data_dir": "~/.config/kioskmate/Browser",
    "theme": "dark",
    "zoom_percent": 125
  },
  "performance": {
    "profile": "raspberry",
    "gpu_mode": "auto",
    "reduce_motion": true
  },
  "watchdog": {
    "enabled": true,
    "max_rss_mb": 900,
    "max_cpu_percent": 180
  }
}
```

Durations are currently stored as Go JSON durations in nanoseconds.

## MQTT

```json
"mqtt": {
  "enabled": true,
  "url": "mqtt://homeassistant.local:1883",
  "version": "3.1.1",
  "username": "kiosk",
  "password": "secret",
  "discovery": "homeassistant",
  "node": "kioskmate",
  "interval": 30000000000
}
```

The generic command topic is:

```text
kioskmate/<node>/command
```

Supported command payloads:

- `start`
- `stop`
- `restart`
- `refresh`
- `next`
- `previous`
- `reboot`
- `shutdown`
- `apt-update`
- `apt-upgrade`

## Packaging

```bash
VERSION=0.0.1-alpha1 ARCH=arm64 bash scripts/package-deb.sh
VERSION=0.0.1-alpha1 ARCH=amd64 bash scripts/package-deb.sh
```

Cross-platform packaging without `dpkg-deb`:

```bash
python scripts/package-deb.py --version 0.0.1-alpha1 --arch arm64 --arch amd64
```

The package installs:

- `/usr/bin/kioskmate`
- `/usr/lib/systemd/user/kioskmate.service`
- `/usr/share/doc/kioskmate/README.md`

Release tags matching `v0*` build and upload:

- `kioskmate_<version>_arm64.deb`
- `kioskmate_<version>_amd64.deb`

## Benchmark

```bash
bash scripts/benchmark.sh 180
```

The script writes a CSV with load average, memory usage and the hottest KioskMate/Chromium processes every two seconds.

## Security

The Admin API requires an authenticated session, bearer token or `X-KioskMate-Token` header for privileged endpoints. Keep the Admin UI inside a trusted LAN and avoid exposing it directly to the internet.
