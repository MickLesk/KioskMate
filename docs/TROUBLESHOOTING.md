# KioskMate Troubleshooting

## First response

Run these commands as the desktop user that owns the kiosk session:

```bash
kioskmate --admin-info
kioskmate --doctor
systemctl --user status kioskmate.service --no-pager --full
journalctl --user -u kioskmate.service -n 200 --no-pager
```

The Admin **Dashboard** is the primary runtime view. It separates display-session readiness, Chromium root state, DevTools control, page navigation, Home Assistant authentication protection and staged recovery.

## Browser does not start

1. Confirm that Chromium exists with `command -v chromium chromium-browser google-chrome-stable`.
2. Confirm that the service runs as the logged-in desktop user, not root.
3. Inspect the Dashboard display-session status. A Wayland socket or X11 socket must exist before Chromium starts.
4. Run **Browser Doctor**. Duplicate Chromium roots using the configured profile are reported separately from normal renderer/GPU child processes.
5. Do not repeatedly press Start. Identical operations are serialized and duplicate UI actions are cancelled, but the retained first failure is more useful than a restart loop.

## White Home Assistant page or 403

KioskMate classifies failures before acting:

- `transport`: Home Assistant could not be reached.
- `page`: the top-level document failed without authentication evidence.
- `session`: the browser reported invalid authentication or an invalid grant.
- `probable ban`: corroborated authentication `401/403` evidence activated the guard.

A protected image or camera `403` alone does not activate the guard. When the guard is active, remove the kiosk address from Home Assistant `ip_bans.yaml`, restart Home Assistant, then run **Reset HA session** once. The old Chromium login data is backed up before it is cleared. KioskMate never retries a stored Home Assistant password.

## MQTT and Home Assistant discovery

Use **MQTT -> Test connection** to view live validation, TCP/TLS, CONNACK and publish stages. MQTT 3.1.1 and MQTT 5.0 are both supported. For TLS, use `mqtts://`, keep certificate verification enabled and configure a CA or SNI override only when required.

Use **Preview discovery changes** before **Publish discovery**. The preview lists retained topics to add, keep and remove, plus entities omitted because the host does not provide the corresponding display/audio/sensor capability. Command results contain a correlation ID and are published below `<base>/<node>/command/result`.

## Updates and privileges

Run the update preflight before installation. It checks release metadata, architecture, package contents, disk space, required commands and administrator authentication, then displays the exact elevated scope. Passwords are held in process memory for at most 15 minutes and are never persisted.

Package upgrades preserve `~/.config/kioskmate/config.json`, create a backup and reload active user services through their real user bus. Run package installation as root or with sudo, but run `systemctl --user` commands as the desktop user.

## Performance capture

```bash
HEALTH_URL=http://127.0.0.1:33333/healthz bash scripts/benchmark.sh 900
```

Compare browser PSS rather than summed RSS. CPU is reported across the complete Chromium tree, where 100 percent equals one fully occupied CPU core. The CSV records renderer/GPU roles, navigation timing, heartbeat, duplicate roots and lifecycle counters.
