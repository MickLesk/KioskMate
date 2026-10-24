# KioskMate

KioskMate is a lightweight Go supervisor for Home Assistant kiosk displays. It runs a small native daemon, starts an external Chromium browser for the dashboard, exposes an embedded Admin UI, and integrates with Home Assistant through MQTT discovery.

The project is inspired by the Home Assistant kiosk workflow popularized by [TouchKio](https://github.com/leukipp/touchkio), but KioskMate starts as its own Go-based implementation with a smaller runtime footprint and a cleaner service/package layout.

## Features

- Native `kioskmate` daemon written in Go.
- External Chromium rendering instead of bundling Electron.
- Embedded Admin UI with setup token, password login and session protection.
- Visual kiosk sequence management with Storybook cards, a flow graph, a guided page wizard, rotation, fixed schedules and MQTT triggers.
- Performance profiles for Raspberry Pi and small kiosk hardware, including a `low-power` Chromium mode.
- Optional tab mode (`kiosk.page_switch: "tabs"`) that keeps recent pages open and switches without reloading.
- Workflows: rules that react to an MQTT message, a time of day or a webhook and show a page for a while, switch the display, set brightness, reload, show a banner or run a script.
- Local pages without an extra server: clock, weather, countdown, news headlines, camera grid, Immich photo frame, an image slideshow from a folder, or your own HTML.
- Scheduled maintenance reloads (every N minutes, at a fixed time or after N rotation rounds) and preloaded pages in tab mode.
- Optional desktop-free mode with the Cage compositor, see [docs/HEADLESS-CAGE.md](docs/HEADLESS-CAGE.md).
- Ready-to-flash Raspberry Pi OS Lite image with Cage, Chromium and first-boot setup from `kioskmate.txt`, see [docs/IMAGE.md](docs/IMAGE.md).
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
- Experimental Windows 10/11 support with Microsoft Edge or Google Chrome, see [docs/WINDOWS.md](docs/WINDOWS.md).

## Status

KioskMate `1.0.0-pre.2` is a pre-release with a redesigned Admin UI (Overview, Programme, Pages, Automations, Device) and inserts that show other pages in between on a fixed rhythm. The current stable release, `0.9.1`, reports undervoltage and missing touch devices, shows CPU per device instead of per core sum, and adds a display backend setting on top of 0.9.0 (workflows, tab mode, local pages, a renderer watchdog, a Raspberry Pi OS image and experimental Windows support). See [CHANGELOG.md](CHANGELOG.md).

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
kioskmate --repair
kioskmate --admin-reset
KIOSKMATE_ADMIN_PASSWORD='new-password' kioskmate --admin-password
```

## Install A Release

For Raspberry Pi / ARM64:

```bash
cd /tmp
wget https://github.com/MickLesk/KioskMate/releases/download/v0.9.1/kioskmate_0.9.1_arm64.deb
sudo apt install ./kioskmate_0.9.1_arm64.deb
systemctl --user enable --now kioskmate.service
```

Run the final `systemctl --user` command as the desktop user that owns the kiosk session, not as `root`. If the package was installed from a root shell, leave that shell with `exit` before enabling the user service.

For amd64, use the `_amd64.deb` asset. Windows builds are the `_windows_amd64.zip` and `_windows_arm64.zip` assets, see [docs/WINDOWS.md](docs/WINDOWS.md).

KioskMate checks for updates when the service starts and every six hours. Available releases appear in the Admin header and Dashboard. Installing a Debian update requires passwordless sudo, a sudo password or the root password. Credentials can optionally remain in process memory for 15 minutes; they are never stored in the config file or on disk.

The updater can run a preflight before installation, validates the downloaded package name, architecture and SHA-256 digest, creates a private configuration backup, records the result in `~/.config/kioskmate/update-history.json` and verifies the expected version after restart. When a previously working package is known, **Settings -> Updates -> Rollback** reinstalls that digest-verified GitHub release with APT's explicit downgrade option.

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

## Workflows

Workflows are rules on top of the page sequence. Create them under **Kiosk -> Pages and workflow -> Workflows**; they are stored in `kiosk.rules`.

```json
{
  "id": "doorbell",
  "name": "Doorbell",
  "trigger": { "type": "mqtt", "topic": "home/doorbell", "payload": "ring" },
  "conditions": { "start": "07:00", "end": "22:00" },
  "actions": [
    { "type": "display", "value": "ON" },
    { "type": "show_page", "page_id": "camera", "duration_seconds": 30 },
    { "type": "overlay", "value": "Someone is at the door", "duration_seconds": 10 }
  ],
  "cooldown_seconds": 30
}
```

- Triggers: `mqtt` (exact topic, optional payload; Home Assistant entity states and presence sensors arrive this way), `time` (`at` as HH:MM, optional `days`), `webhook` (per-rule `token`, at least 16 characters), `touch` (any touch, click, key or wheel input on the kiosk page) and `idle` (no input for `idle_seconds`; fires once, the next input re-arms it).
- Conditions (optional, all must hold): `days`, a `start`/`end` window and `active_page_id`.
- Actions run in order: `show_page` (for `duration_seconds`, 0 means one hour, or with `until_resume` until a `resume_playlist` action), `resume_playlist`, `display` (`ON`/`OFF`, with `duration_seconds` it switches back afterwards), `brightness` (1-100), `reload`, `overlay`, `script` (JavaScript in the current page), `click` and `scroll` (CSS selector, also inside Home Assistant's shadow DOM; scroll also takes pixels) and `sound` (audio URL, or empty for a chime).
- Priority is fixed: a manual override from the Admin UI or Home Assistant beats workflows, workflows beat time rules, time rules beat the rotation. A workflow never replaces or ends a manual override.
- Saving reports likely mistakes as warnings: overlapping time rules, workflows with the same trigger, and workflows sharing a page's MQTT topic.
- A webhook rule is fired with `curl -X POST "http://<kiosk-ip>:33333/api/rules/<id>/trigger?token=<token>"` or the `X-KioskMate-Token` header. It needs no Admin login, so treat the URL like a password; wrong tokens count against the login throttle.
- **Test** in the Admin UI runs the saved actions immediately; the last 50 runs are listed under **Recent runs** and in the event journal.

## Local Pages

Besides web addresses and Home Assistant dashboards, a page can be rendered by KioskMate itself (`source_type` plus a `local` block). The page wizard sets them up; the browser opens them at `http://127.0.0.1:<admin port>/local/<page_id>`, and the URL is filled in automatically.

```json
{ "name": "Clock", "source_type": "clock", "local": { "title": "Lobby", "show_seconds": true } }
{ "name": "Photos", "source_type": "slideshow", "local": { "folder": "/home/pi/Pictures", "interval_seconds": 10, "fit": "contain", "shuffle": true } }
{ "name": "Notice", "source_type": "html", "local": { "html": "<h1>Welcome</h1>" } }
{ "name": "Weather", "source_type": "weather", "local": { "location": "Berlin", "units": "metric" } }
{ "name": "Holiday", "source_type": "countdown", "local": { "title": "Until the holiday", "target": "2027-07-01T08:00", "done_text": "Off we go" } }
{ "name": "News", "source_type": "news", "local": { "feeds": ["https://www.tagesschau.de/infoservices/alle-meldungen-100~rss2.xml"], "interval_seconds": 12 } }
{ "name": "Cameras", "source_type": "camera", "local": { "streams": ["http://192.168.1.10:1984/stream.html?src=frontdoor"] } }
{ "name": "Photos", "source_type": "immich", "local": { "immich_url": "http://immich.local:2283", "immich_api_key": "…", "immich_album": "<album id>", "interval_seconds": 15 } }
```

- `clock`: time and date in the browser's language; `hour12`, `show_seconds` and `hide_date` change the layout. It shifts slightly every minute against burn-in unless reduced motion is on.
- `slideshow`: JPG, PNG, WebP, GIF, AVIF and BMP files from an absolute folder, sorted by name or shuffled, cross-faded every `interval_seconds` (2-86400). New files appear after one pass. Chromium decodes every image at full size, so images close to the screen resolution keep memory low on a Raspberry Pi.
- `html`: up to 256 KB of your own HTML, pasted or loaded from a file in the wizard.
- `weather`: current weather, the next eight hours and six days from [Open-Meteo](https://open-meteo.com) (no API key). It uses `latitude`/`longitude` of the page, or the kiosk location from Programme → Advanced; `units` is `metric` or `imperial`. KioskMate fetches the forecast every 15 minutes and keeps showing the last one while offline.
- `countdown`: days, hours, minutes and seconds until `target` (local time, `YYYY-MM-DDTHH:MM`); afterwards `done_text`, or with `count_up` the time since.
- `news`: headlines from up to five RSS or Atom `feeds`, newest first, one every `interval_seconds` (4-600). KioskMate fetches the feeds every 15 minutes.
- `camera`: up to four `streams` in a grid. A go2rtc stream page (`…/stream.html?src=name`) is played as go2rtc's MP4 stream, which Chromium decodes in hardware; MJPEG and snapshot addresses work too. A stream that freezes for 15 seconds is reconnected. Browsers cannot play RTSP, so put go2rtc in front of RTSP cameras.
- `immich`: a photo frame from an [Immich](https://immich.app) album (`immich_album`, the ID from the album address) or random photos from the library. KioskMate fetches the photos with the API key, so the key never reaches the kiosk browser; the Admin API only reports whether one is stored.
- Local pages need no login but are only served to the device itself (and to a signed-in admin for a preview). They run in a sandbox with their own origin, so their scripts cannot use the Admin API, cookies or local storage.
- With admin TLS the browser gets `--allow-insecure-localhost` so a self-signed certificate is accepted for these pages.

## Home Assistant 403 / White Page Troubleshooting

If Home Assistant returns `403 Forbidden`, KioskMate trips its authentication guard and stops Chromium to prevent a reconnect loop. Remove the kiosk IP from `ip_bans.yaml`, restart Home Assistant, then use **Dashboard -> Reset HA session** exactly once. The reset waits for all Chromium processes, backs up the old session under `~/.config/kioskmate/Browser/SessionBackups`, clears current Chromium authentication and saved login storage, and starts a clean session. KioskMate never stores or automatically resubmits the Home Assistant password; Home Assistant keeps the authenticated session in its browser storage after the successful manual login.

If HTTP checks are OK but the display is white, use **Dashboard -> Refresh snapshot** or **Kiosk -> Pages -> Render check**. For the active Chromium display this captures the real signed-in browser session through the local DevTools connection. Snapshots are only captured on demand and cached briefly.

Regular Home Assistant health checks use the unauthenticated `/manifest.json` endpoint and exponential error backoff. They do not submit or reuse Home Assistant credentials.

KioskMate **Kiosk theme** `dark` and `light` emulate the corresponding OS `prefers-color-scheme` value on Chromium's active rendering target. This matches TouchKio's Electron `nativeTheme` behavior and preserves the Home Assistant user's selected custom theme. Save the browser settings and restart the display after changing the mode. The Dashboard reports whether Home Assistant actually applied the requested mode. Use `force-dark` only for pages or custom cards that ignore `prefers-color-scheme`; it consumes more CPU/GPU.

For multi-dashboard setups, enable **Kiosk -> Display and rendering -> Advanced browser settings -> Separate browser profile per page** when one broken Home Assistant session should not affect every page.

## Diagnostics

The Logs page can show core logs, browser logs, the structured KioskMate event journal, systemd journal, service status and paths. Use:

- **Download logs** for a plain-text log export.
- **Diagnostic bundle** for a ZIP containing redacted config, runtime status and logs.

The event journal is stored at `~/.config/kioskmate/events.jsonl`. It is bounded and rotated automatically. It records browser operations, Home Assistant authentication guards, MQTT connection/command state, admin security actions and maintenance/update jobs without storing passwords or MQTT payloads.

## Packaging

```bash
VERSION=0.9.1 ARCH=arm64 bash scripts/package-deb.sh
VERSION=0.9.1 ARCH=amd64 bash scripts/package-deb.sh
```

The shell command is a compatibility wrapper around the canonical, cross-platform package builder. Both architectures can be built in one invocation without `dpkg-deb`:

```bash
python scripts/package-deb.py --version 0.9.1 --arch arm64 --arch amd64
```

Packages embed a minified Admin UI: the builder runs `go run ./tools/minify` (esbuild, only at build time) and builds with `-tags minified`. Plain `go build` keeps embedding the readable sources.

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
