# KioskMate Supervisor

KioskMate is the Go-based kiosk supervisor for Home Assistant dashboards. It keeps the heavy web renderer in an external Chromium process and keeps the Admin API, MQTT integration, updater and watchdog in a small native daemon.

## Scope

- Go-based `kioskmate` daemon.
- Embedded Admin UI with password/session protection and setup-token recovery.
- Dedicated Admin sections for Dashboard, Kiosk, Scheduler, MQTT, Hardware, System, Terminal, Logs and Settings.
- Multi-page kiosk streaming with manual page switching, rotation rules and time rules.
- External Chromium launch with performance profiles, GPU mode, reduced motion and a process watchdog.
- Browser start, stop, reload, restart, page selection and Home Assistant session reset.
- Privileged jobs for apt update/upgrade, service restart, reboot and shutdown.
- Optional one-shot sudo/root password forwarding for privileged actions. Passwords are not stored.
- Hardware controls for display power, brightness, audio, microphone and keyboard.
- MQTT Home Assistant discovery and remote control entities.
- GitHub release updater with Debian package download and SHA256 verification.
- systemd user service and dependency-free Debian packaging script.
- Raspberry benchmark collection script.
- JSON config stored under `~/.config/kioskmate/config.json`.

## Toolchain

KioskMate targets Go `1.26.4` or newer. The daemon currently uses the Go standard library only.

## Run

```bash
cd v2
go run ./cmd/kioskmate
```

On first start the config file is created with a random admin setup token. Open the printed Admin URL, enter `admin.token` once and create an admin password.

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

## Config

```json
{
  "admin": {
    "bind": "0.0.0.0",
    "port": 33333,
    "token": "generated-token"
  },
  "kiosk": {
    "pages": [
      {"name": "Home", "url": "http://homeassistant.local:8123"}
    ],
    "browser_command": "chromium-browser",
    "extra_args": [],
    "user_data_dir": "~/.config/kioskmate/Browser"
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

Durations are stored as Go JSON durations in nanoseconds for now.

## Browser

The daemon looks for:

- `chromium-browser`
- `chromium`
- `google-chrome-stable`
- `google-chrome`
- `microsoft-edge`

For Raspberry Pi systems with GPU or renderer spikes, use the Admin UI performance profile `raspberry`, GPU mode `software` and reduced motion.

## MQTT

Example:

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

The generic command topic is `kioskmate/<node>/command`.

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

Home Assistant discovery exposes browser state, system controls, display/audio/input controls, page controls, hardware diagnostics, update status and service heartbeat.

## Updater

The Admin UI checks GitHub releases, selects the matching Debian asset for the current architecture, verifies the `sha256:` digest when present and installs through:

```bash
sudo -n apt-get install -y /tmp/kioskmate_*.deb
```

Unattended updates still require passwordless sudo. Interactive system actions can use passwordless sudo, a one-shot sudo password or a one-shot root password through `su`.

## Packaging

```bash
cd v2
VERSION=2.0.0-dev ARCH=arm64 bash scripts/package-deb.sh
VERSION=2.0.0-dev ARCH=amd64 bash scripts/package-deb.sh
```

The package installs:

- `/usr/bin/kioskmate`
- `/usr/lib/systemd/user/kioskmate.service`
- `/usr/share/doc/kioskmate/README.md`

Tags matching `v2*` trigger the release workflow and upload:

- `kioskmate_<version>_arm64.deb`
- `kioskmate_<version>_amd64.deb`

## Benchmark

```bash
cd v2
bash scripts/benchmark.sh 180
```

The script writes a CSV with load average, memory and the hottest KioskMate/Chromium processes every two seconds.

## Security

The Admin API requires an authenticated session, bearer token or `X-Go-Kiosk-Token` header for privileged endpoints. Keep the Admin UI inside a trusted LAN.

## Migration

KioskMate keeps a compatibility importer for the previous config file and imports dashboard URLs, MQTT settings, presentation settings and the admin password hash when no custom KioskMate config exists yet.
