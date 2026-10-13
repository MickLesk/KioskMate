# Ready-made Raspberry Pi image

KioskMate can be built as a complete SD card image, in the spirit of FullPageOS or AnotterKiosk: flash it, put your Wi-Fi and dashboard URL into one text file, boot.

The image is Raspberry Pi OS Lite (Bookworm, 64-bit) plus:

- `cage`, `wlr-randr` (and `wlopm` where the archive has it) for the [desktop-free Cage mode](HEADLESS-CAGE.md),
- Chromium,
- the KioskMate `.deb`,
- a user `kiosk` that runs `kioskmate-cage@kiosk.service` on `tty1`,
- a first-boot service that applies `/boot/firmware/kioskmate.txt`.

It runs on 64-bit boards: Raspberry Pi 3, 4, 5, 400, 500 and Zero 2 W.

## Flash and configure

1. Write the image (`*.img.xz`) to an SD card with Raspberry Pi Imager ("Use custom") or `xzcat image.img.xz | sudo dd of=/dev/sdX bs=4M conv=fsync`. If you use Imager, skip its "OS customisation" step, or at least leave the username empty: the image expects the user `kiosk`, and renaming it breaks the kiosk service.
2. Mount the card's boot partition (`bootfs`) on your computer and edit `kioskmate.txt`:

   ```ini
   WIFI_SSID=MyNetwork
   WIFI_PASSWORD=secret-passphrase
   WIFI_COUNTRY=DE
   HOSTNAME=kiosk-kitchen
   TIMEZONE=Europe/Berlin
   ADMIN_PASSWORD=change-me-please
   START_URL=http://homeassistant.local:8123/lovelace/0
   READ_ONLY=no
   ```

3. Put the card in the Pi and power it on. Raspberry Pi OS resizes the root partition and reboots once; on the next boot the settings are applied and the kiosk opens `START_URL`.
4. Open the Admin UI at `http://<hostname>:33333` (or `http://<ip>:33333`).

All keys are optional; an empty value keeps the image default (hostname `kioskmate`, time zone `Europe/London`, the Home Assistant demo page, Admin UI in setup mode). The example file with comments is also installed as `/usr/share/doc/kioskmate/kioskmate.txt.example`.

| Key | Effect |
| --- | --- |
| `WIFI_SSID`, `WIFI_PASSWORD` | NetworkManager connection `kioskmate-wifi` (`nmcli`). Empty password: open network. Ethernet works without any setting. |
| `WIFI_COUNTRY` | Wi-Fi regulatory country (`raspi-config nonint do_wifi_country`). Raspberry Pi OS keeps Wi-Fi off until it is set. |
| `HOSTNAME` | `hostnamectl set-hostname` plus the `127.0.1.1` line in `/etc/hosts`. |
| `TIMEZONE` | `timedatectl set-timezone`, for example `America/New_York`. |
| `ADMIN_PASSWORD` | `kioskmate --set-admin-password-stdin` as user `kiosk` (at least 8 characters). |
| `START_URL` | `kioskmate --set-start-url`: replaces the URL of the first kiosk page. |
| `READ_ONLY` | `yes` turns on the overlay file system, see below. |

### What happens on the first boot

`kioskmate-firstboot.service` runs once, before the kiosk starts. It parses the file (it is never executed as a script), applies the settings in the order of the table, and then:

- writes `/var/lib/kioskmate/firstboot.done`, so it does not run again,
- blanks `WIFI_PASSWORD` and `ADMIN_PASSWORD` in `kioskmate.txt` and appends a status line with the number of errors,
- logs every step: `journalctl -u kioskmate-firstboot`.

An invalid value is reported and skipped; the other settings are still applied and the kiosk still starts. To apply an edited file again:

```bash
sudo rm /var/lib/kioskmate/firstboot.done
sudo reboot
```

The passwords are blanked from the file, so enter them again in that case.

## Read-only root (`READ_ONLY=yes`)

With `READ_ONLY=yes` the first-boot service applies all other settings first, then runs `raspi-config nonint do_overlayfs 0` and reboots. From then on the root file system is a RAM overlay and the boot partition is mounted read-only, so the SD card is not written during normal operation. This protects against card wear and corruption from power loss.

