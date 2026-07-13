# KioskMate

KioskMate is a lightweight Go supervisor for Home Assistant kiosk displays. It runs a small native daemon, starts an external Chromium browser for the dashboard, exposes an embedded Admin UI, and integrates with Home Assistant through MQTT discovery.

The project is inspired by the Home Assistant kiosk workflow popularized by [TouchKio](https://github.com/leukipp/touchkio), but KioskMate starts as its own Go-based implementation with a smaller runtime footprint and a cleaner service/package layout.

## Features

- Native `kioskmate` daemon written in Go.
- External Chromium rendering instead of bundling Electron.
- Embedded Admin UI with setup token, password login and session protection.
- Kiosk page management with manual switching, rotation and time rules.
- Performance profiles for Raspberry Pi and small kiosk hardware, including a `low-power` Chromium mode.
- Kiosk theme handling with native `dark` mode and optional Chromium `force-dark` mode.
- Browser watchdog for memory/CPU runaway protection.
- Browser start, stop, restart, refresh and active-page controls.
- Persistent local Chromium DevTools control for reloads, navigation and authenticated screenshots without avoidable restarts.
- Home Assistant authentication guard that stops reconnect loops after invalid tokens or IP-ban responses.
- HTTP and render checks for kiosk pages, including Home Assistant 403/auth hints.
- Optional separate browser profiles per kiosk page to isolate Home Assistant sessions.
- Hardware controls for display power, brightness, audio, microphone and keyboard where supported by the OS.
- Home Assistant MQTT discovery for sensors, diagnostics, page health and controls.
- Diagnostic bundle export with redacted config, logs and runtime status.
- System actions for service restart, reboot, shutdown and apt jobs.
- GitHub release updater with Debian package download and digest verification.
- Debian packages for `arm64` and `amd64`.
- JSON config at `~/.config/kioskmate/config.json`.

## Status

KioskMate `0.1.10` is the final reliability-focused release before the planned `0.2.0` Admin UI redesign.

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
wget https://github.com/MickLesk/KioskMate/releases/download/v0.1.10/kioskmate_0.1.10_arm64.deb
sudo apt install ./kioskmate_0.1.10_arm64.deb
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
    "isolate_page_sessions": false,
    "theme": "dark",
    "zoom_percent": 125
  },
  "performance": {
    "profile": "low-power",
    "gpu_mode": "auto",
    "reduce_motion": true
  },
  "watchdog": {
    "enabled": true,
    "max_rss_mb": 900,
    "max_cpu_percent": 300,
    "cpu_grace": 600000000000
  }
}
```

Durations are currently stored as Go JSON durations in nanoseconds. The watchdog treats memory pressure as the main automatic restart signal. CPU-only pressure is tolerated for at least 10 minutes and automatic watchdog restarts are rate-limited to avoid restart loops on busy Raspberry Pi dashboards.

For dashboards with sustained Chromium CPU/GPU load on Raspberry Pi hardware, use **Settings -> Browser and performance -> Performance profile -> Low power** or apply **Safe Mode**. Chromium will still show several processes for a single kiosk window; the low-power profile reduces renderer/raster parallelism and expensive GPU raster features.

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

KioskMate also publishes per-page Home Assistant entities for:

- page activation
- active page state
- page URL/name/index
- HTTP reachability
- HTTP status code
- last page-health error
- last page-health check time

Diagnostic entities also expose the Home Assistant authentication guard, its reason and timestamp, and whether Chromium DevTools control is connected.

If entities become stale after page renames, use **MQTT -> Reset discovery** in the Admin UI. It clears known KioskMate discovery topics and republishes the current set.

## Home Assistant 403 / White Page Troubleshooting

If Home Assistant returns `403 Forbidden`, KioskMate trips its authentication guard and stops Chromium to prevent a reconnect loop. Remove the kiosk IP from `ip_bans.yaml`, restart Home Assistant, then use **Dashboard -> Reset HA session**. The reset waits for all Chromium processes, backs up the old session under `~/.config/kioskmate/Browser/SessionBackups`, clears current Chromium authentication storage and starts a clean session.

If HTTP checks are OK but the display is white, use **Dashboard -> Refresh snapshot** or **Kiosk -> Render check**. For the active Chromium display this captures the real signed-in browser session through the local DevTools connection. Snapshots are only captured on demand and cached briefly.

Regular Home Assistant health checks use the unauthenticated `/manifest.json` endpoint and exponential error backoff. They do not submit or reuse Home Assistant credentials.

For Home Assistant pages, KioskMate **Kiosk theme** `dark` and `light` select Home Assistant's built-in `default` theme and synchronize its native color mode for the signed-in kiosk user through the local Chromium DevTools connection. Save the browser settings and restart the display after changing the mode. The Dashboard reports whether the requested theme was actually applied. This avoids Chromium's expensive page-wide color transformation. Use `force-dark` only for pages or custom cards that still ignore their native dark mode; it consumes more CPU/GPU.

For multi-dashboard setups, enable **Settings -> Browser and performance -> Separate browser profile per page** when one broken Home Assistant session should not affect every page.

## Diagnostics

The Logs page can show core logs, browser logs, systemd journal, service status and paths. Use:

- **Download logs** for a plain-text log export.
- **Diagnostic bundle** for a ZIP containing redacted config, runtime status and logs.

## Packaging

```bash
VERSION=0.1.10 ARCH=arm64 bash scripts/package-deb.sh
VERSION=0.1.10 ARCH=amd64 bash scripts/package-deb.sh
```

Cross-platform packaging without `dpkg-deb`:

```bash
python scripts/package-deb.py --version 0.1.10 --arch arm64 --arch amd64
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

The Admin API requires an authenticated session, bearer token or `X-KioskMate-Token` header for privileged endpoints. Browser sessions use strict same-site cookies and state-changing session requests must have the same origin. Config API responses and exports redact Admin and MQTT secrets. Optional built-in TLS can be configured with `admin.tls_cert` and `admin.tls_key`. Keep the Admin UI inside a trusted LAN and avoid exposing it directly to the internet.
