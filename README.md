# KioskMate

KioskMate is a lightweight Go supervisor for Home Assistant kiosk displays. It runs a small native daemon, starts an external Chromium browser for the dashboard, exposes an embedded Admin UI, and integrates with Home Assistant through MQTT discovery.

The project is inspired by the Home Assistant kiosk workflow popularized by [TouchKio](https://github.com/leukipp/touchkio), but KioskMate starts as its own Go-based implementation with a smaller runtime footprint and a cleaner service/package layout.

## Features

- Native `kioskmate` daemon written in Go.
- External Chromium rendering instead of bundling Electron.
- Embedded Admin UI with setup token, password login and session protection.
- Visual kiosk sequence management with Storybook cards, a flow graph, a guided page wizard, rotation, fixed schedules and MQTT triggers.
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

KioskMate `0.8.0` is the current stable release. The `dev-kiosk-0.9` branch develops the production-readiness work for `0.9.0`: browser lifecycle timing, Chromium role metrics, display-session readiness, duplicate-root cleanup, workflow conflict diagnostics, MQTT command correlation, trusted proxy handling and responsive Admin E2E coverage.

The Admin UI is organized by task:

- **Dashboard** for live status, quick display control and recovery.
- **Kiosk -> Pages and workflow** for dashboard URLs, display order, rotations and fixed time rules.
- **Kiosk -> Display and rendering** for appearance, Home Assistant theme behavior, performance and watchdog settings.
- **MQTT** for broker connectivity and Home Assistant discovery.
- **System** for device/time controls, maintenance jobs and logs.
- **Settings** for access, backups and updates.

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
kioskmate --soak-report
kioskmate --repair
kioskmate --admin-reset
KIOSKMATE_ADMIN_PASSWORD='new-password' kioskmate --admin-password
```

## Install A Release

For Raspberry Pi / ARM64:

```bash
cd /tmp
wget https://github.com/MickLesk/KioskMate/releases/download/v0.8.0/kioskmate_0.8.0_arm64.deb
sudo apt install ./kioskmate_0.8.0_arm64.deb
systemctl --user enable --now kioskmate.service
```

Run the final `systemctl --user` command as the desktop user that owns the kiosk session, not as `root`. If the package was installed from a root shell, leave that shell with `exit` before enabling the user service.

For amd64, use the `_amd64.deb` asset.

KioskMate checks for updates when the service starts and every six hours. Available releases appear in the Admin header and Dashboard. Installing a Debian update requires passwordless sudo, a sudo password or the root password. Credentials can optionally remain in process memory for 15 minutes; they are never stored in the config file or on disk.

The updater can run a preflight before installation, displays the exact privilege scope, validates the downloaded package name, architecture and SHA-256 digest, creates a private configuration backup, records the result in `~/.config/kioskmate/update-history.json` and verifies the expected version after restart. When a previously working package is known, **Settings -> Updates -> Rollback** reinstalls that digest-verified GitHub release with APT's explicit downgrade option.

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
      {
        "page_id": "home_assistant",
        "name": "Home Assistant",
        "url": "http://homeassistant.local:8123",
        "source_type": "home_assistant",
        "display_mode": "duration",
        "duration_seconds": 60,
        "display_options": {"brightness": 100}
      }
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

For dashboards with sustained Chromium CPU/GPU load on Raspberry Pi hardware, use **Kiosk -> Display and rendering -> Performance profile -> Low power** or apply **Safe Mode**. Chromium will still show several processes for a single kiosk window; the low-power profile reduces renderer/raster parallelism and expensive GPU raster features.

Before promoting a build, open **Kiosk -> Display and rendering**, start a new measurement and leave the real dashboard running for 24 hours. The persistent soak report survives service restarts and checks telemetry coverage, browser restarts, duplicate Chromium roots, Home Assistant authentication blocks and recovery-budget exhaustion. Download the JSON report from the same panel or run `kioskmate --soak-report` locally when the Admin UI is unavailable.

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

Diagnostic entities also expose the Home Assistant authentication guard, Chromium DevTools control, MQTT runtime state, last MQTT publish, timezone, NTP server, synchronization state and the complete updater state. Home Assistant receives the actual latest release, update availability/installing state, last check/error and rollback target, plus actions to check updates or trigger a passwordless rollback.

If entities become stale after page renames, use **MQTT -> Reset discovery** in the Admin UI. It clears known KioskMate discovery topics and republishes the current set.

Use **MQTT -> Preview discovery changes** before publishing to see retained topics that will be added, kept, removed or omitted because the host lacks the required hardware capability. Commands accept a JSON envelope such as `{"command":"reload","correlation_id":"ha-42"}` and publish their result to `kioskmate/<node>/command/result`.

## Home Assistant 403 / White Page Troubleshooting

If Home Assistant returns `403 Forbidden`, KioskMate trips its authentication guard and stops Chromium to prevent a reconnect loop. Remove the kiosk IP from `ip_bans.yaml`, restart Home Assistant, then use **Dashboard -> Reset HA session** exactly once. The reset waits for all Chromium processes, backs up the old session under `~/.config/kioskmate/Browser/SessionBackups`, clears current Chromium authentication and saved login storage, and starts a clean session. KioskMate never stores or automatically resubmits the Home Assistant password; Home Assistant keeps the authenticated session in its browser storage after the successful manual login.

If HTTP checks are OK but the display is white, use **Dashboard -> Refresh snapshot** or **Kiosk -> Pages -> Render check**. For the active Chromium display this captures the real signed-in browser session through the local DevTools connection. Snapshots are only captured on demand and cached briefly.

Regular Home Assistant health checks use the unauthenticated `/manifest.json` endpoint and exponential error backoff. They do not submit or reuse Home Assistant credentials.

KioskMate distinguishes a network failure, a protected subresource response, an expired browser session and a corroborated probable IP ban. A lone camera/image `403` is not enough to trip the guard. Once the guard is active, automatic health requests and reconnect attempts pause until an explicit recovery action is selected.

KioskMate **Kiosk theme** `dark` and `light` emulate the corresponding OS `prefers-color-scheme` value on Chromium's active rendering target. This matches TouchKio's Electron `nativeTheme` behavior and preserves the Home Assistant user's selected custom theme. Save the browser settings and restart the display after changing the mode. The Dashboard reports whether Home Assistant actually applied the requested mode. Use `force-dark` only for pages or custom cards that ignore `prefers-color-scheme`; it consumes more CPU/GPU.

For multi-dashboard setups, enable **Kiosk -> Display and rendering -> Advanced browser settings -> Separate browser profile per page** when one broken Home Assistant session should not affect every page.

## Diagnostics

The Logs page can show core logs, browser logs, the structured KioskMate event journal, systemd journal, service status and paths. Use:

- **Download logs** for a plain-text log export.
- **Diagnostic bundle** for a ZIP containing redacted config, runtime status and logs.

The event journal is stored at `~/.config/kioskmate/events.jsonl`. It is bounded and rotated automatically. It records browser operations, Home Assistant authentication guards, MQTT connection/command state, admin security actions and maintenance/update jobs without storing passwords or MQTT payloads.

If Chromium never starts, check **Dashboard -> Health and protection -> Display session**. KioskMate waits for a real Wayland socket (`$XDG_RUNTIME_DIR/$WAYLAND_DISPLAY`) or X11 socket before launching the browser and retains an actionable reason when neither exists. See [Troubleshooting](docs/TROUBLESHOOTING.md) for commands and state interpretation.

## Packaging

```bash
VERSION=0.8.0 ARCH=arm64 bash scripts/package-deb.sh
VERSION=0.8.0 ARCH=amd64 bash scripts/package-deb.sh
```

The shell command is a compatibility wrapper around the canonical, cross-platform package builder. Both architectures can be built in one invocation without `dpkg-deb`:

```bash
python scripts/package-deb.py --version 0.8.0 --arch arm64 --arch amd64
```

The package installs:

- `/usr/bin/kioskmate`
- `/usr/lib/systemd/user/kioskmate.service`
- `/usr/share/doc/kioskmate/README.md`
- `/usr/share/doc/kioskmate/sbom.spdx.json`

Release tags matching `v0*` build and upload:

- `kioskmate_<version>_arm64.deb`
- `kioskmate_<version>_amd64.deb`

## Benchmark

```bash
bash scripts/benchmark.sh 180
```

The script writes a CSV with load average, host memory, browser process count/PSS/CPU, renderer and GPU role metrics, navigation load/first-frame/heartbeat timing, duplicate roots and start/restart/recovery counters every two seconds. Set `HEALTH_URL=http://127.0.0.1:33333/healthz` to include the runtime API fields.

## Security

The Admin API requires an authenticated session, bearer token or `X-KioskMate-Token` header for privileged endpoints. Browser sessions use strict same-site cookies and state-changing session requests must have the same origin. Config API responses and exports redact Admin and MQTT secrets. Optional built-in TLS can be configured with `admin.tls_cert` and `admin.tls_key`. Forwarded client addresses are ignored unless the direct peer is listed in `admin.trusted_proxies`; only configure proxies you control. The built-in terminal is disabled by default and must be explicitly enabled. Keep the Admin UI inside a trusted LAN and avoid exposing it directly to the internet.
