# KioskMate Hardware Notes

KioskMate targets small always-on kiosk devices first, especially Raspberry Pi systems with DSI or HDMI touch displays.

## Recommended Baseline

- Raspberry Pi 4, Raspberry Pi 5 or comparable amd64 mini PC.
- Debian-based OS with a graphical session.
- Chromium or Google Chrome installed.
- `systemd --user` available for `kioskmate.service`.
- `fonts-noto-core` and `fonts-noto-color-emoji` for Home Assistant icons and emoji.

## Optional Commands

KioskMate detects available tools and exposes only what the system can actually do.

| Feature | Typical Requirement |
| --- | --- |
| Display power | `wlopm`, `kscreen-doctor`, `xset` or a working desktop stack |
| Brightness | `/sys/class/backlight/*/brightness` or `ddcutil` |
| Audio | PipeWire/PulseAudio with `pactl` |
| Keyboard | Raspberry Pi OS Wayland with `squeekboard` |
| Reboot/shutdown | passwordless sudo, sudo password or root password for the action |
| apt update/upgrade | passwordless sudo, sudo password or root password for the action |
| Battery | `/sys/class/power_supply/*/capacity` |
| Illuminance | `/sys/bus/iio/devices/*/in_illuminance_raw` |

Unsupported controls remain visible as capability diagnostics with an installation or configuration hint; KioskMate does not publish unusable `null` controls to Home Assistant. Each capability includes a check timestamp so stale detection can be distinguished from a current unsupported result.

## Raspberry Performance

For heavy Home Assistant dashboards on Raspberry hardware, start with:

- Performance profile: `raspberry`
- GPU mode: `auto`
- Reduce motion: enabled
- Watchdog: enabled
- Browser RSS limit: 900-1200 MB
- Browser CPU grace: 10 minutes

Use the benchmark helper before and after changes:

```bash
bash scripts/benchmark.sh 180
```

For comparable results, use the same dashboard, resolution, Chromium package and GPU mode. Set `HEALTH_URL` to the local Admin health endpoint to add navigation and process-role data:

```bash
HEALTH_URL=http://127.0.0.1:33333/healthz bash scripts/benchmark.sh 900
```

For a release-candidate test, reset the runtime measurement in **Kiosk -> Display and rendering** and run the production dashboard for 24 hours. KioskMate persists the measurement window and exposes a downloadable soak report; a passing report requires adequate samples, no duplicate browser root, no authentication block and no restart storm.

Chromium intentionally uses separate browser, renderer, GPU and utility processes. Diagnose role-specific CPU/PSS in the Dashboard instead of treating process count or virtual address space as resident memory.

## Home Assistant Tips

- Prefer one optimized Lovelace dashboard per kiosk screen.
- Reduce animated custom cards on small Raspberry devices.
- Use local URLs where possible, for example `http://homeassistant.local:8123`.
- Install emoji fonts if dashboard titles or cards show placeholder boxes.

## Debugging

```bash
kioskmate --admin-info
kioskmate --doctor
journalctl --user -u kioskmate.service -n 200 --no-pager
systemctl --user status kioskmate.service --no-pager
```

For Wayland, verify that `$XDG_RUNTIME_DIR/$WAYLAND_DISPLAY` exists as a socket. For X11, verify `/tmp/.X11-unix/X${DISPLAY#:}`. Starting the user service from a root shell does not attach it to the desktop user's graphical session.

The Admin UI also exposes status, logs, system jobs, hardware controls and MQTT diagnostics.