The trade-off: **every change made while the overlay is on is lost at the next reboot**. That includes pages, settings and the Admin password changed in the Admin UI, KioskMate updates, `apt` upgrades and the browser profile (Home Assistant logins). Set everything up first, or turn the overlay off for maintenance.

If any setting in `kioskmate.txt` failed, `READ_ONLY=yes` is skipped so you can fix things without fighting the overlay.

Turn the overlay off (two steps, because the boot partition can only be made writable again once the overlay is gone):

```bash
sudo raspi-config nonint do_overlayfs 1   # root writable after the reboot
sudo reboot
sudo raspi-config nonint do_overlayfs 1   # boot partition writable again
sudo reboot
```

Turn it on again with `sudo raspi-config nonint do_overlayfs 0 && sudo reboot`, or interactively under `sudo raspi-config` -> Performance Options -> Overlay File System.

## Logging in

By default the `kiosk` user has no usable password (password login is disabled) and SSH is off; the kiosk is managed through the Admin UI. Choose shell access when building the image:

- `SSH_PUBKEY="ssh-ed25519 AAAA..."` enables SSH for `kiosk` with key login only,
- `KIOSK_PASSWORD=...` sets a password for a local console login on `tty2` (Ctrl+Alt+F2; Cage allows switching VTs). SSH password login stays disabled.

`kiosk` has passwordless `sudo`, like the default user of Raspberry Pi OS, which KioskMate uses for reboot, shutdown and package jobs.

## Building the image

The build uses [pi-gen](https://github.com/RPi-Distro/pi-gen), the tool that builds Raspberry Pi OS, at a pinned release tag. Requirements: Linux with Docker that can run privileged containers, `git`, Go, Python 3, about 20 GB of free disk space, and on x86 hosts `qemu-user-static` with `binfmt_misc` (native arm64 hosts need no emulation).

```bash
VERSION=0.9.0 bash image/build.sh
```

`image/build.sh`:

1. builds `dist/kioskmate_<version>_arm64.deb` with `scripts/package-deb.py` (or uses `KIOSKMATE_DEB=path/to.deb`),
2. clones pi-gen at `PI_GEN_REF` (default `2026-09-15-raspios-bookworm-arm64`) into `build/image/pi-gen`,
3. copies `image/stage-kioskmate` into it as a stage after `stage2` (Raspberry Pi OS Lite) and the `.deb` into the stage, and disables the Lite image export,
4. writes the pi-gen `config` (user `kiosk`, hostname `kioskmate`, `STAGE_LIST="stage0 stage1 stage2 stage-kioskmate"`),
5. runs `build-docker.sh` and copies the result to `dist/image/` with a `SHA256SUMS` file.

Optional environment variables: `SSH_PUBKEY`, `KIOSK_PASSWORD`, `LOCALE_DEFAULT`, `KEYBOARD_KEYMAP`, `KEYBOARD_LAYOUT`, `TIMEZONE_DEFAULT`, `DEPLOY_COMPRESSION` (`xz`, `zip`, `gz`, `none`), `PI_GEN_REF`, `PI_GEN_REPO`, `BUILD_DIR`, `OUT_DIR`. If a build fails half way, pi-gen's container `pigen_work` stays; remove it with `docker rm -v pigen_work` before the next run.

The stage (`image/stage-kioskmate/`):

| Path | Purpose |
| --- | --- |
| `00-packages/` | `cage`, `wlr-randr`; `wlopm` and `overlayroot` when available |
| `01-kioskmate/` | installs Chromium and the `.deb`, runs `setup-cage.sh enable kiosk`, locks the generated password |
| `02-firstboot/` | `kioskmate-firstboot` script and service, `kioskmate.txt` on the boot partition |

### GitHub Actions

The workflow **Raspberry Pi image** (`.github/workflows/image.yml`) runs only on demand (Actions -> Raspberry Pi image -> Run workflow). It builds the `.deb` and the image on a native arm64 runner and uploads `dist/image/` as a workflow artifact for 14 days.
