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

## Raspberry Performance

For heavy Home Assistant dashboards on Raspberry hardware, start with:

- Performance profile: `raspberry`
- GPU mode: `software`
- Reduce motion: enabled
- Watchdog: enabled
- Browser RSS limit: 700-900 MB
- Browser CPU grace: 45 seconds or higher

Use the benchmark helper before and after changes:

```bash
bash scripts/benchmark.sh 180
```

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

The Admin UI also exposes status, logs, system jobs, hardware controls and MQTT diagnostics.
