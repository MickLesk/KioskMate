# KioskMate without a desktop (Cage)

By default KioskMate runs as a `systemd --user` service inside a desktop session (for example labwc on Raspberry Pi OS). The desktop itself (panel, file manager desktop, notification daemon, compositor effects) costs memory and CPU that a kiosk never uses.

The optional Cage mode runs KioskMate on `tty1` inside [Cage](https://github.com/cage-kiosk/cage), a Wayland compositor that shows exactly one application full screen. Chromium is started by KioskMate as before and appears inside Cage. This is the same idea as FullPageOS or AnotterKiosk, but without reflashing an image.

## Requirements

- Raspberry Pi OS Lite (Bookworm or newer) or Raspberry Pi OS with the desktop disabled. Debian works the same way.
- KioskMate installed from the `.deb` package.
- A user that owns the kiosk configuration (`~/.config/kioskmate/config.json`).

## Enable

```bash
sudo /usr/share/kioskmate/cage/setup-cage.sh enable <user>
sudo reboot
```

The script:

1. installs `cage` plus `wlr-randr`/`wlopm` for display power control,
2. installs the PAM session `/etc/pam.d/kioskmate-cage` (logind session with `XDG_RUNTIME_DIR`),
3. adds the user to `video`, `render` and `input`,
4. disables the desktop user service `kioskmate.service`, `getty@tty1` and an enabled display manager,
5. enables `kioskmate-cage@<user>.service`.

The Admin UI stays at `http://<kiosk-ip>:33333`. Logs: `journalctl -u kioskmate-cage@<user>`.

## Disable

```bash
sudo /usr/share/kioskmate/cage/setup-cage.sh disable <user>
sudo reboot
```

This re-enables `getty@tty1`, the first display manager found (lightdm, gdm3 or sddm) and the user service.

## Notes

- `cage -s` allows switching to another VT (Ctrl+Alt+F2) for a local repair shell.
- Display power uses `wlr-randr`; run **Browser Doctor** after the first boot to confirm display power and GPU rasterization.
- On-screen keyboard (squeekboard) and desktop audio applets are not available in Cage mode; volume control via `pactl` works when PipeWire/PulseAudio runs for the user.
- The memory saved depends on the desktop that was running before; measure with `scripts/benchmark.sh` before and after.
