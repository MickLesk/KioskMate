# Hardware recommendations

KioskMate itself needs little (a Go process with a soft 48 MB memory limit in
the low-power profiles). The browser decides how smooth a kiosk runs. These
notes come from real Raspberry Pi 4 setups with Home Assistant dashboards and
camera streams.

## Raspberry Pi

| | Recommendation |
| --- | --- |
| Board | Pi 4 or Pi 5 with 2 GB RAM or more. A 1 GB Pi 4 works for a plain dashboard without cameras. |
| Power supply | The official 5.1 V / 3 A (Pi 4) or 5 V / 5 A (Pi 5) supply with its own cable. Weak supplies cause undervoltage: the CPU throttles and **USB touch controllers drop out**. Check with `vcgencmd get_throttled` (`0x0` is good) or in the Browser Doctor. |
| Storage | A good A1/A2 SD card or, better, an SSD over USB. KioskMate keeps SD writes low (telemetry every 10 min, capped browser cache and log). |
| OS | Raspberry Pi OS (Trixie or Bookworm, 64-bit). With the desktop (labwc) or desktop-free with Cage (KioskMate image). |

### Video memory (camera streams)

The hardware video decoder needs contiguous memory (CMA). With too little,
the kernel log shows `cma_alloc failed`, Chromium decodes in software and one
camera stream can take 100 % CPU or more.

In `/boot/firmware/config.txt`:

```ini
dtoverlay=vc4-kms-v3d,cma-512
# remove a gpu_mem= line: it does not help the KMS driver and takes RAM
```

Boards with 2 GB or more get this automatically with the KioskMate image
(`VIDEO_MEMORY=auto` in `kioskmate.txt`). Reboot after changing it.

### Swap

Swap on the SD card makes a full memory slow and wears out the card. Prefer
compressed swap in RAM (zram); the KioskMate image sets it up. On an existing
installation:

```bash
sudo apt install systemd-zram-generator
printf '[zram0]\nzram-size = min(ram / 2, 2048)\ncompression-algorithm = zstd\n' | sudo tee /etc/systemd/zram-generator.conf
sudo systemctl disable --now dphys-swapfile 2>/dev/null; sudo reboot
```

The Browser Doctor warns when swap lives on the SD card.

## Cameras

* Use a **substream** (640×360 or 1280×720) for dashboards. A Pi 4 decodes
  one 1080p stream in hardware; several, or WebRTC above 720p, overload it.
* With go2rtc, prefer MSE/MP4 over WebRTC on the Pi: Chromium decodes it in
  hardware more reliably.
* The KioskMate camera page disconnects its streams while it is hidden
  (another page in front, display off). A Home Assistant dashboard keeps
  decoding hidden camera cards, so give cameras their own page if CPU is high.
* Device → Display → Runtime telemetry shows the CPU of every page; the Browser
  Doctor names the page that costs most.

## Touchscreens

USB touch controllers are sensitive to undervoltage. If touch stops working,
KioskMate reports "touchscreen missing" (it remembers devices it has seen).
Re-plug the cable, check the supply and see `dmesg | grep -i usb`.

## Windows

See [WINDOWS.md](WINDOWS.md). Any machine that runs Edge or Chrome well works;
KioskMate itself needs about 30 MB of RAM.
